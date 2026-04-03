# model-router-service

Go + Gin service for routing time-series inference requests.

## Endpoints
- `GET /healthz`
- `GET /readyz`
- `POST /route/timeseries`

## Stage-4 routing behavior

`POST /route/timeseries` now executes:
1. Lookup model metadata from `model-registry-service`.
2. Lookup deployment status from `deployment-manager-service`.
3. Prefer deployed in-cluster `service_url` when deployment is ready.
4. Call downstream inference `/infer` endpoint.
5. Persist request summary in `history-service`.

## Error contracts

The router returns explicit errors for:
- model not registered
- model registered but not deployed
- deployment exists but not ready
- downstream inference call failure

## Environment variables
- `MODEL_REGISTRY_SERVICE_URL` (default: `http://localhost:8081`)
- `DEPLOYMENT_MANAGER_SERVICE_URL` (default: `http://localhost:8084`)
- `TIMESERIES_INFERENCE_SERVICE_URL` (fallback default: `http://localhost:8000`)
- `HISTORY_SERVICE_URL` (default: `http://localhost:8082`)
