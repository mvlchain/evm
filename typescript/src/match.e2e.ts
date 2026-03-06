import assert from "node:assert/strict";
import { createHash } from "node:crypto";
import process from "node:process";
import { Contract, JsonRpcProvider, SigningKey, Wallet, getAddress, isAddress } from "ethers";

type PublishIntentRequest = {
  protocol_version?: string;
  board_id?: string;
  chain_id?: string;
  pool_id: string;
  intent_id: string;
  sender: string;
  recipient: string;
  expires_unix: number;
  digest_algorithm: "sha256";
  intent_sign_hash: string;
  context_hash: string;
  terms_hash?: string;
  policy_hash?: string;
  signature: SignatureMetadata;
};

type PublishResponseRequest = {
  protocol_version?: string;
  board_id?: string;
  chain_id?: string;
  pool_id: string;
  intent_id: string;
  response_id: string;
  sender: string;
  recipient: string;
  expires_unix: number;
  digest_algorithm: "sha256";
  intent_sign_hash: string;
  response_sign_hash: string;
  context_hash: string;
  terms_hash?: string;
  policy_hash?: string;
  signature: SignatureMetadata;
};

type PublishFinalizeRequest = {
  protocol_version?: string;
  board_id?: string;
  chain_id?: string;
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
  initiator_signature: SignatureMetadata;
  responder_signature: SignatureMetadata;
};

type SignatureMetadata = {
  signer: string;
  algorithm: "secp256k1" | "ed25519";
  public_key?: string;
  signature: string;
};

type PublishIntentResponse = {
  pool_id: string;
  intent_id: string;
  intent_sign_hash: string;
  stored_unix: number;
};

type PublishResponseResponse = {
  pool_id: string;
  intent_id: string;
  response_id: string;
  response_sign_hash: string;
  stored_unix: number;
};

type PublishFinalizeResponse = {
  pool_id: string;
  intent_id: string;
  response_id: string;
  finalize_id: string;
  finalize_sign_hash: string;
  stored_unix: number;
};

type BoardRecord = {
  record_type: "intent" | "response" | "finalize";
  pool_id: string;
  intent_id: string;
  response_id?: string;
  finalize_id?: string;
  sender: string;
  recipient: string;
  created_unix: number;
  context_hash?: string;
  intent_sign_hash?: string;
  response_sign_hash?: string;
  finalize_sign_hash?: string;
};

type ListRecordsResponse = {
  principal: string;
  records: BoardRecord[];
  next_cursor?: string;
  total: number;
};

const RPC_URL = mustEnv("NODE_RPC_URL", "http://127.0.0.1:26657");
const EVM_RPC_URL = mustEnv("EVM_RPC_URL", "http://127.0.0.1:8545");
const MATCHBOARD_URL = mustEnv("MATCHBOARD_URL", "http://127.0.0.1:8080");
const CHAIN_ID = mustEnv("MATCH_CHAIN_ID", "9001");
const MATCH_PRECOMPILE_ADDRESS = mustEnv(
  "MATCH_PRECOMPILE_ADDRESS",
  "0x0000000000000000000000000000000000000808",
);
const MATCH_EXPECT_ONCHAIN_REPLAY = mustEnv("MATCH_EXPECT_ONCHAIN_REPLAY", "1") === "1";
const MATCH_REQUIRE_PRECOMPILE = mustEnv("MATCH_REQUIRE_PRECOMPILE", "1") === "1";
const ALICE_TOKEN = mustEnv("MATCHBOARD_TOKEN_ALICE", "token-alice");
const BOB_TOKEN = mustEnv("MATCHBOARD_TOKEN_BOB", "token-bob");
const ALICE = mustEnv("MATCHBOARD_PRINCIPAL_ALICE", "0xC6Fe5D33615a1C52c08018c47E8Bc53646A0E101");
const BOB = mustEnv("MATCHBOARD_PRINCIPAL_BOB", "0x963EBDf2e1f8DB8707D05FC75bfeFFBa1B5BaC17");
const VERBOSE = mustEnv("MATCH_E2E_VERBOSE", "1") !== "0";
const ALICE_PRIVATE_KEY = mustPrivateKey(
  "MATCHBOARD_ALICE_PRIVATE_KEY",
  "0x88cbead91aee890d27bf06e003ade3d4e952427e88f88d31d61d3ef5e5d54305",
);
const BOB_PRIVATE_KEY = mustPrivateKey(
  "MATCHBOARD_BOB_PRIVATE_KEY",
  "0x741de4f8988ea941d3ff0287911ca4074e62b7d45c991a51186455366f10b544",
);
const MATCH_PRECOMPILE_ABI = [
  "function hasReplay(string poolId, string intentId) view returns (bool exists)",
  "function getReplay(string poolId, string intentId) view returns (bool found, string matchId)",
  "function getReplayParties(string poolId, string intentId) view returns (bool found, string matchId, string requester, string responder)",
  "function submitMatchCertificate(bytes certificate) returns (string matchId, string replayKey, bytes certificateHash)",
];

function mustEnv(key: string, fallback: string): string {
  return process.env[key] ?? fallback;
}

function mustPrivateKey(key: string, fallback: string): string {
  const raw = (process.env[key] ?? fallback).trim();
  const prefixed = raw.startsWith("0x") ? raw : `0x${raw}`;
  if (!/^0x[0-9a-fA-F]{64}$/.test(prefixed)) {
    throw new Error(`${key} must be a 32-byte hex private key`);
  }
  return prefixed;
}

function nowIso(): string {
  return new Date().toISOString();
}

