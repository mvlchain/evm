# Match Precompile

`x/match` replay index query precompile for EVM contracts.

- Address: `0x0000000000000000000000000000000000000808`
- ABI: [`abi.json`](./abi.json)
- Solidity interface: [`MatchI.sol`](./MatchI.sol)

## Methods

- `hasReplay(string poolId, string intentId) -> bool`
- `getReplay(string poolId, string intentId) -> (bool found, string matchId)`
- `getReplayParties(string poolId, string intentId) -> (bool found, string matchId, string requester, string responder)`

The precompile exposes replay index state (`pool_id`, `intent_id`, `match_id`) and on-chain requester/responder metadata.
It does not expose off-chain plaintext context data.
