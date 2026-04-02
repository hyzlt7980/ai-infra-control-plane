# AI Inference Control Plane Monorepo (Skeleton)

This repository contains starter scaffolding for a Kubernetes-based AI inference control plane.

## Goals
- Cloud-native AI infrastructure platform
- Model registration, deployment, routing, and inference history tracking
- Platform engineering focus (not model training)

## Repository Layout
```text
.
├── services/
│   ├── api-gateway/                      # Go + Gin
│   ├── model-registry-service/           # Go + Gin
│   ├── deployment-manager-service/       # Go + Gin
│   ├── model-router-service/             # Go + Gin
│   ├── history-service/                  # Go + Gin
│   └── timeseries-inference-service/     # Python + FastAPI
├── deploy/k8s/                           # Placeholder Kubernetes manifests
├── scripts/                              # Utility scripts
├── docs/                                 # Documentation
└── .github/workflows/                    # CI workflow
```

## What is implemented now
- Functional first-pass Go services for model registry, routing, and history (in-memory)
- Service-level Dockerfiles
- Kubernetes manifests for all services, including RBAC for deployment manager
- Basic CI smoke workflow

## Not implemented yet
- Production routing/deployment policy logic

## Quick Start
Build image command previews:
```bash
./scripts/build-images.sh
```

Apply placeholder manifests:
```bash
kubectl apply -k deploy/k8s/
```


## deployment-manager-service quick usage

`deployment-manager-service` now creates/deletes Kubernetes Deployments and ClusterIP Services in one namespace.

Example create request:
```bash
curl -X POST http://localhost:8080/deployments \
  -H "Content-Type: application/json" \
  -d '{
    "model_name": "timeseries-v1",
    "image": "ghcr.io/example/timeseries-inference-service:latest",
    "replicas": 1,
    "container_port": 8000
  }'
```

Example status request:
```bash
curl http://localhost:8080/deployments/timeseries-v1
```

Example delete request:
```bash
curl -X DELETE http://localhost:8080/deployments/timeseries-v1
```


## Local module resolution note

`deployment-manager-service` depends on Kubernetes `client-go` modules. In restricted environments where `proxy.golang.org` or `k8s.io` module resolution is blocked, `go mod tidy` / `go test` may fail locally. Run module and test commands from an environment with outbound access to Go module sources.
