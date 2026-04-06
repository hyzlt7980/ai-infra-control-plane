package main

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestMySQLStore(t *testing.T) (*mysqlStore, sqlmock.Sqlmock, func()) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to run miniredis: %v", err)
	}
	cache := &redisCache{client: redis.NewClient(&redis.Options{Addr: mini.Addr()})}
	cleanup := func() {
		_ = cache.client.Close()
		mini.Close()
		_ = db.Close()
	}
	return &mysqlStore{db: db, cache: cache, cacheTTL: 5 * time.Minute}, mock, cleanup
}

func TestMySQLStoreGetModel_CacheMissThenHit(t *testing.T) {
	store, mock, cleanup := newTestMySQLStore(t)
	defer cleanup()

	query := regexp.QuoteMeta(`
SELECT model_name, model_type, version, image, container_port, status
FROM models
WHERE model_name = ?
`)
	rows := sqlmock.NewRows([]string{"model_name", "model_type", "version", "image", "container_port", "status"}).
		AddRow("timeseries-v1", "timeseries", "v1", "img:v1", 8000, "active")
	mock.ExpectQuery(query).WithArgs("timeseries-v1").WillReturnRows(rows)

	ctx := context.Background()
	model, exists, err := store.GetModel(ctx, "timeseries-v1")
	if err != nil || !exists {
		t.Fatalf("expected model exists with no error, got exists=%v err=%v", exists, err)
	}
	if model.ModelName != "timeseries-v1" {
		t.Fatalf("unexpected model: %+v", model)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected sql expectations: %v", err)
	}

	cached, exists, err := store.GetModel(ctx, "timeseries-v1")
	if err != nil || !exists {
		t.Fatalf("expected cached model exists with no error, got exists=%v err=%v", exists, err)
	}
	if cached.ModelName != "timeseries-v1" {
		t.Fatalf("unexpected cached model: %+v", cached)
	}
}

func TestMySQLStoreUpsertModel_InvalidatesCache(t *testing.T) {
	store, mock, cleanup := newTestMySQLStore(t)
	defer cleanup()

	ctx := context.Background()
	if err := store.cache.Set(ctx, modelCacheKey("timeseries-v1"), `{"model_name":"timeseries-v1"}`, time.Minute); err != nil {
		t.Fatalf("failed to seed model cache: %v", err)
	}
	if err := store.cache.Set(ctx, allModelsCacheKey(), `[]`, time.Minute); err != nil {
		t.Fatalf("failed to seed list cache: %v", err)
	}

	execSQL := regexp.QuoteMeta(`
INSERT INTO models (model_name, model_type, version, image, container_port, status)
VALUES (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
model_type = VALUES(model_type),
version = VALUES(version),
image = VALUES(image),
container_port = VALUES(container_port),
status = VALUES(status),
updated_at = CURRENT_TIMESTAMP
`)
	mock.ExpectExec(execSQL).
		WithArgs("timeseries-v1", "timeseries", "v1", "img:v1", 8000, "active").
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := store.UpsertModel(ctx, Model{ModelName: "timeseries-v1", ModelType: "timeseries", Version: "v1", Image: "img:v1", ContainerPort: 8000, Status: "active"})
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	if _, err := store.cache.Get(ctx, modelCacheKey("timeseries-v1")); err == nil {
		t.Fatalf("expected per-model cache to be invalidated")
	}
	if _, err := store.cache.Get(ctx, allModelsCacheKey()); err == nil {
		t.Fatalf("expected list cache to be invalidated")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected sql expectations: %v", err)
	}
}

func TestBuildStoreFallbackToMemory(t *testing.T) {
	store := buildStore(config{Backend: backendMySQL, MySQLDSN: "invalid-dsn", RedisAddr: "localhost:6379", CacheTTL: time.Second})
	if _, ok := store.(*memoryStore); !ok {
		t.Fatalf("expected fallback to memory store when mysql config is invalid")
	}
}
