#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REGISTRY="${IMAGE_REGISTRY:-ghcr.io/example}"
TAG="${IMAGE_TAG:-latest}"
PUSH="${PUSH_IMAGES:-false}"

services=(
  api-gateway
  model-registry-service
  deployment-manager-service
  model-router-service
  history-service
  timeseries-inference-service
)

for svc in "${services[@]}"; do
  image="${REGISTRY}/${svc}:${TAG}"
  echo "[build-images] building ${image}"
  docker build -t "${image}" "${ROOT_DIR}/services/${svc}"
  if [[ "${PUSH}" == "true" ]]; then
    echo "[build-images] pushing ${image}"
    docker push "${image}"
  fi
done

echo "[build-images] complete"
