import { hkdf } from "@noble/hashes/hkdf";
import { sha256 } from "@noble/hashes/sha2";
import { sha512 } from "@noble/hashes/sha512";
import { ed25519, x25519 } from "@noble/curves/ed25519";

const INFO_X3DH = new TextEncoder().encode("X3DH");

/**
 * Convert X25519 private key to Ed25519 private key deterministically
 * Uses SHA-512 hash and proper scalar clamping
 */
function x25519ToEd25519Private(x25519PrivKey: Uint8Array): Uint8Array {
  const hash = sha512(x25519PrivKey);
  const scalar = hash.slice(0, 32);

  // Clamp for Ed25519
  scalar[0] &= 248;
  scalar[31] &= 127;
  scalar[31] |= 64;

  return scalar;
}

/**
 * Convert X25519 public key to Ed25519 public key
 * Derives from the converted private key for consistency
 */
function x25519ToEd25519Public(x25519PrivKey: Uint8Array): Uint8Array {
  const ed25519PrivKey = x25519ToEd25519Private(x25519PrivKey);
  return ed25519.getPublicKey(ed25519PrivKey);
}

/**
 * XEdDSA-style signing: Sign with X25519 private key
 * Converts X25519 → Ed25519 deterministically, then signs
 */
function xeddsaSign(message: Uint8Array, x25519PrivKey: Uint8Array): Uint8Array {
  const ed25519PrivKey = x25519ToEd25519Private(x25519PrivKey);
  return ed25519.sign(message, ed25519PrivKey);
}

/**
 * XEdDSA-style verification: Verify with Ed25519 public key
 * The Ed25519 public key is derived from the signer's X25519 private key
 */
function xeddsaVerify(
  signature: Uint8Array,
  message: Uint8Array,
  ed25519PubKey: Uint8Array  // Ed25519 public key (derived from X25519 private key)
): boolean {
  try {
    return ed25519.verify(signature, message, ed25519PubKey);
  } catch {
    return false;
  }
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
  const ed25519PublicKey = x25519ToEd25519Public(privateKey);
  return { privateKey, publicKey, ed25519PublicKey };
}

/**
 * Generate Signed PreKey signed with XEdDSA (using X25519 identity key)
 */
export function generateSignedPreKey(identityPriv: Uint8Array): SignedPreKey {
  const spkPriv = x25519.utils.randomPrivateKey();
  const spkPub = x25519.getPublicKey(spkPriv);
  const signature = xeddsaSign(spkPub, identityPriv);
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
  return xeddsaVerify(bundle.signature, bundle.signedPreKeyPub, bundle.identityEd25519Pub);
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
