# model-router-service

Go + Gin service for routing time-series inference requests.

## Endpoints
- `GET /healthz`
- `GET /readyz`
- `POST /route/timeseries`

## Routing flow
`POST /route/timeseries`:
1. Accepts `model_name` and `series`.
2. Calls model-registry-service to fetch model metadata.
3. Calls timeseries-inference-service `/infer`.
4. Calls history-service to persist a history record.
5. Returns a unified JSON response.

## Environment variables
- `MODEL_REGISTRY_SERVICE_URL` (default: `http://localhost:8081`)
- `TIMESERIES_INFERENCE_SERVICE_URL` (default: `http://localhost:8000`)
- `HISTORY_SERVICE_URL` (default: `http://localhost:8082`)
