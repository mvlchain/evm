import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { Wallet } from "ethers";

const execFileAsync = promisify(execFile);

const DEFAULT_TARGETS = [50, 100, 500, 1000];
const DEFAULT_DURATION_SEC = 30;
const DEFAULT_SAMPLE_INTERVAL_MS = 1000;

const DEFAULT_BASE_URLS = [
  "http://127.0.0.1:28080",
  "http://127.0.0.1:28081",
  "http://127.0.0.1:28082",
  "http://127.0.0.1:28083",
];

const DEFAULT_CONTAINERS = ["evmdhw0", "evmdhw1", "evmdhw2", "evmdhw3"];

const DEFAULT_TOKEN_ALICE = "token-alice";
const DEFAULT_TOKEN_BOB = "token-bob";

const DEFAULT_SENDER = "0xC6Fe5D33615a1C52c08018c47E8Bc53646A0E101";
const DEFAULT_RECIPIENT = "0x963EBDf2e1f8DB8707D05FC75bfeFFBa1B5BaC17";

const DEFAULT_ALICE_PRIVATE_KEY =
  "0x88cbead91aee890d27bf06e003ade3d4e952427e88f88d31d61d3ef5e5d54305";
const DEFAULT_BOB_PRIVATE_KEY =
  "0x741de4f8988ea941d3ff0287911ca4074e62b7d45c991a51186455366f10b544";

const DEFAULT_INTENT_HASH = "a".repeat(64);
const DEFAULT_CONTEXT_HASH = "b".repeat(64);
const DEFAULT_RESPONSE_HASH = "c".repeat(64);
const DEFAULT_FINALIZE_HASH = "d".repeat(64);

const DEFAULT_EXPIRES_UNIX = 1_893_456_000; // 2030-01-01

const THIS_DIR = path.dirname(fileURLToPath(import.meta.url));
const TYPESCRIPT_DIR = path.resolve(THIS_DIR, "..");

type CliOptions = {
  targets: number[];
  durationSec: number;
  sampleIntervalMs: number;
  workersOverride?: number;
  outputPath?: string;
};

type BenchConfig = {
  baseUrls: string[];
  containers: string[];
  tokenAlice: string;
  tokenBob: string;
  sender: string;
  recipient: string;
  signatures: {
    intent: string;
    response: string;
    finalizeInitiator: string;
    finalizeResponder: string;
  };
};

type ContainerSnapshot = {
  cpuPercent: number;
  memMiB: number;
};

type ResourceSnapshot = {
  clusterCpuPercent: number;
  clusterMemMiB: number;
  perContainer: Record<string, ContainerSnapshot>;
};

type ScenarioResult = {
  targetTps: number;
  workers: number;
  durationSec: number;
  flowsAttempted: number;
  flowsOk: number;
  flowsFail: number;
  flowRatePerSec: number;
  httpRatePerSec: number;
  resource: {
    cpuAvgPercent: number;
    cpuP95Percent: number;
    cpuMaxPercent: number;
    cpuAvgVcpu: number;
    cpuP95Vcpu: number;
    cpuMaxVcpu: number;
    memAvgMiB: number;
    memP95MiB: number;
    memMaxMiB: number;
    memAvgGiB: number;
    memP95GiB: number;
    memMaxGiB: number;
    samples: number;
  };
  recommended: {
    vcpu: number;
    ramGiB: number;
    rule: string;
  };
  errors: Record<string, number>;
};

type ScenarioCounters = {
  nextFlowID: number;
  flowsAttempted: number;
  flowsOk: number;
  flowsFail: number;
  http201: number;
  errors: Map<string, number>;
};

