#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
BENCH_DIR="${ROOT_DIR}/.bench-testnets"

CHAIN_ID="${MATCH_EVM_CHAIN_ID:-hwbench-evm-1}"
START_IP="${MATCH_EVM_START_IP:-192.168.20.2}"
VALIDATOR_COUNT="${MATCH_EVM_VALIDATOR_COUNT:-4}"
COMMIT_TIMEOUT="${MATCH_EVM_COMMIT_TIMEOUT:-500ms}"
EMPTY_BLOCK_INTERVAL="${MATCH_EVM_EMPTY_BLOCK_INTERVAL:-0s}"
TIMEOUT_PROPOSE="${MATCH_EVM_TIMEOUT_PROPOSE:-200ms}"
TIMEOUT_PROPOSE_DELTA="${MATCH_EVM_TIMEOUT_PROPOSE_DELTA:-50ms}"
TIMEOUT_PREVOTE="${MATCH_EVM_TIMEOUT_PREVOTE:-100ms}"
TIMEOUT_PREVOTE_DELTA="${MATCH_EVM_TIMEOUT_PREVOTE_DELTA:-50ms}"
TIMEOUT_PRECOMMIT="${MATCH_EVM_TIMEOUT_PRECOMMIT:-100ms}"
TIMEOUT_PRECOMMIT_DELTA="${MATCH_EVM_TIMEOUT_PRECOMMIT_DELTA:-50ms}"
SKIP_TIMEOUT_COMMIT="${MATCH_EVM_SKIP_TIMEOUT_COMMIT:-true}"

if [[ ! -f "${BENCH_DIR}/node0/evmd/config/genesis.json" ]]; then
  echo "[prepare_evm_bench] initializing testnet files (${VALIDATOR_COUNT} validators, chain-id=${CHAIN_ID})"
  evmd testnet init-files \
    --validator-count "${VALIDATOR_COUNT}" \
    --output-dir "${BENCH_DIR}" \
    --starting-ip-address "${START_IP}" \
    --chain-id "${CHAIN_ID}" \
    --keyring-backend test \
    --commit-timeout "${COMMIT_TIMEOUT}"
else
  echo "[prepare_evm_bench] bench testnet already exists: ${BENCH_DIR}"
fi

export BENCH_DIR VALIDATOR_COUNT COMMIT_TIMEOUT EMPTY_BLOCK_INTERVAL
export TIMEOUT_PROPOSE TIMEOUT_PROPOSE_DELTA TIMEOUT_PREVOTE TIMEOUT_PREVOTE_DELTA
export TIMEOUT_PRECOMMIT TIMEOUT_PRECOMMIT_DELTA SKIP_TIMEOUT_COMMIT
python3 - <<'PY'
from pathlib import Path
import os
import re

bench = Path(os.environ["BENCH_DIR"])
count = int(os.environ["VALIDATOR_COUNT"])
commit_timeout = os.environ["COMMIT_TIMEOUT"]
empty_interval = os.environ["EMPTY_BLOCK_INTERVAL"]
timeout_propose = os.environ["TIMEOUT_PROPOSE"]
timeout_propose_delta = os.environ["TIMEOUT_PROPOSE_DELTA"]
timeout_prevote = os.environ["TIMEOUT_PREVOTE"]
timeout_prevote_delta = os.environ["TIMEOUT_PREVOTE_DELTA"]
timeout_precommit = os.environ["TIMEOUT_PRECOMMIT"]
timeout_precommit_delta = os.environ["TIMEOUT_PRECOMMIT_DELTA"]
skip_timeout_commit = os.environ["SKIP_TIMEOUT_COMMIT"].strip().lower()
if skip_timeout_commit not in ("true", "false"):
    raise SystemExit(f"invalid MATCH_EVM_SKIP_TIMEOUT_COMMIT: {skip_timeout_commit!r} (expected true/false)")

for i in range(count):
    cfg = bench / f"node{i}" / "evmd" / "config" / "config.toml"
    if not cfg.exists():
        continue

    text = cfg.read_text()
    text_new = text
    text_new = re.sub(r'^timeout_propose\s*=\s*".*"$', f'timeout_propose = "{timeout_propose}"', text_new, flags=re.MULTILINE)
    text_new = re.sub(r'^timeout_propose_delta\s*=\s*".*"$', f'timeout_propose_delta = "{timeout_propose_delta}"', text_new, flags=re.MULTILINE)
    text_new = re.sub(r'^timeout_prevote\s*=\s*".*"$', f'timeout_prevote = "{timeout_prevote}"', text_new, flags=re.MULTILINE)
    text_new = re.sub(r'^timeout_prevote_delta\s*=\s*".*"$', f'timeout_prevote_delta = "{timeout_prevote_delta}"', text_new, flags=re.MULTILINE)
    text_new = re.sub(r'^timeout_precommit\s*=\s*".*"$', f'timeout_precommit = "{timeout_precommit}"', text_new, flags=re.MULTILINE)
    text_new = re.sub(r'^timeout_precommit_delta\s*=\s*".*"$', f'timeout_precommit_delta = "{timeout_precommit_delta}"', text_new, flags=re.MULTILINE)
    text_new = re.sub(r'^timeout_commit\s*=\s*".*"$', f'timeout_commit = "{commit_timeout}"', text_new, flags=re.MULTILINE)
    text_new = re.sub(r'^skip_timeout_commit\s*=\s*.*$', f'skip_timeout_commit = {skip_timeout_commit}', text_new, flags=re.MULTILINE)
    text_new = re.sub(r'^create_empty_blocks_interval\s*=\s*".*"$', f'create_empty_blocks_interval = "{empty_interval}"', text_new, flags=re.MULTILINE)

    cfg.write_text(text_new)
    print(
        f"[prepare_evm_bench] tuned {cfg}: "
        f"timeout_propose={timeout_propose}, timeout_prevote={timeout_prevote}, "
        f"timeout_precommit={timeout_precommit}, timeout_commit={commit_timeout}, "
        f"skip_timeout_commit={skip_timeout_commit}, create_empty_blocks_interval={empty_interval}"
    )
PY
