/**
 * matchboard.e2e.ts — simple off-chain matchboard API end-to-end test
 *
 * Tests the matchboard HTTP server only (no on-chain/blockchain required).
 * Matchboard runs in-process inside evmd — start the node first:
 *
 *   ./local_node.sh          # matchboard.enable defaults to true
 *
 * Then run this test:
 *   tsx src/matchboard.e2e.ts
 *   MATCHBOARD_URL=http://127.0.0.1:8080 tsx src/matchboard.e2e.ts
 */

import assert from "node:assert/strict";
import { createHash, randomBytes } from "node:crypto";
import process from "node:process";
import { Contract, JsonRpcProvider, SigningKey, Wallet, getAddress } from "ethers";

// ---------------------------------------------------------------------------
// Config
// ---------------------------------------------------------------------------

const BASE = (process.env.MATCHBOARD_URL ?? "http://127.0.0.1:8080").replace(/\/$/, "");
const EVM_RPC_URL = (process.env.EVM_RPC_URL ?? "http://127.0.0.1:8545").replace(/\/$/, "");
const MATCH_PRECOMPILE_ADDRESS = process.env.MATCH_PRECOMPILE_ADDRESS ?? "0x0000000000000000000000000000000000000808";
const CHAIN_ID = process.env.MATCH_CHAIN_ID ?? "9001";
const VERBOSE = (process.env.VERBOSE ?? "1") !== "0";

const MATCH_PRECOMPILE_ABI = [
  "function hasReplay(string poolId, string intentId) view returns (bool exists)",
  "function getReplay(string poolId, string intentId) view returns (bool found, string matchId)",
  "function getReplayParties(string poolId, string intentId) view returns (bool found, string matchId, string requester, string responder)",
  "function submitMatchCertificate(bytes certificate) returns (string matchId, string replayKey, bytes certificateHash)",
];

// ---------------------------------------------------------------------------
// Protobuf helpers (minimal hand-rolled encoding for match certificate)
// ---------------------------------------------------------------------------

const WIRE_TYPE_VARINT = 0;
const WIRE_TYPE_LEN = 2;
const DIGEST_ALGO_SHA256 = 1;
const SIGNATURE_ALGO_SECP256K1 = 1;
const TYPE_URL_INTENT      = "cosmos-evm/match/v1/INTENT";
const TYPE_URL_RESPONSE    = "cosmos-evm/match/v1/RESPONSE";
const TYPE_URL_FINALIZE    = "cosmos-evm/match/v1/FINALIZE";
const TYPE_URL_CERTIFICATE = "cosmos-evm/match/v1/CERTIFICATE";
const _textEncoder = new TextEncoder();

function concatBytes(...chunks: Uint8Array[]): Uint8Array {
  const total = chunks.reduce((n, c) => n + c.length, 0);
  const out = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) { out.set(chunk, offset); offset += chunk.length; }
  return out;
}

function encodeVarint(value: number | bigint): Uint8Array {
  let v = typeof value === "bigint" ? value : BigInt(value);
  const bytes: number[] = [];
  while (v >= 0x80n) { bytes.push(Number((v & 0x7fn) | 0x80n)); v >>= 7n; }
  bytes.push(Number(v));
  return Uint8Array.from(bytes);
}

function encodeTag(fieldNumber: number, wireType: number): Uint8Array {
  return encodeVarint((fieldNumber << 3) | wireType);
}

function encodeFieldVarint(fieldNumber: number, value: number | bigint): Uint8Array {
  if (value === 0 || value === 0n) return new Uint8Array();
  return concatBytes(encodeTag(fieldNumber, WIRE_TYPE_VARINT), encodeVarint(value));
}

function encodeFieldBytes(fieldNumber: number, value?: Uint8Array): Uint8Array {
  if (!value || value.length === 0) return new Uint8Array();
  return concatBytes(encodeTag(fieldNumber, WIRE_TYPE_LEN), encodeVarint(value.length), value);
}

function encodeFieldString(fieldNumber: number, value?: string): Uint8Array {
  if (!value || value.length === 0) return new Uint8Array();
  return encodeFieldBytes(fieldNumber, _textEncoder.encode(value));
}

function encodeFieldMessage(fieldNumber: number, value: Uint8Array): Uint8Array {
  if (value.length === 0) return new Uint8Array();
  return concatBytes(encodeTag(fieldNumber, WIRE_TYPE_LEN), encodeVarint(value.length), value);
}

function sha256Bytes(data: Uint8Array): Uint8Array {
  return Uint8Array.from(createHash("sha256").update(data).digest());
}

/** hash = sha256( sha256(typeURL) || sha256(proto_payload) ) — matches signbytes.go TypedSignDocHash */
function typedSignDocHash(typeURL: string, payloadEncoded: Uint8Array): Uint8Array {
  const typeURLHash = sha256Bytes(_textEncoder.encode(typeURL));
  const payloadHash = sha256Bytes(payloadEncoded);
  return sha256Bytes(concatBytes(typeURLHash, payloadHash));
}

function hexToBytes(hex: string): Uint8Array {
  const n = hex.startsWith("0x") ? hex.slice(2) : hex;
  return Uint8Array.from(Buffer.from(n, "hex"));
}

