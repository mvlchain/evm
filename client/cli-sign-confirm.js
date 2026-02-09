#!/usr/bin/env node

import { ethers } from "ethers";
import { buildConfirmPayload, signSellerConfirm } from "./auction-sign.js";

function parseArgs(argv) {
  const args = {};
  for (let i = 2; i < argv.length; i++) {
    const key = argv[i];
    if (!key.startsWith("--")) continue;
    const name = key.slice(2);
    const value = argv[i + 1];
    args[name] = value;
    i++;
  }
  return args;
}

function requireArg(args, name) {
  if (!args[name]) {
    console.error(`Missing --${name}`);
    process.exit(1);
  }
  return args[name];
}

async function main() {
  const args = parseArgs(process.argv);
  const priv = process.env.PRIVKEY;
  if (!priv) {
    console.error("Missing PRIVKEY env var");
    process.exit(1);
  }

  const payload = buildConfirmPayload({
    auction_id: requireArg(args, "auction_id"),
    seller: requireArg(args, "seller"),
    winner: requireArg(args, "winner"),
    price: requireArg(args, "price"),
    denom: requireArg(args, "denom"),
    end_height: requireArg(args, "end_height"),
    ask_sig: requireArg(args, "ask_sig"),
    bid_sig: requireArg(args, "bid_sig"),
  });

  const wallet = new ethers.Wallet(priv);
  const sig = await signSellerConfirm(wallet, payload);
  console.log(sig);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