async function main(): Promise<void> {
  const options = parseArgs(process.argv.slice(2));
  const cfg = buildBenchConfig();

  console.log("ℹ️ hardware benchmark config");
  console.log(
    JSON.stringify(
      {
        targets: options.targets,
        durationSec: options.durationSec,
        sampleIntervalMs: options.sampleIntervalMs,
        workersOverride: options.workersOverride ?? null,
        baseUrls: cfg.baseUrls,
        containers: cfg.containers,
        sender: cfg.sender,
        recipient: cfg.recipient,
      },
      null,
      2,
    ),
  );

  await ensureHealth(cfg.baseUrls);

  const results: ScenarioResult[] = [];
  for (const targetTps of options.targets) {
    const workers = options.workersOverride ?? autoWorkers(targetTps);
    const result = await runScenario(targetTps, workers, options.durationSec, options.sampleIntervalMs, cfg);
    results.push(result);
    console.log(
      `✅ target=${targetTps} observed=${result.flowRatePerSec.toFixed(2)} flow/s ` +
        `cpu_p95=${result.resource.cpuP95Vcpu.toFixed(2)} vCPU mem_p95=${result.resource.memP95GiB.toFixed(2)} GiB`,
    );
  }

  printMarkdownTable(results);

  const output = {
    generatedAt: new Date().toISOString(),
    options,
    config: {
      baseUrls: cfg.baseUrls,
      containers: cfg.containers,
      sender: cfg.sender,
      recipient: cfg.recipient,
    },
    results,
  };

  const outputPath = options.outputPath ?? path.join(TYPESCRIPT_DIR, `hardware-bench-${Date.now()}.json`);
  await writeFile(outputPath, JSON.stringify(output, null, 2), "utf-8");
  console.log(`\n📄 wrote benchmark result: ${outputPath}`);
}

function parseArgs(args: string[]): CliOptions {
  const out: CliOptions = {
    targets: parseNumberList(process.env.MATCH_HW_TARGETS) ?? [...DEFAULT_TARGETS],
    durationSec: parseIntWithDefault(process.env.MATCH_HW_DURATION_SEC, DEFAULT_DURATION_SEC),
    sampleIntervalMs: parseIntWithDefault(process.env.MATCH_HW_SAMPLE_MS, DEFAULT_SAMPLE_INTERVAL_MS),
    workersOverride: parseOptionalInt(process.env.MATCH_HW_WORKERS),
    outputPath: process.env.MATCH_HW_OUTPUT,
  };

  for (const arg of args) {
    if (arg.startsWith("--targets=")) {
      out.targets = parseTargetsStrict(arg.slice("--targets=".length));
      continue;
    }
    if (arg.startsWith("--duration=")) {
      out.durationSec = parsePositiveIntStrict(arg.slice("--duration=".length), "duration");
      continue;
    }
    if (arg.startsWith("--sample-ms=")) {
      out.sampleIntervalMs = parsePositiveIntStrict(arg.slice("--sample-ms=".length), "sample-ms");
      continue;
    }
    if (arg.startsWith("--workers=")) {
      out.workersOverride = parsePositiveIntStrict(arg.slice("--workers=".length), "workers");
      continue;
    }
    if (arg.startsWith("--output=")) {
      const value = arg.slice("--output=".length).trim();
      if (!value) {
        throw new Error("--output must not be empty");
      }
      out.outputPath = value;
      continue;
    }
    if (arg === "--help" || arg === "-h") {
      printHelpAndExit();
    }
    throw new Error(`unknown arg: ${arg}`);
  }

  if (out.targets.length === 0) {
    throw new Error("targets must not be empty");
  }

  return out;
}