type PbSig = { signer: string; algorithm: number; signature: Uint8Array };

type IntentPb = { chainId: string; poolId: string; intentId: string; initiator: string; issuedUnix: bigint; expiresUnix: bigint; contextHash: Uint8Array; recipient: string; replayGuard: Uint8Array };
type ResponsePb = { chainId: string; poolId: string; intentId: string; intentSignHash: Uint8Array; responseId: string; responder: string; issuedUnix: bigint; expiresUnix: bigint; contextHash: Uint8Array; recipient: string; replayGuard: Uint8Array; responseType: number };
type FinalizePb = { chainId: string; poolId: string; intentId: string; responseId: string; intentSignHash: Uint8Array; responseSignHash: Uint8Array; finalizeId: string; initiator: string; responder: string; issuedUnix: bigint; expiresUnix: bigint; contextHash: Uint8Array; replayGuard: Uint8Array };
type CertPb = { chainId: string; poolId: string; intentId: string; responseId: string; finalizeId: string; certificateId: string; intentSignHash: Uint8Array; responseSignHash: Uint8Array; finalizeSignHash: Uint8Array; initiator: string; responder: string; issuedUnix: bigint; expiresUnix: bigint; contextHash: Uint8Array; replayGuard: Uint8Array };

function encodeIntentPb(p: IntentPb): Uint8Array {
  return concatBytes(encodeFieldString(3, p.chainId), encodeFieldString(4, p.poolId), encodeFieldString(5, p.intentId), encodeFieldString(6, p.initiator), encodeFieldVarint(8, p.issuedUnix), encodeFieldVarint(9, p.expiresUnix), encodeFieldBytes(10, p.contextHash), encodeFieldString(13, p.recipient), encodeFieldBytes(14, p.replayGuard), encodeFieldVarint(15, DIGEST_ALGO_SHA256));
}
function encodeResponsePb(p: ResponsePb): Uint8Array {
  return concatBytes(encodeFieldString(3, p.chainId), encodeFieldString(4, p.poolId), encodeFieldString(5, p.intentId), encodeFieldBytes(6, p.intentSignHash), encodeFieldString(7, p.responseId), encodeFieldString(8, p.responder), encodeFieldVarint(10, p.issuedUnix), encodeFieldVarint(11, p.expiresUnix), encodeFieldBytes(12, p.contextHash), encodeFieldString(15, p.recipient), encodeFieldBytes(16, p.replayGuard), encodeFieldVarint(17, DIGEST_ALGO_SHA256), encodeFieldVarint(18, p.responseType));
}
function encodeFinalizePb(p: FinalizePb): Uint8Array {
  return concatBytes(encodeFieldString(3, p.chainId), encodeFieldString(4, p.poolId), encodeFieldString(5, p.intentId), encodeFieldString(6, p.responseId), encodeFieldBytes(7, p.intentSignHash), encodeFieldBytes(8, p.responseSignHash), encodeFieldString(9, p.finalizeId), encodeFieldString(10, p.initiator), encodeFieldString(11, p.responder), encodeFieldVarint(13, p.issuedUnix), encodeFieldVarint(14, p.expiresUnix), encodeFieldBytes(15, p.contextHash), encodeFieldBytes(17, p.replayGuard), encodeFieldVarint(18, DIGEST_ALGO_SHA256));
}
function encodeCertPb(p: CertPb): Uint8Array {
  return concatBytes(encodeFieldString(3, p.chainId), encodeFieldString(4, p.poolId), encodeFieldString(5, p.intentId), encodeFieldString(6, p.responseId), encodeFieldString(7, p.finalizeId), encodeFieldString(8, p.certificateId), encodeFieldBytes(9, p.intentSignHash), encodeFieldBytes(10, p.responseSignHash), encodeFieldBytes(11, p.finalizeSignHash), encodeFieldString(12, p.initiator), encodeFieldString(13, p.responder), encodeFieldVarint(14, p.issuedUnix), encodeFieldVarint(15, p.expiresUnix), encodeFieldBytes(16, p.contextHash), encodeFieldBytes(17, p.replayGuard), encodeFieldVarint(18, DIGEST_ALGO_SHA256));
}
function encodePbSig(s: PbSig): Uint8Array {
  return concatBytes(encodeFieldString(1, s.signer), encodeFieldVarint(2, s.algorithm), encodeFieldBytes(4, s.signature));
}

