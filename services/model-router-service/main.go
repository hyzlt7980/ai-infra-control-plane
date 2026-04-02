package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type routeTimeSeriesRequest struct {
	ModelName string    `json:"model_name" binding:"required"`
	Series    []float64 `json:"series" binding:"required,min=1"`
}

type registeredModel struct {
	ModelName     string `json:"model_name"`
	ModelType     string `json:"model_type"`
	Version       string `json:"version"`
	Image         string `json:"image"`
	ContainerPort int    `json:"container_port"`
	Status        string `json:"status"`
}

type modelLookupResponse struct {
	Model registeredModel `json:"model"`
}

type deploymentStatus struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	ReadyReplicas int32  `json:"ready_replicas"`
	ServiceName   string `json:"service_name"`
	ServicePort   int32  `json:"service_port"`
	ServiceURL    string `json:"service_url"`
	Ready         bool   `json:"ready"`
	StatusSummary string `json:"status_summary"`
}

type deploymentLookupResponse struct {
	Deployment deploymentStatus `json:"deployment"`
}

type timeseriesInferRequest struct {
	Series []float64 `json:"series"`
}

type historyCreateRequest struct {
	RequestID string `json:"request_id"`
	ModelName string `json:"model_name"`
	ModelType string `json:"model_type"`
	Status    string `json:"status"`
	Summary   string `json:"summary"`
}

func main() {
	registryServiceURL := getEnv("MODEL_REGISTRY_SERVICE_URL", "http://localhost:8081")
	deploymentManagerServiceURL := getEnv("DEPLOYMENT_MANAGER_SERVICE_URL", "http://localhost:8084")
	timeSeriesInferenceServiceURL := getEnv("TIMESERIES_INFERENCE_SERVICE_URL", "http://localhost:8000")
	historyServiceURL := getEnv("HISTORY_SERVICE_URL", "http://localhost:8082")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	router := gin.Default()

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "model-router-service", "status": "ok"})
	})

	router.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "model-router-service", "status": "ready"})
	})

	router.POST("/route/timeseries", func(c *gin.Context) {
		var req routeTimeSeriesRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid routing payload", "details": err.Error()})
			return
		}

		modelName := strings.ToLower(strings.TrimSpace(req.ModelName))
		model, err := lookupModel(httpClient, registryServiceURL, modelName)
		if err != nil {
			handleRoutingError(c, err)
			return
		}

		deployment, err := lookupDeployment(httpClient, deploymentManagerServiceURL, modelName)
		if err != nil {
			handleRoutingError(c, err)
			return
		}
		if !deployment.Ready || deployment.ReadyReplicas < 1 {
			c.JSON(http.StatusConflict, gin.H{
				"status": "error",
				"error":  "deployment exists but is not ready",
				"model":  model,
				"deployment": gin.H{
					"name":           deployment.Name,
					"namespace":      deployment.Namespace,
					"ready":          deployment.Ready,
					"ready_replicas": deployment.ReadyReplicas,
					"status_summary": deployment.StatusSummary,
				},
			})
			return
		}

		inferenceURL := timeSeriesInferenceServiceURL
		if deployment.ServiceURL != "" {
			inferenceURL = deployment.ServiceURL
		}

		inferenceRaw, err := inferTimeSeries(httpClient, inferenceURL, req.Series)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": "downstream inference call failed", "details": err.Error()})
			return
		}

		requestID := fmt.Sprintf("req-%d", time.Now().UTC().UnixNano())
		historyRecord := historyCreateRequest{
			RequestID: requestID,
			ModelName: model.ModelName,
			ModelType: model.ModelType,
			Status:    "success",
			Summary:   "timeseries inference routed successfully",
		}

		if err := saveHistory(httpClient, historyServiceURL, historyRecord); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": "inference completed but failed to save history", "details": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":     "success",
			"request_id": requestID,
			"model":      model,
			"deployment": deployment,
			"inference":  inferenceRaw,
			"history":    historyRecord,
		})
	})

	_ = router.Run(":8080")
}

type routeError struct {
	kind string
	err  error
}

func (r routeError) Error() string {
	return r.err.Error()
}

func lookupModel(client *http.Client, registryURL, modelName string) (registeredModel, error) {
	url := fmt.Sprintf("%s/models/%s", registryURL, modelName)
	resp, err := client.Get(url)
	if err != nil {
		return registeredModel{}, routeError{kind: "registry_unavailable", err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return registeredModel{}, routeError{kind: "model_not_registered", err: errors.New("model is not registered")}
	}
	if resp.StatusCode != http.StatusOK {
		return registeredModel{}, routeError{kind: "registry_unavailable", err: fmt.Errorf("registry returned status %d", resp.StatusCode)}
	}

	var payload modelLookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return registeredModel{}, routeError{kind: "registry_unavailable", err: err}
	}

	if payload.Model.ModelName == "" {
		return registeredModel{}, routeError{kind: "registry_unavailable", err: errors.New("registry response missing model")}
	}

	return payload.Model, nil
}

func lookupDeployment(client *http.Client, managerURL, modelName string) (deploymentStatus, error) {
	url := fmt.Sprintf("%s/deployments/%s", managerURL, modelName)
	resp, err := client.Get(url)
	if err != nil {
		return deploymentStatus{}, routeError{kind: "deployment_manager_unavailable", err: err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return deploymentStatus{}, routeError{kind: "model_not_deployed", err: errors.New("model is registered but not deployed")}
	}
	if resp.StatusCode != http.StatusOK {
		return deploymentStatus{}, routeError{kind: "deployment_manager_unavailable", err: fmt.Errorf("deployment manager returned status %d", resp.StatusCode)}
	}

	var payload deploymentLookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return deploymentStatus{}, routeError{kind: "deployment_manager_unavailable", err: err}
	}
	if payload.Deployment.Name == "" {
		return deploymentStatus{}, routeError{kind: "deployment_manager_unavailable", err: errors.New("deployment status response missing deployment")}
	}

	return payload.Deployment, nil
}

func handleRoutingError(c *gin.Context, err error) {
	rErr, ok := err.(routeError)
	if !ok {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": "failed to route inference", "details": err.Error()})
		return
	}

	switch rErr.kind {
	case "model_not_registered":
		c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "model is not registered", "details": rErr.Error()})
	case "model_not_deployed":
		c.JSON(http.StatusConflict, gin.H{"status": "error", "error": "model is registered but not deployed", "details": rErr.Error()})
	case "deployment_manager_unavailable":
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": "failed to resolve deployment status", "details": rErr.Error()})
	default:
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": "failed to resolve model", "details": rErr.Error()})
	}
}

func inferTimeSeries(client *http.Client, inferenceURL string, series []float64) (map[string]interface{}, error) {
	requestBody, err := json.Marshal(timeseriesInferRequest{Series: series})
	if err != nil {
		return nil, err
	}

	resp, err := client.Post(fmt.Sprintf("%s/infer", strings.TrimRight(inferenceURL, "/")), "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("timeseries inference returned status %d", resp.StatusCode)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}

	return payload, nil
}

func saveHistory(client *http.Client, historyURL string, req historyCreateRequest) error {
	requestBody, err := json.Marshal(req)
	if err != nil {
		return err
	}

	resp, err := client.Post(fmt.Sprintf("%s/history", strings.TrimRight(historyURL, "/")), "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("history service returned status %d", resp.StatusCode)
	}

	return nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
