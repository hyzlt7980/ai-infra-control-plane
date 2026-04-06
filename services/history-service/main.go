package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
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

type storeBackend string

const (
	backendMemory storeBackend = "memory"
	backendMySQL  storeBackend = "mysql"

	defaultMySQLDSN      = "root:root@tcp(localhost:3306)/ai_control_plane?parseTime=true"
	defaultRedisAddr     = "localhost:6379"
	defaultRedisPassword = ""
	defaultRedisDB       = 0
	defaultCacheTTL      = 60 * time.Second
	defaultServerAddr    = ":8080"
)

type config struct {
	Backend       storeBackend
	MySQLDSN      string
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	CacheTTL      time.Duration
	ServerAddr    string
}

type historyStore interface {
	CreateRecord(ctx context.Context, record HistoryRecord) error
	GetRecord(ctx context.Context, requestID string) (HistoryRecord, bool, error)
}

type cacheClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
}

type memoryStore struct {
	mu      sync.RWMutex
	records map[string]HistoryRecord
}

func newMemoryStore() *memoryStore {
	return &memoryStore{records: make(map[string]HistoryRecord)}
}

func (s *memoryStore) CreateRecord(_ context.Context, record HistoryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records[record.RequestID] = record
	return nil
}

func (s *memoryStore) GetRecord(_ context.Context, requestID string) (HistoryRecord, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, exists := s.records[requestID]
	return record, exists, nil
}

type redisCache struct {
	client *redis.Client
}

func (r *redisCache) Get(ctx context.Context, key string) (string, error) {
	val, err := r.client.Get(ctx, key).Result()
	if errors.Is(err, redis.Nil) {
		return "", errCacheMiss
	}
	return val, err
}

func (r *redisCache) Set(ctx context.Context, key string, value string, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *redisCache) Del(ctx context.Context, keys ...string) error {
	return r.client.Del(ctx, keys...).Err()
}

var errCacheMiss = errors.New("cache miss")

type mysqlStore struct {
	db       *sql.DB
	cache    cacheClient
	cacheTTL time.Duration
}

func (s *mysqlStore) CreateRecord(ctx context.Context, record HistoryRecord) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO history_records (request_id, model_name, model_type, status, created_at, summary)
VALUES (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
model_name = VALUES(model_name),
model_type = VALUES(model_type),
status = VALUES(status),
created_at = VALUES(created_at),
summary = VALUES(summary),
updated_at = CURRENT_TIMESTAMP
`, record.RequestID, record.ModelName, record.ModelType, record.Status, record.CreatedAt, record.Summary)
	if err != nil {
		return err
	}

	if payload, err := json.Marshal(record); err == nil {
		if err := s.cache.Set(ctx, historyCacheKey(record.RequestID), string(payload), s.cacheTTL); err != nil {
			log.Printf("warn: failed to update history cache for %s: %v", record.RequestID, err)
		}
	}
	return nil
}

func (s *mysqlStore) GetRecord(ctx context.Context, requestID string) (HistoryRecord, bool, error) {
	if cached, err := s.cache.Get(ctx, historyCacheKey(requestID)); err == nil {
		var record HistoryRecord
		if unmarshalErr := json.Unmarshal([]byte(cached), &record); unmarshalErr == nil {
			return record, true, nil
		}
	}

	var record HistoryRecord
	err := s.db.QueryRowContext(ctx, `
SELECT request_id, model_name, model_type, status, created_at, summary
FROM history_records
WHERE request_id = ?
`, requestID).Scan(&record.RequestID, &record.ModelName, &record.ModelType, &record.Status, &record.CreatedAt, &record.Summary)
	if errors.Is(err, sql.ErrNoRows) {
		return HistoryRecord{}, false, nil
	}
	if err != nil {
		return HistoryRecord{}, false, err
	}

	if payload, err := json.Marshal(record); err == nil {
		if err := s.cache.Set(ctx, historyCacheKey(requestID), string(payload), s.cacheTTL); err != nil {
			log.Printf("warn: failed to populate history cache for %s: %v", requestID, err)
		}
	}
	return record, true, nil
}

func historyCacheKey(requestID string) string {
	return "history:record:" + requestID
}

func loadConfig() config {
	backend := storeBackend(getEnv("STORE_BACKEND", string(backendMemory)))
	if backend != backendMemory && backend != backendMySQL {
		backend = backendMemory
	}
	return config{
		Backend:       backend,
		MySQLDSN:      getEnv("MYSQL_DSN", defaultMySQLDSN),
		RedisAddr:     getEnv("REDIS_ADDR", defaultRedisAddr),
		RedisPassword: getEnv("REDIS_PASSWORD", defaultRedisPassword),
		RedisDB:       getEnvAsInt("REDIS_DB", defaultRedisDB),
		CacheTTL:      getEnvAsDuration("CACHE_TTL", defaultCacheTTL),
		ServerAddr:    getEnv("SERVER_ADDR", defaultServerAddr),
	}
}

func buildStore(cfg config) historyStore {
	if cfg.Backend == backendMySQL {
		db, err := sql.Open("mysql", cfg.MySQLDSN)
		if err != nil {
			log.Printf("warn: failed to init mysql store, fallback to memory: %v", err)
			return newMemoryStore()
		}
		if err := db.Ping(); err != nil {
			log.Printf("warn: failed to connect mysql, fallback to memory: %v", err)
			_ = db.Close()
			return newMemoryStore()
		}

		cache := &redisCache{client: redis.NewClient(&redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword, DB: cfg.RedisDB})}
		if err := cache.client.Ping(context.Background()).Err(); err != nil {
			log.Printf("warn: failed to connect redis, fallback to memory: %v", err)
			_ = db.Close()
			_ = cache.client.Close()
			return newMemoryStore()
		}
		return &mysqlStore{db: db, cache: cache, cacheTTL: cfg.CacheTTL}
	}
	return newMemoryStore()
}

func buildRouter(store historyStore) *gin.Engine {
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid history payload", "details": err.Error()})
			return
		}

		record := HistoryRecord{RequestID: req.RequestID, ModelName: req.ModelName, ModelType: req.ModelType, Status: req.Status, CreatedAt: time.Now().UTC(), Summary: req.Summary}
		if err := store.CreateRecord(c.Request.Context(), record); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create history record", "details": err.Error()})
			return
		}
		c.JSON(http.StatusCreated, gin.H{"message": "history record created", "record": record})
	})

	router.GET("/history/:request_id", func(c *gin.Context) {
		requestID := c.Param("request_id")
		record, exists, err := store.GetRecord(c.Request.Context(), requestID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read history record", "details": err.Error()})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "history record not found", "request_id": requestID})
			return
		}
		c.JSON(http.StatusOK, gin.H{"record": record})
	})

	return router
}

func main() {
	cfg := loadConfig()
	store := buildStore(cfg)
	router := buildRouter(store)
	_ = router.Run(cfg.ServerAddr)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	value := getEnv(key, fmt.Sprintf("%d", fallback))
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	value := getEnv(key, fallback.String())
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
