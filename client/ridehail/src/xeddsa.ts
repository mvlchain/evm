/**
 * XEdDSA Implementation following Signal Protocol specification
 * https://signal.org/docs/specifications/xeddsa/
 *
 * Implements proper Montgomery (X25519) to Edwards (Ed25519) curve conversion
 * with sign bit handling for signature compatibility.
 */

import { sha512 } from "@noble/hashes/sha512";
import { ed25519 } from "@noble/curves/ed25519";

// Curve25519 parameters
const P = 2n ** 255n - 19n;  // Field prime
const Q = 2n ** 252n + 27742317777372353535851937790883648493n;  // Subgroup order

/**
 * Modular inverse using Extended Euclidean Algorithm
 */
function modInverse(a: bigint, m: bigint): bigint {
  a = ((a % m) + m) % m;
  if (a === 0n) throw new Error("No modular inverse");

  let [oldR, r] = [a, m];
  let [oldS, s] = [1n, 0n];

  while (r !== 0n) {
    const quotient = oldR / r;
    [oldR, r] = [r, oldR - quotient * r];
    [oldS, s] = [s, oldS - quotient * s];
  }

  return ((oldS % m) + m) % m;
}

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
 * u_to_y: Convert Montgomery u-coordinate to Edwards y-coordinate
 * Formula: y = (u - 1) / (u + 1) mod p
 */
function uToY(u: bigint): bigint {
  const uMasked = u & ((1n << 255n) - 1n);  // Mask to 255 bits
  const numerator = (uMasked - 1n + P) % P;
  const denominator = (uMasked + 1n) % P;
  const y = (numerator * modInverse(denominator, P)) % P;
  return y;
}

/**
 * Get sign bit (s) from Edwards point
 * The sign bit is the LSB of the x-coordinate
 */
function getSignBit(edwardsPoint: Uint8Array): number {
  // For Ed25519, point encoding is: y-coordinate (255 bits) + sign bit of x (1 bit)
  // The sign bit is bit 255 (MSB of byte 31)
  return (edwardsPoint[31] >> 7) & 1;
}

/**
 * calculate_key_pair: Convert Montgomery private key to Edwards key pair
 * Ensures the Edwards public key always has sign bit = 0
 */
function calculateKeyPair(k: Uint8Array): {
  edwardsPublic: Uint8Array;
  edwardsPrivate: Uint8Array;
} {
  // Compute Edwards public key from Montgomery private key
  const kScalar = bytesToBigInt(k) & ((1n << 255n) - 1n);

  // Clamp the scalar (standard for X25519)
  const kBytes = new Uint8Array(k);
  kBytes[0] &= 248;
  kBytes[31] &= 127;
  kBytes[31] |= 64;

  // Compute E = k*B (Edwards basepoint multiplication)
  const edwardsPoint = ed25519.getPublicKey(kBytes);

  // Check sign bit
  const signBit = getSignBit(edwardsPoint);

  let edwardsPrivate: Uint8Array;
  let edwardsPublic: Uint8Array;

  if (signBit === 1) {
    // Negate the private key: a = -k mod q
    const kBigInt = bytesToBigInt(kBytes);
    const aNegated = (Q - kBigInt) % Q;
    edwardsPrivate = bigIntToBytes(aNegated);

    // Recompute public key with negated private key
    edwardsPublic = ed25519.getPublicKey(edwardsPrivate);
  } else {
    // Sign bit is already 0, use as-is
    edwardsPrivate = kBytes;
    edwardsPublic = edwardsPoint;
  }

  // Verify sign bit is now 0
  if (getSignBit(edwardsPublic) !== 0) {
    throw new Error("Failed to create Edwards key with sign bit = 0");
  }

  return { edwardsPublic, edwardsPrivate };
}

/**
 * convert_mont: Convert Montgomery public key to Edwards public key
 * Used during signature verification
 */
function convertMont(u: Uint8Array): Uint8Array {
  const uBigInt = bytesToBigInt(u);
  const y = uToY(uBigInt);

  // Encode as Edwards point: y-coordinate with sign bit = 0
  const edwardsPoint = bigIntToBytes(y);
  edwardsPoint[31] &= 0x7F;  // Clear sign bit (bit 255)

  return edwardsPoint;
}

/**
 * XEdDSA Sign: Sign a message with X25519 private key
 *
 * @param k - X25519 private key (32 bytes)
 * @param message - Message to sign
 * @param random - 64 bytes of random data (for nonce generation)
 * @returns Signature (64 bytes: R || s) and Edwards public key
 */
export function xeddsaSign(
  k: Uint8Array,
  message: Uint8Array,
  random: Uint8Array
): Uint8Array {
  if (k.length !== 32) throw new Error("Private key must be 32 bytes");
  if (random.length !== 64) throw new Error("Random data must be 64 bytes");

  // Step 1: Calculate Edwards key pair
  const { edwardsPublic, edwardsPrivate } = calculateKeyPair(k);

  // Step 2: Calculate nonce r = hash1(a || M || Z) mod q
  const hash1Input = new Uint8Array(32 + message.length + 64);
  hash1Input.set(edwardsPrivate, 0);
  hash1Input.set(message, 32);
  hash1Input.set(random, 32 + message.length);

  const hash1 = sha512(hash1Input);
  const rBigInt = bytesToBigInt(hash1) % Q;
  const rBytes = bigIntToBytes(rBigInt);

  // Step 3: R = r*B
  const R = ed25519.getPublicKey(rBytes);

  // Step 4: h = hash(R || A || M) mod q
  const hashInput = new Uint8Array(32 + 32 + message.length);
  hashInput.set(R, 0);
  hashInput.set(edwardsPublic, 32);
  hashInput.set(message, 64);

  const hashOutput = sha512(hashInput);
  const h = bytesToBigInt(hashOutput) % Q;

  // Step 5: s = r + h*a mod q
  const a = bytesToBigInt(edwardsPrivate);
  const s = (rBigInt + (h * a % Q)) % Q;
  const sBytes = bigIntToBytes(s);

  // Step 6: Return signature = R || s
  const signature = new Uint8Array(64);
  signature.set(R, 0);
  signature.set(sBytes, 32);

  return signature;
}

/**
 * XEdDSA Verify: Verify a signature with Edwards public key (not X25519)
 *
 * @param edwardsPub - Edwards public key (32 bytes) from getEdwardsPublicKey()
 * @param message - Message that was signed
 * @param signature - Signature to verify (64 bytes: R || s)
 * @returns true if signature is valid
 */
export function xeddsaVerify(
  edwardsPub: Uint8Array,
  message: Uint8Array,
  signature: Uint8Array
): boolean {
  if (edwardsPub.length !== 32) return false;
  if (signature.length !== 64) return false;

  try {
    // Parse signature
    const R = signature.slice(0, 32);
    const sBytes = signature.slice(32, 64);

    // Check bounds
    const sBigInt = bytesToBigInt(sBytes);
    if (sBigInt >= Q) return false;

    // The Edwards public key is already in the correct format
    const A = edwardsPub;

    // Verify using Ed25519
    // Ed25519.verify checks: R ?= s*B - h*A where h = hash(R || A || M)
    return ed25519.verify(signature, message, A);
  } catch {
    return false;
  }
}

/**
 * Get Edwards public key from X25519 private key
 * This is used to publish the Ed25519 public key alongside X25519 public key
 */
export function getEdwardsPublicKey(x25519PrivateKey: Uint8Array): Uint8Array {
  const { edwardsPublic } = calculateKeyPair(x25519PrivateKey);
  return edwardsPublic;
}
