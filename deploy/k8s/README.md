# Kubernetes Manifests (Placeholders)

These manifests are intentionally minimal for initial platform scaffolding.
They provide deployment and service placeholders for each component.

## Data backend secrets

`model-registry-service` and `history-service` can run with `STORE_BACKEND=mysql`.
In that mode they read credentials from secret `ai-infra-data-backends`.

Use `data-backends-secret.example.yaml` as a template:

```bash
kubectl apply -f deploy/k8s/data-backends-secret.example.yaml
```