function printHelpAndExit(): never {
  console.log(`
Usage:
  npm run bench:hardware -- --targets=50,100,500,1000 --duration=30

Options:
  --targets=LIST       target flow TPS list (default: 50,100,500,1000)
  --duration=SECONDS   duration per target (default: 30)
  --sample-ms=MILLIS   docker stats sampling interval (default: 1000)
  --workers=N          override workers (default: auto by target)
  --output=PATH        write JSON report path

Env overrides:
  MATCH_HW_BASE_URLS       e.g. http://127.0.0.1:28080,http://127.0.0.1:28081,...
  MATCH_HW_CONTAINERS      e.g. evmdhw0,evmdhw1,evmdhw2,evmdhw3
  MATCH_HW_TARGETS         same format as --targets
  MATCH_HW_DURATION_SEC    same as --duration
  MATCH_HW_SAMPLE_MS       same as --sample-ms
  MATCH_HW_WORKERS         same as --workers
  MATCH_HW_OUTPUT          same as --output

  MATCHBOARD_TOKEN_ALICE / MATCHBOARD_TOKEN_BOB
  MATCHBOARD_PRINCIPAL_ALICE / MATCHBOARD_PRINCIPAL_BOB
  MATCHBOARD_ALICE_PRIVATE_KEY / MATCHBOARD_BOB_PRIVATE_KEY
`.trim());
  process.exit(0);
}

function buildBenchConfig(): BenchConfig {
  const baseUrls = parseStringList(process.env.MATCH_HW_BASE_URLS) ?? [...DEFAULT_BASE_URLS];
  const containers = parseStringList(process.env.MATCH_HW_CONTAINERS) ?? [...DEFAULT_CONTAINERS];

  const sender = (process.env.MATCHBOARD_PRINCIPAL_ALICE ?? DEFAULT_SENDER).trim();
  const recipient = (process.env.MATCHBOARD_PRINCIPAL_BOB ?? DEFAULT_RECIPIENT).trim();
  const tokenAlice = (process.env.MATCHBOARD_TOKEN_ALICE ?? DEFAULT_TOKEN_ALICE).trim();
  const tokenBob = (process.env.MATCHBOARD_TOKEN_BOB ?? DEFAULT_TOKEN_BOB).trim();

  if (!sender || !recipient || !tokenAlice || !tokenBob) {
    throw new Error("sender/recipient/token must not be empty");
  }

  const signatures = resolveSignatures(sender, recipient);

  return {
    baseUrls,
    containers,
    tokenAlice,
    tokenBob,
    sender,
    recipient,
    signatures,
  };
}

function resolveSignatures(
  sender: string,
  recipient: string,
): BenchConfig["signatures"] {
  if (isHexAddress(sender) && isHexAddress(recipient)) {
    const alicePrivateKey = (process.env.MATCHBOARD_ALICE_PRIVATE_KEY ?? DEFAULT_ALICE_PRIVATE_KEY).trim();
    const bobPrivateKey = (process.env.MATCHBOARD_BOB_PRIVATE_KEY ?? DEFAULT_BOB_PRIVATE_KEY).trim();

    const aliceWallet = new Wallet(alicePrivateKey);
    const bobWallet = new Wallet(bobPrivateKey);

    if (!equalsHexAddress(aliceWallet.address, sender)) {
      throw new Error(
        `alice private key does not match sender principal: key=${aliceWallet.address} sender=${sender}`,
      );
    }
    if (!equalsHexAddress(bobWallet.address, recipient)) {
      throw new Error(
        `bob private key does not match recipient principal: key=${bobWallet.address} recipient=${recipient}`,
      );
    }

    return {
      intent: aliceWallet.signingKey.sign(toHexHash(DEFAULT_INTENT_HASH)).serialized,
      response: bobWallet.signingKey.sign(toHexHash(DEFAULT_RESPONSE_HASH)).serialized,
      finalizeInitiator: aliceWallet.signingKey.sign(toHexHash(DEFAULT_FINALIZE_HASH)).serialized,
      finalizeResponder: bobWallet.signingKey.sign(toHexHash(DEFAULT_FINALIZE_HASH)).serialized,
    };
  }

  return {
    intent: "dummy-intent-signature",
    response: "dummy-response-signature",
    finalizeInitiator: "dummy-finalize-initiator-signature",
    finalizeResponder: "dummy-finalize-responder-signature",
  };
}

