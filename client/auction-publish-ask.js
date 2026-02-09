import { ethers } from "ethers";
import http from "node:http";
import https from "node:https";
import { URL } from "node:url";
import { spawnSync } from "node:child_process";
import {
  buildAskPayload,
  buildBidPayload,
  buildConfirmPayload,
  signSellerConfirm,
} from "./auction-sign.js";

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

async function main() {
  const args = process.argv.slice(2);
  const rpcUrl = getArg(args, "--rpc", "http://127.0.0.1:8545");
  const sellerKey = getArg(args, "--seller", "dev0");
  const bidderKey = getArg(args, "--bidder", "dev1");
  const auctionId = getArg(args, "--auction-id", "auction-1");
  const denom = getArg(args, "--denom", "atest");
  const minPrice = getArg(args, "--min-price", "1000000000000000000");
  const endHeightStr = getArg(args, "--end-height", "12345");
  const evmdBin = getArg(args, "--evmd", "evmd");
  const chainId = getArg(args, "--chain-id", "9001");
  const nodeRpc = getArg(args, "--node", "tcp://127.0.0.1:26657");
  const homeDir = getArg(args, "--home", `${process.env.HOME}/.evmd`);
  const gasPrices = getArg(args, "--gas-prices", "0atest");
  const skipConfirm = args.includes("--no-confirm");

  const sellerMnemonic = DEV_MNEMONICS[sellerKey];
  if (!sellerMnemonic) {
    throw new Error(`Unknown seller '${sellerKey}'. Use dev0|dev1|dev2|dev3`);
  }
  const bidderMnemonic = DEV_MNEMONICS[bidderKey];
  if (!bidderMnemonic) {
    throw new Error(`Unknown bidder '${bidderKey}'. Use dev0|dev1|dev2|dev3`);
  }

  const sellerWallet = walletFromMnemonic(sellerMnemonic);
  const bidderWallet = walletFromMnemonic(bidderMnemonic);
  const endHeight = Number(endHeightStr);
  if (!Number.isFinite(endHeight) || endHeight <= 0) {
    throw new Error("--end-height must be a positive number");
  }

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
    item_meta: {
      name: "Sample item",
      note: "local dev ask",
    },
    sig: askSig,
  };

  const publishAskResult = await rpcCall(rpcUrl, "mvl_publishAsk", [msg]);
  console.log("publishAsk:", publishAskResult);

  const listItems = await rpcCall(rpcUrl, "mvl_listItems", [20, "0"]);
  const items = listItems?.items || [];
  const found = items.some((raw) => {
    try {
      if (raw && typeof raw === "object" && !Array.isArray(raw)) {
        return raw.auction_id === auctionId;
      }
      if (typeof raw === "string") {
        try {
          const parsed = JSON.parse(raw);
          return parsed.auction_id === auctionId;
        } catch {
          const decoded = Buffer.from(raw, "base64").toString("utf8");
          const parsed = JSON.parse(decoded);
          return parsed.auction_id === auctionId;
        }
      }
      return false;
    } catch {
      return false;
    }
  });
  if (!found) {
    throw new Error(`Published ask not found in mvl_listItems for ${auctionId}`);
  }
  console.log("listItems: found", auctionId);

  const bidPayload = buildBidPayload({
    auction_id: auctionId,
    bidder: bidderWallet.address,
    price: minPrice,
    end_height: endHeight,
  });
  const bidSig = await bidderWallet.signMessage(bidPayload);
  const bidMsg = {
    type: "bid",
    auction_id: auctionId,
    bidder: bidderWallet.address,
    price: minPrice,
    end_height: endHeight,
    sig: bidSig,
  };
  const publishBidResult = await rpcCall(rpcUrl, "mvl_publishBid", [bidMsg]);
  console.log("publishBid:", publishBidResult);

  const payload = buildConfirmPayload({
    auction_id: auctionId,
    seller: sellerWallet.address,
    winner: bidderWallet.address,
    price: minPrice,
    denom,
    end_height: endHeight,
    ask_sig: askSig,
    bid_sig: bidSig,
  });
  const sellerSig = await signSellerConfirm(sellerWallet, payload);

  if (!skipConfirm) {
    const txArgs = [
      "tx",
      "auction",
      "confirm",
      auctionId,
      sellerWallet.address,
      bidderWallet.address,
      minPrice,
      denom,
      String(endHeight),
      askSig,
      bidSig,
      sellerSig,
      "--from",
      sellerKey,
      "--chain-id",
      chainId,
      "--node",
      nodeRpc,
      "--home",
      homeDir,
      "--keyring-backend",
      "test",
      "--gas-prices",
      gasPrices,
    ];

    const tx = spawnSync(evmdBin, txArgs, { stdio: "inherit" });
    if (tx.status !== 0) {
      throw new Error("evmd tx auction confirm failed");
    }

    const onChain = await rpcCall(rpcUrl, "mvl_getAuction", [auctionId]);
    console.log("mvl_getAuction:", onChain);
  } else {
    console.log("confirm skipped (--no-confirm)");
  }

  console.log("seller:", sellerWallet.address);
  console.log("winner:", bidderWallet.address);
}

main().catch((err) => {
  console.error(err.message || err);
  process.exit(1);
});
