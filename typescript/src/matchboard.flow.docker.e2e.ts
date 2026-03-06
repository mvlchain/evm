import { spawn } from "node:child_process";
import { fileURLToPath } from "node:url";
import path from "node:path";

type BoardRecord = {
  record_type: string;
  pool_id: string;
  intent_id: string;
  response_id?: string;
  finalize_id?: string;
};

type ListRecordsResponse = {
  principal: string;
  records: BoardRecord[];
  total: number;
};

type MatchCandidate = {
  match_id: string;
  pool_id: string;
  intent_id: string;
  response_id: string;
  requester: string;
  responder: string;
  score_hash: string;
  expiry_unix: number;
};

type ListMatchCandidatesResponse = {
  matcher: string;
  candidates: MatchCandidate[];
  total: number;
};

type ListProposerMatchesResponse = {
  proposer: string;
  matches: MatchCandidate[];
  canonical_match_batch_hash: string;
  total_pending: number;
};

type BuildProposerMatchesResponse = {
  proposer: string;
  submitter: string;
  canonical_build_hash: string;
  require_certificate: boolean;
  items: Array<{
    match_id: string;
    pool_id: string;
    intent_id: string;
    response_id: string;
    finalize_id?: string;
    has_match_certificate: boolean;
    msg_submit_match_tx_payload?: string;
    msg_payload_hash?: string;
  }>;
};

type ErrorEnvelope = {
  error?: {
    code?: string;
    message?: string;
    detail?: string;
  };
};

type IntentStreamEvent = {
  event_id: string;
  intent_type: string;
  pool_id: string;
  intent_id: string;
  response_id?: string;
  finalize_id?: string;
  requester: string;
  responder: string;
  expiry_unix: number;
  created_unix: number;
  intent_sign_hash?: string;
  response_sign_hash?: string;
  finalize_sign_hash?: string;
};

const THIS_DIR = path.dirname(fileURLToPath(import.meta.url));
const TYPESCRIPT_DIR = path.resolve(THIS_DIR, "..");
const COMPOSE_FILE = path.join(TYPESCRIPT_DIR, "docker-compose.matchboard-gossip.yml");
const PROJECT = process.env.MATCH_DOCKER_PROJECT ?? `match-flow-${Date.now()}`;
const KEEP = process.env.MATCH_DOCKER_KEEP === "1";
const SHOULD_BUILD = process.env.MATCH_DOCKER_BUILD !== "0";

const NODE_A = "http://127.0.0.1:28080";
const NODE_B = "http://127.0.0.1:28081";
const TOKEN_ALICE = "token-alice";
const TOKEN_BOB = "token-bob";

const HASH_A = "a".repeat(64);
const HASH_B = "b".repeat(64);
const HASH_C = "c".repeat(64);
const HASH_D = "d".repeat(64);

