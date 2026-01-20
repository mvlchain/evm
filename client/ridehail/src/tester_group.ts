import { Wallet } from "ethers";
import { sha256, sha512 } from "@noble/hashes/sha2";
import { hmac } from "@noble/hashes/hmac";
import { ed25519, x25519 } from "@noble/curves/ed25519";
import { chacha20poly1305 } from "@noble/ciphers/chacha";
import { keccak256 } from "ethers";
import {
  generateKeyPair,
  initializeInitiator,
  initializeResponder,
  ratchetDecrypt,
  ratchetEncrypt,
  RatchetState
} from "./double_ratchet";
import {
  generateSignedPreKey,
  generateOneTimePreKey,
  deriveSharedSecretInitiator,
  deriveSharedSecretResponder,
  verifySignedPreKey
} from "./x3dh";

// ============================================================================
// Utility Functions
// ============================================================================

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

function randomBytes(length: number): Uint8Array {
  const bytes = new Uint8Array(length);
  for (let i = 0; i < length; i++) {
    bytes[i] = Math.floor(Math.random() * 256);
  }
  return bytes;
}

function groupAdHash(groupId: string, generation: number, messageNumber: number): Uint8Array {
  const encoded = new TextEncoder().encode(
    `${groupId}|${generation}|${messageNumber}`
  );
  return Uint8Array.from(Buffer.from(keccak256(encoded).slice(2), "hex"));
}

function nonceFromCounter(counter: number): Uint8Array {
  const nonce = new Uint8Array(12);
  nonce[8] = (counter >>> 24) & 0xff;
  nonce[9] = (counter >>> 16) & 0xff;
  nonce[10] = (counter >>> 8) & 0xff;
  nonce[11] = counter & 0xff;
  return nonce;
}

function kdfChain(chainKey: Uint8Array): { messageKey: Uint8Array; chainKey: Uint8Array } {
  const INFO_CK = new TextEncoder().encode("DR_CK");
  const messageKey = hmac(sha256, chainKey, INFO_CK);
  const nextChainKey = hmac(sha256, chainKey, new Uint8Array([0x01]));
  return { messageKey, chainKey: nextChainKey };
}

