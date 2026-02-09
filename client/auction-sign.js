import { ethers } from "ethers";

export function buildConfirmPayload({
  auction_id,
  seller,
  winner,
  price,
  denom,
  end_height,
  ask_sig,
  bid_sig,
}) {
  return [
    auction_id,
    seller.toLowerCase(),
    winner.toLowerCase(),
    price,
    denom,
    String(end_height),
    ask_sig,
    bid_sig,
  ].join("|");
}

export function buildAskPayload({
  auction_id,
  seller,
  min_price,
  denom,
  end_height,
}) {
  return [
    auction_id,
    seller.toLowerCase(),
    min_price,
    denom,
    String(end_height),
  ].join("|");
}

export function buildBidPayload({
  auction_id,
  bidder,
  price,
  end_height,
}) {
  return [
    auction_id,
    bidder.toLowerCase(),
    price,
    String(end_height),
  ].join("|");
}

export function buildCancelPayload({ auction_id, actor }) {
  return [auction_id, actor.toLowerCase()].join("|");
}

export async function signSellerConfirm(wallet, payload) {
  return await wallet.signMessage(payload);
}

// Example:
// const wallet = new ethers.Wallet(process.env.PRIVKEY);
// const payload = buildConfirmPayload({ ... });
// const seller_sig = await signSellerConfirm(wallet, payload);
