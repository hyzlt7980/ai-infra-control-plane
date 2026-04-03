# Kubernetes AI Inference Control Plane (Stage 4 Demo)

This repository is a demo-oriented control plane for AI inference on Kubernetes. It wires together model registration, deployment orchestration, request routing, and inference history into one coherent end-to-end flow.

## Top 5 issues that blocked a coherent demo (now addressed)

1. **Router and deployment manager were disconnected**: routing did not use live deployment readiness or service endpoints.
2. **Inference path was broken**: `timeseries-inference-service` had no `/infer` endpoint.
3. **API responses were inconsistent**: cross-service JSON contracts mixed shapes and ad-hoc fields.
4. **Demo bootstrap path was weak**: scripts were mostly placeholders and not usable for a clean local demo setup.
5. **Docs/CI lacked practical guidance**: hard to tell what runs locally vs GitHub CI and how to execute the full stage-4 flow.

## Architecture summary

Services:

- **model-registry-service**: in-memory metadata registry for models.
- **deployment-manager-service**: creates/gets/deletes Kubernetes `Deployment` + `Service` for model workloads.
- **model-router-service**: validates model/deployment state, routes inference, and writes history records.
- **history-service**: stores request history in memory.
- **timeseries-inference-service**: simple inference worker (`/infer`) for demo predictions.
- **api-gateway**: placeholder edge service.

Primary stage-4 request flow:

1. Register metadata in registry.
2. Deploy model workload via deployment manager.
3. Router checks registry + deployment readiness.
4. Router calls deployed in-cluster inference endpoint.
5. Router writes request outcome to history service.

## Service responsibilities

### model-registry-service
- Owns model metadata (`model_name`, `model_type`, `version`, `image`, `container_port`, `status`).
- Exposes register/list/get endpoints.

### deployment-manager-service
- Owns runtime Kubernetes lifecycle for model-serving workloads.
- Returns readiness and in-cluster service URL (`service_url`) for routing.

### model-router-service
- Orchestrates end-to-end routing for `/route/timeseries`.
- Explicitly errors for:
  - model not registered
  - model registered but not deployed
  - deployment exists but not ready
  - downstream inference failures

### history-service
- Writes and reads request records keyed by `request_id`.

## Minimal API contract (stage 4)

### `POST /models/register` (model-registry-service)
Request:
```json
{
  "model_name": "timeseries-v1",
  "model_type": "timeseries",
  "version": "1.0.0",
  "image": "ghcr.io/example/timeseries-inference-service:latest",
  "container_port": 8080,
  "status": "active"
}
```
Response:
```json
{
  "status": "success",
  "model": {
    "model_name": "timeseries-v1",
    "model_type": "timeseries",
    "version": "1.0.0",
    "image": "ghcr.io/example/timeseries-inference-service:latest",
    "container_port": 8080,
    "status": "active"
  }
}
```

### `POST /deployments` (deployment-manager-service)
Request:
```json
{
  "model_name": "timeseries-v1",
  "image": "ghcr.io/example/timeseries-inference-service:latest",
  "replicas": 1,
  "container_port": 8080
}
```
Response:
```json
{
  "status": "success",
  "deployment": {
    "name": "timeseries-v1",
    "namespace": "ai-infra",
    "image": "ghcr.io/example/timeseries-inference-service:latest",
    "replicas": 1,
    "container_port": 8080,
    "service_name": "timeseries-v1",
    "service_port": 8080,
    "service_url": "http://timeseries-v1.ai-infra.svc.cluster.local:8080"
  }
}
```

### `GET /deployments/:name` (deployment-manager-service)
Response:
```json
{
  "status": "success",
  "deployment": {
    "name": "timeseries-v1",
    "namespace": "ai-infra",
    "replicas": 1,
    "ready_replicas": 1,
    "available_replicas": 1,
    "service_name": "timeseries-v1",
    "service_port": 8080,
    "service_url": "http://timeseries-v1.ai-infra.svc.cluster.local:8080",
    "ready": true,
    "status_summary": "healthy"
  }
}
```

