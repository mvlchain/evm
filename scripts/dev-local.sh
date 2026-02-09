#!/usr/bin/env bash

set -euo pipefail

COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.localbin.yml}"
REDIS_ADDR="${MVL_REDIS_ADDR:-127.0.0.1:6379}"
NO_BUILD=false
RESET_CHAIN=false

usage() {
  cat <<'EOF'
Usage: scripts/dev-local.sh [options]

Options:
  --no-build    Skip `make install`
  --reset       Recreate chain data (passes -y)
  -h, --help    Show help
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-build) NO_BUILD=true; shift ;;
    --reset) RESET_CHAIN=true; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "Unknown option: $1"; usage; exit 1 ;;
  esac
done

echo "Starting redis (compose file: ${COMPOSE_FILE})..."
docker compose -f "$COMPOSE_FILE" up -d redis

if [[ "$NO_BUILD" == "false" ]]; then
  echo "Building evmd (make install)..."
  make install
fi

export MVL_REDIS_ADDR="$REDIS_ADDR"

if [[ "$RESET_CHAIN" == "true" ]]; then
  echo "Starting local node (reset chain)..."
  ./local_node.sh -y
else
  echo "Starting local node (reuse chain)..."
  ./local_node.sh --no-install -n
fi
