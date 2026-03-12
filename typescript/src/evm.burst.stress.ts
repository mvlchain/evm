import { writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { JsonRpcProvider, Wallet, isAddress } from "ethers";

const DEFAULT_RPC_URLS = [
  "http://127.0.0.1:28545",
  "http://127.0.0.1:28555",
  "http://127.0.0.1:28565",
  "http://127.0.0.1:28575",
];

const DEFAULT_FUNDER_PRIVATE_KEY =
  "0x88cbead91aee890d27bf06e003ade3d4e952427e88f88d31d61d3ef5e5d54305";
const DEFAULT_RECIPIENT = "0x963EBDf2e1f8DB8707D05FC75bfeFFBa1B5BaC17";
const DEFAULT_WALLETS = 1000;
const DEFAULT_FUND_WEI = "10000000000000000"; // 0.01
const DEFAULT_GAS_PRICE_MULTIPLIER = 10;
const DEFAULT_SETTLE_SEC = 12;
const DEFAULT_TIMEOUT_MS = 120000;

const THIS_DIR = path.dirname(fileURLToPath(import.meta.url));
const TYPESCRIPT_DIR = path.resolve(THIS_DIR, "..");

type Options = {
  rpcUrls: string[];
  funderPrivateKey: string;
  recipient: string;
  walletCount: number;
  fundWei: bigint;
  gasPriceMultiplier: number;
  settleSec: number;
  timeoutMs: number;
  outputPath?: string;
};

type SendResult = {
  ok: boolean;
  txHash?: string;
  error?: string;
  latencyMs: number;
};

async function main(): Promise<void> {
  const opts = parseOptions(process.argv.slice(2));
  const providers = opts.rpcUrls.map((url) => new JsonRpcProvider(url));
  const primary = providers[0]!;
  const funder = new Wallet(opts.funderPrivateKey);

  console.log("ℹ️ burst stress config");
  console.log(
    JSON.stringify(
      {
        rpcUrls: opts.rpcUrls,
        walletCount: opts.walletCount,
        recipient: opts.recipient,
        settleSec: opts.settleSec,
        gasPriceMultiplier: opts.gasPriceMultiplier,
      },
      null,
      2,
    ),
  );

  await ensureReady(opts.rpcUrls, opts.timeoutMs);

  const chainIdHex = await primary.send("eth_chainId", []);
  const chainId = BigInt(typeof chainIdHex === "string" ? chainIdHex : "0x0");
  const baseGasPrice = await fetchGasPriceWithRetry(primary, 1_000_000_000n);
  const gasPrice = baseGasPrice * BigInt(opts.gasPriceMultiplier);

  const wallets = createWallets(opts.walletCount);

  console.log(`ℹ️ funding ${wallets.length} wallets...`);
  const fundingStart = Date.now();
  const funding = await fundWallets({
    providers,
    funder,
    wallets,
    chainId,
    gasPrice,
    fundWei: opts.fundWei,
  });
  const fundingSec = (Date.now() - fundingStart) / 1000;

  await waitWalletFunded(primary, wallets, opts.fundWei, opts.timeoutMs);

  console.log("ℹ️ sending 1 tx per wallet simultaneously...");
  const burstStart = Date.now();
  const burst = await sendBurst({
    providers,
    wallets,
    recipient: opts.recipient,
    chainId,
    gasPrice,
  });
  const burstSec = (Date.now() - burstStart) / 1000;

  await sleep(opts.settleSec * 1000);
  const included = await countIncluded(primary, wallets, 1);

  const latencies = burst.filter((r) => r.ok).map((r) => r.latencyMs);
  const failed = burst.filter((r) => !r.ok);
  const errors = summarizeErrors(failed.map((r) => r.error ?? "unknown"));

  const summary = {
    generatedAt: new Date().toISOString(),
    config: {
      rpcUrls: opts.rpcUrls,
      walletCount: opts.walletCount,
      recipient: opts.recipient,
      settleSec: opts.settleSec,
      gasPriceWei: gasPrice.toString(),
    },
    funding: {
      submitted: funding.submitted,
      failed: funding.failed,
      durationSec: Number(fundingSec.toFixed(3)),
    },
    burst: {
      submitted: burst.filter((r) => r.ok).length,
      failed: failed.length,
      durationSec: Number(burstSec.toFixed(3)),
      rpcSubmitRatePerSec: Number((burst.length / Math.max(0.001, burstSec)).toFixed(2)),
      p50Ms: percentile(latencies, 50),
      p95Ms: percentile(latencies, 95),
      p99Ms: percentile(latencies, 99),
      errors,
    },
    inclusion: {
      included,
      expected: wallets.length,
      inclusionRatePercent: Number(((included / wallets.length) * 100).toFixed(2)),
    },
  };

  console.log("\n### 1000-wallet burst result");
  console.log(JSON.stringify(summary, null, 2));

  const outputPath = opts.outputPath ?? path.join(TYPESCRIPT_DIR, `evm-burst-${Date.now()}.json`);
  await writeFile(outputPath, JSON.stringify(summary, null, 2), "utf-8");
  console.log(`\n📄 wrote burst report: ${outputPath}`);
}

function parseOptions(args: string[]): Options {
  const opts: Options = {
    rpcUrls: parseList(process.env.MATCH_EVM_RPC_URLS) ?? [...DEFAULT_RPC_URLS],
    funderPrivateKey: (process.env.MATCH_EVM_FUNDER_PRIVATE_KEY ?? DEFAULT_FUNDER_PRIVATE_KEY).trim(),
    recipient: (process.env.MATCH_EVM_RECIPIENT ?? DEFAULT_RECIPIENT).trim(),
    walletCount: parseIntDefault(process.env.MATCH_BURST_WALLETS, DEFAULT_WALLETS),
    fundWei: parseBigIntDefault(process.env.MATCH_EVM_FUND_WEI, DEFAULT_FUND_WEI),
    gasPriceMultiplier: parseIntDefault(process.env.MATCH_EVM_GAS_MULTIPLIER, DEFAULT_GAS_PRICE_MULTIPLIER),
    settleSec: parseIntDefault(process.env.MATCH_BURST_SETTLE_SEC, DEFAULT_SETTLE_SEC),
    timeoutMs: parseIntDefault(process.env.MATCH_BURST_TIMEOUT_MS, DEFAULT_TIMEOUT_MS),
    outputPath: process.env.MATCH_BURST_OUTPUT,
  };

  for (const arg of args) {
    if (arg.startsWith("--wallets=")) opts.walletCount = parsePositiveInt(arg.slice(10), "wallets");
    else if (arg.startsWith("--settle=")) opts.settleSec = parsePositiveInt(arg.slice(9), "settle");
    else if (arg.startsWith("--timeout-ms=")) opts.timeoutMs = parsePositiveInt(arg.slice(13), "timeout-ms");
    else if (arg.startsWith("--output=")) opts.outputPath = arg.slice(9).trim();
    else if (arg === "--help" || arg === "-h") {
      printHelpAndExit();
    } else throw new Error(`unknown arg: ${arg}`);
  }

  if (!isAddress(opts.recipient)) throw new Error(`invalid recipient: ${opts.recipient}`);
  if (opts.walletCount < 1) throw new Error(`walletCount must be >=1: ${opts.walletCount}`);
  return opts;
}

function printHelpAndExit(): never {
  console.log(`
Usage:
  npm run bench:evm:burst -- --wallets=1000 --settle=12

Prerequisite:
  npm run bench:evm:up

Env:
  MATCH_EVM_RPC_URLS
  MATCH_EVM_FUNDER_PRIVATE_KEY
  MATCH_EVM_RECIPIENT
  MATCH_EVM_FUND_WEI
  MATCH_EVM_GAS_MULTIPLIER
  MATCH_BURST_WALLETS
  MATCH_BURST_SETTLE_SEC
  MATCH_BURST_TIMEOUT_MS
  MATCH_BURST_OUTPUT
`.trim());
  process.exit(0);
}

async function ensureReady(rpcUrls: string[], timeoutMs: number): Promise<void> {
  for (const url of rpcUrls) {
    const start = Date.now();
    while (Date.now() - start < timeoutMs) {
      try {
        const resp = await rpc(url, "eth_blockNumber", []);
        if (typeof resp === "string" && parseInt(resp, 16) >= 1) break;
      } catch {
        // retry
      }
      await sleep(500);
    }
  }
}

function createWallets(n: number): Wallet[] {
  const out: Wallet[] = [];
  for (let i = 0; i < n; i += 1) {
    const w = Wallet.createRandom();
    out.push(new Wallet(w.privateKey));
  }
  return out;
}

async function fundWallets(params: {
  providers: JsonRpcProvider[];
  funder: Wallet;
  wallets: Wallet[];
  chainId: bigint;
  gasPrice: bigint;
  fundWei: bigint;
}): Promise<{ submitted: number; failed: number }> {
  const { providers, funder, wallets, chainId, gasPrice, fundWei } = params;
  const p0 = providers[0]!;

  let nonce = await p0.getTransactionCount(funder.address, "pending");
  const raws: string[] = [];

  for (const w of wallets) {
    const raw = await funder.signTransaction({
      chainId,
      nonce,
      to: w.address,
      value: fundWei,
      gasLimit: 21_000n,
      gasPrice,
      type: 0,
      data: "0x",
    });
    raws.push(raw);
    nonce += 1;
  }

  const sendResults = await Promise.all(
    raws.map((raw, i) => sendRaw(providers[i % providers.length]!, raw)),
  );

  const submitted = sendResults.filter((r) => r.ok).length;
  const failed = sendResults.length - submitted;
  return { submitted, failed };
}

async function waitWalletFunded(
  provider: JsonRpcProvider,
  wallets: Wallet[],
  minWei: bigint,
  timeoutMs: number,
): Promise<void> {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    const balances = await Promise.all(wallets.map((w) => provider.getBalance(w.address)));
    if (balances.every((b) => b >= minWei)) return;
    await sleep(1000);
  }
  throw new Error(`wallet funding wait timed out (${wallets.length} wallets)`);
}