function buildMatchCertificateBytes(args: {
  chainId: string; poolId: string; intentId: string; responseId: string; finalizeId: string; certificateId: string;
  initiator: string; responder: string; issuedUnix: bigint; expiresUnix: bigint;
  contextHash: Uint8Array;
  alicePrivateKey: string; bobPrivateKey: string;
}): { certificateBytes: Uint8Array; intentSignHash: string; responseSignHash: string; finalizeSignHash: string } {
  const replayGuard = sha256Bytes(_textEncoder.encode(`${args.poolId}|${args.intentId}|${args.responseId}|${args.finalizeId}`));

  const intentPb: IntentPb = { chainId: args.chainId, poolId: args.poolId, intentId: args.intentId, initiator: args.initiator, issuedUnix: args.issuedUnix, expiresUnix: args.expiresUnix, contextHash: args.contextHash, recipient: args.responder, replayGuard };
  const intentHash = typedSignDocHash(TYPE_URL_INTENT, encodeIntentPb(intentPb));

  const responsePb: ResponsePb = { chainId: args.chainId, poolId: args.poolId, intentId: args.intentId, intentSignHash: intentHash, responseId: args.responseId, responder: args.responder, issuedUnix: args.issuedUnix, expiresUnix: args.expiresUnix, contextHash: args.contextHash, recipient: args.initiator, replayGuard, responseType: 1 /* ACCEPT */ };
  const responseHash = typedSignDocHash(TYPE_URL_RESPONSE, encodeResponsePb(responsePb));

  const finalizePb: FinalizePb = { chainId: args.chainId, poolId: args.poolId, intentId: args.intentId, responseId: args.responseId, intentSignHash: intentHash, responseSignHash: responseHash, finalizeId: args.finalizeId, initiator: args.initiator, responder: args.responder, issuedUnix: args.issuedUnix, expiresUnix: args.expiresUnix, contextHash: args.contextHash, replayGuard };
  const finalizeHash = typedSignDocHash(TYPE_URL_FINALIZE, encodeFinalizePb(finalizePb));

  const certPb: CertPb = { chainId: args.chainId, poolId: args.poolId, intentId: args.intentId, responseId: args.responseId, finalizeId: args.finalizeId, certificateId: args.certificateId, intentSignHash: intentHash, responseSignHash: responseHash, finalizeSignHash: finalizeHash, initiator: args.initiator, responder: args.responder, issuedUnix: args.issuedUnix, expiresUnix: args.expiresUnix, contextHash: args.contextHash, replayGuard };
  const certHash = typedSignDocHash(TYPE_URL_CERTIFICATE, encodeCertPb(certPb));

  const aliceSK = new SigningKey(args.alicePrivateKey);
  const bobSK = new SigningKey(args.bobPrivateKey);

  const intentSig = hexToBytes(aliceSK.sign(intentHash).serialized);
  const responseSig = hexToBytes(bobSK.sign(responseHash).serialized);
  const finalizeAliceSig = hexToBytes(aliceSK.sign(finalizeHash).serialized);
  const finalizeBobSig = hexToBytes(bobSK.sign(finalizeHash).serialized);
  const boardSig = hexToBytes(aliceSK.sign(certHash).serialized);

  const signedIntent = concatBytes(encodeFieldMessage(1, encodeIntentPb(intentPb)), encodeFieldMessage(2, encodePbSig({ signer: args.initiator, algorithm: SIGNATURE_ALGO_SECP256K1, signature: intentSig })), encodeFieldBytes(3, intentHash));
  const signedResponse = concatBytes(encodeFieldMessage(1, encodeResponsePb(responsePb)), encodeFieldMessage(2, encodePbSig({ signer: args.responder, algorithm: SIGNATURE_ALGO_SECP256K1, signature: responseSig })), encodeFieldBytes(3, responseHash));
  const signedFinalize = concatBytes(encodeFieldMessage(1, encodeFinalizePb(finalizePb)), encodeFieldMessage(2, encodePbSig({ signer: args.initiator, algorithm: SIGNATURE_ALGO_SECP256K1, signature: finalizeAliceSig })), encodeFieldMessage(3, encodePbSig({ signer: args.responder, algorithm: SIGNATURE_ALGO_SECP256K1, signature: finalizeBobSig })), encodeFieldBytes(4, finalizeHash));

  const certificateBytes = concatBytes(
    encodeFieldMessage(1, encodeCertPb(certPb)),
    encodeFieldMessage(2, signedIntent),
    encodeFieldMessage(3, signedResponse),
    encodeFieldMessage(4, signedFinalize),
    encodeFieldMessage(5, encodePbSig({ signer: args.initiator, algorithm: SIGNATURE_ALGO_SECP256K1, signature: boardSig })),
    encodeFieldBytes(6, certHash),
  );

  return {
    certificateBytes,
    intentSignHash: Buffer.from(intentHash).toString("hex"),
    responseSignHash: Buffer.from(responseHash).toString("hex"),
    finalizeSignHash: Buffer.from(finalizeHash).toString("hex"),
  };
}

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

type Sig = { signer: string; algorithm: "secp256k1"; signature: string };

type IntentReq = {
  pool_id: string;
  intent_id: string;
  sender: string;
  recipient: string;
  expires_unix: number;
  digest_algorithm: "sha256";
  intent_sign_hash: string;
  context_hash: string;
  signature: Sig;
};

type ResponseReq = {
  pool_id: string;
  intent_id: string;
  response_id: string;
  sender: string;
  recipient: string;
  expires_unix: number;
  digest_algorithm: "sha256";
  response_type: "ACCEPT" | "COUNTER_OFFER";
  intent_sign_hash: string;
  response_sign_hash: string;
  context_hash: string;
  signature: Sig;
};

