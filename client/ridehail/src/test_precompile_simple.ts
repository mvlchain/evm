/**
 * Simple precompile test to debug state changes
 */

import { ethers } from "ethers";

const RIDEHAIL_PRECOMPILE_ADDRESS = "0x000000000000000000000000000000000000080a";

const RIDEHAIL_ABI = [
  "function version() view returns (uint256)",
  "function nextRequestId() view returns (uint256)",
  "function createRequest(bytes32 cellTopic, bytes32 regionTopic, bytes32 paramsHash, bytes32 pickupCommit, bytes32 dropoffCommit, uint32 maxDriverEta, uint64 ttl) payable returns (uint256 requestId)",
  "function requests(uint256 requestId) view returns (address rider, bytes32 cellTopic, bytes32 regionTopic, bytes32 paramsHash, bytes32 pickupCommit, bytes32 dropoffCommit, uint256 riderDeposit, uint64 createdAt, uint64 commitEnd, uint64 revealEnd, uint64 ttl, uint32 maxDriverEta, uint32 commitCount, bool canceled, bool matched, uint256 sessionId)"
];

async function main() {
  console.log("\n=== Simple RideHail Precompile Test ===\n");

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
  const nextIdBefore = await rideHail.nextRequestId();
  console.log(`NextRequestId before: ${nextIdBefore}\n`);

  // Try to create request with detailed error handling
  console.log("Attempting to create request...");

  try {
    const cellTopic = ethers.id("cell-9");
    const regionTopic = ethers.id("region-1");
    const paramsHash = ethers.id("params");
    const pickupCommit = ethers.id("pickup");
    const dropoffCommit = ethers.id("dropoff");
    const maxDriverEta = 1800;
    const ttl = 7200;
    const riderDeposit = ethers.parseEther("1.0");

    // Try to estimate gas first
    console.log("Estimating gas...");
    try {
      const gasEstimate = await rideHail.createRequest.estimateGas(
        cellTopic,
        regionTopic,
        paramsHash,
        pickupCommit,
        dropoffCommit,
        maxDriverEta,
        ttl,
        { value: riderDeposit }
      );
      console.log(`Gas estimate: ${gasEstimate}`);
    } catch (gasError: any) {
      console.log(`❌ Gas estimation failed: ${gasError.message}`);
      if (gasError.data) {
        console.log(`  Error data: ${gasError.data}`);
      }
      if (gasError.code) {
        console.log(`  Error code: ${gasError.code}`);
      }
      console.log("\nTrying to send transaction anyway...");
    }

    // Send transaction
    const nonce = await provider.getTransactionCount(wallet.address, 'pending');
    console.log(`Nonce: ${nonce}`);

    const tx = await rideHail.createRequest(
      cellTopic,
      regionTopic,
      paramsHash,
      pickupCommit,
      dropoffCommit,
      maxDriverEta,
      ttl,
      {
        value: riderDeposit,
        nonce: nonce,
        gasLimit: 500000n // Explicit gas limit
      }
    );

    console.log(`Transaction hash: ${tx.hash}`);
    console.log("Waiting 3 seconds for transaction to be mined...");
    await new Promise(resolve => setTimeout(resolve, 3000));

    // Check state changes
    const nextIdAfter = await rideHail.nextRequestId();
    console.log(`\nNextRequestId after: ${nextIdAfter}`);

    if (nextIdAfter > nextIdBefore) {
      console.log("✅ State changed successfully!");

      // Get request details
      const requestId = nextIdBefore;
      const request = await rideHail.requests(requestId);
      console.log(`\nRequest ${requestId} details:`);
      console.log(`  Rider: ${request.rider}`);
      console.log(`  Cell: ${request.cellTopic}`);
      console.log(`  Deposit: ${ethers.formatEther(request.riderDeposit)} ETH`);
    } else {
      console.log("❌ State did NOT change");

      // Try to get transaction via RPC
      console.log("\nTrying to get transaction details...");
      try {
        const txData = await provider.getTransaction(tx.hash);
        if (txData) {
          console.log(`  From: ${txData.from}`);
          console.log(`  To: ${txData.to}`);
          console.log(`  Value: ${ethers.formatEther(txData.value || 0n)} ETH`);
          console.log(`  Gas Limit: ${txData.gasLimit}`);
          console.log(`  Nonce: ${txData.nonce}`);
          console.log(`  Block Number: ${txData.blockNumber}`);
          console.log(`  Block Hash: ${txData.blockHash}`);
        } else {
          console.log("  Transaction not found in mempool/blockchain");
        }
      } catch (error: any) {
        console.log(`  Failed to get transaction: ${error.message}`);
      }
    }

  } catch (error: any) {
    console.log(`\n❌ Error: ${error.message}`);
    if (error.data) {
      console.log(`  Error data: ${error.data}`);
    }
    if (error.transaction) {
      console.log(`  Transaction data: ${JSON.stringify(error.transaction, null, 2)}`);
    }
  }

  console.log("\n=== Test Complete ===\n");
}

main().catch(console.error);
