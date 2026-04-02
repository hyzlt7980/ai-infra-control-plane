# history-service

Go + Gin service for writing and reading inference history records.

## Endpoints
- `GET /healthz`
- `GET /readyz`
- `POST /history`
- `GET /history/:request_id`

## Notes
- Uses in-memory storage (non-persistent).
- `created_at` is generated server-side in UTC.
