# RideHail Precompile

RideHail is available as a static precompile at:

`0x000000000000000000000000000000000000080a`

This precompile mirrors the core RideHail request/match flow and encrypted messaging.

Notes:
- The precompile uses fixed parameters (deposit/bond sizes, commit/reveal windows).
- TODO: bring full parity with `contracts/solidity/RideHail.sol` (rate limits, cancellations, settlement, slashing).
