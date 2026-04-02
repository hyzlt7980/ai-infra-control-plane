# model-registry-service

Go + Gin service for registering and reading model metadata.

## Endpoints
- `GET /healthz`
- `GET /readyz`
- `POST /models/register`
- `GET /models`
- `GET /models/:name`

## Notes
- Uses in-memory storage (non-persistent).
- Validates required fields for registration payload.