function logInfo(message: string, data?: unknown): void {
  if (!VERBOSE) {
    return;
  }
  if (data === undefined) {
    console.log(`[${nowIso()}] ${message}`);
    return;
  }
  console.log(`[${nowIso()}] ${message}`, sanitizeForLog(data));
}

function maskValue(value: string): string {
  if (value.length <= 16) {
    return "***";
  }
  return `${value.slice(0, 10)}...(${value.length})`;
}

function sanitizeForLog(input: unknown): unknown {
  if (input === null || input === undefined) {
    return input;
  }
  if (typeof input === "string") {
    return input;
  }
  if (Array.isArray(input)) {
    return input.map(sanitizeForLog);
  }
  if (typeof input !== "object") {
    return input;
  }

  const out: Record<string, unknown> = {};
  for (const [k, v] of Object.entries(input as Record<string, unknown>)) {
    const key = k.toLowerCase();
    if (
      key.includes("token") ||
      key.includes("private_key") ||
      key.includes("authorization") ||
      key === "signature"
    ) {
      if (typeof v === "string") {
        out[k] = maskValue(v);
      } else if (v && typeof v === "object") {
        const sigObj = v as Record<string, unknown>;
        out[k] = {
          ...sigObj,
          signature:
            typeof sigObj.signature === "string" ? maskValue(sigObj.signature) : sigObj.signature,
        };
      } else {
        out[k] = "***";
      }
      continue;
    }
    if ((key === "initiator_signature" || key === "responder_signature") && v && typeof v === "object") {
      const sigObj = v as Record<string, unknown>;
      out[k] = {
        ...sigObj,
        signature: typeof sigObj.signature === "string" ? maskValue(sigObj.signature) : sigObj.signature,
      };
      continue;
    }
    out[k] = sanitizeForLog(v);
  }
  return out;
}

const WIRE_TYPE_VARINT = 0;
const WIRE_TYPE_LEN = 2;
const DIGEST_ALGO_SHA256 = 1;
const SIGNATURE_ALGO_SECP256K1 = 1;
const SIGN_DOC_TYPE_INTENT = 1;
const SIGN_DOC_TYPE_RESPONSE = 2;
const SIGN_DOC_TYPE_FINALIZE = 3;
const SIGN_DOC_TYPE_CERTIFICATE = 4;
const textEncoder = new TextEncoder();

type IntentPayloadMsg = {
  protocolVersion?: string;
  boardId?: string;
  chainId?: string;
  poolId: string;
  intentId: string;
  initiator: string;
  initiatorNonce?: bigint;
  issuedUnix: bigint;
  expiresUnix: bigint;
  contextHash: Uint8Array;
  termsHash?: Uint8Array;
  policyHash?: Uint8Array;
  recipient?: string;
  replayGuard?: Uint8Array;
  digestAlgorithm: number;
};

type ResponsePayloadMsg = {
  protocolVersion?: string;
  boardId?: string;
  chainId?: string;
  poolId: string;
  intentId: string;
  intentSignHash: Uint8Array;
  responseId: string;
  responder: string;
  responderNonce?: bigint;
  issuedUnix: bigint;
  expiresUnix: bigint;
  contextHash: Uint8Array;
  termsHash?: Uint8Array;
  policyHash?: Uint8Array;
  recipient?: string;
  replayGuard?: Uint8Array;
  digestAlgorithm: number;
};

type FinalizePayloadMsg = {
  protocolVersion?: string;
  boardId?: string;
  chainId?: string;
  poolId: string;
  intentId: string;
  responseId: string;
  intentSignHash: Uint8Array;
  responseSignHash: Uint8Array;
  finalizeId: string;
  initiator: string;
  responder: string;
  finalizeNonce?: bigint;
  issuedUnix: bigint;
  expiresUnix: bigint;
  contextHash: Uint8Array;
  settlementHash?: Uint8Array;
  replayGuard?: Uint8Array;
  digestAlgorithm: number;
};

type CertificatePayloadMsg = {
  protocolVersion?: string;
  boardId?: string;
  chainId?: string;
  poolId: string;
  intentId: string;
  responseId: string;
  finalizeId: string;
  certificateId: string;
  intentSignHash: Uint8Array;
  responseSignHash: Uint8Array;
  finalizeSignHash: Uint8Array;
  initiator: string;
  responder: string;
  issuedUnix: bigint;
  expiresUnix: bigint;
  contextHash: Uint8Array;
  replayGuard?: Uint8Array;
  digestAlgorithm: number;
};

type SignatureMsg = {
  signer: string;
  algorithm: number;
  publicKey?: Uint8Array;
  signature: Uint8Array;
};

type CanonicalBundle = {
  intentPayload: IntentPayloadMsg;
  responsePayload: ResponsePayloadMsg;
  finalizePayload: FinalizePayloadMsg;
  certificatePayload: CertificatePayloadMsg;
  intentHash: Uint8Array;
  responseHash: Uint8Array;
  finalizeHash: Uint8Array;
  certificateHash: Uint8Array;
};

function concatBytes(...chunks: Uint8Array[]): Uint8Array {
  const total = chunks.reduce((n, c) => n + c.length, 0);
  const out = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.length;
  }
  return out;
}

function encodeVarint(value: number | bigint): Uint8Array {
  let v = typeof value === "bigint" ? value : BigInt(value);
  if (v < 0n) {
    throw new Error(`negative varint is not supported: ${value}`);
  }
  const bytes: number[] = [];
  while (v >= 0x80n) {
    bytes.push(Number((v & 0x7fn) | 0x80n));
    v >>= 7n;
  }
  bytes.push(Number(v));
  return Uint8Array.from(bytes);
}

