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

type InboxResponse = {
  principal: string;
  records: BoardRecord[];
  total: number;
};

const THIS_DIR = path.dirname(fileURLToPath(import.meta.url));
const TYPESCRIPT_DIR = path.resolve(THIS_DIR, "..");
const COMPOSE_FILE = path.join(TYPESCRIPT_DIR, "docker-compose.matchboard-gossip.yml");
const PROJECT = process.env.MATCH_DOCKER_PROJECT ?? `match-gossip-${Date.now()}`;
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
  const poolId = `pool-gossip-${suffix}`;
  const intentId = `intent-gossip-${suffix}`;
  const responseId = `response-gossip-${suffix}`;
  const finalizeId = `finalize-gossip-${suffix}`;

  try {
    console.log(`ℹ️ compose project=${PROJECT} build=${SHOULD_BUILD ? "on" : "off"} image=${process.env.EVMD_GOSSIP_IMAGE ?? "evmd-gossip:local"}`);
    const upArgs = ["up", "-d", "--remove-orphans"];
    if (SHOULD_BUILD) {
      upArgs.push("--build");
    }
    await compose(upArgs);

    await waitForHealthy(`${NODE_A}/healthz`, "node-a");
    await waitForHealthy(`${NODE_B}/healthz`, "node-b");

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

    console.log("✅ docker gossip e2e passed");
    console.log(`project=${PROJECT}`);
    console.log(`pool_id=${poolId}`);
    console.log(`intent_id=${intentId}`);
    console.log(`response_id=${responseId}`);
    console.log(`finalize_id=${finalizeId}`);
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
  const timeoutMs = 180_000;
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    try {
      const resp = await fetch(url);
      if (resp.ok) {
        return;
      }
    } catch {
      // retry
    }
    await sleep(800);
  }
  throw new Error(`timeout waiting for ${name} healthz: ${url}`);
}

async function waitForRecord(
  fetchInbox: () => Promise<InboxResponse>,
  match: (record: BoardRecord) => boolean,
  description: string,
): Promise<void> {
  const timeoutMs = 30_000;
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    try {
      const inbox = await fetchInbox();
      if (inbox.records.some(match)) {
        return;
      }
    } catch {
      // retry
    }
    await sleep(500);
  }
  throw new Error(`timeout waiting for ${description}`);
}

async function getInbox(base: string, principal: string, token: string): Promise<InboxResponse> {
  const resp = await fetch(`${base}/v1/inbox?recipient=${encodeURIComponent(principal)}`, {
    headers: { authorization: `Bearer ${token}` },
  });
  const payload = await resp.text();
  if (!resp.ok) {
    throw new Error(`inbox request failed status=${resp.status} body=${payload}`);
  }
  return JSON.parse(payload) as InboxResponse;
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
  console.error("❌ docker gossip e2e failed");
  console.error(err);
  process.exitCode = 1;
});
