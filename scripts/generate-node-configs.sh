#!/bin/bash
# generate-node-configs.sh
#
# Generates genesis.json, config.toml, app.toml for validator, full, and archive nodes.
# Output is written to OUTPUT_DIR (default: ./configs).
#
# Usage:
#   VALIDATOR_MNEMONIC="<mnemonic>" ./scripts/generate-node-configs.sh
#
# Required env vars:
#   VALIDATOR_MNEMONIC        validator operator key mnemonic
#   FAUCET_MNEMONIC           faucet operator key mnemonic
#
# Optional env vars:
#   CHAIN_ID                  chain ID (default: 31451)
#   DENOM                     native denom (default: wei)
#   MONIKER                   validator moniker (default: private-testnet-node1)
#   BASEFEE                   base fee in wei (default: 10000000)
#   OUTPUT_DIR                output directory (default: ./configs)
#   VALIDATOR_NODE_ID         validator node ID for persistent_peers in full/archive config.toml
#                             (if not set, placeholder __VALIDATOR_NODE_ID__ is used)
#   VALIDATOR_ENDPOINT        validator p2p endpoint (host:port) for persistent_peers
#                             (if not set, placeholder __VALIDATOR_ENDPOINT__ is used)
#   FAUCET_MNEMONIC           faucet operator key mnemonic

set -e

# ---------- Config ----------
# mainnet: tada_31450-1, testnet: tada_31451-1
CHAIN_ID="${CHAIN_ID:-tada_31451-1}"
# EVM chain ID parsed from CHAIN_ID (e.g. tada_31451-1 → 31451). Override with EVM_CHAIN_ID=<int>.
EVM_CHAIN_ID="${EVM_CHAIN_ID:-$(echo "$CHAIN_ID" | grep -oE '[0-9]+' | head -1)}"
# default: wei
DENOM="${DENOM:-wei}"
# default: validator 
MONIKER="${MONIKER:-private-testnet-node1}"
# Cosmos gas-prices for gentx (not EVM base fee). 10000000 wei = 10 Mwei, fine for testnet.
BASEFEE="${BASEFEE:-10000000}"
OUTPUT_DIR="${OUTPUT_DIR:-$(pwd)/configs}"
KEYRING="test"
KEYALGO="eth_secp256k1"

VAL_KEY="validator1"

# persistent_peers for full/archive nodes
# PERSISTENT_PEERS = <node_id>@<host>:<port>
if [[ -n "$VALIDATOR_NODE_ID" && -n "$VALIDATOR_ENDPOINT" ]]; then
  PERSISTENT_PEERS="${VALIDATOR_NODE_ID}@${VALIDATOR_ENDPOINT}"
else
  PERSISTENT_PEERS="__VALIDATOR_NODE_ID__@__VALIDATOR_ENDPOINT__"
  if [[ -z "$VALIDATOR_NODE_ID" ]]; then
    echo "Note: VALIDATOR_NODE_ID not set. Using placeholder in full/archive config.toml."
  fi
fi

# ---------- Validate ----------
command -v jq >/dev/null 2>&1 || { echo "Error: jq not installed."; exit 1; }
command -v tadad >/dev/null 2>&1 || { echo "Error: tadad binary not found in PATH."; exit 1; }

if [[ -z "$VALIDATOR_MNEMONIC" ]]; then
  echo "Error: VALIDATOR_MNEMONIC env var is required."
  echo "  Usage: VALIDATOR_MNEMONIC=\"<mnemonic>\" FAUCET_MNEMONIC=\"<mnemonic>\" $0"
  exit 1
fi

if [[ -z "$FAUCET_MNEMONIC" ]]; then
  echo "Error: FAUCET_MNEMONIC env var is required."
  echo "  Usage: VALIDATOR_MNEMONIC=\"<mnemonic>\" FAUCET_MNEMONIC=\"<mnemonic>\" $0"
  exit 1
fi

# ---------- Dev account mnemonics (testnet defaults) ----------
# These are well-known test mnemonics — safe to use on private testnet.
# Override by setting DEV_MNEMONIC_0 .. DEV_MNEMONIC_3 env vars.
DEV_MNEMONIC_0="${DEV_MNEMONIC_0:-copper push brief egg scan entry inform record adjust fossil boss egg comic alien upon aspect dry avoid interest fury window hint race symptom}"
DEV_MNEMONIC_1="${DEV_MNEMONIC_1:-maximum display century economy unlock van census kite error heart snow filter midnight usage egg venture cash kick motor survey drastic edge muffin visual}"
DEV_MNEMONIC_2="${DEV_MNEMONIC_2:-will wear settle write dance topic tape sea glory hotel oppose rebel client problem era video gossip glide during yard balance cancel file rose}"
DEV_MNEMONIC_3="${DEV_MNEMONIC_3:-doll midnight silk carpet brush boring pluck office gown inquiry duck chief aim exit gain never tennis crime fragile ship cloud surface exotic patch}"

