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

## Response highlights

- `POST /deployments` returns `status` + `deployment` object.
- `GET /deployments/:name` returns readiness details and `service_url` used by router.

## Known limitations

- Single namespace only (`PLATFORM_NAMESPACE`).
- One container per Deployment.
- Basic ClusterIP Service only.
- No HPA, dynamic Ingress/ConfigMap/Secret/Namespace creation, or database integration.
