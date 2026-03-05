CHAINDIR="$HOME/.evmd"
# Path variables
CONFIG_TOML=$CHAINDIR/config/config.toml
APP_TOML=$CHAINDIR/config/app.toml
CLIENT_TOML=$CHAINDIR/config/client.toml

# ---------- Config customizations ----------
sed -i.bak 's/timeout_propose = "3s"/timeout_propose = "2s"/g' "$CONFIG_TOML"
sed -i.bak 's/tcp:\/\/127.0.0.1:26657/tcp:\/\/0.0.0.0:26657/g' "$CONFIG_TOML"
sed -i.bak 's/tcp:\/\/127.0.0.1:26656/tcp:\/\/0.0.0.0:26656/g' "$CONFIG_TOML"
sed -i.bak 's/timeout_propose_delta = "500ms"/timeout_propose_delta = "200ms"/g' "$CONFIG_TOML"
sed -i.bak 's/timeout_prevote = "1s"/timeout_prevote = "500ms"/g' "$CONFIG_TOML"
sed -i.bak 's/timeout_prevote_delta = "500ms"/timeout_prevote_delta = "200ms"/g' "$CONFIG_TOML"
sed -i.bak 's/timeout_precommit = "1s"/timeout_precommit = "500ms"/g' "$CONFIG_TOML"
sed -i.bak 's/timeout_precommit_delta = "500ms"/timeout_precommit_delta = "200ms"/g' "$CONFIG_TOML"
sed -i.bak 's/timeout_commit = "5s"/timeout_commit = "1s"/g' "$CONFIG_TOML"
sed -i.bak 's/timeout_broadcast_tx_commit = "10s"/timeout_broadcast_tx_commit = "5s"/g' "$CONFIG_TOML"

# enable prometheus metrics and all APIs for dev node
sed -i.bak 's/prometheus = false/prometheus = true/' "$CONFIG_TOML"
sed -i.bak 's/minimum-gas-prices = "0aatom"/minimum-gas-prices = "0.0001atest"/g' "$CONFIG_TOML"
sed -i.bak 's/prometheus-retention-time  = "0"/prometheus-retention-time  = "1000000000000"/g' "$APP_TOML"
sed -i.bak 's/enabled = false/enabled = true/g' "$APP_TOML"
sed -i.bak 's/enable = false/enable = true/g' "$APP_TOML"
sed -i.bak 's/enable-indexer = false/enable-indexer = true/g' "$APP_TOML"

sed -i.bak 's/chain-id = ""/chain-id = "9001"/g' "$CLIENT_TOML"
sed -i.bak 's/node = "tcp:\/\/localhost:26657"/node = "tcp:\/\/0.0.0.0:26657"/g' "$CLIENT_TOML"

#set peer
#PEERS=[node-id]@[local-ip-address]:26656
#sed -i.bak -e "s/^persistent_peers *=.*/persistent_peers = \"$PEERS\"/" $HOME/.evmd/config/config.toml