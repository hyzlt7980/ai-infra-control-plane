#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

cat <<'EOF'
[bootstrap] Kubernetes AI inference control plane (demo)
EOF

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "[bootstrap] missing required command: $1" >&2
    exit 1
  fi
}

require_cmd go
require_cmd python3
require_cmd kubectl
require_cmd docker

printf "[bootstrap] go version: %s\n" "$(go version)"
printf "[bootstrap] python version: %s\n" "$(python3 --version)"
printf "[bootstrap] kubectl version: %s\n" "$(kubectl version --client --short 2>/dev/null || kubectl version --client)"
printf "[bootstrap] docker version: %s\n" "$(docker --version)"

cat <<EOF

[bootstrap] Next steps:
1) Build images
   ./scripts/build-images.sh
2) Deploy manifests
   ./scripts/deploy.sh
3) Run stage-4 demo flow in README.md

[bootstrap] Repo root: ${ROOT_DIR}
EOF
