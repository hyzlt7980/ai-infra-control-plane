package main

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type HistoryRecord struct {
	RequestID string    `json:"request_id" binding:"required"`
	ModelName string    `json:"model_name" binding:"required"`
	ModelType string    `json:"model_type" binding:"required"`
	Status    string    `json:"status" binding:"required"`
	CreatedAt time.Time `json:"created_at"`
	Summary   string    `json:"summary" binding:"required"`
}

type createHistoryRequest struct {
	RequestID string `json:"request_id" binding:"required"`
	ModelName string `json:"model_name" binding:"required"`
	ModelType string `json:"model_type" binding:"required"`
	Status    string `json:"status" binding:"required"`
	Summary   string `json:"summary" binding:"required"`
}

type historyStore struct {
	mu      sync.RWMutex
	records map[string]HistoryRecord
}

func main() {
	store := &historyStore{records: make(map[string]HistoryRecord)}
	router := gin.Default()

	router.GET("/healthz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "history-service", "status": "ok"})
	})

	router.GET("/readyz", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"service": "history-service", "status": "ready"})
	})

	router.POST("/history", func(c *gin.Context) {
		var req createHistoryRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "invalid history payload", "details": err.Error()})
			return
		}

		record := HistoryRecord{
			RequestID: req.RequestID,
			ModelName: req.ModelName,
			ModelType: req.ModelType,
			Status:    req.Status,
			CreatedAt: time.Now().UTC(),
			Summary:   req.Summary,
		}

		store.mu.Lock()
		store.records[record.RequestID] = record
		store.mu.Unlock()

		c.JSON(http.StatusCreated, gin.H{"status": "success", "record": record})
	})

	router.GET("/history/:request_id", func(c *gin.Context) {
		requestID := c.Param("request_id")

		store.mu.RLock()
		record, exists := store.records[requestID]
		store.mu.RUnlock()

		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"status": "error", "error": "history record not found", "request_id": requestID})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "success", "record": record})
	})

	_ = router.Run(":8080")
}
