# Double Ratchet client flow (TypeScript)

This guide shows how riders and drivers can run a Signal-style Double Ratchet locally and send encrypted
envelopes to `RideHail.postEncryptedMessage`. The chain never decrypts.

## Header format

The on-chain envelope header uses a fixed 73-byte layout:

```
version (1) | dhPub (32) | pn (4) | n (4) | adHash (32)
```

Use the Double Ratchet precompile at `0x0000000000000000000000000000000000000808` to validate the header
before submitting a transaction.

## TypeScript sketch

```ts
import { keccak256 } from "@ethersproject/keccak256";
import { arrayify, hexConcat, hexZeroPad } from "@ethersproject/bytes";
import { ethers } from "ethers";

const PRECOMPILE = "0x0000000000000000000000000000000000000808";
const HEADER_VERSION = 1;

type Envelope = {
  header: Uint8Array;
  ciphertext: Uint8Array;
  pn: number;
  n: number;
};

// Associated data binds the session, participants, and chain.
function adHash(sessionId: bigint, rider: string, driver: string, chainId: bigint): Uint8Array {
  const ad = ethers.utils.defaultAbiCoder.encode(
    ["uint256", "address", "address", "uint256"],
    [sessionId, rider, driver, chainId]
  );
  return arrayify(keccak256(ad));
}

// Header builder: version | dhPub | pn | n | adHash
function buildHeader(dhPub: Uint8Array, pn: number, n: number, ad: Uint8Array): Uint8Array {
  if (dhPub.length !== 32 || ad.length !== 32) {
    throw new Error("invalid dhPub/adHash length");
  }
  const pnBytes = arrayify(hexZeroPad(ethers.utils.hexlify(pn), 4));
  const nBytes = arrayify(hexZeroPad(ethers.utils.hexlify(n), 4));
  return arrayify(hexConcat([Uint8Array.from([HEADER_VERSION]), dhPub, pnBytes, nBytes, ad]));
}

// Placeholder: use a real Double Ratchet library or implementation.
function ratchetEncrypt(
  plaintext: Uint8Array,
  ad: Uint8Array
): { dhPub: Uint8Array; pn: number; n: number; ciphertext: Uint8Array } {
  // Implement Double Ratchet state machine:
  // - derive message key from chain key
  // - AEAD encrypt with ad
  // - advance chain key and counters
  throw new Error("implement me");
}

async function validateEnvelope(
  provider: ethers.providers.Provider,
  header: Uint8Array,
  ciphertext: Uint8Array,
  maxHeaderBytes: number,
  maxCiphertextBytes: number
) {
  const iface = new ethers.utils.Interface([
    "function validateEnvelope(bytes header, bytes ciphertext, uint32 maxHeaderBytes, uint32 maxCiphertextBytes) returns (bool valid, bytes32 envelopeHash, uint8 version, bytes32 dhPub, uint32 pn, uint32 n, bytes32 adHash)"
  ]);
  const callData = iface.encodeFunctionData("validateEnvelope", [
    header,
    ciphertext,
    maxHeaderBytes,
    maxCiphertextBytes
  ]);
  const res = await provider.call({ to: PRECOMPILE, data: callData });
  const [valid] = iface.decodeFunctionResult("validateEnvelope", res);
  if (!valid) throw new Error("invalid envelope");
}

async function riderSendEncrypted(
  provider: ethers.providers.Provider,
  rideHail: ethers.Contract,
  sessionId: bigint,
  rider: string,
  driver: string,
  chainId: bigint,
  maxHeaderBytes: number,
  maxCiphertextBytes: number,
  plaintext: Uint8Array,
  msgIndex: number,
  messageBondWei: bigint
) {
  const ad = adHash(sessionId, rider, driver, chainId);
  const { dhPub, pn, n, ciphertext } = ratchetEncrypt(plaintext, ad);
  const header = buildHeader(dhPub, pn, n, ad);

  await validateEnvelope(provider, header, ciphertext, maxHeaderBytes, maxCiphertextBytes);
  await rideHail.postEncryptedMessage(sessionId, msgIndex, header, ciphertext, { value: messageBondWei });
}
```

## Session bootstrap

- Drivers publish a long-term X25519 identity DH key and an Ed25519 identity signing key, plus a
  signed X25519 prekey on-chain using the Key Registry precompile at
  `0x0000000000000000000000000000000000000809`.
- Riders resolve the bundle after `Matched`, verify the signed prekey with Ed25519, then run X3DH:
  `DH1 = IK_A * SPK_B`, `DH2 = EK_A * IK_B`, `DH3 = EK_A * SPK_B`, and HKDF to derive the shared secret.
- Riders send the first encrypted message using the derived shared secret; drivers respond with their
  ratchet public key in the header and continue the Double Ratchet.
- All subsequent coordination messages are encrypted and posted as ciphertext envelopes on-chain.

## Key registry usage

```ts
const keyRegistry = new ethers.Contract("0x0000000000000000000000000000000000000809", [
  "function publishKeysV2(bytes32 identityDhKey, bytes32 identitySignKey, bytes32 signedPreKey, bytes signature, uint64 expiresAt)",
  "function getKeys(address owner) view returns (tuple(bytes32 identityDhKey, bytes32 identitySignKey, bytes32 signedPreKey, bytes signature, uint64 expiresAt, uint64 updatedAt))"
], signer);

// Driver publishes identity keys + signed prekey (signature verified by clients).
await keyRegistry.publishKeysV2(identityDhKey, identitySignKey, signedPreKey, signature, expiresAt);

// Rider reads and verifies signature off-chain.
const bundle = await keyRegistry.getKeys(driver);
```

## Reorg handling

- Treat `Matched` as soft until finality; hold ratchet state in memory until block confirmations.
- If a reorg removes a message, increment the `msgIndex` and resend with a fresh ratchet step.