async function ensureHealth(baseUrls: string[]): Promise<void> {
  const timeoutMs = Number.parseInt(process.env.MATCH_HW_HEALTH_TIMEOUT_MS ?? "60000", 10);

  for (const base of baseUrls) {
    const url = `${base}/healthz`;
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
      try {
        const resp = await fetch(url);
        if (resp.ok) {
          break;
        }
      } catch {
        // retry until timeout
      }
      await sleep(800);
    }

    try {
      const verify = await fetch(url);
      if (!verify.ok) {
        const body = await verify.text();
        throw new Error(`health check failed url=${url} status=${verify.status} body=${body}`);
      }
    } catch (err) {
      const message = [
        `health check failed for ${url} within ${timeoutMs}ms`,
        `- running URLs: ${baseUrls.join(", ")}`,
        `- hint: start benchmark nodes first:`,
        `  cd typescript`,
        `  npm run bench:hardware:up`,
        `- or override base URLs/containers with MATCH_HW_BASE_URLS, MATCH_HW_CONTAINERS`,
        `- cause: ${String(err)}`,
      ].join("\n");
      throw new Error(message);
    }
  }
}

function autoWorkers(targetTps: number): number {
  return Math.max(4, Math.min(96, Math.ceil(targetTps / 30)));
}

async function runScenario(
  targetTps: number,
  workers: number,
  durationSec: number,
  sampleIntervalMs: number,
  cfg: BenchConfig,
): Promise<ScenarioResult> {
  const counters: ScenarioCounters = {
    nextFlowID: 0,
    flowsAttempted: 0,
    flowsOk: 0,
    flowsFail: 0,
    http201: 0,
    errors: new Map<string, number>(),
  };

  const resourceSamples: ResourceSnapshot[] = [];
  let stopSampler = false;

  const sampler = (async () => {
    while (!stopSampler) {
      try {
        const snap = await collectResourceSnapshot(cfg.containers);
        resourceSamples.push(snap);
      } catch (err) {
        // keep running; startup race or transient docker stats failure shouldn't abort run.
        console.warn(`⚠️ docker stats sample failed: ${String(err)}`);
      }
      await sleep(sampleIntervalMs);
    }
  })();

  const scenarioID = `${Date.now()}-${targetTps}`;
  const startedAt = Date.now();
  const stopAt = startedAt + durationSec * 1000;

  const workerTasks = Array.from({ length: workers }, (_, workerIndex) =>
    workerLoop(workerIndex, targetTps, workers, stopAt, scenarioID, counters, cfg),
  );

  await Promise.all(workerTasks);
  const endedAt = Date.now();

  stopSampler = true;
  await sampler;

  try {
    resourceSamples.push(await collectResourceSnapshot(cfg.containers));
  } catch {
    // ignore final sample failure
  }

  const durationActualSec = Math.max(1e-9, (endedAt - startedAt) / 1000);
  const clusterCpuSeries = resourceSamples.map((s) => s.clusterCpuPercent);
  const clusterMemSeries = resourceSamples.map((s) => s.clusterMemMiB);

  const cpuAvgPercent = mean(clusterCpuSeries);
  const cpuP95Percent = percentile(clusterCpuSeries, 95);
  const cpuMaxPercent = maxOrZero(clusterCpuSeries);

  const memAvgMiB = mean(clusterMemSeries);
  const memP95MiB = percentile(clusterMemSeries, 95);
  const memMaxMiB = maxOrZero(clusterMemSeries);

  const cpuP95Vcpu = cpuP95Percent / 100;
  const recommendedVcpu = Math.max(4, Math.ceil(cpuP95Vcpu * 1.4 + 1));
  const recommendedRamGiB = Math.max(8, Math.ceil((memP95MiB * 1.5 + 1024) / 1024));

  return {
    targetTps,
    workers,
    durationSec,
    flowsAttempted: counters.flowsAttempted,
    flowsOk: counters.flowsOk,
    flowsFail: counters.flowsFail,
    flowRatePerSec: counters.flowsOk / durationActualSec,
    httpRatePerSec: counters.http201 / durationActualSec,
    resource: {
      cpuAvgPercent,
      cpuP95Percent,
      cpuMaxPercent,
      cpuAvgVcpu: cpuAvgPercent / 100,
      cpuP95Vcpu,
      cpuMaxVcpu: cpuMaxPercent / 100,
      memAvgMiB,
      memP95MiB,
      memMaxMiB,
      memAvgGiB: memAvgMiB / 1024,
      memP95GiB: memP95MiB / 1024,
      memMaxGiB: memMaxMiB / 1024,
      samples: resourceSamples.length,
    },
    recommended: {
      vcpu: recommendedVcpu,
      ramGiB: recommendedRamGiB,
      rule: "vcpu=max(4,ceil(cpu_p95_vcpu*1.4+1)); ram=max(8GiB,ceil((mem_p95*1.5+1GiB)/1GiB))",
    },
    errors: mapToObject(counters.errors),
  };
}

