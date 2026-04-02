# model-registry-service

Go + Gin service for registering and reading model metadata.

## Endpoints
- `GET /healthz`
- `GET /readyz`
- `POST /models/register`
- `GET /models`
- `GET /models/:name`

## API notes
- Uses stable JSON envelopes (`status`, plus payload object).
- Normalizes `model_name` to lowercase for consistent cross-service lookup.
- In-memory storage only (non-persistent).