### `POST /route/timeseries` (model-router-service)
Request:
```json
{
  "model_name": "timeseries-v1",
  "series": [10.0, 11.2, 12.1]
}
```
Response:
```json
{
  "status": "success",
  "request_id": "req-...",
  "model": { "...": "registry metadata" },
  "deployment": { "...": "deployment status" },
  "inference": { "prediction": 13.0 },
  "history": {
    "request_id": "req-...",
    "model_name": "timeseries-v1",
    "model_type": "timeseries",
    "status": "success",
    "summary": "timeseries inference routed successfully"
  }
}
```

### `POST /history` (history-service)
Request:
```json
{
  "request_id": "req-123",
  "model_name": "timeseries-v1",
  "model_type": "timeseries",
  "status": "success",
  "summary": "timeseries inference routed successfully"
}
```
Response:
```json
{
  "status": "success",
  "record": {
    "request_id": "req-123",
    "model_name": "timeseries-v1",
    "model_type": "timeseries",
    "status": "success",
    "summary": "timeseries inference routed successfully",
    "created_at": "2026-04-02T00:00:00Z"
  }
}
```

### `GET /history/:request_id` (history-service)
Response:
```json
{
  "status": "success",
  "record": {
    "request_id": "req-123",
    "model_name": "timeseries-v1",
    "model_type": "timeseries",
    "status": "success",
    "summary": "timeseries inference routed successfully",
    "created_at": "2026-04-02T00:00:00Z"
  }
}
```

## Stage-4 end-to-end demo flow

Assuming services are reachable locally via forwarded ports:

```bash
# 1) register
curl -sS -X POST http://localhost:8081/models/register \
  -H 'content-type: application/json' \
  -d '{"model_name":"timeseries-v1","model_type":"timeseries","version":"1.0.0","image":"ghcr.io/example/timeseries-inference-service:latest","container_port":8080,"status":"active"}'

# 2) deploy
curl -sS -X POST http://localhost:8084/deployments \
  -H 'content-type: application/json' \
  -d '{"model_name":"timeseries-v1","replicas":1,"container_port":8080}'

# 3) check status
curl -sS http://localhost:8084/deployments/timeseries-v1

# 4) route inference (also writes history)
curl -sS -X POST http://localhost:8080/route/timeseries \
  -H 'content-type: application/json' \
  -d '{"model_name":"timeseries-v1","series":[10,11.2,12.1]}'

# 5) fetch history
curl -sS http://localhost:8082/history/<request_id>
```

## Local development notes

```bash
./scripts/bootstrap.sh
./scripts/build-images.sh
./scripts/deploy.sh
```

Script behavior:
- `bootstrap.sh`: validates core tooling and prints next steps.
- `build-images.sh`: builds all service images; set `IMAGE_REGISTRY`, `IMAGE_TAG`, and `PUSH_IMAGES=true` to push.
- `deploy.sh`: applies namespace + kustomize manifests.

## Kubernetes deployment notes

- All manifests target namespace `ai-infra`.
- `deployment-manager-service` includes namespaced RBAC and namespace/runtime env config.
- `model-router-service` is configured with downstream service URLs via env vars.
- Health probes are enabled on all workloads.

## CI notes

GitHub Actions (`.github/workflows/ci.yaml`) is intended to run in GitHub-hosted runners with external module/package access:

- Go services: `go mod download`, `go test`, `go build`
- Python service: dependency install + syntax smoke check

If local environments are restricted, use CI for authoritative build/test validation.

## Known limitations

- In-memory storage only for registry/history (non-persistent).
- Single namespace (`ai-infra`) demo posture.
- No auth, DB, HPA, ingress automation, redis, frontend, or advanced observability.
- Inference logic is intentionally simple for demo clarity.