async function sendBurst(params: {
  providers: JsonRpcProvider[];
  wallets: Wallet[];
  recipient: string;
  chainId: bigint;
  gasPrice: bigint;
}): Promise<SendResult[]> {
  const { providers, wallets, recipient, chainId, gasPrice } = params;

  const raws = await Promise.all(
    wallets.map((w) =>
      w.signTransaction({
        chainId,
        nonce: 0,
        to: recipient,
        value: 1n,
        gasLimit: 21_000n,
        gasPrice,
        type: 0,
        data: "0x",
      }),
    ),
  );

  const started = Date.now();
  const promises = raws.map(async (raw, i) => {
    const provider = providers[i % providers.length]!;
    const t0 = Date.now();
    const r = await sendRaw(provider, raw);
    return { ...r, latencyMs: Date.now() - t0 };
  });

  const out = await Promise.all(promises);
  const elapsed = Date.now() - started;
  console.log(`ℹ️ burst submit completed in ${elapsed}ms for ${wallets.length} tx`);
  return out;
}

async function countIncluded(provider: JsonRpcProvider, wallets: Wallet[], minNonce: number): Promise<number> {
  const nonces = await Promise.all(wallets.map((w) => provider.getTransactionCount(w.address, "latest")));
  return nonces.filter((n) => n >= minNonce).length;
}

