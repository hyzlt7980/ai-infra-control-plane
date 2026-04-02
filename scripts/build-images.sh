#!/usr/bin/env bash
set -euo pipefail

services=(
  api-gateway
  model-registry-service
  deployment-manager-service
  model-router-service
  history-service
  timeseries-inference-service
)

for svc in "${services[@]}"; do
  echo "docker build -t ghcr.io/example/${svc}:latest services/${svc}"
done