async function workerLoop(
  workerIndex: number,
  targetTps: number,
  totalWorkers: number,
  stopAtMs: number,
  scenarioID: string,
  counters: ScenarioCounters,
  cfg: BenchConfig,
): Promise<void> {
  const flowIntervalMs = (1000 * totalWorkers) / Math.max(1, targetTps);
  let nextRunAt = Date.now() + (flowIntervalMs * workerIndex) / totalWorkers;

  while (Date.now() < stopAtMs) {
    const waitMs = nextRunAt - Date.now();
    if (waitMs > 1) {
      await sleep(waitMs);
    }

    const flowID = counters.nextFlowID;
    counters.nextFlowID += 1;
    counters.flowsAttempted += 1;

    const ok = await executeFlow(flowID, scenarioID, cfg);
    if (ok.ok) {
      counters.flowsOk += 1;
      counters.http201 += 3;
    } else {
      counters.flowsFail += 1;
      for (const key of ok.errorKeys) {
        counters.errors.set(key, (counters.errors.get(key) ?? 0) + 1);
      }
    }

    nextRunAt += flowIntervalMs;
  }
}

async function executeFlow(
  flowID: number,
  scenarioID: string,
  cfg: BenchConfig,
): Promise<{ ok: true } | { ok: false; errorKeys: string[] }> {
  const base = cfg.baseUrls[flowID % cfg.baseUrls.length];
  if (!base) {
    return { ok: false, errorKeys: ["base_url:missing"] };
  }

  const poolID = `hw-${scenarioID}`;
  const intentID = `intent-${scenarioID}-${flowID}`;
  const responseID = `response-${scenarioID}-${flowID}`;
  const finalizeID = `finalize-${scenarioID}-${flowID}`;

  const intentReq = {
    pool_id: poolID,
    intent_id: intentID,
    sender: cfg.sender,
    recipient: cfg.recipient,
    expires_unix: DEFAULT_EXPIRES_UNIX,
    digest_algorithm: "sha256",
    intent_sign_hash: DEFAULT_INTENT_HASH,
    context_hash: DEFAULT_CONTEXT_HASH,
    signature: {
      signer: cfg.sender,
      algorithm: "secp256k1",
      signature: cfg.signatures.intent,
    },
  };

  const responseReq = {
    pool_id: poolID,
    intent_id: intentID,
    response_id: responseID,
    sender: cfg.recipient,
    recipient: cfg.sender,
    expires_unix: DEFAULT_EXPIRES_UNIX,
    digest_algorithm: "sha256",
    intent_sign_hash: DEFAULT_INTENT_HASH,
    response_sign_hash: DEFAULT_RESPONSE_HASH,
    context_hash: DEFAULT_CONTEXT_HASH,
    signature: {
      signer: cfg.recipient,
      algorithm: "secp256k1",
      signature: cfg.signatures.response,
    },
  };

  const finalizeReq = {
    pool_id: poolID,
    intent_id: intentID,
    response_id: responseID,
    finalize_id: finalizeID,
    sender: cfg.sender,
    recipient: cfg.recipient,
    expires_unix: DEFAULT_EXPIRES_UNIX,
    digest_algorithm: "sha256",
    intent_sign_hash: DEFAULT_INTENT_HASH,
    response_sign_hash: DEFAULT_RESPONSE_HASH,
    finalize_sign_hash: DEFAULT_FINALIZE_HASH,
    context_hash: DEFAULT_CONTEXT_HASH,
    initiator_signature: {
      signer: cfg.sender,
      algorithm: "secp256k1",
      signature: cfg.signatures.finalizeInitiator,
    },
    responder_signature: {
      signer: cfg.recipient,
      algorithm: "secp256k1",
      signature: cfg.signatures.finalizeResponder,
    },
  };

  const e1 = await postExpect201(`${base}/v1/intents`, cfg.tokenAlice, intentReq, "/v1/intents");
  if (e1 !== null) {
    return { ok: false, errorKeys: [e1] };
  }

  const e2 = await postExpect201(`${base}/v1/responses`, cfg.tokenBob, responseReq, "/v1/responses");
  if (e2 !== null) {
    return { ok: false, errorKeys: [e2] };
  }

  const e3 = await postExpect201(`${base}/v1/finalize`, cfg.tokenAlice, finalizeReq, "/v1/finalize");
  if (e3 !== null) {
    return { ok: false, errorKeys: [e3] };
  }

  return { ok: true };
}

