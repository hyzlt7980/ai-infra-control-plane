# history-service

Go + Gin service for writing and reading inference history records.

## Endpoints
- `GET /healthz`
- `GET /readyz`
- `POST /history`
- `GET /history/:request_id`

## API notes
- Uses stable JSON envelopes (`status`, plus payload object).
- `created_at` is generated server-side in UTC.
- In-memory storage only (non-persistent).
