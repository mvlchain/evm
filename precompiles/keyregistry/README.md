# Key Registry Precompile

The Key Registry precompile stores X3DH identity keys and signed prekeys on-chain using EVM storage.

Address: `0x0000000000000000000000000000000000000809`

## ABI

`publishKeysV2(bytes32 identityDhKey, bytes32 identitySignKey, bytes32 signedPreKey, bytes signature, uint64 expiresAt)`

`getKeys(address owner) -> (identityDhKey, identitySignKey, signedPreKey, signature, expiresAt, updatedAt)`

Notes:
- The precompile validates non-zero keys and non-empty signatures (<= 96 bytes).
- Signatures are stored as dynamic bytes.
