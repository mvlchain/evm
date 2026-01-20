/**
 * X3DH Implementation with Signal-compliant XEdDSA
 *
 * Uses proper XEdDSA implementation that follows Signal Protocol specification
 * for Montgomery ↔ Edwards curve conversion with sign bit handling.
 */

import { hkdf } from "@noble/hashes/hkdf";
import { sha256 } from "@noble/hashes/sha2";
import { x25519 } from "@noble/curves/ed25519";
import { xeddsaSign, xeddsaVerifyWithEdwardsPub, getEdwardsPublicKey } from "./xeddsa_simple";

const INFO_X3DH = new TextEncoder().encode("X3DH");

/**
 * Generate random bytes for nonce in XEdDSA signing
 */
function randomBytes(length: number): Uint8Array {
  const bytes = new Uint8Array(length);
  if (typeof crypto !== 'undefined' && crypto.getRandomValues) {
    crypto.getRandomValues(bytes);
  } else {
    const nodeCrypto = require('crypto');
    const buffer = nodeCrypto.randomBytes(length);
    bytes.set(buffer);
  }
  return bytes;
}

/**
 * Identity Key - Single X25519 key for both DH and signatures (Signal standard)
 * Uses XEdDSA for signing with X25519 keys
 */
export type IdentityKey = {
  privateKey: Uint8Array;
  publicKey: Uint8Array;
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
 * Generate Identity Key (single X25519 key for both DH and XEdDSA signing)
 * Also computes the corresponding Ed25519 public key for verification
 */
export function generateIdentityKey(): IdentityKey & { ed25519PublicKey: Uint8Array } {
  const privateKey = x25519.utils.randomPrivateKey();
  const publicKey = x25519.getPublicKey(privateKey);
  const ed25519PublicKey = getEdwardsPublicKey(privateKey);
  return { privateKey, publicKey, ed25519PublicKey };
}

/**
 * Generate Signed PreKey signed with XEdDSA (using X25519 identity key)
 */
export function generateSignedPreKey(identityPriv: Uint8Array): SignedPreKey {
  const spkPriv = x25519.utils.randomPrivateKey();
  const spkPub = x25519.getPublicKey(spkPriv);
  const random = randomBytes(64);  // 64 bytes of random data for XEdDSA nonce
  const { signature } = xeddsaSign(identityPriv, spkPub, random);
  return { keyPair: { privateKey: spkPriv, publicKey: spkPub }, signature };
}

export function generateOneTimePreKey(): OneTimePreKey {
  const priv = x25519.utils.randomPrivateKey();
  const pub = x25519.getPublicKey(priv);
  return { privateKey: priv, publicKey: pub };
}

/**
 * Verify Signed PreKey signature using XEdDSA
 */
export function verifySignedPreKey(bundle: X3DHBundle): boolean {
  // Use the Edwards public key from the bundle for verification
  return xeddsaVerifyWithEdwardsPub(bundle.identityEd25519Pub, bundle.signedPreKeyPub, bundle.signature);
}

/**
 * Derive shared secret as initiator (Alice) - Single identity key version
 */
export function deriveSharedSecretInitiator(
  identityPrivA: Uint8Array,
  ephemeralPrivA: Uint8Array,
  bundleB: X3DHBundle
): Uint8Array {
  const dh1 = x25519.getSharedSecret(identityPrivA, bundleB.signedPreKeyPub);
  const dh2 = x25519.getSharedSecret(ephemeralPrivA, bundleB.identityPub);
  const dh3 = x25519.getSharedSecret(ephemeralPrivA, bundleB.signedPreKeyPub);
  const ikm = bundleB.oneTimePreKeyPub
    ? concatBytes(dh1, dh2, dh3, x25519.getSharedSecret(ephemeralPrivA, bundleB.oneTimePreKeyPub))
    : concatBytes(dh1, dh2, dh3);
  return hkdf(sha256, ikm, undefined, INFO_X3DH, 32);
}

/**
 * Derive shared secret as responder (Bob) - Single identity key version
 */
export function deriveSharedSecretResponder(
  identityPrivB: Uint8Array,
  signedPreKeyPrivB: Uint8Array,
  identityPubA: Uint8Array,
  ephemeralPubA: Uint8Array,
  oneTimePreKeyPrivB?: Uint8Array
): Uint8Array {
  const dh1 = x25519.getSharedSecret(signedPreKeyPrivB, identityPubA);
  const dh2 = x25519.getSharedSecret(identityPrivB, ephemeralPubA);
  const dh3 = x25519.getSharedSecret(signedPreKeyPrivB, ephemeralPubA);
  const ikm = oneTimePreKeyPrivB
    ? concatBytes(dh1, dh2, dh3, x25519.getSharedSecret(oneTimePreKeyPrivB, ephemeralPubA))
    : concatBytes(dh1, dh2, dh3);
  return hkdf(sha256, ikm, undefined, INFO_X3DH, 32);
}

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
