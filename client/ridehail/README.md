# RideHail client tests (TypeScript)

Minimal client-side checks for Double Ratchet envelopes, key registry publishing, and RideHail messaging.

## Setup

```
npm install
```

## Environment

Set these variables before running tests:

- `RPC_URL` (required)
- `PRIVATE_KEY` (required)
- `CHAIN_ID` (optional, default: provider chainId)
- `RIDEHAIL_ADDRESS` (optional, default: `0x000000000000000000000000000000000000080a`)
- `KEYREGISTRY_ADDRESS` (optional, default: `0x0000000000000000000000000000000000000809`)
- `SESSION_ID` (optional, for message posting)
- `MESSAGE_BOND_WEI` (optional, default: 0)
- `MAX_HEADER_BYTES` (optional, default: 256)
- `MAX_CIPHERTEXT_BYTES` (optional, default: 512)

## Run

```
npm test
```

## Sample flow script

This script walks through the end-to-end flow with X3DH + Double Ratchet:

```
RPC_URL=... RIDER_KEY=... DRIVER_KEY=... RIDEHAIL_ADDRESS=... npm run flow
```

Defaults:
- `MESSAGE_BOND_WEI` defaults to `0.01` ether to match the RideHail precompile.
