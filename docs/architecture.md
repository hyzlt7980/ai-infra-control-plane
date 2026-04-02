# AI Inference Control Plane (Skeleton)

## Scope
This repository scaffolds a Kubernetes-based control plane for AI inference workloads.

## Services
- `api-gateway`: Ingress and external API entrypoint.
- `model-registry-service`: Model metadata registration and lookup.
- `deployment-manager-service`: Desired-state deployment orchestration (future).
- `model-router-service`: Request routing to model backends (future).
- `history-service`: Inference event and metadata history tracking.
- `timeseries-inference-service`: Timeseries inference endpoint and experimentation.

## Notes
- Business logic intentionally deferred.
- Dynamic Kubernetes API interactions intentionally deferred.
- Initial focus: clean structure, health endpoints, and deployable placeholders.