# ---------- Setup temp working directory ----------
TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

CHAINDIR="$TMPDIR/nodedata"
GENESIS="$CHAINDIR/config/genesis.json"
TMP_GENESIS="$CHAINDIR/config/tmp_genesis.json"
CONFIG_TOML="$CHAINDIR/config/config.toml"
APP_TOML="$CHAINDIR/config/app.toml"

echo "==> Initializing chain (chain-id: $CHAIN_ID, denom: $DENOM)"

# ---------- Init ----------
tadad config set client chain-id "$CHAIN_ID" --home "$CHAINDIR"
tadad config set client keyring-backend "$KEYRING" --home "$CHAINDIR"

# create validator account using secp256k1
echo "$VALIDATOR_MNEMONIC" | tadad keys add "$VAL_KEY" --recover \
  --keyring-backend "$KEYRING" --algo "$KEYALGO" --home "$CHAINDIR"

# create validator block create/signing key using ed25519 with init command
echo "$VALIDATOR_MNEMONIC" | tadad init "$MONIKER" -o \
  --chain-id "$CHAIN_ID" --home "$CHAINDIR" --recover

# ---------- Genesis: denom ----------
echo "==> Customizing genesis.json"
jq ".app_state[\"staking\"][\"params\"][\"bond_denom\"]=\"$DENOM\""                         "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
jq ".app_state[\"gov\"][\"deposit_params\"][\"min_deposit\"][0][\"denom\"]=\"$DENOM\""      "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
jq ".app_state[\"gov\"][\"params\"][\"min_deposit\"][0][\"denom\"]=\"$DENOM\""              "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
jq ".app_state[\"gov\"][\"params\"][\"expedited_min_deposit\"][0][\"denom\"]=\"$DENOM\""   "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
jq ".app_state[\"evm\"][\"params\"][\"evm_denom\"]=\"$DENOM\""                             "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
jq ".app_state[\"mint\"][\"params\"][\"mint_denom\"]=\"$DENOM\""                           "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

# ---------- Genesis: bank denom metadata ----------
jq '.app_state["bank"]["denom_metadata"]=[{"description":"The native EVM token.","denom_units":[{"denom":"wei","exponent":0,"aliases":["attoeth"]},{"denom":"gwei","exponent":9,"aliases":[]},{"denom":"eth","exponent":18,"aliases":[]}],"base":"wei","display":"eth","name":"Ether","symbol":"ETH","uri":"","uri_hash":""}]' \
  "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

# ---------- Genesis: EVM / precompiles ----------
jq '.app_state["evm"]["params"]["active_static_precompiles"]=[
  "0x0000000000000000000000000000000000000100",
  "0x0000000000000000000000000000000000000400",
  "0x0000000000000000000000000000000000000800",
  "0x0000000000000000000000000000000000000801",
  "0x0000000000000000000000000000000000000802",
  "0x0000000000000000000000000000000000000803",
  "0x0000000000000000000000000000000000000804",
  "0x0000000000000000000000000000000000000805",
  "0x0000000000000000000000000000000000000806",
  "0x0000000000000000000000000000000000000807"
]' "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

jq '.app_state.erc20.native_precompiles=["0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"]' \
  "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
jq ".app_state.erc20.token_pairs=[{
  contract_owner: 1,
  erc20_address: \"0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE\",
  denom: \"$DENOM\",
  enabled: true
}]" "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

# ---------- Genesis: block gas limit ----------
jq '.consensus.params.block.max_gas="10000000"' "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

# ---------- Genesis: governance periods ----------
sed -i.bak 's/"max_deposit_period": "172800s"/"max_deposit_period": "30s"/g'       "$GENESIS"
sed -i.bak 's/"voting_period": "172800s"/"voting_period": "30s"/g'                 "$GENESIS"
sed -i.bak 's/"expedited_voting_period": "86400s"/"expedited_voting_period": "15s"/g' "$GENESIS"

# ---------- Genesis: staking / slashing testnet params ----------
jq '.app_state["staking"]["params"]["unbonding_time"]="21s"'          "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
jq '.app_state["slashing"]["params"]["downtime_jail_duration"]="60s"' "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
jq '.app_state["slashing"]["params"]["signed_blocks_window"]="10"'    "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

# ---------- Genesis: accounts ----------
echo "==> Adding genesis accounts"

tadad genesis add-genesis-account "$VAL_KEY" 100000000000000000000000000"$DENOM" \
  --keyring-backend "$KEYRING" --home "$CHAINDIR"

