#!/usr/bin/env bash

set -euo pipefail

ROLE="${1:-}"
NODE1_HOME="${NODE1_HOME:-/data/node1/.evmd}"
CHAIN_ID="${CHAIN_ID:-9001}"
KEYRING="test"
KEYALGO="eth_secp256k1"
DATA_DIR="${DATA_DIR:-/data}"
INIT_FLAG="${DATA_DIR}/.multinode/init.done"
FORCE_INIT="${FORCE_INIT:-0}"

DEV_MNEMONICS=(
  "copper push brief egg scan entry inform record adjust fossil boss egg comic alien upon aspect dry avoid interest fury window hint race symptom"
  "maximum display century economy unlock van census kite error heart snow filter midnight usage egg venture cash kick motor survey drastic edge muffin visual"
  "will wear settle write dance topic tape sea glory hotel oppose rebel client problem era video gossip glide during yard balance cancel file rose"
  "doll midnight silk carpet brush boring pluck office gown inquiry duck chief aim exit gain never tennis crime fragile ship cloud surface exotic patch"
)

wait_for_file() {
  local path="$1"
  local i=0
  while [[ ! -f "$path" ]]; do
    i=$((i + 1))
    if [[ $i -gt 120 ]]; then
      echo "timeout waiting for $path"
      exit 1
    fi
    sleep 1
  done
}

wait_for_node1_id() {
  local i=0
  while [[ $i -le 120 ]]; do
    if NODE1_ID="$(curl -sf http://evmd-node1:26657/status | jq -r '.result.node_info.id' 2>/dev/null)"; then
      if [[ -n "$NODE1_ID" && "$NODE1_ID" != "null" ]]; then
        echo "$NODE1_ID"
        return 0
      fi
    fi
    i=$((i + 1))
    sleep 1
  done
  return 1
}

key_exists() {
  local name="$1"
  local home="$2"
  evmd keys show "$name" --home "$home" --keyring-backend "$KEYRING" >/dev/null 2>&1
}

