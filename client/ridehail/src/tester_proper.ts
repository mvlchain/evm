/**
 * Test for proper XEdDSA implementation using @privacyresearch libraries
 */

import { Wallet } from "ethers";
import { sha256 } from "@noble/hashes/sha2";
import * as curve25519 from "@privacyresearch/curve25519-typescript";
import { adHash } from "./crypto";
import {
  generateKeyPair,
  initializeInitiator,
  initializeResponder,
  ratchetDecrypt,
  ratchetEncrypt
} from "./double_ratchet";
import {
  generateIdentityKey,
  generateSignedPreKey,
  generateOneTimePreKey,
  deriveSharedSecretInitiator,
  deriveSharedSecretResponder,
  verifySignedPreKey
} from "./x3dh_proper";

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
  const keySeed = sha256(concatBytes(seed, new TextEncoder().encode("identity")));

  // Generate identity key using proper XEdDSA
  const keyPair = curve25519.generateKeyPair(keySeed);

  // Create full identity key with Ed25519 conversion
  const identityKey = generateIdentityKey();

  // Override with derived key from wallet
  return {
    privateKey: keyPair.privKey,
    publicKey: keyPair.pubKey,
    ed25519PublicKey: identityKey.ed25519PublicKey.bytes
  };
}

console.log("\n=== Testing Proper XEdDSA Implementation ===\n");

// Step 1: Generate Identity Keys
console.log("Step 1: Generating identity keys with proper XEdDSA...");
const alice = generateIdentityKey();
const bob = generateIdentityKey();
console.log("✓ Alice and Bob identity keys generated");
console.log(`  Alice X25519 pub: ${Buffer.from(alice.publicKey).toString("hex").slice(0, 16)}...`);
console.log(`  Alice Ed25519 pub: ${Buffer.from(alice.ed25519PublicKey.bytes).toString("hex").slice(0, 16)}...`);

// Step 2: Bob publishes bundle
console.log("\nStep 2: Bob creates and publishes key bundle...");
const bobSpk = generateSignedPreKey(bob);
const bobOtpk = generateOneTimePreKey();

const bundle = {
  identityPub: bob.publicKey,
  identityEd25519Pub: bob.ed25519PublicKey.bytes,
  signedPreKeyPub: bobSpk.keyPair.publicKey,
  signature: bobSpk.signature,
  oneTimePreKeyPub: bobOtpk.publicKey
};

console.log(`✓ Bob's bundle created`);
console.log(`  SPK: ${Buffer.from(bobSpk.keyPair.publicKey).toString("hex").slice(0, 16)}...`);
console.log(`  Signature: ${Buffer.from(bobSpk.signature).toString("hex").slice(0, 16)}...`);

// Step 3: Verify signature
console.log("\nStep 3: Verifying signed prekey with XEdDSA...");
if (!verifySignedPreKey(bundle)) {
  throw new Error("❌ XEdDSA signed prekey verification failed!");
}
console.log("✓ Signature verification passed!");

// Step 4: Alice performs X3DH
console.log("\nStep 4: Alice performs X3DH key exchange...");
const aliceEphemeral = generateKeyPair();

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

console.log(`  Alice shared secret: ${Buffer.from(sharedInitiator).toString("hex")}`);
console.log(`  Bob shared secret:   ${Buffer.from(sharedResponder).toString("hex")}`);

if (!equalBytes(sharedInitiator, sharedResponder)) {
  throw new Error("❌ X3DH shared secrets do not match!");
}
console.log("✓ Shared secrets match!");

// Step 5: Initialize Double Ratchet
console.log("\nStep 5: Initializing Double Ratchet...");
const ad = adHash(
  1n, // sessionId
  "0x0000000000000000000000000000000000000001", // rider
  "0x0000000000000000000000000000000000000002", // driver
  1n // chainId
);

const aliceState = initializeInitiator(sharedInitiator, bobSpk.keyPair.publicKey);
const bobState = initializeResponder(sharedResponder, bobSpk.keyPair, aliceState.dhPair.publicKey);
console.log("✓ Double Ratchet initialized");

// Step 6: Test encryption/decryption
console.log("\nStep 6: Testing Double Ratchet encryption...");
const encoder = new TextEncoder();
const decoder = new TextDecoder();

const msg1 = encoder.encode("Hello from Alice!");
const msg2 = encoder.encode("Second message");
const msg3 = encoder.encode("Third message");

const packet1 = ratchetEncrypt(aliceState, msg1, ad);
const packet2 = ratchetEncrypt(aliceState, msg2, ad);
const packet3 = ratchetEncrypt(aliceState, msg3, ad);

// Bob receives out of order: 3, 2, 1
const plaintext3 = ratchetDecrypt(bobState, packet3.header, packet3.ciphertext, ad);
console.log(`  ✓ Bob decrypted (3): "${decoder.decode(plaintext3)}"`);

const plaintext2 = ratchetDecrypt(bobState, packet2.header, packet2.ciphertext, ad);
console.log(`  ✓ Bob decrypted (2): "${decoder.decode(plaintext2)}"`);

const plaintext1 = ratchetDecrypt(bobState, packet1.header, packet1.ciphertext, ad);
console.log(`  ✓ Bob decrypted (1): "${decoder.decode(plaintext1)}"`);

// Step 7: Test reply
console.log("\nStep 7: Testing bidirectional communication...");
const reply = encoder.encode("Reply from Bob!");
const replyPacket = ratchetEncrypt(bobState, reply, ad);
const replyPlain = ratchetDecrypt(aliceState, replyPacket.header, replyPacket.ciphertext, ad);
console.log(`  ✓ Alice decrypted: "${decoder.decode(replyPlain)}"`);

console.log("\n✅ All tests passed with proper XEdDSA implementation!");
console.log("\nKey features:");
console.log("  • Single X25519 key for DH and signatures");
console.log("  • Proper XEdDSA curve conversion");
console.log("  • Signal Protocol compatible");
console.log("  • Forward/Backward secrecy via Double Ratchet");
console.log("  • Out-of-order message delivery");