function encodeTag(fieldNumber: number, wireType: number): Uint8Array {
  return encodeVarint((fieldNumber << 3) | wireType);
}

function encodeFieldVarint(fieldNumber: number, value: number | bigint): Uint8Array {
  if (value === 0 || value === 0n) {
    return new Uint8Array();
  }
  return concatBytes(encodeTag(fieldNumber, WIRE_TYPE_VARINT), encodeVarint(value));
}

function encodeFieldEnum(fieldNumber: number, value: number): Uint8Array {
  return encodeFieldVarint(fieldNumber, value);
}

function encodeFieldBytes(fieldNumber: number, value?: Uint8Array): Uint8Array {
  if (!value || value.length === 0) {
    return new Uint8Array();
  }
  return concatBytes(encodeTag(fieldNumber, WIRE_TYPE_LEN), encodeVarint(value.length), value);
}

function encodeFieldString(fieldNumber: number, value?: string): Uint8Array {
  if (!value || value.length === 0) {
    return new Uint8Array();
  }
  return encodeFieldBytes(fieldNumber, textEncoder.encode(value));
}

function encodeFieldMessage(fieldNumber: number, value: Uint8Array): Uint8Array {
  if (value.length === 0) {
    return new Uint8Array();
  }
  return concatBytes(encodeTag(fieldNumber, WIRE_TYPE_LEN), encodeVarint(value.length), value);
}

function sha256Bytes(data: Uint8Array): Uint8Array {
  return Uint8Array.from(createHash("sha256").update(data).digest());
}

function hexToBytes(hex: string): Uint8Array {
  const normalized = hex.startsWith("0x") ? hex.slice(2) : hex;
  if (normalized.length % 2 !== 0 || !/^[0-9a-fA-F]*$/.test(normalized)) {
    throw new Error(`invalid hex value: ${hex}`);
  }
  return Uint8Array.from(Buffer.from(normalized, "hex"));
}

function bytesToHex(bytes: Uint8Array): string {
  return Buffer.from(bytes).toString("hex");
}

function toHexPrefixed(bytes: Uint8Array): string {
  return `0x${bytesToHex(bytes)}`;
}

function encodeIntentPayload(p: IntentPayloadMsg): Uint8Array {
  return concatBytes(
    encodeFieldString(1, p.protocolVersion),
    encodeFieldString(2, p.boardId),
    encodeFieldString(3, p.chainId),
    encodeFieldString(4, p.poolId),
    encodeFieldString(5, p.intentId),
    encodeFieldString(6, p.initiator),
    encodeFieldVarint(7, p.initiatorNonce ?? 0n),
    encodeFieldVarint(8, p.issuedUnix),
    encodeFieldVarint(9, p.expiresUnix),
    encodeFieldBytes(10, p.contextHash),
    encodeFieldBytes(11, p.termsHash),
    encodeFieldBytes(12, p.policyHash),
    encodeFieldString(13, p.recipient),
    encodeFieldBytes(14, p.replayGuard),
    encodeFieldEnum(15, p.digestAlgorithm),
  );
}

function encodeIntentSignDoc(payload: IntentPayloadMsg): Uint8Array {
  return concatBytes(
    encodeFieldEnum(1, SIGN_DOC_TYPE_INTENT),
    encodeFieldMessage(2, encodeIntentPayload(payload)),
  );
}

function encodeResponsePayload(p: ResponsePayloadMsg): Uint8Array {
  return concatBytes(
    encodeFieldString(1, p.protocolVersion),
    encodeFieldString(2, p.boardId),
    encodeFieldString(3, p.chainId),
    encodeFieldString(4, p.poolId),
    encodeFieldString(5, p.intentId),
    encodeFieldBytes(6, p.intentSignHash),
    encodeFieldString(7, p.responseId),
    encodeFieldString(8, p.responder),
    encodeFieldVarint(9, p.responderNonce ?? 0n),
    encodeFieldVarint(10, p.issuedUnix),
    encodeFieldVarint(11, p.expiresUnix),
    encodeFieldBytes(12, p.contextHash),
    encodeFieldBytes(13, p.termsHash),
    encodeFieldBytes(14, p.policyHash),
    encodeFieldString(15, p.recipient),
    encodeFieldBytes(16, p.replayGuard),
    encodeFieldEnum(17, p.digestAlgorithm),
  );
}

function encodeResponseSignDoc(payload: ResponsePayloadMsg): Uint8Array {
  return concatBytes(
    encodeFieldEnum(1, SIGN_DOC_TYPE_RESPONSE),
    encodeFieldMessage(2, encodeResponsePayload(payload)),
  );
}

function encodeFinalizePayload(p: FinalizePayloadMsg): Uint8Array {
  return concatBytes(
    encodeFieldString(1, p.protocolVersion),
    encodeFieldString(2, p.boardId),
    encodeFieldString(3, p.chainId),
    encodeFieldString(4, p.poolId),
    encodeFieldString(5, p.intentId),
    encodeFieldString(6, p.responseId),
    encodeFieldBytes(7, p.intentSignHash),
    encodeFieldBytes(8, p.responseSignHash),
    encodeFieldString(9, p.finalizeId),
    encodeFieldString(10, p.initiator),
    encodeFieldString(11, p.responder),
    encodeFieldVarint(12, p.finalizeNonce ?? 0n),
    encodeFieldVarint(13, p.issuedUnix),
    encodeFieldVarint(14, p.expiresUnix),
    encodeFieldBytes(15, p.contextHash),
    encodeFieldBytes(16, p.settlementHash),
    encodeFieldBytes(17, p.replayGuard),
    encodeFieldEnum(18, p.digestAlgorithm),
  );
}

