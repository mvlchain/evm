import { Wallet } from "ethers";
import { sha256, sha512 } from "@noble/hashes/sha2";
import { ed25519, x25519 } from "@noble/curves/ed25519";
import { adHash } from "./crypto";
import {
  generateKeyPair,
  initializeInitiator,
  initializeResponder,
  ratchetDecrypt,
  ratchetEncrypt
} from "./double_ratchet";
import {
  deriveSharedSecretInitiator,
  deriveSharedSecretResponder,
  generateOneTimePreKey,
  generateSignedPreKey,
  verifySignedPreKey
} from "./x3dh";

function equalBytes(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i += 1) {
    if (a[i] !== b[i]) return false;
  }
  return true;
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

function deriveIdentityKeyFromEthers(wallet: { privateKey: string }) {
  const seed = Buffer.from(wallet.privateKey.slice(2), "hex");
  // Use single X25519 key derived from wallet seed
  const keySeed = sha256(concatBytes(seed, new TextEncoder().encode("identity")));
  const privateKey = keySeed;
  const publicKey = x25519.getPublicKey(privateKey);

  // Derive Ed25519 public key for XEdDSA signature verification
  const hash = sha512(privateKey);
  const scalar = hash.slice(0, 32);
  scalar[0] &= 248;
  scalar[31] &= 127;
  scalar[31] |= 64;
  const ed25519PublicKey = ed25519.getPublicKey(scalar);

  return {
    privateKey,
    publicKey,
    ed25519PublicKey
  };
}

// --- X3DH + Double Ratchet 테스트 ---
const ad = adHash(
  1n, // sessionId
  "0x0000000000000000000000000000000000000001", // rider
  "0x0000000000000000000000000000000000000002", // driver
  1n // chainId
);

// Bob publishes a bundle (IK, SPK, signature)
const bobWallet = process.env.BOB_PRIVKEY ? new Wallet(process.env.BOB_PRIVKEY) : Wallet.createRandom();
const bob = deriveIdentityKeyFromEthers(bobWallet);
const bobSpk = generateSignedPreKey(bob.privateKey);
const bobOtpk = generateOneTimePreKey();

const bundle = {
  identityPub: bob.publicKey,
  identityEd25519Pub: bob.ed25519PublicKey,  // Ed25519 public key for XEdDSA verification
  signedPreKeyPub: bobSpk.keyPair.publicKey,
  signature: bobSpk.signature,
  oneTimePreKeyPub: bobOtpk.publicKey
};

if (!verifySignedPreKey(bundle)) {
  throw new Error("X3DH signed prekey verification failed");
}

// Alice creates her identity keys + ephemeral
const aliceWallet = process.env.ALICE_PRIVKEY ? new Wallet(process.env.ALICE_PRIVKEY) : Wallet.createRandom();
const alice = deriveIdentityKeyFromEthers(aliceWallet);
const aliceEphemeral = generateKeyPair();

// X3DH shared secret (initiator/responder must match)
const sharedInitiator = deriveSharedSecretInitiator(
  alice.privateKey,
  aliceEphemeral.privateKey,
  bundle
);
const sharedResponder = deriveSharedSecretResponder(
  bob.privateKey,
  bobSpk.keyPair.privateKey,
  alice.publicKey,
  aliceEphemeral.publicKey,
  bobOtpk.privateKey
);

console.log("sharedInitiator", Buffer.from(sharedInitiator).toString("hex"));
console.log("sharedResponder", Buffer.from(sharedResponder).toString("hex"));

if (!equalBytes(sharedInitiator, sharedResponder)) {
  throw new Error("X3DH shared secrets do not match");
}

// Initialize double ratchet from shared secret
const aliceState = initializeInitiator(sharedInitiator, bobSpk.keyPair.publicKey);
const bobState = initializeResponder(sharedResponder, bobSpk.keyPair, aliceState.dhPair.publicKey);

const encoder = new TextEncoder();
const decoder = new TextDecoder();

const msg1 = encoder.encode("message 1");
const msg2 = encoder.encode("message 2");

const packet1 = ratchetEncrypt(aliceState, msg1, ad);
const packet2 = ratchetEncrypt(aliceState, msg2, ad);
const packet3 = ratchetEncrypt(aliceState, encoder.encode("message 3"), ad);

const plaintext3 = ratchetDecrypt(bobState, packet3.header, packet3.ciphertext, ad);
console.log("Bob 수신 3:", decoder.decode(plaintext3));
const plaintext2 = ratchetDecrypt(bobState, packet2.header, packet2.ciphertext, ad);
console.log("Bob 수신 2:", decoder.decode(plaintext2));
const plaintext1 = ratchetDecrypt(bobState, packet1.header, packet1.ciphertext, ad);
console.log("Bob 수신 1:", decoder.decode(plaintext1));

const reply = encoder.encode("reply from bob");
const replyPacket = ratchetEncrypt(bobState, reply, ad);
const replyPlain = ratchetDecrypt(aliceState, replyPacket.header, replyPacket.ciphertext, ad);
console.log("Alice 수신:", decoder.decode(replyPlain));