async function main(): Promise<void> {
  const suffix = `${Date.now()}-${Math.floor(Math.random() * 1_000_000)}`;
  const poolId = `pool-flow-${suffix}`;
  const intentId = `intent-flow-${suffix}`;
  const responseId = `response-flow-${suffix}`;
  const finalizeId = `finalize-flow-${suffix}`;

  try {
    console.log(`ℹ️ compose project=${PROJECT} build=${SHOULD_BUILD ? "on" : "off"} image=${process.env.EVMD_GOSSIP_IMAGE ?? "evmd-gossip:local"}`);
    const upArgs = ["up", "-d", "--remove-orphans"];
    if (SHOULD_BUILD) {
      upArgs.push("--build");
    }
    await compose(upArgs);

    await waitForHealthy(`${NODE_A}/healthz`, "node-a");
    await waitForHealthy(`${NODE_B}/healthz`, "node-b");

    // internal gossip auth must reject unauthenticated relay.
    {
      const unauthorized = await fetch(`${NODE_B}/v1/internal/gossip/intents`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({}),
      });
      if (unauthorized.status !== 401) {
        throw new Error(`expected gossip auth 401, got ${unauthorized.status}`);
      }
    }

    // Start SSE subscription before posting the request intent.
    const sseIntentPromise = waitForSSEIntentEvent(
      `${NODE_B}/v1/stream/intents?intent_type=request&responder=bob`,
      TOKEN_BOB,
      (evt) => evt.pool_id === poolId && evt.intent_id === intentId && evt.intent_type === "request",
      "request SSE on node-b",
    );

    await post(`${NODE_A}/v1/intents`, TOKEN_ALICE, {
      pool_id: poolId,
      intent_id: intentId,
      sender: "alice",
      recipient: "bob",
      expires_unix: 1_893_456_000,
      digest_algorithm: "sha256",
      intent_sign_hash: HASH_A,
      context_hash: HASH_B,
      signature: {
        signer: "alice",
        algorithm: "secp256k1",
        signature: "dummy-intent-signature",
      },
    }, 201);

    await sseIntentPromise;

    await waitForRecord(
      () => getInbox(NODE_B, "bob", TOKEN_BOB),
      (r) => r.record_type === "intent" && r.pool_id === poolId && r.intent_id === intentId,
      "gossiped intent on node-b inbox(bob)",
    );

    await post(`${NODE_A}/v1/responses`, TOKEN_BOB, {
      pool_id: poolId,
      intent_id: intentId,
      response_id: responseId,
      sender: "bob",
      recipient: "alice",
      expires_unix: 1_893_456_000,
      digest_algorithm: "sha256",
      intent_sign_hash: HASH_A,
      response_sign_hash: HASH_C,
      context_hash: HASH_B,
      signature: {
        signer: "bob",
        algorithm: "secp256k1",
        signature: "dummy-response-signature",
      },
    }, 201);

    await waitForRecord(
      () => getInbox(NODE_B, "alice", TOKEN_ALICE),
      (r) =>
        r.record_type === "response" &&
        r.pool_id === poolId &&
        r.intent_id === intentId &&
        r.response_id === responseId,
      "gossiped response on node-b inbox(alice)",
    );

    const matcherBeforeFinalize = await getMatchCandidates(NODE_B, TOKEN_ALICE);
    const preFinalizeCandidate = matcherBeforeFinalize.candidates.find(
      (c) => c.pool_id === poolId && c.intent_id === intentId && c.response_id === responseId,
    );
    if (!preFinalizeCandidate) {
      throw new Error("expected matcher candidate before finalize");
    }

    const proposerBeforeFinalize = await getProposerMatches(NODE_B, TOKEN_ALICE);
    if (!proposerBeforeFinalize.matches.some((m) => m.match_id === preFinalizeCandidate.match_id)) {
      throw new Error("expected proposer match before finalize");
    }

    const builtBeforeFinalize = await postJSON<BuildProposerMatchesResponse>(
      `${NODE_B}/v1/proposer/matches/build`,
      TOKEN_ALICE,
      {
        match_ids: [preFinalizeCandidate.match_id],
        require_certificate: false,
      },
      200,
    );
    if (builtBeforeFinalize.items.length !== 1 || builtBeforeFinalize.items[0]?.has_match_certificate) {
      throw new Error("expected build without certificate before finalize");
    }

    await post(`${NODE_A}/v1/finalize`, TOKEN_ALICE, {
      pool_id: poolId,
      intent_id: intentId,
      response_id: responseId,
      finalize_id: finalizeId,
      sender: "alice",
      recipient: "bob",
      expires_unix: 1_893_456_000,
      digest_algorithm: "sha256",
      intent_sign_hash: HASH_A,
      response_sign_hash: HASH_C,
      finalize_sign_hash: HASH_D,
      context_hash: HASH_B,
      initiator_signature: {
        signer: "alice",
        algorithm: "secp256k1",
        signature: "dummy-finalize-initiator-signature",
      },
      responder_signature: {
        signer: "bob",
        algorithm: "secp256k1",
        signature: "dummy-finalize-responder-signature",
      },
    }, 201);

    await waitForRecord(
      () => getInbox(NODE_B, "bob", TOKEN_BOB),
      (r) =>
        r.record_type === "finalize" &&
        r.pool_id === poolId &&
        r.intent_id === intentId &&
        r.response_id === responseId &&
        r.finalize_id === finalizeId,
      "gossiped finalize on node-b inbox(bob)",
    );

    // finalize exists but match_certificate is missing => require_certificate=true should fail.
    const buildStrictResp = await postRaw(`${NODE_B}/v1/proposer/matches/build`, TOKEN_ALICE, {
      match_ids: [preFinalizeCandidate.match_id],
      require_certificate: true,
    });
    if (buildStrictResp.status !== 409) {
      throw new Error(`expected strict build to fail with 409, got ${buildStrictResp.status}`);
    }
    const strictErr = parseErrorEnvelope(buildStrictResp.payload);
    if (strictErr?.error?.code !== "ERROR_CODE_STATE_CONFLICT") {
      throw new Error(`unexpected strict build error payload: ${buildStrictResp.payload}`);
    }

    const builtAfterFinalize = await postJSON<BuildProposerMatchesResponse>(
      `${NODE_B}/v1/proposer/matches/build`,
      TOKEN_ALICE,
      {
        match_ids: [preFinalizeCandidate.match_id],
        require_certificate: false,
      },
      200,
    );
    if (builtAfterFinalize.items.length !== 1 || builtAfterFinalize.items[0]?.finalize_id !== finalizeId) {
      throw new Error("expected build item with finalize_id after finalize");
    }

    // atomic rollback: one missing ID should reject the whole commit.
    const rollbackResp = await postRaw(`${NODE_B}/v1/proposer/matches/commit`, TOKEN_ALICE, {
      match_ids: [preFinalizeCandidate.match_id, "missing-match-id"],
    });
    if (rollbackResp.status !== 409) {
      throw new Error(`expected rollback commit status=409, got ${rollbackResp.status}`);
    }

    const proposerAfterRollback = await getProposerMatches(NODE_B, TOKEN_ALICE);
    if (!proposerAfterRollback.matches.some((m) => m.match_id === preFinalizeCandidate.match_id)) {
      throw new Error("expected proposer match to remain after rollback");
    }

    await post(`${NODE_B}/v1/proposer/matches/commit`, TOKEN_ALICE, {
      match_ids: [preFinalizeCandidate.match_id],
    }, 200);

    await waitForCondition(
      async () => {
        const listed = await getProposerMatches(NODE_B, TOKEN_ALICE);
        return listed.total_pending === 0;
      },
      20_000,
      400,
      "wait proposer matches to become empty",
    );

    console.log("✅ docker flow e2e passed");
    console.log(`project=${PROJECT}`);
    console.log(`pool_id=${poolId}`);
    console.log(`intent_id=${intentId}`);
    console.log(`response_id=${responseId}`);
    console.log(`finalize_id=${finalizeId}`);
    console.log(`match_id=${preFinalizeCandidate.match_id}`);
  } finally {
    if (!KEEP) {
      await compose(["down", "-v", "--remove-orphans"]).catch((err: unknown) => {
        console.warn("⚠️ docker compose down failed:", err);
      });
    } else {
      console.log("ℹ️ MATCH_DOCKER_KEEP=1, containers left running");
      console.log(`   docker compose -p ${PROJECT} -f ${COMPOSE_FILE} down -v`);
    }
  }
}