function encodeFinalizeSignDoc(payload: FinalizePayloadMsg): Uint8Array {
  return concatBytes(
    encodeFieldEnum(1, SIGN_DOC_TYPE_FINALIZE),
    encodeFieldMessage(2, encodeFinalizePayload(payload)),
  );
}

function encodeCertificatePayload(p: CertificatePayloadMsg): Uint8Array {
  return concatBytes(
    encodeFieldString(1, p.protocolVersion),
    encodeFieldString(2, p.boardId),
    encodeFieldString(3, p.chainId),
    encodeFieldString(4, p.poolId),
    encodeFieldString(5, p.intentId),
    encodeFieldString(6, p.responseId),
    encodeFieldString(7, p.finalizeId),
    encodeFieldString(8, p.certificateId),
    encodeFieldBytes(9, p.intentSignHash),
    encodeFieldBytes(10, p.responseSignHash),
    encodeFieldBytes(11, p.finalizeSignHash),
    encodeFieldString(12, p.initiator),
    encodeFieldString(13, p.responder),
    encodeFieldVarint(14, p.issuedUnix),
    encodeFieldVarint(15, p.expiresUnix),
    encodeFieldBytes(16, p.contextHash),
    encodeFieldBytes(17, p.replayGuard),
    encodeFieldEnum(18, p.digestAlgorithm),
  );
}

function encodeCertificateSignDoc(payload: CertificatePayloadMsg): Uint8Array {
  return concatBytes(
    encodeFieldEnum(1, SIGN_DOC_TYPE_CERTIFICATE),
    encodeFieldMessage(2, encodeCertificatePayload(payload)),
  );
}

function encodeSignature(sig: SignatureMsg): Uint8Array {
  return concatBytes(
    encodeFieldString(1, sig.signer),
    encodeFieldEnum(2, sig.algorithm),
    encodeFieldBytes(3, sig.publicKey),
    encodeFieldBytes(4, sig.signature),
  );
}

function encodeSignedIntent(input: {
  payload: IntentPayloadMsg;
  signature: SignatureMsg;
  signBytesHash: Uint8Array;
}): Uint8Array {
  return concatBytes(
    encodeFieldMessage(1, encodeIntentPayload(input.payload)),
    encodeFieldMessage(2, encodeSignature(input.signature)),
    encodeFieldBytes(3, input.signBytesHash),
  );
}

function encodeSignedResponse(input: {
  payload: ResponsePayloadMsg;
  signature: SignatureMsg;
  signBytesHash: Uint8Array;
}): Uint8Array {
  return concatBytes(
    encodeFieldMessage(1, encodeResponsePayload(input.payload)),
    encodeFieldMessage(2, encodeSignature(input.signature)),
    encodeFieldBytes(3, input.signBytesHash),
  );
}

function encodeSignedFinalize(input: {
  payload: FinalizePayloadMsg;
  initiatorSignature: SignatureMsg;
  responderSignature: SignatureMsg;
  signBytesHash: Uint8Array;
}): Uint8Array {
  return concatBytes(
    encodeFieldMessage(1, encodeFinalizePayload(input.payload)),
    encodeFieldMessage(2, encodeSignature(input.initiatorSignature)),
    encodeFieldMessage(3, encodeSignature(input.responderSignature)),
    encodeFieldBytes(4, input.signBytesHash),
  );
}

function encodeMatchCertificate(input: {
  payload: CertificatePayloadMsg;
  intent: { payload: IntentPayloadMsg; signature: SignatureMsg; signBytesHash: Uint8Array };
  response: { payload: ResponsePayloadMsg; signature: SignatureMsg; signBytesHash: Uint8Array };
  finalize: {
    payload: FinalizePayloadMsg;
    initiatorSignature: SignatureMsg;
    responderSignature: SignatureMsg;
    signBytesHash: Uint8Array;
  };
  boardSignature: SignatureMsg;
  signBytesHash: Uint8Array;
}): Uint8Array {
  return concatBytes(
    encodeFieldMessage(1, encodeCertificatePayload(input.payload)),
    encodeFieldMessage(2, encodeSignedIntent(input.intent)),
    encodeFieldMessage(3, encodeSignedResponse(input.response)),
    encodeFieldMessage(4, encodeSignedFinalize(input.finalize)),
    encodeFieldMessage(5, encodeSignature(input.boardSignature)),
    encodeFieldBytes(6, input.signBytesHash),
  );
}

