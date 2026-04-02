#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
NAMESPACE="${PLATFORM_NAMESPACE:-ai-infra}"

kubectl apply -f "${ROOT_DIR}/deploy/k8s/namespace.yaml"
kubectl apply -k "${ROOT_DIR}/deploy/k8s"

cat <<EOF
[deploy] Applied manifests into namespace: ${NAMESPACE}
[deploy] Check status:
  kubectl -n ${NAMESPACE} get deploy,svc
EOF