async function waitForHealthy(url: string, name: string): Promise<void> {
  await waitForCondition(
    async () => {
      try {
        const resp = await fetch(url);
        return resp.ok;
      } catch {
        return false;
      }
    },
    180_000,
    800,
    `wait for ${name} healthz`,
  );
}

async function waitForRecord(
  fetchInbox: () => Promise<ListRecordsResponse>,
  match: (record: BoardRecord) => boolean,
  description: string,
): Promise<void> {
  await waitForCondition(
    async () => {
      const inbox = await fetchInbox();
      return inbox.records.some(match);
    },
    30_000,
    500,
    description,
  );
}

async function waitForCondition(
  check: () => Promise<boolean>,
  timeoutMs: number,
  intervalMs: number,
  description: string,
): Promise<void> {
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    try {
      if (await check()) {
        return;
      }
    } catch {
      // retry
    }
    await sleep(intervalMs);
  }
  throw new Error(`timeout: ${description}`);
}

async function waitForSSEIntentEvent(
  url: string,
  token: string,
  match: (event: IntentStreamEvent) => boolean,
  description: string,
): Promise<IntentStreamEvent> {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 30_000);
  try {
    const resp = await fetch(url, {
      method: "GET",
      headers: {
        authorization: `Bearer ${token}`,
      },
      signal: controller.signal,
    });
    if (!resp.ok || !resp.body) {
      throw new Error(`SSE request failed status=${resp.status}`);
    }

    const reader = resp.body.getReader();
    const decoder = new TextDecoder();
    let buf = "";

    while (true) {
      const { value, done } = await reader.read();
      if (done) {
        break;
      }
      buf += decoder.decode(value, { stream: true });
      let nl = buf.indexOf("\n");
      while (nl >= 0) {
        const line = buf.slice(0, nl).trimEnd();
        buf = buf.slice(nl + 1);
        if (line.startsWith("data: ")) {
          const raw = line.slice("data: ".length);
          let parsed: IntentStreamEvent;
          try {
            parsed = JSON.parse(raw) as IntentStreamEvent;
          } catch {
            nl = buf.indexOf("\n");
            continue;
          }
          if (match(parsed)) {
            return parsed;
          }
        }
        nl = buf.indexOf("\n");
      }
    }

    throw new Error(`SSE stream closed before ${description}`);
  } catch (err: unknown) {
    if ((err as { name?: string }).name === "AbortError") {
      throw new Error(`timeout waiting for ${description}`);
    }
    throw err;
  } finally {
    clearTimeout(timeout);
    controller.abort();
  }
}