function deriveIdentityKeyFromEthers(wallet: { privateKey: string }) {
  const seed = Buffer.from(wallet.privateKey.slice(2), "hex");
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

// ============================================================================
// Sender Key Types & Functions (Signal Protocol Style)
// ============================================================================

export type SenderKey = {
  chainKey: Uint8Array;
  signaturePrivateKey: Uint8Array;
  signaturePublicKey: Uint8Array;
  generation: number;
  messageNumber: number;
};

export type GroupMessage = {
  senderKeyId: string;  // "userId:generation"
  messageNumber: number;
  ciphertext: Uint8Array;
  signature: Uint8Array;
};

export type StoredSenderKey = {
  chainKey: Uint8Array;
  signaturePublicKey: Uint8Array;
  generation: number;
  messageNumber: number;
  skippedMessageKeys: Map<number, Uint8Array>;  // messageNumber → messageKey
};

// Generate a new Sender Key
function generateSenderKey(): SenderKey {
  const signaturePrivateKey = randomBytes(32);
  const signaturePublicKey = ed25519.getPublicKey(signaturePrivateKey);

  return {
    chainKey: randomBytes(32),
    signaturePrivateKey,
    signaturePublicKey,
    generation: 1,
    messageNumber: 0
  };
}

// Encrypt group message with Sender Key
function encryptGroupMessage(
  senderKey: SenderKey,
  plaintext: Uint8Array,
  groupId: string,
  userId: string
): GroupMessage {
  // Derive message key from chain key
  const { messageKey, chainKey } = kdfChain(senderKey.chainKey);
  senderKey.chainKey = chainKey;  // Ratchet forward

  const messageNumber = senderKey.messageNumber;
  const nonce = nonceFromCounter(messageNumber);
  const ad = groupAdHash(groupId, senderKey.generation, messageNumber);

  // Encrypt with ChaCha20-Poly1305
  const aead = chacha20poly1305(messageKey, nonce, ad);
  const ciphertext = aead.encrypt(plaintext);

  // Sign ciphertext to prevent tampering
  const signature = ed25519.sign(ciphertext, senderKey.signaturePrivateKey);

  senderKey.messageNumber++;

  return {
    senderKeyId: `${userId}:${senderKey.generation}`,
    messageNumber,
    ciphertext,
    signature
  };
}

// Decrypt group message with stored Sender Key
function decryptGroupMessage(
  storedSenderKey: StoredSenderKey,
  message: GroupMessage,
  groupId: string
): Uint8Array {
  // Verify signature first
  const valid = ed25519.verify(message.signature, message.ciphertext, storedSenderKey.signaturePublicKey);
  if (!valid) {
    throw new Error("Invalid signature on group message");
  }

  // Check if we already have this message key (skipped message)
  const skippedKey = storedSenderKey.skippedMessageKeys.get(message.messageNumber);
  if (skippedKey) {
    storedSenderKey.skippedMessageKeys.delete(message.messageNumber);
    const nonce = nonceFromCounter(message.messageNumber);
    const ad = groupAdHash(groupId, storedSenderKey.generation, message.messageNumber);
    const aead = chacha20poly1305(skippedKey, nonce, ad);
    return aead.decrypt(message.ciphertext);
  }

  // Forward chain key if we missed messages, storing skipped keys
  while (storedSenderKey.messageNumber < message.messageNumber) {
    const { messageKey, chainKey } = kdfChain(storedSenderKey.chainKey);
    storedSenderKey.skippedMessageKeys.set(storedSenderKey.messageNumber, messageKey);
    storedSenderKey.chainKey = chainKey;
    storedSenderKey.messageNumber++;
  }

  // Derive message key for current message
  const { messageKey, chainKey } = kdfChain(storedSenderKey.chainKey);
  storedSenderKey.chainKey = chainKey;
  storedSenderKey.messageNumber++;

  const nonce = nonceFromCounter(message.messageNumber);
  const ad = groupAdHash(groupId, storedSenderKey.generation, message.messageNumber);

  // Decrypt with ChaCha20-Poly1305
  const aead = chacha20poly1305(messageKey, nonce, ad);
  return aead.decrypt(message.ciphertext);
}

// Encode Sender Key for distribution via 1:1 session
function encodeSenderKey(senderKey: SenderKey): Uint8Array {
  // Format: generation(4) + chainKey(32) + signaturePubKey(32)
  const encoded = new Uint8Array(4 + 32 + 32);
  const view = new DataView(encoded.buffer);
  view.setUint32(0, senderKey.generation, false);  // big-endian
  encoded.set(senderKey.chainKey, 4);
  encoded.set(senderKey.signaturePublicKey, 36);
  return encoded;
}

// Decode Sender Key from distribution message
function decodeSenderKey(encoded: Uint8Array): StoredSenderKey {
  if (encoded.length !== 68) {
    throw new Error("Invalid sender key encoding");
  }

  const view = new DataView(encoded.buffer, encoded.byteOffset);
  const generation = view.getUint32(0, false);  // big-endian
  const chainKey = encoded.slice(4, 36);
  const signaturePublicKey = encoded.slice(36, 68);

  return {
    chainKey: new Uint8Array(chainKey),  // Create new copy
    signaturePublicKey,
    generation,
    messageNumber: 0,
    skippedMessageKeys: new Map()
  };
}

// ============================================================================
// Test: Group Messaging with Sender Keys
// ============================================================================

console.log("\n=== Group Messaging Test (1:N with Sender Keys) ===\n");

const groupId = "ride-group-123";

// Step 1: Create 4 group members (Alice, Bob, Charlie, Dave)
console.log("Step 1: Creating group members...");
const aliceWallet = Wallet.createRandom();
const bobWallet = Wallet.createRandom();
const charlieWallet = Wallet.createRandom();
const daveWallet = Wallet.createRandom();

const alice = deriveIdentityKeyFromEthers(aliceWallet);
const bob = deriveIdentityKeyFromEthers(bobWallet);
const charlie = deriveIdentityKeyFromEthers(charlieWallet);
const dave = deriveIdentityKeyFromEthers(daveWallet);

console.log("✓ Alice, Bob, Charlie, Dave created");

// Step 2: Establish pairwise 1:1 sessions (X3DH + Double Ratchet)
console.log("\nStep 2: Establishing 1:1 sessions between all members...");

// Helper to establish session between two users
function establishSession(
  initiatorName: string,
  initiator: ReturnType<typeof deriveIdentityKeyFromEthers>,
  responderName: string,
  responder: ReturnType<typeof deriveIdentityKeyFromEthers>
): { initiatorState: RatchetState; responderState: RatchetState } {
  // Responder publishes bundle
  const responderSpk = generateSignedPreKey(responder.privateKey);
  const responderOtpk = generateOneTimePreKey();

  const bundle = {
    identityPub: responder.publicKey,
    identityEd25519Pub: responder.ed25519PublicKey,
    signedPreKeyPub: responderSpk.keyPair.publicKey,
    signature: responderSpk.signature,
    oneTimePreKeyPub: responderOtpk.publicKey
  };

  if (!verifySignedPreKey(bundle)) {
    throw new Error("Signed prekey verification failed");
  }

  // Initiator creates ephemeral key
  const initiatorEphemeral = generateKeyPair();

  // X3DH key exchange
  const sharedInitiator = deriveSharedSecretInitiator(
    initiator.privateKey,
    initiatorEphemeral.privateKey,
    bundle
  );

  const sharedResponder = deriveSharedSecretResponder(
    responder.privateKey,
    responderSpk.keyPair.privateKey,
    initiator.publicKey,
    initiatorEphemeral.publicKey,
    responderOtpk.privateKey
  );

  if (!equalBytes(sharedInitiator, sharedResponder)) {
    throw new Error("Shared secrets don't match");
  }

  // Initialize Double Ratchet
  const initiatorState = initializeInitiator(sharedInitiator, responderSpk.keyPair.publicKey);
  const responderState = initializeResponder(sharedResponder, responderSpk.keyPair, initiatorState.dhPair.publicKey);

  console.log(`  ✓ ${initiatorName} ←→ ${responderName}`);

  return { initiatorState, responderState };
}

// Alice establishes sessions with all others (she will be the sender)
const aliceBobSession = establishSession("Alice", alice, "Bob", bob);
const aliceCharlieSession = establishSession("Alice", alice, "Charlie", charlie);
const aliceDaveSession = establishSession("Alice", alice, "Dave", dave);

// Store sessions for Alice
const aliceSessions = new Map<string, RatchetState>([
  ["Bob", aliceBobSession.initiatorState],
  ["Charlie", aliceCharlieSession.initiatorState],
  ["Dave", aliceDaveSession.initiatorState]
]);

// Store sessions for recipients
const bobSessions = new Map<string, RatchetState>([
  ["Alice", aliceBobSession.responderState]
]);

const charlieSessions = new Map<string, RatchetState>([
  ["Alice", aliceCharlieSession.responderState]
]);

const daveSessions = new Map<string, RatchetState>([
  ["Alice", aliceDaveSession.responderState]
]);

// Step 3: Alice generates her Sender Key
console.log("\nStep 3: Alice generates Sender Key...");
const aliceSenderKey = generateSenderKey();
console.log(`✓ Alice Sender Key (generation ${aliceSenderKey.generation}) created`);

// Step 4: Alice distributes her Sender Key to all members via 1:1 sessions
console.log("\nStep 4: Alice distributes Sender Key to all members...");

const senderKeyMessage = encodeSenderKey(aliceSenderKey);
const encoder = new TextEncoder();
const decoder = new TextDecoder();

// Distribute to Bob
const bobAd = new Uint8Array(32);  // Simplified AD for this test
const toBobPacket = ratchetEncrypt(aliceSessions.get("Bob")!, senderKeyMessage, bobAd);
const bobReceivedKey = ratchetDecrypt(bobSessions.get("Alice")!, toBobPacket.header, toBobPacket.ciphertext, bobAd);
const bobStoredAliceSenderKey = decodeSenderKey(bobReceivedKey);
console.log("  ✓ Bob received Alice's Sender Key");

// Distribute to Charlie
const charlieAd = new Uint8Array(32);
const toCharliePacket = ratchetEncrypt(aliceSessions.get("Charlie")!, senderKeyMessage, charlieAd);
const charlieReceivedKey = ratchetDecrypt(charlieSessions.get("Alice")!, toCharliePacket.header, toCharliePacket.ciphertext, charlieAd);
const charlieStoredAliceSenderKey = decodeSenderKey(charlieReceivedKey);
console.log("  ✓ Charlie received Alice's Sender Key");

// Distribute to Dave
const daveAd = new Uint8Array(32);
const toDavePacket = ratchetEncrypt(aliceSessions.get("Dave")!, senderKeyMessage, daveAd);
const daveReceivedKey = ratchetDecrypt(daveSessions.get("Alice")!, toDavePacket.header, toDavePacket.ciphertext, daveAd);
const daveStoredAliceSenderKey = decodeSenderKey(daveReceivedKey);
console.log("  ✓ Dave received Alice's Sender Key");

// Step 5: Alice sends group message (encrypted ONCE, broadcasted to all)
console.log("\nStep 5: Alice sends group message (1x encryption, broadcast to all)...");

const groupPlaintext = encoder.encode("🚗 Ride arriving in 5 minutes!");
const groupMessage1 = encryptGroupMessage(aliceSenderKey, groupPlaintext, groupId, "Alice");
console.log(`✓ Alice encrypted: "${decoder.decode(groupPlaintext)}"`);
console.log(`  Message size: ${groupMessage1.ciphertext.length} bytes`);
console.log(`  (Same ciphertext broadcasted to Bob, Charlie, Dave)`);

// Step 6: All recipients decrypt the same group message
console.log("\nStep 6: Recipients decrypt group message...");

const bobDecrypted = decryptGroupMessage(bobStoredAliceSenderKey, groupMessage1, groupId);
console.log(`  ✓ Bob decrypted: "${decoder.decode(bobDecrypted)}"`);

const charlieDecrypted = decryptGroupMessage(charlieStoredAliceSenderKey, groupMessage1, groupId);
console.log(`  ✓ Charlie decrypted: "${decoder.decode(charlieDecrypted)}"`);

const daveDecrypted = decryptGroupMessage(daveStoredAliceSenderKey, groupMessage1, groupId);
console.log(`  ✓ Dave decrypted: "${decoder.decode(daveDecrypted)}"`);

// Verify all decrypted the same message
if (!equalBytes(bobDecrypted, groupPlaintext) ||
    !equalBytes(charlieDecrypted, groupPlaintext) ||
    !equalBytes(daveDecrypted, groupPlaintext)) {
  throw new Error("Group message decryption failed!");
}

// Step 7: Alice sends another group message (Forward Secrecy via chain key ratcheting)
console.log("\nStep 7: Alice sends second group message (Forward Secrecy)...");

const groupPlaintext2 = encoder.encode("📍 Pickup location updated");
const groupMessage2 = encryptGroupMessage(aliceSenderKey, groupPlaintext2, groupId, "Alice");
console.log(`✓ Alice encrypted: "${decoder.decode(groupPlaintext2)}"`);

const bobDecrypted2 = decryptGroupMessage(bobStoredAliceSenderKey, groupMessage2, groupId);
console.log(`  ✓ Bob decrypted: "${decoder.decode(bobDecrypted2)}"`);

const charlieDecrypted2 = decryptGroupMessage(charlieStoredAliceSenderKey, groupMessage2, groupId);
console.log(`  ✓ Charlie decrypted: "${decoder.decode(charlieDecrypted2)}"`);

const daveDecrypted2 = decryptGroupMessage(daveStoredAliceSenderKey, groupMessage2, groupId);
console.log(`  ✓ Dave decrypted: "${decoder.decode(daveDecrypted2)}"`);

// Step 8: Test out-of-order message delivery
console.log("\nStep 8: Testing out-of-order message delivery...");

const msg3 = encryptGroupMessage(aliceSenderKey, encoder.encode("Message 3"), groupId, "Alice");
const msg4 = encryptGroupMessage(aliceSenderKey, encoder.encode("Message 4"), groupId, "Alice");
const msg5 = encryptGroupMessage(aliceSenderKey, encoder.encode("Message 5"), groupId, "Alice");

// Bob receives out of order: 5, 3, 4
const bobMsg5 = decryptGroupMessage(bobStoredAliceSenderKey, msg5, groupId);
console.log(`  ✓ Bob received (5): "${decoder.decode(bobMsg5)}"`);

const bobMsg3 = decryptGroupMessage(bobStoredAliceSenderKey, msg3, groupId);
console.log(`  ✓ Bob received (3): "${decoder.decode(bobMsg3)}"`);

const bobMsg4 = decryptGroupMessage(bobStoredAliceSenderKey, msg4, groupId);
console.log(`  ✓ Bob received (4): "${decoder.decode(bobMsg4)}"`);

console.log("\n✅ All group messaging tests passed!");
console.log("\nKey advantages:");
console.log("  • 1x encryption → N recipients (efficient!)");
console.log("  • Forward Secrecy (chain key ratcheting)");
console.log("  • Message authentication (Ed25519 signatures)");
console.log("  • Out-of-order delivery support");
console.log("  • Built on existing 1:1 infrastructure");
