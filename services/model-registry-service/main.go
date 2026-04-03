package main

import (
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

type Model struct {
	ModelName     string `json:"model_name" binding:"required"`
	ModelType     string `json:"model_type" binding:"required"`
	Version       string `json:"version" binding:"required"`
	Image         string `json:"image" binding:"required"`
	ContainerPort int    `json:"container_port" binding:"required,gte=1,lte=65535"`
	Status        string `json:"status" binding:"required"`
}

type registerModelRequest struct {
	ModelName     string `json:"model_name" binding:"required"`
	ModelType     string `json:"model_type" binding:"required"`
	Version       string `json:"version" binding:"required"`
	Image         string `json:"image" binding:"required"`
	ContainerPort int    `json:"container_port" binding:"required,gte=1,lte=65535"`
	Status        string `json:"status" binding:"required"`
}

type registryStore struct {
	mu     sync.RWMutex
	models map[string]Model
}

func main() {
	store := &registryStore{models: make(map[string]Model)}
	router := gin.Default()

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "model-registry-service", "status": "ok"})
	})

	router.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "model-registry-service", "status": "ready"})
	})

	router.POST("/models/register", func(c *gin.Context) {
		var req registerModelRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid model registration payload", "details": err.Error()})
			return
		}

		model := Model{
			ModelName:     strings.ToLower(strings.TrimSpace(req.ModelName)),
			ModelType:     strings.TrimSpace(req.ModelType),
			Version:       strings.TrimSpace(req.Version),
			Image:         strings.TrimSpace(req.Image),
			ContainerPort: req.ContainerPort,
			Status:        strings.TrimSpace(req.Status),
		}

		store.mu.Lock()
		store.models[model.ModelName] = model
		store.mu.Unlock()

		c.JSON(http.StatusCreated, gin.H{"status": "success", "model": model})
	})

	router.GET("/models", func(c *gin.Context) {
		store.mu.RLock()
		models := make([]Model, 0, len(store.models))
		for _, m := range store.models {
			models = append(models, m)
		}
		store.mu.RUnlock()

		c.JSON(http.StatusOK, gin.H{"status": "success", "models": models})
	})

	router.GET("/models/:name", func(c *gin.Context) {
		name := strings.ToLower(strings.TrimSpace(c.Param("name")))

		store.mu.RLock()
		model, exists := store.models[name]
		store.mu.RUnlock()

		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "model not found", "model_name": name})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "success", "model": model})
	})

	_ = router.Run(":8080")
}