type FinalizeReq = {
  pool_id: string;
  intent_id: string;
  response_id: string;
  finalize_id: string;
  sender: string;
  recipient: string;
  expires_unix: number;
  digest_algorithm: "sha256";
  intent_sign_hash: string;
  response_sign_hash: string;
  finalize_sign_hash: string;
  context_hash: string;
  initiator_signature: Sig;
  responder_signature: Sig;
};

type CancelIntentReq = {
  pool_id: string;
  intent_id: string;
  canceller: string;
  signature: Sig;
};

type CancelResponseReq = {
  pool_id: string;
  intent_id: string;
  response_id: string;
  canceller: string;
  signature: Sig;
};

type MatchCandidate = { match_id: string; pool_id: string; intent_id: string; requester: string; responder: string };
type CandidatesResp = { candidates: MatchCandidate[]; total: number };
type InboxResp = { records: { record_type: string; response_sign_hash?: string; finalize_sign_hash?: string }[]; total: number };

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function randomHex32(): string {
  return randomBytes(32).toString("hex");
}

function sha256Hex(input: string): string {
  return createHash("sha256").update(input).digest("hex");
}

function signHex(hashHex: string, privateKey: string): string {
  return new SigningKey(privateKey).sign(`0x${hashHex}`).serialized;
}

function newWallet(): { address: string; privateKey: string } {
  const w = Wallet.createRandom();
  return { address: w.address, privateKey: w.privateKey };
}

function runId(): string {
  return `${Date.now()}-${Math.floor(Math.random() * 1_000_000)}`;
}

function log(msg: string, data?: unknown): void {
  if (!VERBOSE) return;
  const ts = new Date().toISOString();
  data === undefined ? console.log(`[${ts}] ${msg}`) : console.log(`[${ts}] ${msg}`, data);
}