async function postExpect201(url: string, token: string, body: unknown, endpoint: string): Promise<string | null> {
  try {
    const resp = await fetch(url, {
      method: "POST",
      headers: {
        "content-type": "application/json",
        authorization: `Bearer ${token}`,
      },
      body: JSON.stringify(body),
    });

    if (resp.status === 201) {
      return null;
    }

    const payload = (await resp.text()).slice(0, 256);
    return `${endpoint}:${resp.status}:${payload}`;
  } catch (err) {
    return `${endpoint}:network:${String(err)}`;
  }
}

async function collectResourceSnapshot(containers: string[]): Promise<ResourceSnapshot> {
  const args = [
    "stats",
    "--no-stream",
    "--format",
    "{{json .}}",
    ...containers,
  ];

  const { stdout } = await execFileAsync("docker", args, {
    maxBuffer: 16 * 1024 * 1024,
  });

  const lines = stdout
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.length > 0);

  if (lines.length === 0) {
    throw new Error("docker stats returned no lines");
  }

  let clusterCpuPercent = 0;
  let clusterMemMiB = 0;
  const perContainer: Record<string, ContainerSnapshot> = {};

  for (const line of lines) {
    const row = JSON.parse(line) as Record<string, string>;
    const name = row.Name;
    if (typeof name !== "string" || !name) {
      continue;
    }

    const cpuPercent = parseCpuPercent(row.CPUPerc ?? "0");
    const memMiB = parseMemMiB(row.MemUsage ?? "0MiB / 0MiB");

    clusterCpuPercent += cpuPercent;
    clusterMemMiB += memMiB;
    perContainer[name] = { cpuPercent, memMiB };
  }

  return {
    clusterCpuPercent,
    clusterMemMiB,
    perContainer,
  };
}

function parseCpuPercent(raw: string): number {
  const normalized = raw.replace("%", "").trim();
  const value = Number.parseFloat(normalized);
  if (!Number.isFinite(value) || value < 0) {
    return 0;
  }
  return value;
}

function parseMemMiB(memUsage: string): number {
  const [usedRaw] = memUsage.split("/");
  if (!usedRaw) {
    return 0;
  }

  const text = usedRaw.trim();
  const match = text.match(/^([0-9]+(?:\.[0-9]+)?)\s*([KMGTP]?i?)B$/i);
  if (!match) {
    return 0;
  }

  const value = Number.parseFloat(match[1] ?? "0");
  const unit = (match[2] ?? "").toUpperCase();

  const factor: Record<string, number> = {
    "": 1 / (1024 * 1024),
    K: 1 / 1024,
    KI: 1 / 1024,
    M: 1,
    MI: 1,
    G: 1024,
    GI: 1024,
    T: 1024 * 1024,
    TI: 1024 * 1024,
    P: 1024 * 1024 * 1024,
    PI: 1024 * 1024 * 1024,
  };

  return value * (factor[unit] ?? 0);
}

