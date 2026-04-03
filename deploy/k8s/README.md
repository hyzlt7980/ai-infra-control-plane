# Kubernetes manifests (demo)

These manifests are scoped for a single-namespace demo (`ai-infra`).

## Included components

- `namespace.yaml`
- `api-gateway`
- `model-registry-service`
- `deployment-manager-service` (with ServiceAccount, Role, RoleBinding)
- `model-router-service`
- `history-service`
- `timeseries-inference-service`

## Notes

- Router env vars point to in-cluster service DNS names.
- Deployment manager defaults to `PLATFORM_NAMESPACE=ai-infra` and default model image/port.
- Liveness/readiness probes are kept on all services.

Apply:

```bash
kubectl apply -k deploy/k8s
```