async function req<T>(method: "GET" | "POST", path: string, body?: unknown): Promise<{ status: number; data: T }> {
  const url = `${BASE}${path}`;
  const res = await fetch(url, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : {},
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  const text = await res.text();
  log(`${method} ${path} → ${res.status}`, body !== undefined ? "(request sent)" : undefined);
  let data: T;
  try {
    data = JSON.parse(text) as T;
  } catch {
    throw new Error(`non-JSON response from ${method} ${path} (${res.status}): ${text}`);
  }
  return { status: res.status, data };
}

/** Cancel-intent sign domain: sha256("CANCEL_INTENT:<len>:<poolId>:<len>:<intentId>") */
function cancelIntentHash(poolId: string, intentId: string): string {
  const msg = `CANCEL_INTENT:${poolId.length}:${poolId}:${intentId.length}:${intentId}`;
  return sha256Hex(msg);
}

/** Cancel-response sign domain: sha256("CANCEL_RESPONSE:<len>:<poolId>:<len>:<intentId>:<len>:<responseId>") */
function cancelResponseHash(poolId: string, intentId: string, responseId: string): string {
  const msg = `CANCEL_RESPONSE:${poolId.length}:${poolId}:${intentId.length}:${intentId}:${responseId.length}:${responseId}`;
  return sha256Hex(msg);
}

function pass(name: string): void {
  console.log(`  ✓ ${name}`);
}

/** Query on-chain match history via the match precompile contract. */
async function queryMatchOnChain(
  poolId: string,
  intentId: string,
): Promise<{ exists: boolean; matchId?: string; requester?: string; responder?: string }> {
  const provider = new JsonRpcProvider(EVM_RPC_URL);
  const contract = new Contract(MATCH_PRECOMPILE_ADDRESS, MATCH_PRECOMPILE_ABI, provider);
  const exists: boolean = await contract.hasReplay(poolId, intentId);
  if (!exists) return { exists: false };
  const [, matchId, requester, responder]: [boolean, string, string, string] =
    await contract.getReplayParties(poolId, intentId);
  return { exists: true, matchId, requester, responder };
}

// ---------------------------------------------------------------------------
// Test cases
// ---------------------------------------------------------------------------

async function testHappyPath(): Promise<void> {
  const id = runId();
  const alice = newWallet();
  const bob = newWallet();
  const expires = Math.floor(Date.now() / 1000) + 600;

  const poolId = `pool-${id}`;
  const intentId = `intent-${id}`;
  const responseId = `response-${id}`;
  const finalizeId = `finalize-${id}`;

  const intentHash = randomHex32();
  const responseHash = randomHex32();
  const finalizeHash = randomHex32();
  const contextHash = randomHex32();

  // 1. Alice posts intent
  const intentRes = await req<unknown>("POST", "/v1/intents", {
    pool_id: poolId, intent_id: intentId,
    sender: alice.address, recipient: bob.address,
    expires_unix: expires, digest_algorithm: "sha256",
    intent_sign_hash: intentHash, context_hash: contextHash,
    signature: { signer: alice.address, algorithm: "secp256k1", signature: signHex(intentHash, alice.privateKey) },
  } satisfies IntentReq);
  console.log("POST /v1/intents", intentRes.status, intentRes.data);
  assert.equal(intentRes.status, 201);
  pass("Alice: POST /v1/intents → 201");

  // 2. Bob posts response
  const responseRes = await req<unknown>("POST", "/v1/responses", {
    pool_id: poolId, intent_id: intentId, response_id: responseId,
    sender: bob.address, recipient: alice.address,
    expires_unix: expires, digest_algorithm: "sha256",
    response_type: "ACCEPT",
    intent_sign_hash: intentHash, response_sign_hash: responseHash, context_hash: contextHash,
    signature: { signer: bob.address, algorithm: "secp256k1", signature: signHex(responseHash, bob.privateKey) },
  } satisfies ResponseReq);
  console.log("POST /v1/responses", responseRes.status, responseRes.data);
  assert.equal(responseRes.status, 201);
  pass("Bob: POST /v1/responses → 201");

  // 3. Alice finalizes (accepts Bob's response)
  const finalizeRes = await req<unknown>("POST", "/v1/finalize", {
    pool_id: poolId, intent_id: intentId, response_id: responseId, finalize_id: finalizeId,
    sender: alice.address, recipient: bob.address,
    expires_unix: expires, digest_algorithm: "sha256",
    intent_sign_hash: intentHash, response_sign_hash: responseHash, finalize_sign_hash: finalizeHash,
    context_hash: contextHash,
    initiator_signature: { signer: alice.address, algorithm: "secp256k1", signature: signHex(finalizeHash, alice.privateKey) },
    responder_signature: { signer: bob.address, algorithm: "secp256k1", signature: signHex(finalizeHash, bob.privateKey) },
  } satisfies FinalizeReq);
  console.log("POST /v1/finalize", finalizeRes.status, finalizeRes.data);
  assert.equal(finalizeRes.status, 201);
  pass("Alice: POST /v1/finalize → 201");

  // 4. Alice inbox: should contain Bob's response
  const aliceInbox = await req<InboxResp>("GET", `/v1/inbox?recipient=${alice.address}`);
  console.log("GET /v1/inbox (alice)", aliceInbox.status, aliceInbox.data.records);
  assert.equal(aliceInbox.status, 200);
  assert.ok(
    aliceInbox.data.records.some((r) => r.record_type === "response" && r.response_sign_hash === responseHash),
    "alice inbox should contain bob's response",
  );
  pass("GET /v1/inbox (alice) contains response");

  // 5. Alice outbox: should contain intent + finalize
  const aliceOutbox = await req<InboxResp>("GET", `/v1/outbox?sender=${alice.address}`);
  console.log("GET /v1/outbox (alice)", aliceOutbox.status, aliceOutbox.data.records);
  assert.equal(aliceOutbox.status, 200);
  assert.ok(aliceOutbox.data.records.some((r) => r.record_type === "intent"), "alice outbox: missing intent");
  assert.ok(aliceOutbox.data.records.some((r) => r.record_type === "finalize" && r.finalize_sign_hash === finalizeHash), "alice outbox: missing finalize");
  pass("GET /v1/outbox (alice) contains intent + finalize");

  // 6. Bob inbox: should contain intent + finalize
  const bobInbox = await req<InboxResp>("GET", `/v1/inbox?recipient=${bob.address}`);
  console.log("GET /v1/inbox (bob)", bobInbox.status, bobInbox.data.records);
  assert.equal(bobInbox.status, 200);
  assert.ok(bobInbox.data.records.some((r) => r.record_type === "intent"), "bob inbox: missing intent");
  assert.ok(bobInbox.data.records.some((r) => r.record_type === "finalize"), "bob inbox: missing finalize");
  pass("GET /v1/inbox (bob) contains intent + finalize");

  // 7. Bob outbox: should contain response
  const bobOutbox = await req<InboxResp>("GET", `/v1/outbox?sender=${bob.address}`);
  console.log("GET /v1/outbox (bob)", bobOutbox.status, bobOutbox.data.records);
  assert.equal(bobOutbox.status, 200);
  assert.ok(bobOutbox.data.records.some((r) => r.record_type === "response" && r.response_sign_hash === responseHash), "bob outbox: missing response");
  pass("GET /v1/outbox (bob) contains response");
}

async function testCancelIntent(): Promise<void> {
  const id = runId();
  const alice = newWallet();
  const bob = newWallet();
  const expires = Math.floor(Date.now() / 1000) + 600;

  const poolId = `pool-ci-${id}`;
  const intentId = `intent-ci-${id}`;
  const responseId = `response-ci-${id}`;

  const intentHash = randomHex32();
  const responseHash = randomHex32();
  const contextHash = randomHex32();

  // Alice posts intent
  const intentRes = await req<unknown>("POST", "/v1/intents", {
    pool_id: poolId, intent_id: intentId,
    sender: alice.address, recipient: bob.address,
    expires_unix: expires, digest_algorithm: "sha256",
    intent_sign_hash: intentHash, context_hash: contextHash,
    signature: { signer: alice.address, algorithm: "secp256k1", signature: signHex(intentHash, alice.privateKey) },
  } satisfies IntentReq);
  console.log("POST /v1/intents", intentRes.status, intentRes.data);
  assert.equal(intentRes.status, 201);

  // Bob posts response
  const responseRes = await req<unknown>("POST", "/v1/responses", {
    pool_id: poolId, intent_id: intentId, response_id: responseId,
    sender: bob.address, recipient: alice.address,
    expires_unix: expires, digest_algorithm: "sha256",
    response_type: "ACCEPT",
    intent_sign_hash: intentHash, response_sign_hash: responseHash, context_hash: contextHash,
    signature: { signer: bob.address, algorithm: "secp256k1", signature: signHex(responseHash, bob.privateKey) },
  } satisfies ResponseReq);
  console.log("POST /v1/responses", responseRes.status, responseRes.data);
  assert.equal(responseRes.status, 201);

  // Pair should appear in matcher candidates
  const before = await req<CandidatesResp>("GET", "/v1/matcher/candidates");
  console.log("GET /v1/matcher/candidates (before cancel)", before.status, before.data.candidates);
  assert.equal(before.status, 200);
  assert.ok(before.data.candidates.some((c) => c.pool_id === poolId && c.intent_id === intentId), "pair should be visible before cancel");
  pass("matcher candidates: pair visible before cancel");

  // Alice cancels intent
  const cancelRes = await req<unknown>("POST", "/v1/intents/cancel", {
    pool_id: poolId, intent_id: intentId, canceller: alice.address,
    signature: { signer: alice.address, algorithm: "secp256k1", signature: signHex(cancelIntentHash(poolId, intentId), alice.privateKey) },
  } satisfies CancelIntentReq);
  console.log("POST /v1/intents/cancel", cancelRes.status, cancelRes.data);
  assert.equal(cancelRes.status, 200);
  pass("POST /v1/intents/cancel → 200");

  // Pair should be gone from candidates
  const after = await req<CandidatesResp>("GET", "/v1/matcher/candidates");
  console.log("GET /v1/matcher/candidates (after cancel)", after.status, after.data.candidates);
  assert.equal(after.status, 200);
  assert.ok(!after.data.candidates.some((c) => c.pool_id === poolId && c.intent_id === intentId), "cancelled intent should not appear in candidates");
  pass("matcher candidates: pair hidden after cancel");
}

async function testCancelResponse(): Promise<void> {
  const id = runId();
  const alice = newWallet();
  const bob = newWallet();
  const expires = Math.floor(Date.now() / 1000) + 600;

  const poolId = `pool-cr-${id}`;
  const intentId = `intent-cr-${id}`;
  const responseId = `response-cr-${id}`;

  const intentHash = randomHex32();
  const responseHash = randomHex32();
  const contextHash = randomHex32();

  // Alice posts intent
  const intentRes = await req<unknown>("POST", "/v1/intents", {
    pool_id: poolId, intent_id: intentId,
    sender: alice.address, recipient: bob.address,
    expires_unix: expires, digest_algorithm: "sha256",
    intent_sign_hash: intentHash, context_hash: contextHash,
    signature: { signer: alice.address, algorithm: "secp256k1", signature: signHex(intentHash, alice.privateKey) },
  } satisfies IntentReq);
  console.log("POST /v1/intents", intentRes.status, intentRes.data);
  assert.equal(intentRes.status, 201);

  // Bob posts response
  const responseRes = await req<unknown>("POST", "/v1/responses", {
    pool_id: poolId, intent_id: intentId, response_id: responseId,
    sender: bob.address, recipient: alice.address,
    expires_unix: expires, digest_algorithm: "sha256",
    response_type: "ACCEPT",
    intent_sign_hash: intentHash, response_sign_hash: responseHash, context_hash: contextHash,
    signature: { signer: bob.address, algorithm: "secp256k1", signature: signHex(responseHash, bob.privateKey) },
  } satisfies ResponseReq);
  console.log("POST /v1/responses", responseRes.status, responseRes.data);
  assert.equal(responseRes.status, 201);

  // Pair should appear in matcher candidates
  const before = await req<CandidatesResp>("GET", "/v1/matcher/candidates");
  console.log("GET /v1/matcher/candidates (before cancel)", before.status, before.data.candidates);
  assert.ok(before.data.candidates.some((c) => c.pool_id === poolId), "pair should be visible before cancel");
  pass("matcher candidates: pair visible before cancel");

  // Bob cancels response
  const cancelRes = await req<unknown>("POST", "/v1/responses/cancel", {
    pool_id: poolId, intent_id: intentId, response_id: responseId, canceller: bob.address,
    signature: { signer: bob.address, algorithm: "secp256k1", signature: signHex(cancelResponseHash(poolId, intentId, responseId), bob.privateKey) },
  } satisfies CancelResponseReq);
  console.log("POST /v1/responses/cancel", cancelRes.status, cancelRes.data);
  assert.equal(cancelRes.status, 200);
  pass("POST /v1/responses/cancel → 200");

  // Pair should be gone from candidates
  const after = await req<CandidatesResp>("GET", "/v1/matcher/candidates");
  console.log("GET /v1/matcher/candidates (after cancel)", after.status, after.data.candidates);
  assert.ok(!after.data.candidates.some((c) => c.pool_id === poolId), "cancelled response pair should not appear");
  pass("matcher candidates: pair hidden after cancel");
}

async function testCancelledIntentBlocksFinalize(): Promise<void> {
  const id = runId();
  const alice = newWallet();
  const bob = newWallet();
  const expires = Math.floor(Date.now() / 1000) + 600;

  const poolId = `pool-cf-${id}`;
  const intentId = `intent-cf-${id}`;
  const responseId = `response-cf-${id}`;
  const finalizeId = `finalize-cf-${id}`;

  const intentHash = randomHex32();
  const responseHash = randomHex32();
  const finalizeHash = randomHex32();
  const contextHash = randomHex32();

  // Alice posts intent
  const intentRes = await req<unknown>("POST", "/v1/intents", {
    pool_id: poolId, intent_id: intentId,
    sender: alice.address, recipient: bob.address,
    expires_unix: expires, digest_algorithm: "sha256",
    intent_sign_hash: intentHash, context_hash: contextHash,
    signature: { signer: alice.address, algorithm: "secp256k1", signature: signHex(intentHash, alice.privateKey) },
  } satisfies IntentReq);
  console.log("POST /v1/intents", intentRes.status, intentRes.data);
  assert.equal(intentRes.status, 201);

  // Bob posts response
  const responseRes = await req<unknown>("POST", "/v1/responses", {
    pool_id: poolId, intent_id: intentId, response_id: responseId,
    sender: bob.address, recipient: alice.address,
    expires_unix: expires, digest_algorithm: "sha256",
    response_type: "ACCEPT",
    intent_sign_hash: intentHash, response_sign_hash: responseHash, context_hash: contextHash,
    signature: { signer: bob.address, algorithm: "secp256k1", signature: signHex(responseHash, bob.privateKey) },
  } satisfies ResponseReq);
  console.log("POST /v1/responses", responseRes.status, responseRes.data);
  assert.equal(responseRes.status, 201);

  // Alice cancels intent
  const cancelRes = await req<unknown>("POST", "/v1/intents/cancel", {
    pool_id: poolId, intent_id: intentId, canceller: alice.address,
    signature: { signer: alice.address, algorithm: "secp256k1", signature: signHex(cancelIntentHash(poolId, intentId), alice.privateKey) },
  } satisfies CancelIntentReq);
  console.log("POST /v1/intents/cancel", cancelRes.status, cancelRes.data);
  assert.equal(cancelRes.status, 200);

  // Attempt finalize — should fail because intent is cancelled
  const finalizeRes = await req<unknown>("POST", "/v1/finalize", {
    pool_id: poolId, intent_id: intentId, response_id: responseId, finalize_id: finalizeId,
    sender: alice.address, recipient: bob.address,
    expires_unix: expires, digest_algorithm: "sha256",
    intent_sign_hash: intentHash, response_sign_hash: responseHash, finalize_sign_hash: finalizeHash,
    context_hash: contextHash,
    initiator_signature: { signer: alice.address, algorithm: "secp256k1", signature: signHex(finalizeHash, alice.privateKey) },
    responder_signature: { signer: bob.address, algorithm: "secp256k1", signature: signHex(finalizeHash, bob.privateKey) },
  } satisfies FinalizeReq);
  console.log("POST /v1/finalize (after cancel)", finalizeRes.status, finalizeRes.data);
  assert.equal(finalizeRes.status, 400, `finalize after cancel should return 400, got ${finalizeRes.status}`);
  pass("POST /v1/finalize after cancel → 400 (blocked)");
}

async function testOnChainMatchQuery(): Promise<void> {
  const id = runId();
  const alice = newWallet();
  const bob = newWallet();
  const now = Math.floor(Date.now() / 1000);
  const issuedUnix = BigInt(now);
  const expiresUnix = BigInt(now + 600);

  const poolId = `pool-oc-${id}`;
  const intentId = `intent-oc-${id}`;
  const responseId = `response-oc-${id}`;
  const finalizeId = `finalize-oc-${id}`;
  const certificateId = `cert-oc-${id}`;
  const contextHash = sha256Bytes(_textEncoder.encode(`ctx-${id}`));
  const contextHashHex = Buffer.from(contextHash).toString("hex");

  // Build canonical certificate bytes (proper protobuf encoding with valid signatures)
  const { certificateBytes, intentSignHash, responseSignHash, finalizeSignHash } = buildMatchCertificateBytes({
    chainId: CHAIN_ID,
    poolId, intentId, responseId, finalizeId, certificateId,
    initiator: alice.address, responder: bob.address,
    issuedUnix, expiresUnix, contextHash,
    alicePrivateKey: alice.privateKey, bobPrivateKey: bob.privateKey,
  });

  // 1. Alice posts intent
  const intentRes = await req<unknown>("POST", "/v1/intents", {
    pool_id: poolId, intent_id: intentId,
    sender: alice.address, recipient: bob.address,
    expires_unix: Number(expiresUnix), digest_algorithm: "sha256",
    intent_sign_hash: intentSignHash, context_hash: contextHashHex,
    signature: { signer: alice.address, algorithm: "secp256k1", signature: signHex(intentSignHash, alice.privateKey) },
  } satisfies IntentReq);
  console.log("POST /v1/intents", intentRes.status, intentRes.data);
  assert.equal(intentRes.status, 201);
  pass("Alice: POST /v1/intents → 201");

  // 2. Bob posts response
  const responseRes = await req<unknown>("POST", "/v1/responses", {
    pool_id: poolId, intent_id: intentId, response_id: responseId,
    sender: bob.address, recipient: alice.address,
    expires_unix: Number(expiresUnix), digest_algorithm: "sha256",
    response_type: "ACCEPT",
    intent_sign_hash: intentSignHash, response_sign_hash: responseSignHash, context_hash: contextHashHex,
    signature: { signer: bob.address, algorithm: "secp256k1", signature: signHex(responseSignHash, bob.privateKey) },
  } satisfies ResponseReq);
  console.log("POST /v1/responses", responseRes.status, responseRes.data);
  assert.equal(responseRes.status, 201);
  pass("Bob: POST /v1/responses → 201");

  // 3. Alice accepts: POST /v1/finalize with match_certificate
  //    matchboard enqueues it as ABCI operation → injected into next block → on-chain precompile records it
  const finalizeRes = await req<unknown>("POST", "/v1/finalize", {
    pool_id: poolId, intent_id: intentId, response_id: responseId, finalize_id: finalizeId,
    sender: alice.address, recipient: bob.address,
    expires_unix: Number(expiresUnix), digest_algorithm: "sha256",
    intent_sign_hash: intentSignHash, response_sign_hash: responseSignHash, finalize_sign_hash: finalizeSignHash,
    context_hash: contextHashHex,
    // base64-encoded certificate — Go's json.Unmarshal decodes []byte fields from base64
    match_certificate: Buffer.from(certificateBytes).toString("base64"),
    initiator_signature: { signer: alice.address, algorithm: "secp256k1", signature: signHex(finalizeSignHash, alice.privateKey) },
    responder_signature: { signer: bob.address, algorithm: "secp256k1", signature: signHex(finalizeSignHash, bob.privateKey) },
  });
  console.log("POST /v1/finalize", finalizeRes.status, finalizeRes.data);
  assert.equal(finalizeRes.status, 201, `POST /v1/finalize expected 201, got ${finalizeRes.status}: ${JSON.stringify(finalizeRes.data)}`);
  pass("Alice: POST /v1/finalize (with certificate) → 201");

  // 4. Check if EVM RPC is reachable; if not, skip on-chain assertion
  try {
    const provider = new JsonRpcProvider(EVM_RPC_URL);
    await provider.getBlockNumber();
  } catch {
    log(`EVM RPC not reachable at ${EVM_RPC_URL} — skipping on-chain assertion`);
    pass("on-chain match query (skipped — EVM RPC not available)");
    return;
  }

  // 5. Poll hasReplay until ABCI injects the op and the block is committed (~1-2 blocks)
  const POLL_INTERVAL_MS = 500;
  const POLL_TIMEOUT_MS = 15_000;
  const deadline = Date.now() + POLL_TIMEOUT_MS;
  let result = await queryMatchOnChain(poolId, intentId);
  while (!result.exists && Date.now() < deadline) {
    await new Promise((r) => setTimeout(r, POLL_INTERVAL_MS));
    result = await queryMatchOnChain(poolId, intentId);
  }
  log("on-chain match query result", result);

  console.log("on-chain match query result", result);
  assert.ok(result.exists, "match should be on-chain after ABCI injection (timeout after 15s)");
  assert.ok(result.matchId, "matchId should be set");
  assert.equal(getAddress(result.requester!), getAddress(alice.address), "requester should be alice");
  assert.equal(getAddress(result.responder!), getAddress(bob.address), "responder should be bob");
  pass(`on-chain match query: matchId=${result.matchId}`);
}

// ---------------------------------------------------------------------------
// Runner
// ---------------------------------------------------------------------------

const TESTS: Array<[string, () => Promise<void>]> = [
  ["happy path: intent → response → finalize → inbox/outbox", testHappyPath],
  ["cancel intent: blocks matching", testCancelIntent],
  ["cancel response: blocks matching", testCancelResponse],
  ["cancel intent: blocks finalize", testCancelledIntentBlocksFinalize],
  ["on-chain match query via precompile", testOnChainMatchQuery],
];

async function main(): Promise<void> {
  try {
    const res = await fetch(`${BASE}/v1/inbox`);
    if (res.status === 404) throw new Error("404");
    log("matchboard reachable", { url: BASE });
  } catch {
    console.error(`matchboard not reachable at ${BASE}`);
    console.error(`start evmd first:  ./local_node.sh`);
    console.error(`or set:            MATCHBOARD_URL=http://<host>:<port>`);
    process.exit(1);
  }

  let passed = 0;
  let failed = 0;

  for (const [name, fn] of TESTS) {
    console.log(`\n▶ ${name}`);
    try {
      await fn();
      console.log(`  → PASS`);
      passed++;
    } catch (err) {
      console.error(`  → FAIL`, err);
      failed++;
    }
  }

  console.log(`\n${passed + failed} tests: ${passed} passed, ${failed} failed`);
  if (failed > 0) process.exit(1);
}

main().catch((err) => {
  console.error("unexpected error", err);
  process.exit(1);
});
