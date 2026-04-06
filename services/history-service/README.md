# history-service

Go + Gin service for writing and reading inference history records.

## Endpoints
- `GET /healthz`
- `GET /readyz`
- `POST /history`
- `GET /history/:request_id`

## Storage backends
- `STORE_BACKEND=memory` (default): in-memory storage.
- `STORE_BACKEND=mysql`: MySQL persistence + Redis cache-aside for reads.

## Environment variables
- `STORE_BACKEND` (`memory` | `mysql`, default `memory`)
- `MYSQL_DSN` (default `root:root@tcp(localhost:3306)/ai_control_plane?parseTime=true`)
- `REDIS_ADDR` (default `localhost:6379`)
- `REDIS_PASSWORD` (default empty)
- `REDIS_DB` (default `0`)
- `CACHE_TTL` (default `60s`)
- `SERVER_ADDR` (default `:8080`)

## Notes
- `created_at` is generated server-side in UTC.
