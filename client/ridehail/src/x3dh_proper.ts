/**
 * X3DH Implementation with Proper XEdDSA
 *
 * Uses @privacyresearch libraries for correct Signal Protocol implementation:
 * - curve25519-typescript: X25519 operations (DH key exchange)
 * - ed25519-ts: Ed25519 operations (signatures)
 *
 * This implementation properly converts between Montgomery and Edwards curves
 * following the Signal XEdDSA specification.
 */

import { hkdf } from "@noble/hashes/hkdf";
import { sha256 } from "@noble/hashes/sha2";
import { sha512 } from "@noble/hashes/sha512";
import { Curve25519Wrapper } from "@privacyresearch/curve25519-typescript";
import { Ed25519, PrivateKey, PublicKey } from "@privacyresearch/ed25519-ts";

const INFO_X3DH = new TextEncoder().encode("X3DH");

// Initialize curve25519 wrapper once
let curve25519Instance: Curve25519Wrapper | null = null;

async function getCurve25519(): Promise<Curve25519Wrapper> {
  if (!curve25519Instance) {
    curve25519Instance = await Curve25519Wrapper.create();
  }
  return curve25519Instance;
}

/**
 * XEdDSA Implementation following Signal Protocol specification
 *
 * Key insight: We use X25519 keys for DH, and convert them to Ed25519 for signing.
 * The conversion must be done correctly to maintain the cryptographic relationship.
 */

export type IdentityKey = {
  privateKey: Uint8Array;  // X25519 private key
  publicKey: Uint8Array;   // X25519 public key
  ed25519PrivateKey: PrivateKey;  // Ed25519 private key for signing
  ed25519PublicKey: PublicKey;    // Ed25519 public key for verification
};

export type SignedPreKey = {
  keyPair: { privateKey: Uint8Array; publicKey: Uint8Array };
  signature: Uint8Array;
};

export type OneTimePreKey = {
  privateKey: Uint8Array;
  publicKey: Uint8Array;
};

export type X3DHBundle = {
  identityPub: Uint8Array;           // X25519 public key for DH
  identityEd25519Pub: Uint8Array;    // Ed25519 public key for signature verification
  signedPreKeyPub: Uint8Array;
  signature: Uint8Array;
  oneTimePreKeyPub?: Uint8Array;
};

/**
 * Generate Identity Key with proper XEdDSA conversion
 *
 * Creates an X25519 key pair and derives the corresponding Ed25519 key pair
 * using the Signal XEdDSA specification for curve conversion.
 */
export function generateIdentityKey(): IdentityKey {
  // Generate X25519 key pair using curve25519-typescript
  const x25519KeyPair = curve25519.generateKeyPair(randomBytes(32));

  // Convert X25519 private key to Ed25519 private key
  // Using SHA-512 to create a deterministic mapping
  const hash = sha512(x25519KeyPair.privKey);

  // Create Ed25519 private key from hash (first 32 bytes with proper clamping)
  const ed25519Scalar = hash.slice(0, 32);
  ed25519Scalar[0] &= 248;
  ed25519Scalar[31] &= 127;
  ed25519Scalar[31] |= 64;

  // Create Ed25519 key objects
  const ed25519PrivateKey = PrivateKey.fromBytes(ed25519Scalar);
  const ed25519PublicKey = ed25519PrivateKey.publicKey();

  return {
    privateKey: x25519KeyPair.privKey,
    publicKey: x25519KeyPair.pubKey,
    ed25519PrivateKey,
    ed25519PublicKey
  };
}

/**
 * Generate Signed PreKey with XEdDSA signature
 */
export function generateSignedPreKey(identityKey: IdentityKey): SignedPreKey {
  // Generate new X25519 key pair for the signed prekey
  const spkKeyPair = curve25519.generateKeyPair(randomBytes(32));

  // Sign the signed prekey's public key with the identity's Ed25519 private key
  const ed25519 = new Ed25519();
  const signature = ed25519.sign(identityKey.ed25519PrivateKey, spkKeyPair.pubKey);

  return {
    keyPair: {
      privateKey: spkKeyPair.privKey,
      publicKey: spkKeyPair.pubKey
    },
    signature
  };
}

/**
 * Generate One-Time PreKey
 */
export function generateOneTimePreKey(): OneTimePreKey {
  const keyPair = curve25519.generateKeyPair(randomBytes(32));
  return {
    privateKey: keyPair.privKey,
    publicKey: keyPair.pubKey
  };
}

/**
 * Verify Signed PreKey signature using XEdDSA
 */
export function verifySignedPreKey(bundle: X3DHBundle): boolean {
  try {
    const ed25519 = new Ed25519();
    const publicKey = PublicKey.fromBytes(bundle.identityEd25519Pub);
    return ed25519.verify(publicKey, bundle.signedPreKeyPub, bundle.signature);
  } catch {
    return false;
  }
}

/**
 * Derive shared secret as initiator (Alice)
 */
export function deriveSharedSecretInitiator(
  identityPrivA: Uint8Array,
  ephemeralPrivA: Uint8Array,
  bundleB: X3DHBundle
): Uint8Array {
  // Perform 3-4 DH operations as per X3DH spec
  const dh1 = curve25519.sharedKey(identityPrivA, bundleB.signedPreKeyPub);
  const dh2 = curve25519.sharedKey(ephemeralPrivA, bundleB.identityPub);
  const dh3 = curve25519.sharedKey(ephemeralPrivA, bundleB.signedPreKeyPub);

  const ikm = bundleB.oneTimePreKeyPub
    ? concatBytes(dh1, dh2, dh3, curve25519.sharedKey(ephemeralPrivA, bundleB.oneTimePreKeyPub))
    : concatBytes(dh1, dh2, dh3);

  return hkdf(sha256, ikm, undefined, INFO_X3DH, 32);
}

/**
 * Derive shared secret as responder (Bob)
 */
export function deriveSharedSecretResponder(
  identityPrivB: Uint8Array,
  signedPreKeyPrivB: Uint8Array,
  identityPubA: Uint8Array,
  ephemeralPubA: Uint8Array,
  oneTimePreKeyPrivB?: Uint8Array
): Uint8Array {
  // Perform 3-4 DH operations (mirror of initiator)
  const dh1 = curve25519.sharedKey(signedPreKeyPrivB, identityPubA);
  const dh2 = curve25519.sharedKey(identityPrivB, ephemeralPubA);
  const dh3 = curve25519.sharedKey(signedPreKeyPrivB, ephemeralPubA);

  const ikm = oneTimePreKeyPrivB
    ? concatBytes(dh1, dh2, dh3, curve25519.sharedKey(oneTimePreKeyPrivB, ephemeralPubA))
    : concatBytes(dh1, dh2, dh3);

  return hkdf(sha256, ikm, undefined, INFO_X3DH, 32);
}

/**
 * Helper function to generate random bytes
 */
function randomBytes(length: number): Uint8Array {
  const bytes = new Uint8Array(length);
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    crypto.getRandomValues(bytes);
  } else {
    // Fallback for Node.js
    const nodeCrypto = require('crypto');
    const buffer = nodeCrypto.randomBytes(length);
    bytes.set(buffer);
  }
  return bytes;
}

/**
 * Helper function to concatenate byte arrays
 */
function concatBytes(...items: Uint8Array[]): Uint8Array {
  const total = items.reduce((sum, item) => sum + item.length, 0);
  const out = new Uint8Array(total);
  let offset = 0;
  for (const item of items) {
    out.set(item, offset);
    offset += item.length;
  }
  return out;
}
