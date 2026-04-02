package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
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
	timeSeriesInferenceServiceURL := getEnv("TIMESERIES_INFERENCE_SERVICE_URL", "http://localhost:8000")
	historyServiceURL := getEnv("HISTORY_SERVICE_URL", "http://localhost:8082")

	httpClient := &http.Client{Timeout: 10 * time.Second}
	router := gin.Default()

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "model-router-service",
			"status":  "ok",
		})
	})

	router.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "model-router-service",
			"status":  "ready",
		})
	})

	router.POST("/route/timeseries", func(c *gin.Context) {
		var req routeTimeSeriesRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid routing payload", "details": err.Error()})
			return
		}

		model, err := lookupModel(httpClient, registryServiceURL, req.ModelName)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to resolve model", "details": err.Error()})
			return
		}

		inferenceRaw, err := inferTimeSeries(httpClient, timeSeriesInferenceServiceURL, req.Series)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "failed to call inference service", "details": err.Error()})
			return
		}

		requestID := fmt.Sprintf("req-%d", time.Now().UTC().UnixNano())
		historyStatus := "success"
		historySummary := "timeseries inference routed successfully"

		if err := saveHistory(httpClient, historyServiceURL, historyCreateRequest{
			RequestID: requestID,
			ModelName: model.ModelName,
			ModelType: model.ModelType,
			Status:    historyStatus,
			Summary:   historySummary,
		}); err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "inference completed but failed to save history", "details": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"request_id": requestID,
			"model":      model,
			"inference":  inferenceRaw,
			"status":     historyStatus,
		})
	})

	_ = router.Run(":8080")
}

func lookupModel(client *http.Client, registryURL, modelName string) (registeredModel, error) {
	url := fmt.Sprintf("%s/models/%s", registryURL, modelName)
	resp, err := client.Get(url)
	if err != nil {
		return registeredModel{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return registeredModel{}, fmt.Errorf("registry returned status %d", resp.StatusCode)
	}

	var payload modelLookupResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return registeredModel{}, err
	}

	if payload.Model.ModelName == "" {
		return registeredModel{}, fmt.Errorf("registry response missing model")
	}

	return payload.Model, nil
}

func inferTimeSeries(client *http.Client, inferenceURL string, series []float64) (map[string]interface{}, error) {
	requestBody, err := json.Marshal(timeseriesInferRequest{Series: series})
	if err != nil {
		return nil, err
	}

	resp, err := client.Post(fmt.Sprintf("%s/infer", inferenceURL), "application/json", bytes.NewBuffer(requestBody))
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

	resp, err := client.Post(fmt.Sprintf("%s/history", historyURL), "application/json", bytes.NewBuffer(requestBody))
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
