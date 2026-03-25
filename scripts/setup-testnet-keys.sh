#!/bin/bash
# setup-testnet-keys.sh
#
# Generates validator mnemonic, faucet mnemonic, and node_key.json (if not present),
# then runs generate-node-configs.sh with all inputs.
#
# Keys are stored in KEYS_DIR (default: ./keys) and reused on subsequent runs.
# Keep KEYS_DIR contents secret — do not commit to version control.
#
# Usage:
#   ./scripts/setup-testnet-keys.sh
#
# Optional env vars:
#   KEYS_DIR      directory to store/read keys (default: ./keys)
#   OUTPUT_DIR    passed through to generate-node-configs.sh (default: ./configs)
#   CHAIN_ID      passed through to generate-node-configs.sh
#   DENOM         passed through to generate-node-configs.sh
#   MONIKER       passed through to generate-node-configs.sh
#   VALIDATOR_ENDPOINT  validator p2p endpoint (host:port) for persistent_peers

set -e

command -v tadad >/dev/null 2>&1 || { echo "Error: tadad binary not found in PATH."; exit 1; }
command -v jq    >/dev/null 2>&1 || { echo "Error: jq not installed."; exit 1; }

KEYS_DIR="${KEYS_DIR:-$(pwd)/keys}"
mkdir -p "$KEYS_DIR"
chmod 700 "$KEYS_DIR"

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# ============================================================
# 1. node_key.json
# ============================================================
if [[ ! -f "$KEYS_DIR/node_key.json" ]]; then
  echo "==> Generating node_key.json"
  TMPINIT="$(mktemp -d)"
  tadad init keygen --home "$TMPINIT" >/dev/null 2>&1
  cp "$TMPINIT/config/node_key.json" "$KEYS_DIR/node_key.json"
  chmod 600 "$KEYS_DIR/node_key.json"
  rm -rf "$TMPINIT"
  echo "    Saved: $KEYS_DIR/node_key.json"
else
  echo "==> Using existing node_key.json"
fi

# ============================================================
# 2. Calculate validator node ID from node_key.json
# ============================================================
TMPNODEID="$(mktemp -d)"
mkdir -p "$TMPNODEID/config"
cp "$KEYS_DIR/node_key.json" "$TMPNODEID/config/node_key.json"
VALIDATOR_NODE_ID="$(tadad comet show-node-id --home "$TMPNODEID")"
rm -rf "$TMPNODEID"
echo "==> Validator Node ID: $VALIDATOR_NODE_ID"

# ============================================================
# 3. Validator mnemonic
# ============================================================
if [[ ! -f "$KEYS_DIR/validator_mnemonic.txt" ]]; then
  echo "==> Generating validator mnemonic"
  TMPKEYS="$(mktemp -d)"
  MNEMONIC_OUTPUT="$(tadad keys add validator --keyring-backend test \
    --algo eth_secp256k1 --home "$TMPKEYS" 2>&1)"
  VALIDATOR_MNEMONIC="$(echo "$MNEMONIC_OUTPUT" | tail -1 | tr -d '\r')"
  rm -rf "$TMPKEYS"
  echo "$VALIDATOR_MNEMONIC" > "$KEYS_DIR/validator_mnemonic.txt"
  chmod 600 "$KEYS_DIR/validator_mnemonic.txt"
  echo "    Saved: $KEYS_DIR/validator_mnemonic.txt"
else
  echo "==> Using existing validator mnemonic"
  VALIDATOR_MNEMONIC="$(cat "$KEYS_DIR/validator_mnemonic.txt")"
fi

# ============================================================
# 4. Faucet mnemonic
# ============================================================
if [[ ! -f "$KEYS_DIR/faucet_mnemonic.txt" ]]; then
  echo "==> Generating faucet mnemonic"
  TMPKEYS="$(mktemp -d)"
  MNEMONIC_OUTPUT="$(tadad keys add faucet --keyring-backend test \
    --algo eth_secp256k1 --home "$TMPKEYS" 2>&1)"
  FAUCET_MNEMONIC="$(echo "$MNEMONIC_OUTPUT" | tail -1 | tr -d '\r')"
  rm -rf "$TMPKEYS"
  echo "$FAUCET_MNEMONIC" > "$KEYS_DIR/faucet_mnemonic.txt"
  chmod 600 "$KEYS_DIR/faucet_mnemonic.txt"
  echo "    Saved: $KEYS_DIR/faucet_mnemonic.txt"
else
  echo "==> Using existing faucet mnemonic"
  FAUCET_MNEMONIC="$(cat "$KEYS_DIR/faucet_mnemonic.txt")"
fi

# ============================================================
# 5. Run generate-node-configs.sh
# ============================================================
echo ""
echo "==> Running generate-node-configs.sh"

VALIDATOR_MNEMONIC="$VALIDATOR_MNEMONIC" \
FAUCET_MNEMONIC="$FAUCET_MNEMONIC" \
VALIDATOR_NODE_ID="$VALIDATOR_NODE_ID" \
VALIDATOR_ENDPOINT="${VALIDATOR_ENDPOINT:-}" \
OUTPUT_DIR="${OUTPUT_DIR:-$(pwd)/configs}" \
CHAIN_ID="${CHAIN_ID:-}" \
DENOM="${DENOM:-}" \
MONIKER="${MONIKER:-}" \
  "$SCRIPT_DIR/generate-node-configs.sh"

echo ""
echo "================================================================"
echo "  validator_mnemonic  (upload to 1Password: vaults/testnet/items/validator-mnemonic)"
echo "================================================================"
echo "$VALIDATOR_MNEMONIC"
echo ""
echo "================================================================"
echo "  faucet_mnemonic"
echo "================================================================"
echo "$FAUCET_MNEMONIC"
echo ""
echo "================================================================"
echo "  Keys saved to: $KEYS_DIR  (keep secret, do NOT commit)"
echo "    node_key.json           -> 1Password: vaults/testnet/items/validator-node-key-json"
echo "    validator_mnemonic.txt  -> 1Password: vaults/testnet/items/validator-mnemonic"
echo "    faucet_mnemonic.txt"
echo "================================================================"
