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

type modelStore interface {
	UpsertModel(ctx context.Context, model Model) error
	ListModels(ctx context.Context) ([]Model, error)
	GetModel(ctx context.Context, name string) (Model, bool, error)
}

type cacheClient interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttl time.Duration) error
	Del(ctx context.Context, keys ...string) error
}

type memoryStore struct {
	mu     sync.RWMutex
	models map[string]Model
}

func newMemoryStore() *memoryStore {
	return &memoryStore{models: make(map[string]Model)}
}

func (s *memoryStore) UpsertModel(_ context.Context, model Model) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.models[model.ModelName] = model
	return nil
}

func (s *memoryStore) ListModels(_ context.Context) ([]Model, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	models := make([]Model, 0, len(s.models))
	for _, m := range s.models {
		models = append(models, m)
	}
	return models, nil
}

func (s *memoryStore) GetModel(_ context.Context, name string) (Model, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	model, exists := s.models[name]
	return model, exists, nil
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

func (s *mysqlStore) UpsertModel(ctx context.Context, model Model) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO models (model_name, model_type, version, image, container_port, status)
VALUES (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
model_type = VALUES(model_type),
version = VALUES(version),
image = VALUES(image),
container_port = VALUES(container_port),
status = VALUES(status),
updated_at = CURRENT_TIMESTAMP
`, model.ModelName, model.ModelType, model.Version, model.Image, model.ContainerPort, model.Status)
	if err != nil {
		return err
	}
	if err := s.cache.Del(ctx, modelCacheKey(model.ModelName), allModelsCacheKey()); err != nil {
		log.Printf("warn: failed to invalidate cache for model=%s: %v", model.ModelName, err)
	}
	return nil
}

func (s *mysqlStore) ListModels(ctx context.Context) ([]Model, error) {
	if cached, err := s.cache.Get(ctx, allModelsCacheKey()); err == nil {
		var models []Model
		if unmarshalErr := json.Unmarshal([]byte(cached), &models); unmarshalErr == nil {
			return models, nil
		}
	}

	rows, err := s.db.QueryContext(ctx, `
SELECT model_name, model_type, version, image, container_port, status
FROM models
ORDER BY model_name ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	models := make([]Model, 0)
	for rows.Next() {
		var m Model
		if err := rows.Scan(&m.ModelName, &m.ModelType, &m.Version, &m.Image, &m.ContainerPort, &m.Status); err != nil {
			return nil, err
		}
		models = append(models, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if payload, err := json.Marshal(models); err == nil {
		if err := s.cache.Set(ctx, allModelsCacheKey(), string(payload), s.cacheTTL); err != nil {
			log.Printf("warn: failed to populate all models cache: %v", err)
		}
	}

	return models, nil
}

func (s *mysqlStore) GetModel(ctx context.Context, name string) (Model, bool, error) {
	if cached, err := s.cache.Get(ctx, modelCacheKey(name)); err == nil {
		var model Model
		if unmarshalErr := json.Unmarshal([]byte(cached), &model); unmarshalErr == nil {
			return model, true, nil
		}
	}

	var m Model
	err := s.db.QueryRowContext(ctx, `
SELECT model_name, model_type, version, image, container_port, status
FROM models
WHERE model_name = ?
`, name).Scan(&m.ModelName, &m.ModelType, &m.Version, &m.Image, &m.ContainerPort, &m.Status)
	if errors.Is(err, sql.ErrNoRows) {
		return Model{}, false, nil
	}
	if err != nil {
		return Model{}, false, err
	}

	if payload, err := json.Marshal(m); err == nil {
		if err := s.cache.Set(ctx, modelCacheKey(name), string(payload), s.cacheTTL); err != nil {
			log.Printf("warn: failed to populate model cache for %s: %v", name, err)
		}
	}

	return m, true, nil
}

func modelCacheKey(name string) string {
	return "model_registry:model:" + name
}

func allModelsCacheKey() string {
	return "model_registry:models:all"
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

func buildStore(cfg config) modelStore {
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

		cache := &redisCache{client: redis.NewClient(&redis.Options{
			Addr:     cfg.RedisAddr,
			Password: cfg.RedisPassword,
			DB:       cfg.RedisDB,
		})}
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

func buildRouter(store modelStore) *gin.Engine {
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
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid model registration payload", "details": err.Error()})
			return
		}

		model := Model{ModelName: req.ModelName, ModelType: req.ModelType, Version: req.Version, Image: req.Image, ContainerPort: req.ContainerPort, Status: req.Status}
		if err := store.UpsertModel(c.Request.Context(), model); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register model", "details": err.Error()})
			return
		}

		c.JSON(http.StatusCreated, gin.H{"message": "model registered", "model": model})
	})

	router.GET("/models", func(c *gin.Context) {
		models, err := store.ListModels(c.Request.Context())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list models", "details": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"models": models})
	})

	router.GET("/models/:name", func(c *gin.Context) {
		name := c.Param("name")
		model, exists, err := store.GetModel(c.Request.Context(), name)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get model", "details": err.Error()})
			return
		}
		if !exists {
			c.JSON(http.StatusNotFound, gin.H{"error": "model not found", "model_name": name})
			return
		}
		c.JSON(http.StatusOK, gin.H{"model": model})
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
