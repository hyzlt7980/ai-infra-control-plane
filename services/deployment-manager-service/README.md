# deployment-manager-service

Go + Gin service that manages model-serving workloads in Kubernetes.

## Required configuration
Environment variables:

- `PLATFORM_NAMESPACE` (default: `ai-infra`)
- `DEFAULT_MODEL_IMAGE` (required unless `POST /deployments` request sets `image`)
- `DEFAULT_CONTAINER_PORT` (default: `8080`)

## Kubernetes config behavior
The service resolves Kubernetes credentials in this order:

1. In-cluster config (`rest.InClusterConfig`) when running inside Kubernetes.
2. Local kubeconfig fallback (`$KUBECONFIG` or `~/.kube/config`) for local dev.

## Endpoints
- `GET /healthz`
- `GET /readyz`
- `POST /deployments`
- `GET /deployments/:name`
- `DELETE /deployments/:name`

## Request validation/defaulting
- `model_name` is required and must be a valid DNS-1123 label.
- `image` falls back to `DEFAULT_MODEL_IMAGE`.
- `replicas` defaults to `1` and must be `>= 1`.
- `container_port` falls back to `DEFAULT_CONTAINER_PORT` and must be `>= 1`.

## Usage examples
Create a deployment + ClusterIP service:

```bash
curl -X POST http://localhost:8080/deployments \
  -H 'Content-Type: application/json' \
  -d '{
    "model_name": "timeseries-v1",
    "replicas": 1
  }'
```

Get deployment status:

```bash
curl http://localhost:8080/deployments/timeseries-v1
```

Delete deployment and service:

```bash
curl -X DELETE http://localhost:8080/deployments/timeseries-v1
```

## Known limitations
- Single namespace only (`PLATFORM_NAMESPACE`).
- One container per Deployment.
- Basic ClusterIP Service only.
- No HPA, dynamic Ingress/ConfigMap/Secret/Namespace creation, or database integration.

## Local build/test notes
If your environment blocks module download from `proxy.golang.org`/`k8s.io`, use a network that can access Go module sources to run:

```bash
cd services/deployment-manager-service
go mod tidy
go test ./...
```
