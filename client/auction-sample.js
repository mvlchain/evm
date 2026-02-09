import { ethers } from "ethers";
import { buildConfirmPayload, signSellerConfirm } from "./auction-sign.js";

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

async function main() {
  const sellerWallet = walletFromMnemonic(DEV_MNEMONICS.dev0);
  const winnerWallet = walletFromMnemonic(DEV_MNEMONICS.dev1);

  const payload = buildConfirmPayload({
    auction_id: "auction-1",
    seller: sellerWallet.address,
    winner: winnerWallet.address,
    price: "1000000000000000000", // 1 token in wei-like units
    denom: "atest",
    end_height: 12345,
    ask_sig: "0xASKSIG_PLACEHOLDER",
    bid_sig: "0xBIDSIG_PLACEHOLDER",
  });

  const seller_sig = await signSellerConfirm(sellerWallet, payload);
  const winner_sig = await winnerWallet.signMessage(payload);

  console.log("seller:", sellerWallet.address);
  console.log("winner:", winnerWallet.address);
  console.log("payload:", payload);
  console.log("seller_sig:", seller_sig);
  console.log("winner_sig:", winner_sig);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