function buildCanonicalBundle(args: {
  protocolVersion: string;
  boardId: string;
  chainId: string;
  poolId: string;
  intentId: string;
  responseId: string;
  finalizeId: string;
  certificateId: string;
  initiator: string;
  responder: string;
  issuedUnix: bigint;
  expiresUnix: bigint;
  contextHashHex: string;
}): CanonicalBundle {
  const contextHash = hexToBytes(args.contextHashHex);
  const replayGuard = sha256Bytes(
    textEncoder.encode(`${args.poolId}|${args.intentId}|${args.responseId}|${args.finalizeId}`),
  );

  const intentPayload: IntentPayloadMsg = {
    protocolVersion: args.protocolVersion,
    boardId: args.boardId,
    chainId: args.chainId,
    poolId: args.poolId,
    intentId: args.intentId,
    initiator: args.initiator,
    issuedUnix: args.issuedUnix,
    expiresUnix: args.expiresUnix,
    contextHash,
    recipient: args.responder,
    replayGuard,
    digestAlgorithm: DIGEST_ALGO_SHA256,
  };
  const intentHash = sha256Bytes(encodeIntentSignDoc(intentPayload));

  const responsePayload: ResponsePayloadMsg = {
    protocolVersion: args.protocolVersion,
    boardId: args.boardId,
    chainId: args.chainId,
    poolId: args.poolId,
    intentId: args.intentId,
    intentSignHash: intentHash,
    responseId: args.responseId,
    responder: args.responder,
    issuedUnix: args.issuedUnix,
    expiresUnix: args.expiresUnix,
    contextHash,
    recipient: args.initiator,
    replayGuard,
    digestAlgorithm: DIGEST_ALGO_SHA256,
  };
  const responseHash = sha256Bytes(encodeResponseSignDoc(responsePayload));

  const finalizePayload: FinalizePayloadMsg = {
    protocolVersion: args.protocolVersion,
    boardId: args.boardId,
    chainId: args.chainId,
    poolId: args.poolId,
    intentId: args.intentId,
    responseId: args.responseId,
    intentSignHash: intentHash,
    responseSignHash: responseHash,
    finalizeId: args.finalizeId,
    initiator: args.initiator,
    responder: args.responder,
    issuedUnix: args.issuedUnix,
    expiresUnix: args.expiresUnix,
    contextHash,
    replayGuard,
    digestAlgorithm: DIGEST_ALGO_SHA256,
  };
  const finalizeHash = sha256Bytes(encodeFinalizeSignDoc(finalizePayload));

  const certificatePayload: CertificatePayloadMsg = {
    protocolVersion: args.protocolVersion,
    boardId: args.boardId,
    chainId: args.chainId,
    poolId: args.poolId,
    intentId: args.intentId,
    responseId: args.responseId,
    finalizeId: args.finalizeId,
    certificateId: args.certificateId,
    intentSignHash: intentHash,
    responseSignHash: responseHash,
    finalizeSignHash: finalizeHash,
    initiator: args.initiator,
    responder: args.responder,
    issuedUnix: args.issuedUnix,
    expiresUnix: args.expiresUnix,
    contextHash,
    replayGuard,
    digestAlgorithm: DIGEST_ALGO_SHA256,
  };
  const certificateHash = sha256Bytes(encodeCertificateSignDoc(certificatePayload));

  return {
    intentPayload,
    responsePayload,
    finalizePayload,
    certificatePayload,
    intentHash,
    responseHash,
    finalizeHash,
    certificateHash,
  };
}

function toDigestHex(hashHex: string): string {
  if (!/^[0-9a-fA-F]{64}$/.test(hashHex)) {
    throw new Error(`invalid hash format for signing: ${hashHex}`);
  }
  return `0x${hashHex.toLowerCase()}`;
}

function signHash(hashHex: string, privateKey: string): string {
  const signingKey = new SigningKey(privateKey);
  return signingKey.sign(toDigestHex(hashHex)).serialized;
}

function signDigestBytes(hash: Uint8Array, privateKey: string): string {
  if (hash.length !== 32) {
    throw new Error(`hash must be 32 bytes, got ${hash.length}`);
  }
  const signingKey = new SigningKey(privateKey);
  return signingKey.sign(hash).serialized;
}

function assertPrincipalPrivateKeyMatch(principal: string, privateKey: string, label: string): void {
  if (!isAddress(principal)) {
    return;
  }
  const derived = new Wallet(privateKey).address;
  assert.equal(
    getAddress(principal),
    getAddress(derived),
    `${label} principal and private key mismatch: ${principal} != ${derived}`,
  );
}

function testHash(c: string): string {
  return c.repeat(64);
}

async function ensureNodeIsRunning(): Promise<void> {
  logInfo("checking node status", { url: `${RPC_URL}/status` });
  const statusRes = await fetch(`${RPC_URL}/status`);
  assert.equal(
    statusRes.status,
    200,
    `node status endpoint failed: ${statusRes.status} ${statusRes.statusText}`,
  );

  const status = (await statusRes.json()) as {
    result?: { sync_info?: { latest_block_height?: string } };
  };
  const height = Number(status.result?.sync_info?.latest_block_height ?? "0");
  assert.ok(Number.isFinite(height) && height >= 0, "invalid latest_block_height from node status");
  logInfo("node is reachable", { latest_block_height: height });
}

async function ensureEvmRpcIsRunning(): Promise<void> {
  try {
    const provider = new JsonRpcProvider(EVM_RPC_URL);
    const blockNumber = await provider.getBlockNumber();
    logInfo("evm rpc is reachable", { evm_rpc_url: EVM_RPC_URL, block_number: blockNumber });
  } catch (err) {
    throw new Error(
      `failed to connect to EVM JSON-RPC at ${EVM_RPC_URL} (set EVM_RPC_URL or start node JSON-RPC)`,
      { cause: err },
    );
  }
}

async function ensureMatchboardIsRunning(): Promise<void> {
  try {
    logInfo("checking matchboard endpoint", { url: `${MATCHBOARD_URL}/v1/inbox` });
    const res = await fetch(`${MATCHBOARD_URL}/v1/inbox`);
    assert.notEqual(
      res.status,
      404,
      `matchboard endpoint not found at ${MATCHBOARD_URL}; verify MATCHBOARD_URL`,
    );
    logInfo("matchboard is reachable", { status: res.status });
  } catch (err) {
    throw new Error(
      `failed to connect to matchboard at ${MATCHBOARD_URL} (start node with ./local_node.sh so matchboard runs in-process)`,
      { cause: err },
    );
  }
}