async function getInbox(base: string, principal: string, token: string): Promise<ListRecordsResponse> {
  const resp = await fetch(`${base}/v1/inbox?recipient=${encodeURIComponent(principal)}`, {
    headers: { authorization: `Bearer ${token}` },
  });
  const payload = await resp.text();
  if (!resp.ok) {
    throw new Error(`inbox request failed status=${resp.status} body=${payload}`);
  }
  return JSON.parse(payload) as ListRecordsResponse;
}

async function getMatchCandidates(base: string, token: string): Promise<ListMatchCandidatesResponse> {
  const resp = await fetch(`${base}/v1/matcher/candidates?limit=50`, {
    headers: { authorization: `Bearer ${token}` },
  });
  const payload = await resp.text();
  if (!resp.ok) {
    throw new Error(`matcher candidates request failed status=${resp.status} body=${payload}`);
  }
  return JSON.parse(payload) as ListMatchCandidatesResponse;
}

async function getProposerMatches(base: string, token: string): Promise<ListProposerMatchesResponse> {
  const resp = await fetch(`${base}/v1/proposer/matches?limit=50`, {
    headers: { authorization: `Bearer ${token}` },
  });
  const payload = await resp.text();
  if (!resp.ok) {
    throw new Error(`proposer matches request failed status=${resp.status} body=${payload}`);
  }
  return JSON.parse(payload) as ListProposerMatchesResponse;
}

async function post(url: string, token: string, body: unknown, expectedStatus: number): Promise<void> {
  const resp = await fetch(url, {
    method: "POST",
    headers: {
      authorization: `Bearer ${token}`,
      "content-type": "application/json",
    },
    body: JSON.stringify(body),
  });
  const payload = await resp.text();
  if (resp.status !== expectedStatus) {
    throw new Error(`post failed url=${url} status=${resp.status} body=${payload}`);
  }
}

async function postJSON<T>(url: string, token: string, body: unknown, expectedStatus: number): Promise<T> {
  const raw = await postRaw(url, token, body);
  if (raw.status !== expectedStatus) {
    throw new Error(`post failed url=${url} status=${raw.status} body=${raw.payload}`);
  }
  return JSON.parse(raw.payload) as T;
}

async function postRaw(url: string, token: string, body: unknown): Promise<{ status: number; payload: string }> {
  const resp = await fetch(url, {
    method: "POST",
    headers: {
      authorization: `Bearer ${token}`,
      "content-type": "application/json",
    },
    body: JSON.stringify(body),
  });
  return { status: resp.status, payload: await resp.text() };
}

function parseErrorEnvelope(raw: string): ErrorEnvelope | null {
  try {
    return JSON.parse(raw) as ErrorEnvelope;
  } catch {
    return null;
  }
}

function compose(args: string[]): Promise<void> {
  return run("docker", ["compose", "-p", PROJECT, "-f", COMPOSE_FILE, ...args], TYPESCRIPT_DIR);
}

function run(cmd: string, args: string[], cwd: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const child = spawn(cmd, args, {
      cwd,
      stdio: "inherit",
      env: process.env,
    });
    child.on("error", reject);
    child.on("exit", (code) => {
      if (code === 0) {
        resolve();
        return;
      }
      reject(new Error(`${cmd} ${args.join(" ")} exited with code ${String(code)}`));
    });
  });
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

void main().catch((err: unknown) => {
  console.error("❌ docker flow e2e failed");
  console.error(err);
  process.exitCode = 1;
});