echo "$FAUCET_MNEMONIC" | tadad keys add "faucetAccount" --recover \
  --keyring-backend "$KEYRING" --algo "$KEYALGO" --home "$CHAINDIR"
  
tadad genesis add-genesis-account "faucetAccount" 1000000000000000000000000000000000"$DENOM" \
  --keyring-backend "$KEYRING" --home "$CHAINDIR"

for i in 0 1 2 3; do
  mnemonic_var="DEV_MNEMONIC_${i}"
  echo "${!mnemonic_var}" | tadad keys add "dev${i}" --recover \
    --keyring-backend "$KEYRING" --algo "$KEYALGO" --home "$CHAINDIR"
  tadad genesis add-genesis-account "dev${i}" 1000000000000000000000"$DENOM" \
    --keyring-backend "$KEYRING" --home "$CHAINDIR"
done

# ---------- Genesis: gentx + collect ----------
echo "==> Generating validator gentx"
tadad genesis gentx "$VAL_KEY" 1000000000000000000000"$DENOM" \
  --gas-prices "${BASEFEE}${DENOM}" \
  --keyring-backend "$KEYRING" \
  --chain-id "$CHAIN_ID" \
  --home "$CHAINDIR"

tadad genesis collect-gentxs --home "$CHAINDIR"

# ---------- Genesis: fix residual default denom from gentx ----------
jq "(.app_state.genutil.gen_txs[].body.messages[].value.denom) = \"$DENOM\""                            "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
jq "(.app_state.genutil.gen_txs[].auth_info.fee.amount[].denom) = \"$DENOM\""                           "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
jq "(.app_state.bank.balances[].coins[] | select(.denom == \"atest\") | .denom) = \"$DENOM\""           "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
jq "(.app_state.bank.supply[] | select(.denom == \"atest\") | .denom) = \"$DENOM\""                     "$GENESIS" > "$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

tadad genesis validate-genesis --home "$CHAINDIR"
echo "==> genesis.json validated OK"

# ---------- config.toml: consensus timeouts (shared base) ----------
# These are applied to all node types.
apply_consensus_timeouts() {
  local toml="$1"
  sed -i.bak 's/timeout_propose = "3s"/timeout_propose = "2s"/g'                   "$toml"
  sed -i.bak 's/timeout_propose_delta = "500ms"/timeout_propose_delta = "200ms"/g' "$toml"
  sed -i.bak 's/timeout_prevote = "1s"/timeout_prevote = "500ms"/g'                "$toml"
  sed -i.bak 's/timeout_prevote_delta = "500ms"/timeout_prevote_delta = "200ms"/g' "$toml"
  sed -i.bak 's/timeout_precommit = "1s"/timeout_precommit = "500ms"/g'            "$toml"
  sed -i.bak 's/timeout_precommit_delta = "500ms"/timeout_precommit_delta = "200ms"/g' "$toml"
  sed -i.bak 's/timeout_commit = "5s"/timeout_commit = "1s"/g'                     "$toml"
  sed -i.bak 's/timeout_broadcast_tx_commit = "10s"/timeout_broadcast_tx_commit = "5s"/g' "$toml"
}

# ---------- Output directories ----------
mkdir -p "$OUTPUT_DIR/validator" "$OUTPUT_DIR/full" "$OUTPUT_DIR/archive"

# ============================================================
# VALIDATOR
# ============================================================
echo "==> Generating validator configs"

cp "$GENESIS" "$OUTPUT_DIR/validator/genesis.json"

# config.toml
cp "$CONFIG_TOML" "$OUTPUT_DIR/validator/config.toml"
apply_consensus_timeouts "$OUTPUT_DIR/validator/config.toml"
sed -i.bak 's/prometheus = false/prometheus = true/'                              "$OUTPUT_DIR/validator/config.toml"
sed -i.bak 's/type = "flood"/type = "app"/g'                                      "$OUTPUT_DIR/validator/config.toml"

# app.toml
cp "$APP_TOML" "$OUTPUT_DIR/validator/app.toml"
sed -i.bak 's/prometheus-retention-time  = "0"/prometheus-retention-time  = "1000000000000"/g' "$OUTPUT_DIR/validator/app.toml"
sed -i.bak 's/enabled = false/enabled = true/g'                                   "$OUTPUT_DIR/validator/app.toml"
sed -i.bak 's/enable = false/enable = true/g'                                     "$OUTPUT_DIR/validator/app.toml"
sed -i.bak 's/enable-indexer = false/enable-indexer = true/g'                    "$OUTPUT_DIR/validator/app.toml"
sed -i.bak "s/evm-chain-id = [0-9]*/evm-chain-id = $EVM_CHAIN_ID/g"             "$OUTPUT_DIR/validator/app.toml"

