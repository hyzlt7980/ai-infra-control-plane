package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

const (
	serviceManagedByLabel = "deployment-manager-service"
	defaultHTTPPort       = ":8080"
	requestTimeout        = 10 * time.Second
)

type Config struct {
	Namespace            string
	DefaultModelImage    string
	DefaultContainerPort int32
}

type Server struct {
	kubeClient kubernetes.Interface
	cfg        Config
}

type CreateDeploymentRequest struct {
	ModelName     string `json:"model_name" binding:"required"`
	Image         string `json:"image"`
	Replicas      *int32 `json:"replicas"`
	ContainerPort *int32 `json:"container_port"`
}

type CreateDeploymentResponse struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Image         string `json:"image"`
	Replicas      int32  `json:"replicas"`
	ContainerPort int32  `json:"container_port"`
	ServiceName   string `json:"service_name"`
}

type GetDeploymentResponse struct {
	Name              string `json:"name"`
	Namespace         string `json:"namespace"`
	Replicas          int32  `json:"replicas"`
	ReadyReplicas     int32  `json:"ready_replicas"`
	AvailableReplicas int32  `json:"available_replicas"`
	ServiceName       string `json:"service_name"`
	StatusSummary     string `json:"status_summary"`
}

type DeleteDeploymentResponse struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	ServiceName      string `json:"service_name"`
	DeploymentExists bool   `json:"deployment_existed"`
	ServiceExists    bool   `json:"service_existed"`
	Deleted          bool   `json:"deleted"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

func main() {
	cfg, err := loadConfig()
	if err != nil {
		panic(fmt.Errorf("load config: %w", err))
	}

	restCfg, err := loadKubeConfig()
	if err != nil {
		panic(fmt.Errorf("load kubernetes config: %w", err))
	}

	kubeClient, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		panic(fmt.Errorf("create kubernetes client: %w", err))
	}

	router := newRouter(&Server{kubeClient: kubeClient, cfg: cfg})
	if err := router.Run(defaultHTTPPort); err != nil {
		panic(fmt.Errorf("run http server: %w", err))
	}
}

func newRouter(server *Server) *gin.Engine {
	router := gin.Default()

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "deployment-manager-service", "status": "ok"})
	})

	router.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "deployment-manager-service", "status": "ready"})
	})

	router.POST("/deployments", server.createDeployment)
	router.GET("/deployments/:name", server.getDeployment)
	router.DELETE("/deployments/:name", server.deleteDeployment)

	return router
}

func loadConfig() (Config, error) {
	port, err := parseInt32FromEnv("DEFAULT_CONTAINER_PORT", 8080)
	if err != nil {
		return Config{}, err
	}

	return Config{
		Namespace:            getEnvOrDefault("PLATFORM_NAMESPACE", "ai-infra"),
		DefaultModelImage:    strings.TrimSpace(os.Getenv("DEFAULT_MODEL_IMAGE")),
		DefaultContainerPort: port,
	}, nil
}

func parseInt32FromEnv(key string, fallback int32) (int32, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("invalid %s: must be an integer > 0", key)
	}
	return int32(parsed), nil
}

func loadKubeConfig() (*rest.Config, error) {
	inClusterCfg, err := rest.InClusterConfig()
	if err == nil {
		return inClusterCfg, nil
	}

	kubeConfigPath := strings.TrimSpace(os.Getenv("KUBECONFIG"))
	if kubeConfigPath == "" {
		home := homedir.HomeDir()
		if home == "" {
			return nil, errors.New("in-cluster config unavailable and cannot resolve home directory for kubeconfig")
		}
		kubeConfigPath = filepath.Join(home, ".kube", "config")
	}

	return clientcmd.BuildConfigFromFlags("", kubeConfigPath)
}

func (s *Server) createDeployment(c *gin.Context) {
	var req CreateDeploymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondErr(c, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	name := strings.ToLower(strings.TrimSpace(req.ModelName))
	if errs := validation.IsDNS1123Label(name); len(errs) > 0 {
		respondErr(c, http.StatusBadRequest, "model_name must be a valid DNS-1123 label")
		return
	}

	image := strings.TrimSpace(req.Image)
	if image == "" {
		image = s.cfg.DefaultModelImage
	}
	if image == "" {
		respondErr(c, http.StatusBadRequest, "image is required when DEFAULT_MODEL_IMAGE is not set")
		return
	}

	replicas := int32(1)
	if req.Replicas != nil {
		replicas = *req.Replicas
	}
	if replicas < 1 {
		respondErr(c, http.StatusBadRequest, "replicas must be >= 1")
		return
	}

	containerPort := s.cfg.DefaultContainerPort
	if req.ContainerPort != nil {
		containerPort = *req.ContainerPort
	}
	if containerPort < 1 {
		respondErr(c, http.StatusBadRequest, "container_port must be >= 1")
		return
	}

	deployment := buildDeployment(name, s.cfg.Namespace, image, replicas, containerPort)
	service := buildService(name, s.cfg.Namespace, containerPort)

	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	if _, err := s.kubeClient.AppsV1().Deployments(s.cfg.Namespace).Create(ctx, deployment, metav1.CreateOptions{}); err != nil {
		if apierrors.IsAlreadyExists(err) {
			respondErr(c, http.StatusConflict, "deployment already exists")
			return
		}
		respondErr(c, http.StatusInternalServerError, fmt.Sprintf("create deployment: %v", err))
		return
	}

	if _, err := s.kubeClient.CoreV1().Services(s.cfg.Namespace).Create(ctx, service, metav1.CreateOptions{}); err != nil {
		_ = s.kubeClient.AppsV1().Deployments(s.cfg.Namespace).Delete(ctx, name, metav1.DeleteOptions{})
		if apierrors.IsAlreadyExists(err) {
			respondErr(c, http.StatusConflict, "service already exists")
			return
		}
		respondErr(c, http.StatusInternalServerError, fmt.Sprintf("create service: %v", err))
		return
	}

	c.JSON(http.StatusCreated, CreateDeploymentResponse{
		Name:          name,
		Namespace:     s.cfg.Namespace,
		Image:         image,
		Replicas:      replicas,
		ContainerPort: containerPort,
		ServiceName:   name,
	})
}

func (s *Server) getDeployment(c *gin.Context) {
	name := strings.ToLower(strings.TrimSpace(c.Param("name")))
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	dep, err := s.kubeClient.AppsV1().Deployments(s.cfg.Namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			respondErr(c, http.StatusNotFound, "deployment not found")
			return
		}
		respondErr(c, http.StatusInternalServerError, fmt.Sprintf("get deployment: %v", err))
		return
	}

	serviceName := ""
	if _, err := s.kubeClient.CoreV1().Services(s.cfg.Namespace).Get(ctx, name, metav1.GetOptions{}); err == nil {
		serviceName = name
	}

	replicas := int32(0)
	if dep.Spec.Replicas != nil {
		replicas = *dep.Spec.Replicas
	}

	c.JSON(http.StatusOK, GetDeploymentResponse{
		Name:              dep.Name,
		Namespace:         dep.Namespace,
		Replicas:          replicas,
		ReadyReplicas:     dep.Status.ReadyReplicas,
		AvailableReplicas: dep.Status.AvailableReplicas,
		ServiceName:       serviceName,
		StatusSummary:     summarizeStatus(dep),
	})
}

func (s *Server) deleteDeployment(c *gin.Context) {
	name := strings.ToLower(strings.TrimSpace(c.Param("name")))
	ctx, cancel := context.WithTimeout(c.Request.Context(), requestTimeout)
	defer cancel()

	deploymentExisted := true
	if err := s.kubeClient.AppsV1().Deployments(s.cfg.Namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			deploymentExisted = false
		} else {
			respondErr(c, http.StatusInternalServerError, fmt.Sprintf("delete deployment: %v", err))
			return
		}
	}

	serviceExisted := true
	if err := s.kubeClient.CoreV1().Services(s.cfg.Namespace).Delete(ctx, name, metav1.DeleteOptions{}); err != nil {
		if apierrors.IsNotFound(err) {
			serviceExisted = false
		} else {
			respondErr(c, http.StatusInternalServerError, fmt.Sprintf("delete service: %v", err))
			return
		}
	}

	c.JSON(http.StatusOK, DeleteDeploymentResponse{
		Name:             name,
		Namespace:        s.cfg.Namespace,
		ServiceName:      name,
		DeploymentExists: deploymentExisted,
		ServiceExists:    serviceExisted,
		Deleted:          deploymentExisted || serviceExisted,
	})
}

func buildDeployment(name, namespace, image string, replicas, containerPort int32) *appsv1.Deployment {
	labels := managedLabels(name)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name:  "model",
					Image: image,
					Ports: []corev1.ContainerPort{{ContainerPort: containerPort}},
				}}},
			},
		},
	}
}

func buildService(name, namespace string, containerPort int32) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: managedLabels(name)},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": name},
			Ports: []corev1.ServicePort{{
				Name:       "http",
				Port:       containerPort,
				TargetPort: intstr.FromInt32(containerPort),
				Protocol:   corev1.ProtocolTCP,
			}},
		},
	}
}

func managedLabels(name string) map[string]string {
	return map[string]string{"app": name, "managed-by": serviceManagedByLabel}
}

func summarizeStatus(dep *appsv1.Deployment) string {
	desired := int32(0)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	if desired == 0 {
		return "scaled-to-zero"
	}
	if dep.Status.AvailableReplicas == desired {
		return "healthy"
	}
	if dep.Status.ReadyReplicas > 0 {
		return "progressing"
	}
	return "pending"
}

func respondErr(c *gin.Context, status int, message string) {
	c.JSON(status, ErrorResponse{Error: message})
}

func getEnvOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}
