#!/bin/bash
# entrypoint.sh
#
# Node entrypoint for validator, full, and archive nodes.
# Called automatically when the Docker container starts.
#
# Required env vars:
#   NODE_TYPE           node type: validator | full | archive
#
# Optional env vars:
#   TADAD_HOME          node home directory (default: /data/tadad)
#   MONIKER             node moniker (default: private-testnet-node1)
#   CHAIN_ID            chain ID (default: tada_31451-1)
#   LOG_LEVEL           log level (default: info)
#   TRACE               trace flag, e.g. "--trace" (default: "")
#
# Secrets (injected via K8s Secret):
#   VALIDATOR_MNEMONIC  validator mnemonic (validator node only)

set -e

# ---------- Config ----------
TADAD_HOME="${TADAD_HOME:-/data/tadad}"
MONIKER="${MONIKER:-private-testnet-node1}"
CHAIN_ID="${CHAIN_ID:-tada_31451-1}"
NODE_TYPE="${NODE_TYPE:-}"
LOG_LEVEL="${LOG_LEVEL:-info}"
TRACE="${TRACE:-}"

# ---------- Validate ----------
if [[ -z "$NODE_TYPE" ]]; then
  echo "Error: NODE_TYPE env var is required (validator | full | archive)"
  exit 1
fi

if [[ "$NODE_TYPE" == "validator" && -z "$VALIDATOR_MNEMONIC" ]]; then
  echo "Error: VALIDATOR_MNEMONIC env var is required for validator node"
  exit 1
fi

echo "==> Starting $NODE_TYPE node (chain-id: $CHAIN_ID, home: $TADAD_HOME)"

# ---------- Init ----------
# validator: init --recover to generate priv_validator_key.json from mnemonic
# full/archive: plain init (random keys, overwritten by config in next step)
if [[ "$NODE_TYPE" == "validator" ]]; then
  echo "==> Initializing with mnemonic (generates priv_validator_key.json)"
  echo "$VALIDATOR_MNEMONIC" | tadad init "$MONIKER" \
    --chain-id "$CHAIN_ID" --home "$TADAD_HOME" --overwrite --recover
else
  tadad init "$MONIKER" --chain-id "$CHAIN_ID" --home "$TADAD_HOME" --overwrite
fi

# ---------- Config files (from ConfigMap mount) ----------
# ConfigMap is mounted at /configs/<node-type>/
CONFIG_SRC="/configs/${NODE_TYPE}"

if [[ ! -d "$CONFIG_SRC" ]]; then
  echo "Error: ConfigMap mount not found at $CONFIG_SRC"
  exit 1
fi

echo "==> Copying configs from $CONFIG_SRC"
cp "$CONFIG_SRC/genesis.json" "$TADAD_HOME/config/genesis.json"
cp "$CONFIG_SRC/config.toml"  "$TADAD_HOME/config/config.toml"
cp "$CONFIG_SRC/app.toml"     "$TADAD_HOME/config/app.toml"

# ---------- node_key.json (validator node only) ----------
if [[ "$NODE_TYPE" == "validator" ]]; then
  echo "==> Copying node_key.json from secret"
  if [[ ! -f "/secrets/node_key.json" ]]; then
    echo "Error: /secrets/node_key.json not found"
    exit 1
  fi
  cp /secrets/node_key.json "$TADAD_HOME/config/node_key.json"
fi

# ---------- Start ----------
echo "==> Starting tadad ($NODE_TYPE)"

# TRACE를 배열로 처리해 빈 문자열 인자 방지
TRACE_ARGS=()
[[ -n "$TRACE" ]] && TRACE_ARGS=("$TRACE")

case "$NODE_TYPE" in
  validator)
    exec tadad start "${TRACE_ARGS[@]}" \
      --pruning nothing \
      --log_level "$LOG_LEVEL" \
      --minimum-gas-prices=0.0001wei \
      --evm.min-tip=0 \
      --home "$TADAD_HOME" \
      --json-rpc.api eth,net,web3 \
      --chain-id "$CHAIN_ID"
    ;;
  full)
    exec tadad start "${TRACE_ARGS[@]}" \
      --pruning default \
      --log_level "$LOG_LEVEL" \
      --minimum-gas-prices=0.0001wei \
      --evm.min-tip=0 \
      --home "$TADAD_HOME" \
      --json-rpc.api eth,txpool,personal,net,debug,web3 \
      --chain-id "$CHAIN_ID"
    ;;
  archive)
    exec tadad start "${TRACE_ARGS[@]}" \
      --pruning nothing \
      --log_level "$LOG_LEVEL" \
      --minimum-gas-prices=0.0001wei \
      --evm.min-tip=0 \
      --home "$TADAD_HOME" \
      --json-rpc.api eth,txpool,personal,net,debug,web3 \
      --chain-id "$CHAIN_ID"
    ;;
  *)
    echo "Error: unknown NODE_TYPE=$NODE_TYPE (validator | full | archive)"
    exit 1
    ;;
esac