# ============================================================
# FULL NODE
# ============================================================
echo "==> Generating full node configs"

cp "$OUTPUT_DIR/validator/genesis.json" "$OUTPUT_DIR/full/genesis.json"

# config.toml
cp "$CONFIG_TOML" "$OUTPUT_DIR/full/config.toml"
apply_consensus_timeouts "$OUTPUT_DIR/full/config.toml"
sed -i.bak "s|persistent_peers = \"\"|persistent_peers = \"$PERSISTENT_PEERS\"|g" "$OUTPUT_DIR/full/config.toml"
# RPC: open for external access
sed -i.bak 's|laddr = "tcp://127.0.0.1:26657"|laddr = "tcp://0.0.0.0:26657"|g'   "$OUTPUT_DIR/full/config.toml"

# app.toml
cp "$APP_TOML" "$OUTPUT_DIR/full/app.toml"
sed -i.bak 's/enabled = false/enabled = true/g'                                   "$OUTPUT_DIR/full/app.toml"
sed -i.bak 's/enable = false/enable = true/g'                                     "$OUTPUT_DIR/full/app.toml"
# JSON-RPC: open for external access
sed -i.bak 's|address = "127.0.0.1:8545"|address = "0.0.0.0:8545"|g'             "$OUTPUT_DIR/full/app.toml"
sed -i.bak "s/evm-chain-id = [0-9]*/evm-chain-id = $EVM_CHAIN_ID/g"             "$OUTPUT_DIR/full/app.toml"
# Snapshot: enable for state sync
sed -i.bak 's/snapshot-interval = 0/snapshot-interval = 1000/g'                  "$OUTPUT_DIR/full/app.toml"
sed -i.bak 's/snapshot-keep-recent = 2/snapshot-keep-recent = 2/g'               "$OUTPUT_DIR/full/app.toml"

# ============================================================
# ARCHIVE NODE
# ============================================================
echo "==> Generating archive node configs"

cp "$OUTPUT_DIR/validator/genesis.json" "$OUTPUT_DIR/archive/genesis.json"

# config.toml
cp "$CONFIG_TOML" "$OUTPUT_DIR/archive/config.toml"
apply_consensus_timeouts "$OUTPUT_DIR/archive/config.toml"
sed -i.bak "s|persistent_peers = \"\"|persistent_peers = \"$PERSISTENT_PEERS\"|g" "$OUTPUT_DIR/archive/config.toml"

# app.toml
cp "$APP_TOML" "$OUTPUT_DIR/archive/app.toml"
sed -i.bak 's/enabled = false/enabled = true/g'                                   "$OUTPUT_DIR/archive/app.toml"
sed -i.bak 's/enable = false/enable = true/g'                                     "$OUTPUT_DIR/archive/app.toml"
sed -i.bak 's/enable-indexer = false/enable-indexer = true/g'                    "$OUTPUT_DIR/archive/app.toml"
# Archive: keep all historical state
sed -i.bak 's/pruning = "default"/pruning = "nothing"/g'                          "$OUTPUT_DIR/archive/app.toml"
sed -i.bak "s/evm-chain-id = [0-9]*/evm-chain-id = $EVM_CHAIN_ID/g"             "$OUTPUT_DIR/archive/app.toml"

# ---------- Cleanup .bak files ----------
find "$OUTPUT_DIR" -name "*.bak" -delete

echo ""
echo "==> Done. Output written to: $OUTPUT_DIR"
echo ""
echo "  configs/validator/genesis.json      <- shared with full/archive nodes"
echo "  configs/validator/config.toml"
echo "  configs/validator/app.toml"
echo "  configs/full/config.toml"
echo "  configs/full/app.toml"
echo "  configs/archive/config.toml"
echo "  configs/archive/app.toml"
echo ""
echo "================================================================"
echo "  node_key.json  (upload to 1Password: vaults/testnet/items/validator-node-key-json)"
echo "================================================================"
cat "$CHAINDIR/config/node_key.json"
echo ""
echo "================================================================"
echo "  priv_validator_key.json is NOT saved — regenerated at Pod start"
echo "  via: tadad init --recover (VALIDATOR_MNEMONIC)"
echo "================================================================"
if [[ "$PERSISTENT_PEERS" == *"__VALIDATOR"* ]]; then
  echo ""
  echo "  WARNING: persistent_peers in full/archive config.toml contains placeholders."
  echo "  Set VALIDATOR_NODE_ID and VALIDATOR_ENDPOINT before running, or update manually:"
  echo "    VALIDATOR_NODE_ID=\$(tadad comet show-node-id --home <validator-home>)"
  echo "    VALIDATOR_ENDPOINT=<host>:26656"
fi