function percentile(values: number[], p: number): number {
  if (values.length === 0) {
    return 0;
  }
  const sorted = [...values].sort((a, b) => a - b);
  const pos = Math.max(0, Math.ceil((p / 100) * sorted.length) - 1);
  return sorted[pos] ?? sorted[sorted.length - 1] ?? 0;
}

function mean(values: number[]): number {
  if (values.length === 0) {
    return 0;
  }
  const sum = values.reduce((acc, value) => acc + value, 0);
  return sum / values.length;
}

function maxOrZero(values: number[]): number {
  if (values.length === 0) {
    return 0;
  }
  return Math.max(...values);
}

function mapToObject(m: Map<string, number>): Record<string, number> {
  const out: Record<string, number> = {};
  for (const [k, v] of m.entries()) {
    out[k] = v;
  }
  return out;
}

function printMarkdownTable(results: ScenarioResult[]): void {
  const header = [
    "| target TPS | observed flow/s | CPU p95 (vCPU) | RAM p95 (GiB) | recommended vCPU | recommended RAM (GiB) |",
    "|---:|---:|---:|---:|---:|---:|",
  ];
  const rows = results.map((r) => {
    return `| ${r.targetTps} | ${r.flowRatePerSec.toFixed(1)} | ${r.resource.cpuP95Vcpu.toFixed(2)} | ${r.resource.memP95GiB.toFixed(2)} | ${r.recommended.vcpu} | ${r.recommended.ramGiB} |`;
  });

  console.log("\n### TPS sizing table (measured)");
  console.log([...header, ...rows].join("\n"));
}

function parseStringList(raw: string | undefined): string[] | undefined {
  if (!raw) {
    return undefined;
  }
  const items = raw
    .split(",")
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
  return items.length > 0 ? items : undefined;
}

function parseNumberList(raw: string | undefined): number[] | undefined {
  if (!raw) {
    return undefined;
  }
  return parseTargetsStrict(raw);
}

function parseTargetsStrict(raw: string): number[] {
  const tokens = raw
    .split(",")
    .map((item) => item.trim())
    .filter((item) => item.length > 0);

  if (tokens.length === 0) {
    throw new Error("target list is empty");
  }

  return tokens.map((token) => parsePositiveIntStrict(token, "target"));
}

function parseIntWithDefault(raw: string | undefined, fallback: number): number {
  if (!raw || raw.trim() === "") {
    return fallback;
  }
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    return fallback;
  }
  return parsed;
}

function parseOptionalInt(raw: string | undefined): number | undefined {
  if (!raw || raw.trim() === "") {
    return undefined;
  }
  return parsePositiveIntStrict(raw, "workers");
}

function parsePositiveIntStrict(raw: string, field: string): number {
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    throw new Error(`${field} must be positive integer: ${raw}`);
  }
  return parsed;
}

function toHexHash(hashNoPrefix: string): `0x${string}` {
  const normalized = hashNoPrefix.trim().toLowerCase();
  if (!/^[0-9a-f]{64}$/.test(normalized)) {
    throw new Error(`invalid 32-byte hex hash: ${hashNoPrefix}`);
  }
  return `0x${normalized}`;
}

function isHexAddress(value: string): boolean {
  return /^0x[0-9a-fA-F]{40}$/.test(value.trim());
}

function equalsHexAddress(a: string, b: string): boolean {
  return a.trim().toLowerCase() === b.trim().toLowerCase();
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

void main().catch((err: unknown) => {
  console.error("❌ hardware benchmark failed");
  console.error(err);
  process.exitCode = 1;
});
