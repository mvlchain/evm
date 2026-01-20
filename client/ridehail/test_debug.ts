/**
 * Debug Test Script - Check transaction encoding
 */

import { ethers } from "ethers";

const RIDEHAIL_ABI = [
  "function createRequest(bytes32 cellTopic, bytes32 regionTopic, bytes32 paramsHash, bytes32 pickupCommit, bytes32 dropoffCommit, uint32 maxDriverEta, uint64 ttl) returns (uint256)",
];

const RIDEHAIL_ADDRESS = "0x000000000000000000000000000000000000080a";
const RPC_URL = "http://localhost:8545";
const CHAIN_ID = 262144;

async function main() {
  console.log("🔍 Debug Transaction Encoding\n");

  // Setup
  const provider = new ethers.JsonRpcProvider(RPC_URL, CHAIN_ID);
  const wallet = new ethers.Wallet(
    "0xE9B1D63E8ACD7FE676ACB43AFB390D4B0202DAB61ABEC9CF2A561E4BECB147DE",
    provider
  );

  console.log(`Wallet: ${wallet.address}`);
  const balance = await provider.getBalance(wallet.address);
  console.log(`Balance: ${ethers.formatEther(balance)} ETH\n`);

  // Create contract instance
  const contract = new ethers.Contract(RIDEHAIL_ADDRESS, RIDEHAIL_ABI, wallet);

  // Test parameters
  const cellTopic = ethers.hexlify(ethers.randomBytes(32));
  const regionTopic = ethers.hexlify(ethers.randomBytes(32));
  const paramsHash = ethers.hexlify(ethers.randomBytes(32));
  const pickupCommit = ethers.hexlify(ethers.randomBytes(32));
  const dropoffCommit = ethers.hexlify(ethers.randomBytes(32));
  const maxDriverEta = 300;
  const ttl = 600;

  console.log("📦 Test Parameters:");
  console.log(`  Cell Topic: ${cellTopic}`);
  console.log(`  Max Driver ETA: ${maxDriverEta}`);
  console.log(`  TTL: ${ttl}\n`);

  // Encode function call
  const iface = new ethers.Interface(RIDEHAIL_ABI);
  const encodedData = iface.encodeFunctionData("createRequest", [
    cellTopic,
    regionTopic,
    paramsHash,
    pickupCommit,
    dropoffCommit,
    maxDriverEta,
    ttl,
  ]);

  console.log("🔧 Encoded Function Data:");
  console.log(`  Length: ${encodedData.length} chars`);
  console.log(`  Data: ${encodedData.slice(0, 66)}... (truncated)`);
  console.log(`  Method ID: ${encodedData.slice(0, 10)}\n`);

  // Try sending transaction
  try {
    console.log("📤 Sending transaction manually...");

    // Manual transaction construction
    const txRequest = {
      to: RIDEHAIL_ADDRESS,
      from: wallet.address,
      data: encodedData,
      gasLimit: 5000000,
    };

    console.log(`   Transaction request:`, txRequest);

    const tx = await wallet.sendTransaction(txRequest);

    console.log(`✅ Transaction sent: ${tx.hash}`);
    console.log(`   Waiting for confirmation...`);

    const receipt = await tx.wait();
    if (!receipt) {
      throw new Error("Transaction receipt is null");
    }

    console.log(`✅ Transaction confirmed!`);
    console.log(`   Block: ${receipt.blockNumber}`);
    console.log(`   Status: ${receipt.status}`);
    console.log(`   Gas Used: ${receipt.gasUsed.toString()}\n`);

    // Check if there are logs
    if (receipt.logs && receipt.logs.length > 0) {
      console.log("📋 Logs:");
      for (const log of receipt.logs) {
        try {
          const parsed = contract.interface.parseLog(log);
          console.log(`   Event: ${parsed?.name}`);
          console.log(`   Args:`, parsed?.args);
        } catch (e) {
          console.log(`   Raw log:`, log);
        }
      }
    }

  } catch (error: any) {
    console.error("\n❌ Transaction failed:");
    console.error(`   Message: ${error.message}`);

    if (error.receipt) {
      console.error(`   Block: ${error.receipt.blockNumber}`);
      console.error(`   Status: ${error.receipt.status}`);
      console.error(`   Gas Used: ${error.receipt.gasUsed.toString()}`);
    }

    if (error.data) {
      console.error(`   Error Data: ${error.data}`);
    }

    throw error;
  }
}

main()
  .then(() => {
    console.log("\n✅ Debug test passed!");
    process.exit(0);
  })
  .catch((error) => {
    console.error("\n❌ Debug test failed:", error);
    process.exit(1);
  });