case "$ROLE" in
  node1)
    export HOME="${HOME:-/data/node1}"
    if [[ "$FORCE_INIT" == "1" ]]; then
      rm -rf "${DATA_DIR}/node1" "${DATA_DIR}/node2" "${DATA_DIR}/node3" "${DATA_DIR}/node4"
      rm -f "$INIT_FLAG"
    fi

    if [[ ! -f "$INIT_FLAG" ]]; then
      if [[ -f "$NODE1_HOME/config/genesis.json" && "$FORCE_INIT" != "1" ]]; then
        echo "genesis already exists. remove ./.multinode or set FORCE_INIT=1"
        exit 1
      fi

      echo "initializing multi-validator genesis..."
      mkdir -p "$DATA_DIR"
      # init node homes
      for i in 1 2 3 4; do
        if [[ "$FORCE_INIT" == "1" ]]; then
          evmd init "node${i}" --chain-id "$CHAIN_ID" --home "${DATA_DIR}/node${i}/.evmd" -o
        else
          evmd init "node${i}" --chain-id "$CHAIN_ID" --home "${DATA_DIR}/node${i}/.evmd"
        fi
      done

      GENESIS="${NODE1_HOME}/config/genesis.json"
      CONFIG_TOML="${NODE1_HOME}/config/config.toml"
      APP_TOML="${NODE1_HOME}/config/app.toml"
      TMP_GENESIS="${NODE1_HOME}/config/tmp_genesis.json"

      # apply same local dev tweaks as local_node.sh
      jq '.app_state["staking"]["params"]["bond_denom"]="atest"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
      jq '.app_state["gov"]["deposit_params"]["min_deposit"][0]["denom"]="atest"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
      jq '.app_state["gov"]["params"]["min_deposit"][0]["denom"]="atest"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
      jq '.app_state["gov"]["params"]["expedited_min_deposit"][0]["denom"]="atest"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
      jq '.app_state["mint"]["params"]["mint_denom"]="atest"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
      jq '.app_state["bank"]["denom_metadata"]=[{"description":"The native staking token for evmd.","denom_units":[{"denom":"atest","exponent":0,"aliases":["attotest"]},{"denom":"test","exponent":18,"aliases":[]}],"base":"atest","display":"test","name":"Test Token","symbol":"TEST","uri":"","uri_hash":""}]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
      jq '.app_state["evm"]["params"]["active_static_precompiles"]=["0x0000000000000000000000000000000000000100","0x0000000000000000000000000000000000000400","0x0000000000000000000000000000000000000800","0x0000000000000000000000000000000000000801","0x0000000000000000000000000000000000000802","0x0000000000000000000000000000000000000803","0x0000000000000000000000000000000000000804","0x0000000000000000000000000000000000000805","0x0000000000000000000000000000000000000806","0x0000000000000000000000000000000000000807"]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
      jq '.app_state["evm"]["params"]["evm_denom"]="atest"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
      jq '.app_state.erc20.native_precompiles=["0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
      jq '.app_state.erc20.token_pairs=[{contract_owner:1,erc20_address:"0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",denom:"atest",enabled:true}]' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"
      jq '.consensus.params.block.max_gas="10000000"' "$GENESIS" >"$TMP_GENESIS" && mv "$TMP_GENESIS" "$GENESIS"

      sed -i.bak 's/"max_deposit_period": "172800s"/"max_deposit_period": "30s"/g' "$GENESIS"
      sed -i.bak 's/"voting_period": "172800s"/"voting_period": "30s"/g' "$GENESIS"
      sed -i.bak 's/"expedited_voting_period": "86400s"/"expedited_voting_period": "15s"/g' "$GENESIS"

      sed -i.bak 's/timeout_propose = "3s"/timeout_propose = "2s"/g' "$CONFIG_TOML"
      sed -i.bak 's/timeout_propose_delta = "500ms"/timeout_propose_delta = "200ms"/g' "$CONFIG_TOML"
      sed -i.bak 's/timeout_prevote = "1s"/timeout_prevote = "500ms"/g' "$CONFIG_TOML"
      sed -i.bak 's/timeout_prevote_delta = "500ms"/timeout_prevote_delta = "200ms"/g' "$CONFIG_TOML"
      sed -i.bak 's/timeout_precommit = "1s"/timeout_precommit = "500ms"/g' "$CONFIG_TOML"
      sed -i.bak 's/timeout_precommit_delta = "500ms"/timeout_precommit_delta = "200ms"/g' "$CONFIG_TOML"
      sed -i.bak 's/timeout_commit = "5s"/timeout_commit = "1s"/g' "$CONFIG_TOML"
      sed -i.bak 's/timeout_broadcast_tx_commit = "10s"/timeout_broadcast_tx_commit = "5s"/g' "$CONFIG_TOML"

      sed -i.bak 's/addrbook_strict = true/addrbook_strict = false/' "$CONFIG_TOML"
      sed -i.bak 's/allow_duplicate_ip = false/allow_duplicate_ip = true/' "$CONFIG_TOML"
      sed -i.bak 's/prometheus = false/prometheus = true/' "$CONFIG_TOML"
      sed -i.bak 's/prometheus-retention-time  = "0"/prometheus-retention-time  = "1000000000000"/g' "$APP_TOML"
      sed -i.bak 's/enabled = false/enabled = true/g' "$APP_TOML"
      sed -i.bak 's/enable = false/enable = true/g' "$APP_TOML"
      sed -i.bak 's/enable-indexer = false/enable-indexer = true/g' "$APP_TOML"

      # add dev accounts
      for i in 0 1 2 3; do
        name="dev${i}"
        mnemonic="${DEV_MNEMONICS[$i]}"
        if ! key_exists "$name" "$NODE1_HOME"; then
          echo "$mnemonic" | evmd keys add "$name" --recover --keyring-backend "$KEYRING" --algo "$KEYALGO" --home "$NODE1_HOME"
        fi
        addr="$(evmd keys show "$name" -a --keyring-backend "$KEYRING" --home "$NODE1_HOME")"
        evmd genesis add-genesis-account "$addr" 1000000000000000000000atest --home "$NODE1_HOME"
      done

      # create validator keys in each node home and add genesis accounts
      for i in 1 2 3 4; do
        vname="val${i}"
        vhome="${DATA_DIR}/node${i}/.evmd"
        if ! key_exists "$vname" "$vhome"; then
          evmd keys add "$vname" --keyring-backend "$KEYRING" --algo "$KEYALGO" --home "$vhome"
        fi
        vaddr="$(evmd keys show "$vname" -a --keyring-backend "$KEYRING" --home "$vhome")"
        evmd genesis add-genesis-account "$vaddr" 100000000000000000000000000atest --home "$NODE1_HOME"
      done

      # sync updated genesis to other nodes before gentx validation
      for i in 2 3 4; do
        nhome="${DATA_DIR}/node${i}/.evmd"
        cp "$GENESIS" "${nhome}/config/genesis.json"
      done

      # gentx for each validator
      rm -rf "${NODE1_HOME}/config/gentx"
      mkdir -p "${NODE1_HOME}/config/gentx"
      for i in 1 2 3 4; do
        vname="val${i}"
        vhome="${DATA_DIR}/node${i}/.evmd"
        evmd genesis gentx "$vname" 1000000000000000000000atest --gas-prices 0atest --keyring-backend "$KEYRING" --chain-id "$CHAIN_ID" --home "$vhome"
        if compgen -G "${vhome}/config/gentx/*.json" > /dev/null; then
          cp "${vhome}/config/gentx/"*.json "${NODE1_HOME}/config/gentx/" || true
        fi
      done

      evmd genesis collect-gentxs --home "$NODE1_HOME"
      evmd genesis validate-genesis --home "$NODE1_HOME"

      # copy finalized genesis + configs to other nodes
      for i in 2 3 4; do
        nhome="${DATA_DIR}/node${i}/.evmd"
        cp "$GENESIS" "${nhome}/config/genesis.json"
        cp "$CONFIG_TOML" "${nhome}/config/config.toml"
        cp "$APP_TOML" "${nhome}/config/app.toml"
      done

      mkdir -p "$(dirname "$INIT_FLAG")"
      touch "$INIT_FLAG"
    fi

    exec evmd start \
      --pruning nothing \
      --log_level info \
      --minimum-gas-prices=0atest \
      --evm.min-tip=0 \
      --home "$NODE1_HOME" \
      --rpc.laddr tcp://0.0.0.0:26657 \
      --json-rpc.address 0.0.0.0:8545 \
      --json-rpc.ws-address 0.0.0.0:8546 \
      --json-rpc.api eth,txpool,personal,net,debug,web3,mvl \
      --chain-id "$CHAIN_ID"
    ;;
  node2|node3|node4)
    export HOME="${HOME:-/data/${ROLE}}"
    NODE_HOME="${HOME}/.evmd"
    wait_for_file "$INIT_FLAG"

    NODE1_ID="$(wait_for_node1_id || true)"
    if [[ -z "${NODE1_ID:-}" ]]; then
      echo "failed to read node1 id from node1 rpc"
      exit 1
    fi

    exec evmd start \
      --pruning nothing \
      --log_level info \
      --minimum-gas-prices=0atest \
      --evm.min-tip=0 \
      --home "$NODE_HOME" \
      --rpc.laddr tcp://0.0.0.0:26657 \
      --json-rpc.address 0.0.0.0:8545 \
      --json-rpc.ws-address 0.0.0.0:8546 \
      --json-rpc.api eth,txpool,personal,net,debug,web3,mvl \
      --chain-id "$CHAIN_ID" \
      --p2p.seeds "${NODE1_ID}@evmd-node1:26656"
    ;;
  *)
    echo "usage: $0 node1|node2|node3|node4"
    exit 1
    ;;
esac