async function requestJson<T>(args: {
  method: "GET" | "POST";
  path: string;
  token: string;
  body?: unknown;
}): Promise<{ status: number; data: T; rawText: string }> {
  logInfo("request", {
    method: args.method,
    path: args.path,
    token: args.token,
    body: args.body,
  });

  const headers: Record<string, string> = {
    Authorization: `Bearer ${args.token}`,
  };
  if (args.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }

  const res = await fetch(`${MATCHBOARD_URL}${args.path}`, {
    method: args.method,
    headers,
    body: args.body === undefined ? undefined : JSON.stringify(args.body),
  });

  const rawText = await res.text();
  logInfo("response", {
    method: args.method,
    path: args.path,
    status: res.status,
    body: rawText,
  });

  let data: T;
  try {
    data = JSON.parse(rawText) as T;
  } catch (err) {
    throw new Error(`failed to parse JSON (${args.method} ${args.path}): ${rawText}`, {
      cause: err,
    });
  }

  return { status: res.status, data, rawText };
}

function assertStatus(status: number, expected: number, context: string): void {
  assert.equal(status, expected, `${context} expected HTTP ${expected}, got ${status}`);
}

async function queryMatchPrecompile(poolId: string, intentId: string): Promise<{
  available: boolean;
  partiesAvailable: boolean;
  exists: boolean;
  found: boolean;
  matchId: string;
  requester: string;
  responder: string;
}> {
  logInfo("querying match precompile", {
    evm_rpc_url: EVM_RPC_URL,
    precompile_address: MATCH_PRECOMPILE_ADDRESS,
    pool_id: poolId,
    intent_id: intentId,
  });
  const provider = new JsonRpcProvider(EVM_RPC_URL);
  const precompile = new Contract(MATCH_PRECOMPILE_ADDRESS, MATCH_PRECOMPILE_ABI, provider);
  try {
    const exists = Boolean(await precompile.hasReplay(poolId, intentId));
    const replay = (await precompile.getReplay(poolId, intentId)) as unknown as [boolean, string];
    const found = Boolean(replay[0]);
    const matchId = String(replay[1] ?? "");
    let partiesAvailable = true;
    let requester = "";
    let responder = "";

    try {
      const parties = (await precompile.getReplayParties(poolId, intentId)) as unknown as [
        boolean,
        string,
        string,
        string,
      ];
      requester = String(parties[2] ?? "");
      responder = String(parties[3] ?? "");
    } catch (partiesErr) {
      const asObj = partiesErr as { code?: string; value?: unknown; reason?: string };
      const reason = typeof asObj?.reason === "string" ? asObj.reason : "";
      if (
        (asObj?.code === "BAD_DATA" && asObj?.value === "0x") ||
        (asObj?.code === "CALL_EXCEPTION" && reason.includes("no method with id"))
      ) {
        partiesAvailable = false;
      } else {
        throw partiesErr;
      }
    }

    logInfo("match precompile response", {
      available: true,
      parties_available: partiesAvailable,
      exists,
      found,
      match_id: matchId,
      requester,
      responder,
    });
    return { available: true, partiesAvailable, exists, found, matchId, requester, responder };
  } catch (err) {
    const asObj = err as { code?: string; value?: unknown; shortMessage?: string };
    if (asObj?.code === "BAD_DATA" && asObj?.value === "0x") {
      logInfo("match precompile unavailable or method inactive", {
        precompile_address: MATCH_PRECOMPILE_ADDRESS,
        code: asObj.code,
        short_message: asObj.shortMessage,
      });
      return {
        available: false,
        partiesAvailable: false,
        exists: false,
        found: false,
        matchId: "",
        requester: "",
        responder: "",
      };
    }
    throw err;
  }
}

