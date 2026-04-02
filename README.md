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
- Starter service entrypoints with health endpoints only
- Service-level Dockerfiles
- Placeholder Kubernetes Deployment + Service manifests
- Basic CI smoke workflow

## Not implemented yet
- Full business logic
- Dynamic Kubernetes API calls
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
