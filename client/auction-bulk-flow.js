import { ethers } from "ethers";
import http from "node:http";
import https from "node:https";
import { URL } from "node:url";
import { buildAskPayload, buildBidPayload } from "./auction-sign.js";

// Default funded dev accounts from local_node.sh
const DEV_MNEMONICS = {
  dev0:
    "copper push brief egg scan entry inform record adjust fossil boss egg comic alien upon aspect dry avoid interest fury window hint race symptom",
  dev1:
    "maximum display century economy unlock van census kite error heart snow filter midnight usage egg venture cash kick motor survey drastic edge muffin visual",
  dev2:
    "will wear settle write dance topic tape sea glory hotel oppose rebel client problem era video gossip glide during yard balance cancel file rose",
  dev3:
    "doll midnight silk carpet brush boring pluck office gown inquiry duck chief aim exit gain never tennis crime fragile ship cloud surface exotic patch",
};

function walletFromMnemonic(mnemonic, index = 0) {
  return ethers.HDNodeWallet.fromPhrase(
    mnemonic,
    undefined,
    `m/44'/60'/0'/0/${index}`
  );
}

function getArg(args, name, def) {
  const idx = args.indexOf(name);
  if (idx === -1) return def;
  const val = args[idx + 1];
  if (!val || val.startsWith("--")) {
    throw new Error(`Missing value for ${name}`);
  }
  return val;
}

async function rpcCall(url, method, params) {
  const body = JSON.stringify({
    jsonrpc: "2.0",
    id: 1,
    method,
    params,
  });
  const u = new URL(url);
  const client = u.protocol === "https:" ? https : http;

  return await new Promise((resolve, reject) => {
    const req = client.request(
      {
        hostname: u.hostname,
        port: u.port || (u.protocol === "https:" ? 443 : 80),
        path: u.pathname || "/",
        method: "POST",
        headers: {
          "content-type": "application/json",
          "content-length": Buffer.byteLength(body),
        },
      },
      (res) => {
        let data = "";
        res.setEncoding("utf8");
        res.on("data", (chunk) => (data += chunk));
        res.on("end", () => {
          try {
            const json = JSON.parse(data || "{}");
            if (json.error) {
              reject(new Error(`${json.error.code}: ${json.error.message}`));
              return;
            }
            resolve(json.result);
          } catch (err) {
            reject(err);
          }
        });
      }
    );
    req.on("error", reject);
    req.write(body);
    req.end();
  });
}

function parseListItem(raw) {
  if (raw && typeof raw === "object" && !Array.isArray(raw)) return raw;
  if (typeof raw === "string") {
    try {
      return JSON.parse(raw);
    } catch {
      const decoded = Buffer.from(raw, "base64").toString("utf8");
      return JSON.parse(decoded);
    }
  }
  return null;
}

async function main() {
  const args = process.argv.slice(2);
  const rpcUrl = getArg(args, "--rpc", "http://127.0.0.1:8545");
  const sellerKey = getArg(args, "--seller", "dev0");
  const auctionPrefix = getArg(args, "--auction-prefix", "auction-bulk");
  const denom = getArg(args, "--denom", "atest");
  const minPrice = getArg(args, "--min-price", "1000000000000000000");
  const endHeightStr = getArg(args, "--end-height", "12345");
  const askCount = Number(getArg(args, "--ask-count", "3"));
  const bidsPerAsk = Number(getArg(args, "--bids-per-ask", "2"));

  const sellerMnemonic = DEV_MNEMONICS[sellerKey];
  if (!sellerMnemonic) {
    throw new Error(`Unknown seller '${sellerKey}'. Use dev0|dev1|dev2|dev3`);
  }

  const sellerWallet = walletFromMnemonic(sellerMnemonic);
  const endHeight = Number(endHeightStr);
  if (!Number.isFinite(endHeight) || endHeight <= 0) {
    throw new Error("--end-height must be a positive number");
  }

  const bidders = ["dev1", "dev2", "dev3"]
    .filter((k) => k !== sellerKey)
    .map((k) => ({ key: k, wallet: walletFromMnemonic(DEV_MNEMONICS[k]) }));
  if (bidders.length === 0) {
    throw new Error("No bidders available (seller uses all dev keys)");
  }

  const auctionIds = [];
  for (let i = 0; i < askCount; i++) {
    const auctionId = `${auctionPrefix}-${i + 1}`;
    const askPayload = buildAskPayload({
      auction_id: auctionId,
      seller: sellerWallet.address,
      min_price: minPrice,
      denom,
      end_height: endHeight,
    });
    const askSig = await sellerWallet.signMessage(askPayload);
    const msg = {
      type: "ask",
      auction_id: auctionId,
      seller: sellerWallet.address,
      denom,
      min_price: minPrice,
      end_height: endHeight,
      item_meta: { name: `Item ${i + 1}`, note: "bulk ask" },
      sig: askSig,
    };
    await rpcCall(rpcUrl, "mvl_publishAsk", [msg]);
    auctionIds.push(auctionId);
  }
  console.log("published asks:", auctionIds.join(", "));

  for (const auctionId of auctionIds) {
    for (let i = 0; i < bidsPerAsk; i++) {
      const bidder = bidders[i % bidders.length];
      const price = (BigInt(minPrice) + BigInt(i) * 1000n).toString();
      const bidPayload = buildBidPayload({
        auction_id: auctionId,
        bidder: bidder.wallet.address,
        price,
        end_height: endHeight,
      });
      const bidSig = await bidder.wallet.signMessage(bidPayload);
      const bidMsg = {
        type: "bid",
        auction_id: auctionId,
        bidder: bidder.wallet.address,
        price,
        end_height: endHeight,
        sig: bidSig,
      };
      await rpcCall(rpcUrl, "mvl_publishBid", [bidMsg]);
    }
  }
  console.log("published bids per ask:", bidsPerAsk);

  for (const auctionId of auctionIds) {
    const res = await rpcCall(rpcUrl, "mvl_listBids", [auctionId, 0, 50]);
    const bids = (res?.bids || []).map(parseListItem).filter(Boolean);
    console.log(`bids for ${auctionId}: ${bids.length}`);
    for (const bid of bids) {
      console.log(
        `  - bidder=${bid.bidder} price=${bid.price} end_height=${bid.end_height}`
      );
    }
    if (bids.length > 0) {
      const top = bids.reduce((a, b) => {
        const ap = BigInt(a.price || "0");
        const bp = BigInt(b.price || "0");
        return bp > ap ? b : a;
      });
      console.log(`  top bid: bidder=${top.bidder} price=${top.price}`);
    }
  }
}

main().catch((err) => {
  console.error(err.message || err);
  process.exit(1);
});
