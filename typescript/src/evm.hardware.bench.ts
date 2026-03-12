import { execFile } from "node:child_process";
import { promisify } from "node:util";
import { writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { JsonRpcProvider, Wallet, isAddress } from "ethers";

const execFileAsync = promisify(execFile);

const DEFAULT_TARGETS = [50, 100, 500, 1000];
const DEFAULT_DURATION_SEC = 30;
const DEFAULT_SETTLE_SEC = 10;
const DEFAULT_SAMPLE_INTERVAL_MS = 1000;
const DEFAULT_GAS_PRICE_MULTIPLIER = 10;
const DEFAULT_SENDER_COUNT = 32;
const DEFAULT_FUND_WEI = "10000000000000000"; // 0.01 ETH-like unit per sender
const DEFAULT_SENDER_NONCE_WAIT_MS = 120000;
const FALLBACK_GAS_PRICE_WEI = 1_000_000_000n;

const DEFAULT_RPC_URLS = [
  "http://127.0.0.1:28545",
  "http://127.0.0.1:28555",
  "http://127.0.0.1:28565",
  "http://127.0.0.1:28575",
];

const DEFAULT_CONTAINERS = ["evmevm0", "evmevm1", "evmevm2", "evmevm3"];

const DEFAULT_FUNDER_PRIVATE_KEY =
  "0x88cbead91aee890d27bf06e003ade3d4e952427e88f88d31d61d3ef5e5d54305";
const DEFAULT_RECIPIENT = "0x963EBDf2e1f8DB8707D05FC75bfeFFBa1B5BaC17";

const THIS_DIR = path.dirname(fileURLToPath(import.meta.url));
const TYPESCRIPT_DIR = path.resolve(THIS_DIR, "..");

type CliOptions = {
  targets: number[];
  durationSec: number;
  settleSec: number;
  sampleIntervalMs: number;
  workersOverride?: number;
  senderCount?: number;
  outputPath?: string;
};

type BenchConfig = {
  rpcUrls: string[];
  containers: string[];
  funderPrivateKey: string;
  recipient: string;
  gasPriceMultiplier: number;
  senderCount: number;
  fundWei: bigint;
  senderNonceWaitMs: number;
};

type ResourceSnapshot = {
  clusterCpuPercent: number;
  clusterMemMiB: number;
};

type ScenarioResult = {
  targetTps: number;
  workers: number;
  durationSec: number;
  settleSec: number;
  gasPriceWei: string;
  chainId: string;
  funder: string;
  senderCount: number;
  recipient: string;
  txAttempted: number;
  txSent: number;
  txFailed: number;
  txIncluded: number;
  txPendingAfterSettle: number;
  sendRatePerSec: number;
  includedRatePerSec: number;
  failRatePercent: number;
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
  nextSenderIndex: number;
  nextNonces: number[];
  attempted: number;
  sent: number;
  failed: number;
  errorCounts: Map<string, number>;
};

async function main(): Promise<void> {
  const options = parseArgs(process.argv.slice(2));
  const cfg = buildConfig(options);

  const providers = cfg.rpcUrls.map((url) => new JsonRpcProvider(url));
  const funderWallet = new Wallet(cfg.funderPrivateKey);

  console.log("ℹ️ EVM tx benchmark config");
  console.log(
    JSON.stringify(
      {
        targets: options.targets,
        durationSec: options.durationSec,
        settleSec: options.settleSec,
        sampleIntervalMs: options.sampleIntervalMs,
        workersOverride: options.workersOverride ?? null,
        rpcUrls: cfg.rpcUrls,
        containers: cfg.containers,
        funder: funderWallet.address,
        senderCount: cfg.senderCount,
        senderNonceWaitMs: cfg.senderNonceWaitMs,
        recipient: cfg.recipient,
      },
      null,
      2,
    ),
  );

  await ensureRpcHealthy(cfg.rpcUrls);

  const chainId = await providers[0]!.send("eth_chainId", []);
  if (typeof chainId !== "string") {
    throw new Error(`unexpected eth_chainId type: ${typeof chainId}`);
  }

  const results: ScenarioResult[] = [];

  for (const targetTps of options.targets) {
    const workers = options.workersOverride ?? autoWorkers(targetTps);
    const result = await runScenario({
      cfg,
      providers,
      funderWallet,
      chainId,
      targetTps,
      workers,
      durationSec: options.durationSec,
      settleSec: options.settleSec,
      sampleIntervalMs: options.sampleIntervalMs,
    });
    results.push(result);

    console.log(
      `✅ target=${targetTps} sent=${result.sendRatePerSec.toFixed(2)} tx/s ` +
        `included=${result.includedRatePerSec.toFixed(2)} tx/s ` +
        `cpu_p95=${result.resource.cpuP95Vcpu.toFixed(2)} vCPU mem_p95=${result.resource.memP95GiB.toFixed(2)} GiB`,
    );
  }

  printMarkdownTable(results);

  const output = {
    generatedAt: new Date().toISOString(),
    options,
    config: {
      rpcUrls: cfg.rpcUrls,
      containers: cfg.containers,
      funder: funderWallet.address,
      senderCount: cfg.senderCount,
      recipient: cfg.recipient,
      gasPriceMultiplier: cfg.gasPriceMultiplier,
      fundWei: cfg.fundWei.toString(),
      senderNonceWaitMs: cfg.senderNonceWaitMs,
    },
    results,
  };

  const outputPath = options.outputPath ?? path.join(TYPESCRIPT_DIR, `evm-hardware-bench-${Date.now()}.json`);
  await writeFile(outputPath, JSON.stringify(output, null, 2), "utf-8");
  console.log(`\n📄 wrote benchmark result: ${outputPath}`);
}

function parseArgs(args: string[]): CliOptions {
  const out: CliOptions = {
    targets: parseNumberList(process.env.MATCH_EVM_TARGETS) ?? [...DEFAULT_TARGETS],
    durationSec: parseIntWithDefault(process.env.MATCH_EVM_DURATION_SEC, DEFAULT_DURATION_SEC),
    settleSec: parseIntWithDefault(process.env.MATCH_EVM_SETTLE_SEC, DEFAULT_SETTLE_SEC),
    sampleIntervalMs: parseIntWithDefault(process.env.MATCH_EVM_SAMPLE_MS, DEFAULT_SAMPLE_INTERVAL_MS),
    workersOverride: parseOptionalInt(process.env.MATCH_EVM_WORKERS),
    senderCount: parseOptionalInt(process.env.MATCH_EVM_SENDER_COUNT),
    outputPath: process.env.MATCH_EVM_OUTPUT,
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
    if (arg.startsWith("--settle=")) {
      out.settleSec = parsePositiveIntStrict(arg.slice("--settle=".length), "settle");
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
    if (arg.startsWith("--sender-count=")) {
      out.senderCount = parsePositiveIntStrict(arg.slice("--sender-count=".length), "sender-count");
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

  return out;
}

function printHelpAndExit(): never {
  console.log(`
Usage:
  npm run bench:evm -- --targets=50,100,500,1000 --duration=30 --settle=10

Prerequisite:
  npm run bench:evm:up

Options:
  --targets=LIST       target EVM tx TPS list (default: 50,100,500,1000)
  --duration=SECONDS   duration per target (default: 30)
  --settle=SECONDS     wait time for inclusion after send phase (default: 10)
  --sample-ms=MILLIS   docker stats sampling interval (default: 1000)
  --workers=N          override workers (default: auto by target)
  --sender-count=N     number of funded sending wallets (default: 32)
  --output=PATH        write JSON report path

Env overrides:
  MATCH_EVM_RPC_URLS       e.g. http://127.0.0.1:28545,...
  MATCH_EVM_CONTAINERS     e.g. evmevm0,evmevm1,evmevm2,evmevm3
  MATCH_EVM_TARGETS        same format as --targets
  MATCH_EVM_DURATION_SEC   same as --duration
  MATCH_EVM_SETTLE_SEC     same as --settle
  MATCH_EVM_SAMPLE_MS      same as --sample-ms
  MATCH_EVM_WORKERS        same as --workers
  MATCH_EVM_SENDER_COUNT   same as --sender-count
  MATCH_EVM_OUTPUT         same as --output
  MATCH_EVM_GAS_MULTIPLIER integer >= 1 (default: 10)
  MATCH_EVM_SENDER_NONCE_WAIT_MS wait previous sender tx finalization (default: 120000)
  MATCH_EVM_FUND_WEI       fund amount per sender wallet (default: 10000000000000000)

  MATCH_EVM_FUNDER_PRIVATE_KEY
  MATCH_EVM_RECIPIENT
`.trim());
  process.exit(0);
}

function buildConfig(options: CliOptions): BenchConfig {
  const rpcUrls = parseStringList(process.env.MATCH_EVM_RPC_URLS) ?? [...DEFAULT_RPC_URLS];
  const containers = parseStringList(process.env.MATCH_EVM_CONTAINERS) ?? [...DEFAULT_CONTAINERS];

  const funderPrivateKey = (process.env.MATCH_EVM_FUNDER_PRIVATE_KEY ?? DEFAULT_FUNDER_PRIVATE_KEY).trim();
  const recipient = (process.env.MATCH_EVM_RECIPIENT ?? DEFAULT_RECIPIENT).trim();
  const senderCount = options.senderCount ?? DEFAULT_SENDER_COUNT;
  const fundWeiRaw = (process.env.MATCH_EVM_FUND_WEI ?? DEFAULT_FUND_WEI).trim();
  const senderNonceWaitMs = parseIntWithDefault(
    process.env.MATCH_EVM_SENDER_NONCE_WAIT_MS,
    DEFAULT_SENDER_NONCE_WAIT_MS,
  );
  const gasPriceMultiplier = parseIntWithDefault(
    process.env.MATCH_EVM_GAS_MULTIPLIER,
    DEFAULT_GAS_PRICE_MULTIPLIER,
  );

  if (!funderPrivateKey) {
    throw new Error("funder private key must not be empty");
  }
  if (!isAddress(recipient)) {
    throw new Error(`invalid recipient address: ${recipient}`);
  }
  if (gasPriceMultiplier < 1) {
    throw new Error(`MATCH_EVM_GAS_MULTIPLIER must be >= 1: ${gasPriceMultiplier}`);
  }
  if (senderCount < 1) {
    throw new Error(`senderCount must be >= 1: ${senderCount}`);
  }
  if (senderNonceWaitMs < 1000) {
    throw new Error(`MATCH_EVM_SENDER_NONCE_WAIT_MS must be >= 1000: ${senderNonceWaitMs}`);
  }

  let fundWei: bigint;
  try {
    fundWei = BigInt(fundWeiRaw);
  } catch (err) {
    throw new Error(`invalid MATCH_EVM_FUND_WEI: ${fundWeiRaw} (${String(err)})`);
  }
  if (fundWei <= 0n) {
    throw new Error(`MATCH_EVM_FUND_WEI must be positive: ${fundWeiRaw}`);
  }

  return {
    rpcUrls,
    containers,
    funderPrivateKey,
    recipient,
    gasPriceMultiplier,
    senderCount,
    fundWei,
    senderNonceWaitMs,
  };
}

async function ensureRpcHealthy(rpcUrls: string[]): Promise<void> {
  const timeoutMs = Number.parseInt(process.env.MATCH_EVM_HEALTH_TIMEOUT_MS ?? "60000", 10);

  for (const url of rpcUrls) {
    const start = Date.now();
    let ok = false;

    while (Date.now() - start < timeoutMs) {
      try {
        const body = {
          jsonrpc: "2.0",
          id: 1,
          method: "eth_chainId",
          params: [] as unknown[],
        };
        const resp = await fetch(url, {
          method: "POST",
          headers: { "content-type": "application/json" },
          body: JSON.stringify(body),
        });
        if (resp.ok) {
          const blockResp = await fetch(url, {
            method: "POST",
            headers: { "content-type": "application/json" },
            body: JSON.stringify({
              jsonrpc: "2.0",
              id: 2,
              method: "eth_blockNumber",
              params: [] as unknown[],
            }),
          });
          if (blockResp.ok) {
            const payload = (await blockResp.json()) as { result?: string };
            const blockHex = payload.result;
            if (typeof blockHex === "string" && parseInt(blockHex, 16) >= 1) {
              ok = true;
              break;
            }
          }
        }
      } catch {
        // retry
      }
      await sleep(800);
    }

    if (!ok) {
      throw new Error(
        [
          `RPC health check timeout ${timeoutMs}ms: ${url}`,
          `- hint: cd typescript && npm run bench:evm:up`,
          `- override with MATCH_EVM_RPC_URLS if your ports are different`,
        ].join("\n"),
      );
    }
  }
}

function autoWorkers(targetTps: number): number {
  return Math.max(4, Math.min(128, Math.ceil(targetTps / 40)));
}

async function runScenario(params: {
  cfg: BenchConfig;
  providers: JsonRpcProvider[];
  funderWallet: Wallet;
  chainId: string;
  targetTps: number;
  workers: number;
  durationSec: number;
  settleSec: number;
  sampleIntervalMs: number;
}): Promise<ScenarioResult> {
  const { cfg, providers, funderWallet, chainId, targetTps, workers, durationSec, settleSec, sampleIntervalMs } = params;

  const primary = providers[0]!;
  const funder = funderWallet.address;

  await waitProviderReady(primary, 60_000);
  await waitNoPending(primary, funder, 45_000);

  const gasPriceBase = await fetchGasPriceWithRetry(primary, FALLBACK_GAS_PRICE_WEI);
  const gasPrice = gasPriceBase * BigInt(cfg.gasPriceMultiplier);

  const senders = await createSenderWallets({
    count: cfg.senderCount,
    provider: primary,
    chainId,
    funder: funderWallet,
    fundWei: cfg.fundWei,
    gasPrice: gasPrice * 2n,
  });

  const latestNoncesStart = await Promise.all(
    senders.map((senderWallet) => primary.getTransactionCount(senderWallet.address, "latest")),
  );
  const pendingNoncesStart = await Promise.all(
    senders.map((senderWallet) => primary.getTransactionCount(senderWallet.address, "pending")),
  );

  const counters: ScenarioCounters = {
    nextSenderIndex: 0,
    nextNonces: pendingNoncesStart,
    attempted: 0,
    sent: 0,
    failed: 0,
    errorCounts: new Map<string, number>(),
  };

  const resourceSamples: ResourceSnapshot[] = [];
  let stopSampler = false;

  const sampler = (async () => {
    while (!stopSampler) {
      try {
        const snap = await collectResourceSnapshot(cfg.containers);
        resourceSamples.push(snap);
      } catch (err) {
        console.warn(`⚠️ docker stats sample failed: ${String(err)}`);
      }
      await sleep(sampleIntervalMs);
    }
  })();

  const scenarioID = `${Date.now()}-${targetTps}`;
  const startedAt = Date.now();
  const stopAt = startedAt + durationSec * 1000;

  await Promise.all(
    Array.from({ length: workers }, (_, workerIndex) =>
      txWorker({
        workerIndex,
        workers,
        targetTps,
        stopAt,
        scenarioID,
        chainId,
        gasPrice,
        senders,
        recipient: cfg.recipient,
        provider: primary,
        senderNonceWaitMs: cfg.senderNonceWaitMs,
        counters,
      }),
    ),
  );

  const endedAt = Date.now();

  stopSampler = true;
  await sampler;

  await sleep(settleSec * 1000);

  const latestNoncesEnd = await Promise.all(
    senders.map((senderWallet) => primary.getTransactionCount(senderWallet.address, "latest")),
  );
  const pendingNoncesEnd = await Promise.all(
    senders.map((senderWallet) => primary.getTransactionCount(senderWallet.address, "pending")),
  );

  try {
    resourceSamples.push(await collectResourceSnapshot(cfg.containers));
  } catch {
    // ignore final sample error
  }

  let included = 0;
  let pendingAfterSettle = 0;
  for (let i = 0; i < senders.length; i += 1) {
    included += Math.max(0, (latestNoncesEnd[i] ?? 0) - (latestNoncesStart[i] ?? 0));
    pendingAfterSettle += Math.max(0, (pendingNoncesEnd[i] ?? 0) - (latestNoncesEnd[i] ?? 0));
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
  const recommendedVcpu = Math.max(4, Math.ceil(cpuP95Vcpu * 1.5 + 1));
  const recommendedRamGiB = Math.max(8, Math.ceil((memP95MiB / 1024) * 1.5 + 2));

  return {
    targetTps,
    workers,
    durationSec,
    settleSec,
    gasPriceWei: gasPrice.toString(),
    chainId,
    funder,
    senderCount: senders.length,
    recipient: cfg.recipient,
    txAttempted: counters.attempted,
    txSent: counters.sent,
    txFailed: counters.failed,
    txIncluded: included,
    txPendingAfterSettle: pendingAfterSettle,
    sendRatePerSec: counters.sent / durationActualSec,
    includedRatePerSec: included / durationActualSec,
    failRatePercent: counters.attempted > 0 ? (counters.failed / counters.attempted) * 100 : 0,
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
      rule: "vcpu=max(4,ceil(cpu_p95_vcpu*1.5+1)); ram=max(8GiB,ceil(mem_p95_gib*1.5+2))",
    },
    errors: mapToObject(counters.errorCounts),
  };
}

async function txWorker(params: {
  workerIndex: number;
  workers: number;
  targetTps: number;
  stopAt: number;
  scenarioID: string;
  chainId: string;
  gasPrice: bigint;
  senders: Wallet[];
  recipient: string;
  provider: JsonRpcProvider;
  senderNonceWaitMs: number;
  counters: ScenarioCounters;
}): Promise<void> {
  const {
    workerIndex,
    workers,
    targetTps,
    stopAt,
    scenarioID,
    chainId,
    gasPrice,
    senders,
    recipient,
    provider,
    senderNonceWaitMs,
    counters,
  } = params;

  const intervalMs = (1000 * workers) / Math.max(1, targetTps);
  let nextRunAt = Date.now() + (intervalMs * workerIndex) / workers;

  while (Date.now() < stopAt) {
    const waitMs = nextRunAt - Date.now();
    if (waitMs > 1) {
      await sleep(waitMs);
    }

    const senderIndex = counters.nextSenderIndex % senders.length;
    counters.nextSenderIndex += 1;

    const wallet = senders[senderIndex];
    if (!wallet) {
      counters.failed += 1;
      counters.errorCounts.set("sender_wallet_missing", (counters.errorCounts.get("sender_wallet_missing") ?? 0) + 1);
      nextRunAt += intervalMs;
      continue;
    }

    let nonce = counters.nextNonces[senderIndex] ?? 0;

    const currentLatest = await provider.getTransactionCount(wallet.address, "latest");
    if (currentLatest > nonce) {
      nonce = currentLatest;
      counters.nextNonces[senderIndex] = nonce;
    }

    const ready = await waitAddressLatestNonceAtLeast(provider, wallet.address, nonce, senderNonceWaitMs);
    if (!ready) {
      counters.failed += 1;
      counters.errorCounts.set(
        "sender_nonce_wait_timeout",
        (counters.errorCounts.get("sender_nonce_wait_timeout") ?? 0) + 1,
      );
      nextRunAt += intervalMs;
      continue;
    }

    counters.attempted += 1;

    const tx = {
      chainId: BigInt(chainId),
      nonce,
      to: recipient,
      value: 1n,
      gasLimit: 21_000n,
      gasPrice,
      type: 0,
      data: "0x" as const,
    };

    try {
      const raw = await wallet.signTransaction(tx);
      await provider.send("eth_sendRawTransaction", [raw]);
      counters.sent += 1;
      counters.nextNonces[senderIndex] = nonce + 1;
    } catch (err) {
      const key = classifyRpcError(err, scenarioID);

      if (key === "underpriced" || key === "nonce_too_low" || key === "already_known") {
        const [latestAfterErr, pendingAfterErr] = await Promise.all([
          provider.getTransactionCount(wallet.address, "latest"),
          provider.getTransactionCount(wallet.address, "pending"),
        ]);
        const synced = Math.max(nonce, latestAfterErr, pendingAfterErr);
        counters.nextNonces[senderIndex] = synced;
      }

      counters.failed += 1;
      counters.errorCounts.set(key, (counters.errorCounts.get(key) ?? 0) + 1);
    }

    nextRunAt += intervalMs;
  }
}

function classifyRpcError(err: unknown, scenarioID: string): string {
  const raw = String(err).toLowerCase();
  const normalized = raw.replaceAll(scenarioID.toLowerCase(), "{scenario}");

  if (normalized.includes("nonce too low")) return "nonce_too_low";
  if (normalized.includes("already known")) return "already_known";
  if (normalized.includes("replacement transaction underpriced")) return "underpriced";
  if (normalized.includes("mempool") && normalized.includes("full")) return "mempool_full";
  if (normalized.includes("insufficient funds")) return "insufficient_funds";
  if (normalized.includes("timeout")) return "timeout";

  const compact = normalized.replace(/\s+/g, " ").slice(0, 180);
  return `rpc_error:${compact}`;
}

function isUnderpricedError(err: unknown): boolean {
  return String(err).toLowerCase().includes("replacement transaction underpriced");
}

function isNonceTooLowError(err: unknown): boolean {
  return String(err).toLowerCase().includes("nonce too low");
}

function isAlreadyKnownError(err: unknown): boolean {
  return String(err).toLowerCase().includes("already known");
}

async function createSenderWallets(params: {
  count: number;
  provider: JsonRpcProvider;
  chainId: string;
  funder: Wallet;
  fundWei: bigint;
  gasPrice: bigint;
}): Promise<Wallet[]> {
  const { count, provider, chainId, funder, fundWei, gasPrice } = params;

  const out: Wallet[] = [];
  for (let i = 0; i < count; i += 1) {
    const random = Wallet.createRandom();
    out.push(new Wallet(random.privateKey));
  }

  await fundSenderWallets({
    provider,
    chainId,
    funder,
    senders: out,
    fundWei,
    gasPrice,
  });

  return out;
}

async function fundSenderWallets(params: {
  provider: JsonRpcProvider;
  chainId: string;
  funder: Wallet;
  senders: Wallet[];
  fundWei: bigint;
  gasPrice: bigint;
}): Promise<void> {
  const { provider, chainId, funder, senders, fundWei, gasPrice } = params;
  if (senders.length === 0) {
    return;
  }

  const balances = await Promise.all(senders.map((w) => provider.getBalance(w.address)));

  const targets: Wallet[] = [];
  for (let i = 0; i < senders.length; i += 1) {
    const wallet = senders[i];
    if (!wallet) continue;

    const currentBalance = balances[i] ?? 0n;
    if (currentBalance >= fundWei) {
      continue;
    }
    targets.push(wallet);
  }

  let localNonce = await provider.getTransactionCount(funder.address, "pending");

  for (const wallet of targets) {
    let sent = false;
    let lastErr: unknown;
    let txNonce = localNonce;

    for (let attempt = 0; attempt < 5; attempt += 1) {
      try {
        const raw = await funder.signTransaction({
          chainId: BigInt(chainId),
          nonce: txNonce,
          to: wallet.address,
          value: fundWei,
          gasLimit: 21_000n,
          gasPrice,
          type: 0,
          data: "0x",
        });
        await provider.send("eth_sendRawTransaction", [raw]);
        sent = true;
        localNonce = txNonce + 1;
        break;
      } catch (err) {
        lastErr = err;
        if (isUnderpricedError(err) || isNonceTooLowError(err) || isAlreadyKnownError(err)) {
          const pending = await provider.getTransactionCount(funder.address, "pending");
          txNonce = Math.max(txNonce + 1, pending);
          localNonce = Math.max(localNonce, txNonce);
        }
        await sleep(300 + attempt * 400);
      }
    }

    if (!sent) {
      throw new Error(`failed to fund sender ${wallet.address}: ${String(lastErr)}`);
    }
  }

  if (targets.length === 0) {
    return;
  }

  const timeoutMs = Number.parseInt(process.env.MATCH_EVM_FUND_TIMEOUT_MS ?? "180000", 10);
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    const fundedBalances = await Promise.all(targets.map((w) => provider.getBalance(w.address)));
    const ready = fundedBalances.every((bal) => bal >= fundWei);
    if (ready) {
      return;
    }
    await sleep(1000);
  }

  throw new Error(`funding senders timed out after ${timeoutMs}ms (targets=${targets.length})`);
}

async function waitProviderReady(provider: JsonRpcProvider, timeoutMs: number): Promise<void> {
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    try {
      const block = await provider.send("eth_blockNumber", []);
      if (typeof block === "string" && parseInt(block, 16) >= 1) {
        return;
      }
    } catch (err) {
      if (!isNodeNotReadyError(err)) {
        // transient network issue or other unexpected RPC errors: keep retrying until timeout
      }
    }
    await sleep(800);
  }
  throw new Error(`provider not ready within ${timeoutMs}ms`);
}

async function waitNoPending(provider: JsonRpcProvider, sender: string, timeoutMs: number): Promise<void> {
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    const [latest, pending] = await Promise.all([
      provider.getTransactionCount(sender, "latest"),
      provider.getTransactionCount(sender, "pending"),
    ]);
    if (pending === latest) {
      return;
    }
    await sleep(700);
  }
  throw new Error(`pending tx queue not drained for sender ${sender} within ${timeoutMs}ms`);
}

async function fetchGasPriceWithRetry(
  provider: JsonRpcProvider,
  fallback: bigint,
  attempts = 20,
): Promise<bigint> {
  let lastErr: unknown;
  for (let i = 0; i < attempts; i += 1) {
    try {
      const raw = await provider.send("eth_gasPrice", []);
      const value = BigInt(typeof raw === "string" ? raw : "0x0");
      if (value > 0n) {
        return value;
      }
    } catch (err) {
      lastErr = err;
    }
    await sleep(400);
  }
  console.warn(`⚠️ eth_gasPrice failed; using fallback=${fallback.toString()} wei cause=${String(lastErr)}`);
  return fallback;
}

async function waitAddressLatestNonceAtLeast(
  provider: JsonRpcProvider,
  address: string,
  expectedNonce: number,
  timeoutMs: number,
): Promise<boolean> {
  const started = Date.now();
  while (Date.now() - started < timeoutMs) {
    const latest = await provider.getTransactionCount(address, "latest");
    if (latest >= expectedNonce) {
      return true;
    }
    await sleep(300);
  }
  return false;
}

function isNodeNotReadyError(err: unknown): boolean {
  const msg = String(err).toLowerCase();
  return msg.includes("not ready") || msg.includes("invalid height");
}

async function collectResourceSnapshot(containers: string[]): Promise<ResourceSnapshot> {
  const { stdout } = await execFileAsync("docker", ["stats", "--no-stream", "--format", "{{json .}}", ...containers], {
    maxBuffer: 16 * 1024 * 1024,
  });

  const lines = stdout
    .split("\n")
    .map((line) => line.trim())
    .filter((line) => line.length > 0);

  if (lines.length === 0) {
    throw new Error("docker stats returned no data; verify MATCH_EVM_CONTAINERS");
  }

  let clusterCpuPercent = 0;
  let clusterMemMiB = 0;

  for (const line of lines) {
    const row = JSON.parse(line) as Record<string, string>;
    const cpu = parseCpuPercent(row.CPUPerc ?? "0");
    const mem = parseMemMiB(row.MemUsage ?? "0MiB / 0MiB");
    clusterCpuPercent += cpu;
    clusterMemMiB += mem;
  }

  return {
    clusterCpuPercent,
    clusterMemMiB,
  };
}

function parseCpuPercent(raw: string): number {
  const n = Number.parseFloat(raw.replace("%", "").trim());
  if (!Number.isFinite(n) || n < 0) return 0;
  return n;
}

function parseMemMiB(rawUsage: string): number {
  const [usedRaw] = rawUsage.split("/");
  if (!usedRaw) return 0;

  const match = usedRaw.trim().match(/^([0-9]+(?:\.[0-9]+)?)\s*([KMGTP]?i?)B$/i);
  if (!match) return 0;

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

function printMarkdownTable(results: ScenarioResult[]): void {
  const lines = [
    "\n### EVM TPS sizing table (measured)",
    "| target TPS | sent tx/s | included tx/s | fail % | CPU p95 (vCPU) | RAM p95 (GiB) | recommended vCPU | recommended RAM (GiB) |",
    "|---:|---:|---:|---:|---:|---:|---:|---:|",
  ];

  for (const r of results) {
    lines.push(
      `| ${r.targetTps} | ${r.sendRatePerSec.toFixed(1)} | ${r.includedRatePerSec.toFixed(1)} | ${r.failRatePercent.toFixed(2)} | ${r.resource.cpuP95Vcpu.toFixed(2)} | ${r.resource.memP95GiB.toFixed(2)} | ${r.recommended.vcpu} | ${r.recommended.ramGiB} |`,
    );
  }

  console.log(lines.join("\n"));
}

function parseStringList(raw: string | undefined): string[] | undefined {
  if (!raw) return undefined;
  const items = raw
    .split(",")
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
  return items.length > 0 ? items : undefined;
}

function parseNumberList(raw: string | undefined): number[] | undefined {
  if (!raw) return undefined;
  return parseTargetsStrict(raw);
}

function parseTargetsStrict(raw: string): number[] {
  const tokens = raw
    .split(",")
    .map((item) => item.trim())
    .filter((item) => item.length > 0);
  if (tokens.length === 0) {
    throw new Error("targets are empty");
  }
  return tokens.map((token) => parsePositiveIntStrict(token, "target"));
}

function parseIntWithDefault(raw: string | undefined, fallback: number): number {
  if (!raw || raw.trim() === "") return fallback;
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) return fallback;
  return parsed;
}

function parseOptionalInt(raw: string | undefined): number | undefined {
  if (!raw || raw.trim() === "") return undefined;
  return parsePositiveIntStrict(raw, "workers");
}

function parsePositiveIntStrict(raw: string, field: string): number {
  const parsed = Number.parseInt(raw, 10);
  if (!Number.isFinite(parsed) || parsed <= 0) {
    throw new Error(`${field} must be positive integer: ${raw}`);
  }
  return parsed;
}

function percentile(values: number[], p: number): number {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const pos = Math.max(0, Math.ceil((p / 100) * sorted.length) - 1);
  return sorted[pos] ?? sorted[sorted.length - 1] ?? 0;
}

function mean(values: number[]): number {
  if (values.length === 0) return 0;
  return values.reduce((acc, value) => acc + value, 0) / values.length;
}

function maxOrZero(values: number[]): number {
  if (values.length === 0) return 0;
  return Math.max(...values);
}

function mapToObject(map: Map<string, number>): Record<string, number> {
  const out: Record<string, number> = {};
  for (const [k, v] of map.entries()) {
    out[k] = v;
  }
  return out;
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

void main().catch((err: unknown) => {
  console.error("❌ EVM hardware benchmark failed");
  console.error(err);
  process.exitCode = 1;
});
