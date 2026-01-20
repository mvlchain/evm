/**
 * Test Bech32 precompile to verify precompiles work
 */

import { ethers } from "ethers";

const BECH32_PRECOMPILE = "0x0000000000000000000000000000000000000800";

const BECH32_ABI = [
  "function hexToBech32(address addr, string prefix) returns (string bech32Address)"
];

async function main() {
  console.log("\n=== Bech32 Precompile Test ===\n");

  const provider = new ethers.JsonRpcProvider("http://localhost:8545");
  const privateKey = "0x88cbead91aee890d27bf06e003ade3d4e952427e88f88d31d61d3ef5e5d54305";
  const wallet = new ethers.Wallet(privateKey, provider);

  console.log(`Wallet: ${wallet.address}`);

  const bech32 = new ethers.Contract(BECH32_PRECOMPILE, BECH32_ABI, wallet);

  try {
    const tx = await bech32.hexToBech32(wallet.address, "cosmos");
    console.log(`Transaction sent: ${tx.hash}`);
    const receipt = await tx.wait();
    console.log(`✅ Bech32 precompile works!`);
    console.log(`   Status: ${receipt.status}`);
  } catch (error: any) {
    console.log(`❌ Bech32 precompile failed: ${error.message}`);
  }

  console.log("\n=== Test Complete ===\n");
}

main().catch(console.error);