async function sendRaw(provider: JsonRpcProvider, raw: string): Promise<SendResult> {
  const t0 = Date.now();
  try {
    const txHash = await provider.send("eth_sendRawTransaction", [raw]);
    return { ok: true, txHash: typeof txHash === "string" ? txHash : undefined, latencyMs: Date.now() - t0 };
  } catch (err) {
    return { ok: false, error: classifyError(err), latencyMs: Date.now() - t0 };
  }
}

async function fetchGasPriceWithRetry(provider: JsonRpcProvider, fallback: bigint): Promise<bigint> {
  let last: unknown;
  for (let i = 0; i < 15; i += 1) {
    try {
      const raw = await provider.send("eth_gasPrice", []);
      const v = BigInt(typeof raw === "string" ? raw : "0x0");
      if (v > 0n) return v;
    } catch (err) {
      last = err;
    }
    await sleep(300);
  }
  console.warn(`⚠️ eth_gasPrice failed; fallback ${fallback.toString()} cause=${String(last)}`);
  return fallback;
}

async function rpc(url: string, method: string, params: unknown[]): Promise<unknown> {
  const resp = await fetch(url, {
    method: "POST",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ jsonrpc: "2.0", id: Date.now(), method, params }),
  });
  const payload = (await resp.json()) as { result?: unknown; error?: { message?: string } };
  if (payload.error) throw new Error(payload.error.message ?? "rpc error");
  return payload.result;
}

function classifyError(err: unknown): string {
  const s = String(err).toLowerCase();
  if (s.includes("underpriced")) return "underpriced";
  if (s.includes("nonce too low")) return "nonce_too_low";
  if (s.includes("already known")) return "already_known";
  if (s.includes("insufficient funds")) return "insufficient_funds";
  if (s.includes("timeout")) return "timeout";
  return s.slice(0, 160);
}

function summarizeErrors(errors: string[]): Record<string, number> {
  const out: Record<string, number> = {};
  for (const e of errors) out[e] = (out[e] ?? 0) + 1;
  return out;
}

function percentile(values: number[], p: number): number {
  if (values.length === 0) return 0;
  const sorted = [...values].sort((a, b) => a - b);
  const idx = Math.max(0, Math.ceil((p / 100) * sorted.length) - 1);
  return Number((sorted[idx] ?? 0).toFixed(2));
}

function parseList(raw: string | undefined): string[] | undefined {
  if (!raw) return undefined;
  const v = raw.split(",").map((x) => x.trim()).filter(Boolean);
  return v.length ? v : undefined;
}

function parseIntDefault(raw: string | undefined, fallback: number): number {
  if (!raw) return fallback;
  const n = Number.parseInt(raw, 10);
  return Number.isFinite(n) && n > 0 ? n : fallback;
}

function parsePositiveInt(raw: string, field: string): number {
  const n = Number.parseInt(raw, 10);
  if (!Number.isFinite(n) || n <= 0) throw new Error(`${field} must be positive int: ${raw}`);
  return n;
}

function parseBigIntDefault(raw: string | undefined, fallback: string): bigint {
  try {
    return BigInt((raw ?? fallback).trim());
  } catch {
    return BigInt(fallback);
  }
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

void main().catch((err: unknown) => {
  console.error("❌ evm burst stress failed");
  console.error(err);
  process.exitCode = 1;
});
