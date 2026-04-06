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

func newHistoryMySQLStore(t *testing.T) (*mysqlStore, sqlmock.Sqlmock, func()) {
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

func TestMySQLStoreGetRecord_CacheMissThenHit(t *testing.T) {
	store, mock, cleanup := newHistoryMySQLStore(t)
	defer cleanup()

	createdAt := time.Now().UTC().Truncate(time.Second)
	query := regexp.QuoteMeta(`
SELECT request_id, model_name, model_type, status, created_at, summary
FROM history_records
WHERE request_id = ?
`)
	rows := sqlmock.NewRows([]string{"request_id", "model_name", "model_type", "status", "created_at", "summary"}).
		AddRow("req-1", "timeseries-v1", "timeseries", "succeeded", createdAt, "ok")
	mock.ExpectQuery(query).WithArgs("req-1").WillReturnRows(rows)

	ctx := context.Background()
	record, exists, err := store.GetRecord(ctx, "req-1")
	if err != nil || !exists {
		t.Fatalf("expected record exists with no error, got exists=%v err=%v", exists, err)
	}
	if record.RequestID != "req-1" {
		t.Fatalf("unexpected record: %+v", record)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unexpected sql expectations: %v", err)
	}

	cached, exists, err := store.GetRecord(ctx, "req-1")
	if err != nil || !exists {
		t.Fatalf("expected cached record exists with no error, got exists=%v err=%v", exists, err)
	}
	if cached.RequestID != "req-1" {
		t.Fatalf("unexpected cached record: %+v", cached)
	}
}

func TestMySQLStoreCreateRecord_UpdatesCache(t *testing.T) {
	store, mock, cleanup := newHistoryMySQLStore(t)
	defer cleanup()

	execSQL := regexp.QuoteMeta(`
INSERT INTO history_records (request_id, model_name, model_type, status, created_at, summary)
VALUES (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE
model_name = VALUES(model_name),
model_type = VALUES(model_type),
status = VALUES(status),
created_at = VALUES(created_at),
summary = VALUES(summary),
updated_at = CURRENT_TIMESTAMP
`)
	now := time.Now().UTC().Truncate(time.Second)
	record := HistoryRecord{RequestID: "req-2", ModelName: "timeseries-v1", ModelType: "timeseries", Status: "running", CreatedAt: now, Summary: "in progress"}

	mock.ExpectExec(execSQL).
		WithArgs(record.RequestID, record.ModelName, record.ModelType, record.Status, record.CreatedAt, record.Summary).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := store.CreateRecord(context.Background(), record); err != nil {
		t.Fatalf("create record failed: %v", err)
	}

	cached, err := store.cache.Get(context.Background(), historyCacheKey(record.RequestID))
	if err != nil {
		t.Fatalf("expected cache to be updated: %v", err)
	}
	if cached == "" {
		t.Fatalf("expected non-empty cached payload")
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
