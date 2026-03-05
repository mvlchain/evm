import assert from "node:assert/strict";
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
const MATCH_PRECOMPILE_ADDRESS = mustEnv(
  "MATCH_PRECOMPILE_ADDRESS",
  "0x0000000000000000000000000000000000000808",
);
const MATCH_EXPECT_ONCHAIN_REPLAY = mustEnv("MATCH_EXPECT_ONCHAIN_REPLAY", "0") === "1";
const MATCH_REQUIRE_PRECOMPILE = "1";
const ALICE_TOKEN = mustEnv("MATCHBOARD_TOKEN_ALICE", "token-alice");
const BOB_TOKEN = mustEnv("MATCHBOARD_TOKEN_BOB", "token-bob");
const ALICE = mustEnv("MATCHBOARD_PRINCIPAL_ALICE", "alice");
const BOB = mustEnv("MATCHBOARD_PRINCIPAL_BOB", "bob");
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
      `failed to connect to matchboard at ${MATCHBOARD_URL} (start server: go run ./server/matchboard/cmd/matchboardd)`,
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
  assertPrincipalPrivateKeyMatch(ALICE, ALICE_PRIVATE_KEY, "alice");
  assertPrincipalPrivateKeyMatch(BOB, BOB_PRIVATE_KEY, "bob");
  logInfo("principal/private-key checks passed");

  const now = Math.floor(Date.now() / 1000);
  const expiresUnix = now + 600;
  const runId = `${now}-${Math.floor(Math.random() * 1_000_000)}`;
  logInfo("test run identifiers", { runId, expiresUnix });

  const intentSignHash = testHash("1");
  const responseSignHash = testHash("2");
  const finalizeSignHash = testHash("3");
  const intentContextHash = testHash("a");
  const responseContextHash = testHash("b");
  const finalizeContextHash = testHash("c");

  const intentReq: PublishIntentRequest = {
    pool_id: `pool-ts-${runId}`,
    intent_id: `intent-ts-${runId}`,
    sender: ALICE,
    recipient: BOB,
    expires_unix: expiresUnix,
    digest_algorithm: "sha256",
    intent_sign_hash: intentSignHash,
    context_hash: intentContextHash,
    signature: {
      signer: ALICE,
      algorithm: "secp256k1",
      signature: signHash(intentSignHash, ALICE_PRIVATE_KEY),
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
    pool_id: intentReq.pool_id,
    intent_id: intentReq.intent_id,
    response_id: `response-ts-${runId}`,
    sender: BOB,
    recipient: ALICE,
    expires_unix: expiresUnix,
    digest_algorithm: "sha256",
    intent_sign_hash: intentSignHash,
    response_sign_hash: responseSignHash,
    context_hash: responseContextHash,
    signature: {
      signer: BOB,
      algorithm: "secp256k1",
      signature: signHash(responseSignHash, BOB_PRIVATE_KEY),
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
    pool_id: intentReq.pool_id,
    intent_id: intentReq.intent_id,
    response_id: responseReq.response_id,
    finalize_id: `finalize-ts-${runId}`,
    sender: ALICE,
    recipient: BOB,
    expires_unix: expiresUnix,
    digest_algorithm: "sha256",
    intent_sign_hash: intentSignHash,
    response_sign_hash: responseSignHash,
    finalize_sign_hash: finalizeSignHash,
    context_hash: finalizeContextHash,
    initiator_signature: {
      signer: ALICE,
      algorithm: "secp256k1",
      signature: signHash(finalizeSignHash, ALICE_PRIVATE_KEY),
    },
    responder_signature: {
      signer: BOB,
      algorithm: "secp256k1",
      signature: signHash(finalizeSignHash, BOB_PRIVATE_KEY),
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

  const replay = await queryMatchPrecompile(intentReq.pool_id, intentReq.intent_id);
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
