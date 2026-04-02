package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/gin-gonic/gin"
)

func TestPostDeploymentsCreatesDeploymentAndService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := fake.NewSimpleClientset()
	srv := &Server{kubeClient: client, cfg: Config{Namespace: "ai-infra", DefaultModelImage: "example/image:1", DefaultContainerPort: 8000}}
	router := newRouter(srv)

	body := []byte(`{"model_name":"timeseries-v1"}`)
	req := httptest.NewRequest(http.MethodPost, "/deployments", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", resp.Code, resp.Body.String())
	}

	var payload CreateDeploymentResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Image != "example/image:1" || payload.ContainerPort != 8000 || payload.Replicas != 1 {
		t.Fatalf("unexpected defaults in response: %+v", payload)
	}

	dep, err := client.AppsV1().Deployments("ai-infra").Get(req.Context(), "timeseries-v1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("deployment not found: %v", err)
	}
	if dep.Spec.Template.Labels["app"] != "timeseries-v1" || dep.Spec.Template.Labels["managed-by"] != serviceManagedByLabel {
		t.Fatalf("deployment labels mismatch: %+v", dep.Spec.Template.Labels)
	}
	if dep.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort != 8000 {
		t.Fatalf("deployment container port mismatch")
	}

	svc, err := client.CoreV1().Services("ai-infra").Get(req.Context(), "timeseries-v1", metav1.GetOptions{})
	if err != nil {
		t.Fatalf("service not found: %v", err)
	}
	if svc.Spec.Type != corev1.ServiceTypeClusterIP {
		t.Fatalf("expected ClusterIP service")
	}
	if svc.Spec.Selector["app"] != dep.Spec.Template.Labels["app"] {
		t.Fatalf("service selector mismatch")
	}
	if svc.Spec.Ports[0].Port != 8000 || svc.Spec.Ports[0].TargetPort.IntVal != 8000 {
		t.Fatalf("service ports mismatch: %+v", svc.Spec.Ports[0])
	}
}

func TestGetDeploymentReturnsStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	replicas := int32(2)
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "timeseries-v1", Namespace: "ai-infra"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
			Status:     appsv1.DeploymentStatus{ReadyReplicas: 2, AvailableReplicas: 2},
		},
		&corev1.Service{ObjectMeta: metav1.ObjectMeta{Name: "timeseries-v1", Namespace: "ai-infra"}},
	)
	srv := &Server{kubeClient: client, cfg: Config{Namespace: "ai-infra"}}
	router := newRouter(srv)

	req := httptest.NewRequest(http.MethodGet, "/deployments/timeseries-v1", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}

	var payload GetDeploymentResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Name != "timeseries-v1" || payload.Namespace != "ai-infra" || payload.ServiceName != "timeseries-v1" {
		t.Fatalf("unexpected payload identity: %+v", payload)
	}
	if payload.Replicas != 2 || payload.ReadyReplicas != 2 || payload.AvailableReplicas != 2 || payload.StatusSummary != "healthy" {
		t.Fatalf("unexpected deployment status payload: %+v", payload)
	}
}

func TestDeleteDeploymentIsIdempotent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := fake.NewSimpleClientset()
	srv := &Server{kubeClient: client, cfg: Config{Namespace: "ai-infra"}}
	router := newRouter(srv)

	req := httptest.NewRequest(http.MethodDelete, "/deployments/nonexistent", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resp.Code, resp.Body.String())
	}

	var payload DeleteDeploymentResponse
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Deleted {
		t.Fatalf("expected deleted=false when nothing existed: %+v", payload)
	}
	if payload.DeploymentExists || payload.ServiceExists {
		t.Fatalf("expected existence flags false: %+v", payload)
	}
}