async function main(): Promise<void> {
  logInfo("match e2e started", {
    rpc_url: RPC_URL,
    evm_rpc_url: EVM_RPC_URL,
    matchboard_url: MATCHBOARD_URL,
    alice_principal: ALICE,
    bob_principal: BOB,
    verbose: VERBOSE,
  });
  await ensureNodeIsRunning();
  await ensureEvmRpcIsRunning();
  await ensureMatchboardIsRunning();
  assert.ok(isAddress(ALICE), `MATCHBOARD_PRINCIPAL_ALICE must be an ethereum address, got: ${ALICE}`);
  assert.ok(isAddress(BOB), `MATCHBOARD_PRINCIPAL_BOB must be an ethereum address, got: ${BOB}`);
  assertPrincipalPrivateKeyMatch(ALICE, ALICE_PRIVATE_KEY, "alice");
  assertPrincipalPrivateKeyMatch(BOB, BOB_PRIVATE_KEY, "bob");
  logInfo("principal/private-key checks passed");

  const now = Math.floor(Date.now() / 1000);
  const issuedUnix = BigInt(now);
  const expiresUnix = now + 600;
  const expiresUnixBig = BigInt(expiresUnix);
  const runId = `${now}-${Math.floor(Math.random() * 1_000_000)}`;
  logInfo("test run identifiers", { runId, expiresUnix });

  const poolId = `pool-ts-${runId}`;
  const intentId = `intent-ts-${runId}`;
  const responseId = `response-ts-${runId}`;
  const finalizeId = `finalize-ts-${runId}`;
  const certificateId = `certificate-ts-${runId}`;
  const contextHashHex = testHash("a");
  const protocolVersion = "match/v1";
  const boardId = "matchboard-e2e";
  const chainId = CHAIN_ID;

  const bundle = buildCanonicalBundle({
    protocolVersion,
    boardId,
    chainId,
    poolId,
    intentId,
    responseId,
    finalizeId,
    certificateId,
    initiator: ALICE,
    responder: BOB,
    issuedUnix,
    expiresUnix: expiresUnixBig,
    contextHashHex,
  });

  const intentSignHash = bytesToHex(bundle.intentHash);
  const responseSignHash = bytesToHex(bundle.responseHash);
  const finalizeSignHash = bytesToHex(bundle.finalizeHash);
  const certificateSignHash = bytesToHex(bundle.certificateHash);
  const intentSignatureHex = signDigestBytes(bundle.intentHash, ALICE_PRIVATE_KEY);
  const responseSignatureHex = signDigestBytes(bundle.responseHash, BOB_PRIVATE_KEY);
  const finalizeInitiatorSignatureHex = signDigestBytes(bundle.finalizeHash, ALICE_PRIVATE_KEY);
  const finalizeResponderSignatureHex = signDigestBytes(bundle.finalizeHash, BOB_PRIVATE_KEY);
  const boardSignatureHex = signDigestBytes(bundle.certificateHash, ALICE_PRIVATE_KEY);

  const intentReq: PublishIntentRequest = {
    protocol_version: protocolVersion,
    board_id: boardId,
    chain_id: chainId,
    pool_id: poolId,
    intent_id: intentId,
    sender: ALICE,
    recipient: BOB,
    expires_unix: expiresUnix,
    digest_algorithm: "sha256",
    intent_sign_hash: intentSignHash,
    context_hash: contextHashHex,
    signature: {
      signer: ALICE,
      algorithm: "secp256k1",
      signature: intentSignatureHex,
    },
  };

  const intentRes = await requestJson<PublishIntentResponse>({
    method: "POST",
    path: "/v1/intents",
    token: ALICE_TOKEN,
    body: intentReq,
  });
  assertStatus(intentRes.status, 201, "publish intent");
  assert.equal(intentRes.data.pool_id, intentReq.pool_id);
  assert.equal(intentRes.data.intent_id, intentReq.intent_id);
  assert.equal(intentRes.data.intent_sign_hash, intentSignHash);

  const responseReq: PublishResponseRequest = {
    protocol_version: protocolVersion,
    board_id: boardId,
    chain_id: chainId,
    pool_id: poolId,
    intent_id: intentId,
    response_id: responseId,
    sender: BOB,
    recipient: ALICE,
    expires_unix: expiresUnix,
    digest_algorithm: "sha256",
    intent_sign_hash: intentSignHash,
    response_sign_hash: responseSignHash,
    context_hash: contextHashHex,
    signature: {
      signer: BOB,
      algorithm: "secp256k1",
      signature: responseSignatureHex,
    },
  };

  const responseRes = await requestJson<PublishResponseResponse>({
    method: "POST",
    path: "/v1/responses",
    token: BOB_TOKEN,
    body: responseReq,
  });
  assertStatus(responseRes.status, 201, "publish response");
  assert.equal(responseRes.data.pool_id, responseReq.pool_id);
  assert.equal(responseRes.data.intent_id, responseReq.intent_id);
  assert.equal(responseRes.data.response_id, responseReq.response_id);
  assert.equal(responseRes.data.response_sign_hash, responseSignHash);

  const finalizeReq: PublishFinalizeRequest = {
    protocol_version: protocolVersion,
    board_id: boardId,
    chain_id: chainId,
    pool_id: poolId,
    intent_id: intentId,
    response_id: responseId,
    finalize_id: finalizeId,
    sender: ALICE,
    recipient: BOB,
    expires_unix: expiresUnix,
    digest_algorithm: "sha256",
    intent_sign_hash: intentSignHash,
    response_sign_hash: responseSignHash,
    finalize_sign_hash: finalizeSignHash,
    context_hash: contextHashHex,
    initiator_signature: {
      signer: ALICE,
      algorithm: "secp256k1",
      signature: finalizeInitiatorSignatureHex,
    },
    responder_signature: {
      signer: BOB,
      algorithm: "secp256k1",
      signature: finalizeResponderSignatureHex,
    },
  };

  const finalizeRes = await requestJson<PublishFinalizeResponse>({
    method: "POST",
    path: "/v1/finalize",
    token: ALICE_TOKEN,
    body: finalizeReq,
  });
  assertStatus(finalizeRes.status, 201, "publish finalize");
  assert.equal(finalizeRes.data.pool_id, finalizeReq.pool_id);
  assert.equal(finalizeRes.data.intent_id, finalizeReq.intent_id);
  assert.equal(finalizeRes.data.response_id, finalizeReq.response_id);
  assert.equal(finalizeRes.data.finalize_id, finalizeReq.finalize_id);
  assert.equal(finalizeRes.data.finalize_sign_hash, finalizeSignHash);

  const aliceInbox = await requestJson<ListRecordsResponse>({
    method: "GET",
    path: "/v1/inbox",
    token: ALICE_TOKEN,
  });
  assertStatus(aliceInbox.status, 200, "alice inbox");
  assert.equal(aliceInbox.data.principal, ALICE);
  assert.ok(aliceInbox.data.records.length >= 1, `alice inbox should include response: ${aliceInbox.rawText}`);
  assert.ok(aliceInbox.data.records.some((r) => r.record_type === "response" && r.response_sign_hash === responseSignHash));

  const aliceOutbox = await requestJson<ListRecordsResponse>({
    method: "GET",
    path: "/v1/outbox",
    token: ALICE_TOKEN,
  });
  assertStatus(aliceOutbox.status, 200, "alice outbox");
  assert.equal(aliceOutbox.data.principal, ALICE);
  assert.ok(aliceOutbox.data.records.some((r) => r.record_type === "intent" && r.intent_sign_hash === intentSignHash));
  assert.ok(aliceOutbox.data.records.some((r) => r.record_type === "finalize" && r.finalize_sign_hash === finalizeSignHash));

  const bobInbox = await requestJson<ListRecordsResponse>({
    method: "GET",
    path: "/v1/inbox",
    token: BOB_TOKEN,
  });
  assertStatus(bobInbox.status, 200, "bob inbox");
  assert.equal(bobInbox.data.principal, BOB);
  assert.ok(bobInbox.data.records.some((r) => r.record_type === "intent"));
  assert.ok(bobInbox.data.records.some((r) => r.record_type === "finalize"));

  const bobOutbox = await requestJson<ListRecordsResponse>({
    method: "GET",
    path: "/v1/outbox",
    token: BOB_TOKEN,
  });
  assertStatus(bobOutbox.status, 200, "bob outbox");
  assert.equal(bobOutbox.data.principal, BOB);
  assert.ok(bobOutbox.data.records.some((r) => r.record_type === "response" && r.response_sign_hash === responseSignHash));

  const certificateBytes = encodeMatchCertificate({
    payload: bundle.certificatePayload,
    intent: {
      payload: bundle.intentPayload,
      signature: {
        signer: ALICE,
        algorithm: SIGNATURE_ALGO_SECP256K1,
        signature: hexToBytes(intentSignatureHex),
      },
      signBytesHash: bundle.intentHash,
    },
    response: {
      payload: bundle.responsePayload,
      signature: {
        signer: BOB,
        algorithm: SIGNATURE_ALGO_SECP256K1,
        signature: hexToBytes(responseSignatureHex),
      },
      signBytesHash: bundle.responseHash,
    },
    finalize: {
      payload: bundle.finalizePayload,
      initiatorSignature: {
        signer: ALICE,
        algorithm: SIGNATURE_ALGO_SECP256K1,
        signature: hexToBytes(finalizeInitiatorSignatureHex),
      },
      responderSignature: {
        signer: BOB,
        algorithm: SIGNATURE_ALGO_SECP256K1,
        signature: hexToBytes(finalizeResponderSignatureHex),
      },
      signBytesHash: bundle.finalizeHash,
    },
    boardSignature: {
      signer: ALICE,
      algorithm: SIGNATURE_ALGO_SECP256K1,
      signature: hexToBytes(boardSignatureHex),
    },
    signBytesHash: bundle.certificateHash,
  });

  logInfo("submitting certificate on-chain via match precompile", {
    precompile_address: MATCH_PRECOMPILE_ADDRESS,
    pool_id: poolId,
    intent_id: intentId,
    response_id: responseId,
    finalize_id: finalizeId,
    certificate_id: certificateId,
    certificate_sign_hash: certificateSignHash,
    certificate_bytes_len: certificateBytes.length,
  });
  const provider = new JsonRpcProvider(EVM_RPC_URL);
  const submitter = new Wallet(ALICE_PRIVATE_KEY, provider);
  const writablePrecompile = new Contract(MATCH_PRECOMPILE_ADDRESS, MATCH_PRECOMPILE_ABI, submitter);
  const submitTx = await writablePrecompile.submitMatchCertificate(certificateBytes, { gasLimit: 1_500_000 });
  const submitReceipt = await submitTx.wait();
  assert.ok(submitReceipt, "submitMatchCertificate transaction receipt is missing");
  assert.equal(submitReceipt.status, 1, `submitMatchCertificate reverted: tx=${submitTx.hash}`);
  logInfo("on-chain submit tx committed", {
    tx_hash: submitTx.hash,
    block_number: submitReceipt.blockNumber,
    gas_used: submitReceipt.gasUsed.toString(),
  });

  const replay = await queryMatchPrecompile(poolId, intentId);
  if (!replay.available) {
    if (MATCH_REQUIRE_PRECOMPILE || MATCH_EXPECT_ONCHAIN_REPLAY) {
      assert.fail(
        `match precompile ${MATCH_PRECOMPILE_ADDRESS} is not available on current node; ` +
          "enable precompile or unset MATCH_REQUIRE_PRECOMPILE/MATCH_EXPECT_ONCHAIN_REPLAY",
      );
    }
    logInfo("skipping on-chain replay assertion because precompile is unavailable");
  } else {
    assert.equal(replay.exists, replay.found, "hasReplay/getReplay result mismatch");
    if (MATCH_EXPECT_ONCHAIN_REPLAY) {
      assert.equal(replay.found, true, "on-chain replay expected but not found");
      assert.notEqual(replay.matchId.trim(), "", "on-chain matchId should not be empty");
      if (replay.partiesAvailable) {
        assert.notEqual(replay.requester.trim(), "", "on-chain requester should not be empty");
        assert.notEqual(replay.responder.trim(), "", "on-chain responder should not be empty");
        assert.equal(getAddress(replay.requester), getAddress(ALICE), "on-chain requester mismatch");
        assert.equal(getAddress(replay.responder), getAddress(BOB), "on-chain responder mismatch");
      }
    } else {
      logInfo(
        "on-chain replay enforcement is disabled (set MATCH_EXPECT_ONCHAIN_REPLAY=1 to require replay existence)",
        replay,
      );
    }
  }

  logInfo("match e2e test finished successfully");
  console.log("match e2e test passed");
}

main().catch((err) => {
  console.error("match e2e test failed", err);
  process.exit(1);
});
