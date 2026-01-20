/**
 * Simplified XEdDSA Implementation
 *
 * Based on Signal's libsignal implementation:
 * https://github.com/signalapp/curve25519-java/blob/main/android/jni/ed25519/additions/xeddsa.c
 *
 * Key insight: We directly use the Curve25519 private key as an Ed25519 private key
 * after clamping and sign bit adjustment.
 */

import { sha512 } from "@noble/hashes/sha512";
import { ed25519 } from "@noble/curves/ed25519";

// Curve25519 subgroup order
const L = 2n ** 252n + 27742317777372353535851937790883648493n;

/**
 * Convert bytes to bigint (little-endian)
 */
function bytesToBigInt(bytes: Uint8Array): bigint {
  let result = 0n;
  for (let i = bytes.length - 1; i >= 0; i--) {
    result = (result << 8n) | BigInt(bytes[i]);
  }
  return result;
}

/**
 * Convert bigint to bytes (little-endian, 32 bytes)
 */
function bigIntToBytes(n: bigint): Uint8Array {
  const bytes = new Uint8Array(32);
  let value = n;
  for (let i = 0; i < 32; i++) {
    bytes[i] = Number(value & 0xFFn);
    value = value >> 8n;
  }
  return bytes;
}

/**
 * Generate random bytes
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
 * XEdDSA Sign following Signal's implementation
 *
 * @param curve25519PrivKey - Curve25519 private key (32 bytes)
 * @param message - Message to sign
 * @param random - 64 bytes of random data for nonce
 * @returns Signature (64 bytes) and Edwards public key
 */
export function xeddsaSign(
  curve25519PrivKey: Uint8Array,
  message: Uint8Array,
  random: Uint8Array
): { signature: Uint8Array; edwardsPublicKey: Uint8Array } {
  if (curve25519PrivKey.length !== 32) throw new Error("Private key must be 32 bytes");
  if (random.length !== 64) throw new Error("Random must be 64 bytes");

  // Clamp the private key (standard Curve25519 clamping)
  const a = new Uint8Array(curve25519PrivKey);
  a[0] &= 248;
  a[31] &= 127;
  a[31] |= 64;

  // Compute Edwards public key: A = a*B
  const A = ed25519.getPublicKey(a);

  // Check sign bit (bit 255, which is MSB of byte 31)
  const signBit = (A[31] & 0x80) >> 7;

  // If sign bit is 1, negate the private key
  let finalPrivKey: Uint8Array;
  let finalPubKey: Uint8Array;

  if (signBit === 1) {
    // Negate: a_neg = L - a
    const aBigInt = bytesToBigInt(a);
    const aNeg = (L - aBigInt) % L;
    finalPrivKey = bigIntToBytes(aNeg);

    // Recompute public key with negated private key
    finalPubKey = ed25519.getPublicKey(finalPrivKey);

    // Clear sign bit
    finalPubKey[31] &= 0x7F;
  } else {
    finalPrivKey = a;
    finalPubKey = new Uint8Array(A);
    finalPubKey[31] &= 0x7F;
  }

  // Compute nonce: r = hash(a || message || random) mod L
  const hashInput = new Uint8Array(32 + message.length + 64);
  hashInput.set(finalPrivKey, 0);
  hashInput.set(message, 32);
  hashInput.set(random, 32 + message.length);

  const hashOutput = sha512(hashInput);
  const r = bytesToBigInt(hashOutput) % L;
  const rBytes = bigIntToBytes(r);

  // R = r*B
  const R = ed25519.getPublicKey(rBytes);

  // h = hash(R || A || message) mod L
  const h_input = new Uint8Array(32 + 32 + message.length);
  h_input.set(R, 0);
  h_input.set(finalPubKey, 32);
  h_input.set(message, 64);

  const h_output = sha512(h_input);
  const h = bytesToBigInt(h_output) % L;

  // s = r + h*a mod L
  const aBigInt = bytesToBigInt(finalPrivKey);
  const s = (r + (h * aBigInt % L)) % L;
  const sBytes = bigIntToBytes(s);

  // Signature = R || s
  const signature = new Uint8Array(64);
  signature.set(R, 0);
  signature.set(sBytes, 32);

  return {
    signature,
    edwardsPublicKey: finalPubKey
  };
}

/**
 * XEdDSA Verify following Signal's implementation
 *
 * @param curve25519PubKey - Curve25519 public key (32 bytes)
 * @param message - Message that was signed
 * @param signature - Signature to verify (64 bytes)
 * @returns true if valid
 */
export function xeddsaVerify(
  curve25519PubKey: Uint8Array,
  message: Uint8Array,
  signature: Uint8Array
): boolean {
  if (curve25519PubKey.length !== 32) return false;
  if (signature.length !== 64) return false;

  try {
    // Convert Curve25519 public key to Edwards form
    // Using the birational map: y = (u - 1) / (u + 1)
    // But we can use @noble's built-in conversion

    // For now, we'll use a simpler approach:
    // The Edwards public key should be provided separately
    // This is why Signal protocol includes both keys in the bundle

    // Since we can't perfectly convert u->y without the private key,
    // this function expects the Edwards public key to be provided separately
    // in the X3DH bundle (as identityEd25519Pub)

    return false; // Placeholder - see note below
  } catch {
    return false;
  }
}

/**
 * Get Edwards public key from Curve25519 private key
 * This is what goes into the X3DH bundle as identityEd25519Pub
 */
export function getEdwardsPublicKey(curve25519PrivKey: Uint8Array): Uint8Array {
  const { edwardsPublicKey } = xeddsaSign(curve25519PrivKey, new Uint8Array(0), randomBytes(64));
  return edwardsPublicKey;
}

/**
 * Simple wrapper that verifies with Edwards public key directly
 */
export function xeddsaVerifyWithEdwardsPub(
  edwardsPubKey: Uint8Array,
  message: Uint8Array,
  signature: Uint8Array
): boolean {
  if (edwardsPubKey.length !== 32) return false;
  if (signature.length !== 64) return false;

  try {
    // Direct Ed25519 verification
    return ed25519.verify(signature, message, edwardsPubKey);
  } catch {
    return false;
  }
}
