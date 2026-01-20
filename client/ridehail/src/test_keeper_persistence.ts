/**
 * Test Keeper persistence WITHOUT deposit (to avoid precision bank issue)
 */

import { ethers } from "ethers";

const RIDEHAIL_PRECOMPILE_ADDRESS = "0x000000000000000000000000000000000000080a";

const RIDEHAIL_ABI = [
  "function version() view returns (uint256)",
  "function nextRequestId() view returns (uint256)",
  "function nextSessionId() view returns (uint256)",
  "function createRequest(bytes32 cellTopic, bytes32 regionTopic, bytes32 paramsHash, bytes32 pickupCommit, bytes32 dropoffCommit, uint32 maxDriverEta, uint64 ttl) payable returns (uint256 requestId)",
];

async function main() {
  console.log("\n=== Testing Keeper Persistence (No Deposit) ===\n");

  const provider = new ethers.JsonRpcProvider("http://localhost:8545");
  const privateKey = "0x88cbead91aee890d27bf06e003ade3d4e952427e88f88d31d61d3ef5e5d54305";
  const wallet = new ethers.Wallet(privateKey, provider);

  console.log(`Wallet: ${wallet.address}`);
  console.log(`Balance: ${ethers.formatEther(await provider.getBalance(wallet.address))} ETH\n`);

  const rideHail = new ethers.Contract(RIDEHAIL_PRECOMPILE_ADDRESS, RIDEHAIL_ABI, wallet);

  // Check version
  const version = await rideHail.version();
  console.log(`Version: ${version}`);

  // Check nextRequestId before
  const nextRequestIdBefore = await rideHail.nextRequestId();
  console.log(`NextRequestId before: ${nextRequestIdBefore}`);

  // Check nextSessionId before
  const nextSessionIdBefore = await rideHail.nextSessionId();
  console.log(`NextSessionId before: ${nextSessionIdBefore}\n`);

  // Try to create request WITHOUT value (to test Keeper persistence without deposit issue)
  console.log("Creating request without deposit...");

  try {
    const cellTopic = ethers.id("cell-9");
    const regionTopic = ethers.id("region-1");
    const paramsHash = ethers.id("params");
    const pickupCommit = ethers.id("pickup");
    const dropoffCommit = ethers.id("dropoff");
    const maxDriverEta = 1800;
    const ttl = 7200;

    const tx = await rideHail.createRequest(
      cellTopic,
      regionTopic,
      paramsHash,
      pickupCommit,
      dropoffCommit,
      maxDriverEta,
      ttl,
      {
        value: 0n, // No deposit to avoid precision bank error
        gasLimit: 500000n
      }
    );

    console.log(`Transaction hash: ${tx.hash}`);
    console.log("Waiting for transaction to be mined...");

    const receipt = await tx.wait();
    console.log(`✅ Transaction mined in block ${receipt.blockNumber}`);
    console.log(`Gas used: ${receipt.gasUsed}\n`);

    // Check if state changed
    const nextRequestIdAfter = await rideHail.nextRequestId();
    console.log(`NextRequestId after: ${nextRequestIdAfter}`);

    const nextSessionIdAfter = await rideHail.nextSessionId();
    console.log(`NextSessionId after: ${nextSessionIdAfter}\n`);

    if (nextRequestIdAfter > nextRequestIdBefore) {
      console.log("✅ SUCCESS: nextRequestId incremented!");
      console.log(`   Before: ${nextRequestIdBefore} → After: ${nextRequestIdAfter}`);
      console.log("\n🎉 Keeper persistence is working!");
    } else {
      console.log("❌ FAIL: nextRequestId did NOT increment");
      console.log(`   Still at: ${nextRequestIdAfter}`);
    }

  } catch (error: any) {
    console.log(`\n❌ Error: ${error.message}`);
    if (error.data) {
      console.log(`  Error data: ${error.data}`);
    }
    if (error.reason) {
      console.log(`  Reason: ${error.reason}`);
    }
  }

  console.log("\n=== Test Complete ===\n");
}

main().catch(console.error);
