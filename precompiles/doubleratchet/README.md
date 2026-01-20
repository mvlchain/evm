# Double Ratchet Precompile

The Double Ratchet precompile validates encrypted envelope formatting without decrypting any payloads.

Address: `0x0000000000000000000000000000000000000808`

## Envelope format

The precompile assumes a fixed header format:

- `version` (1 byte): must be `0x01`
- `dhPub` (32 bytes): sender DH ratchet public key
- `pn` (4 bytes): previous chain length
- `n` (4 bytes): message number in current chain
- `adHash` (32 bytes): hash of associated data (e.g., sessionId, participants, chainId)

Total header length: 73 bytes.

## ABI

`validateEnvelope(bytes header, bytes ciphertext, uint32 maxHeaderBytes, uint32 maxCiphertextBytes)`

Returns:
- `valid` (bool)
- `envelopeHash` (bytes32): `keccak256(header || ciphertext)`
- `version` (uint8)
- `dhPub` (bytes32)
- `pn` (uint32)
- `n` (uint32)
- `adHash` (bytes32)

The precompile reverts on malformed data (invalid length/version, empty ciphertext, or size over limit).
