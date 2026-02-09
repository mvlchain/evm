#!/usr/bin/env bash

set -euo pipefail

mkdir -p ./bin

docker run --rm \
  -v "$PWD":/src \
  -w /src \
  golang:1.25.5 \
  bash -lc "export PATH=/usr/local/go/bin:\$PATH && apt-get update && apt-get install -y --no-install-recommends make git jq && command -v go >/dev/null && make install && cp /go/bin/evmd /src/bin/evmd"
