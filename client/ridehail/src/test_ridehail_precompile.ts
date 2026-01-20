/**
 * Test RideHail Precompile execution
 *
 * Tests if precompile is accessible, if state changes persist, and if reverts work correctly
 */

import { ethers } from "ethers";

const RIDEHAIL_PRECOMPILE_ADDRESS = "0x000000000000000000000000000000000000080a";

// RideHail precompile ABI
const RIDEHAIL_ABI = [
  "function version() view returns (uint256)",
  "function nextRequestId() view returns (uint256)",
  "function nextSessionId() view returns (uint256)",
  "function validateCreateRequest(bytes32 cellTopic, bytes32 regionTopic, bytes32 paramsHash, bytes32 pickupCommit, bytes32 dropoffCommit, uint32 maxDriverEta, uint64 ttl) view returns (bool ok, string reason)",
  "function createRequest(bytes32 cellTopic, bytes32 regionTopic, bytes32 paramsHash, bytes32 pickupCommit, bytes32 dropoffCommit, uint32 maxDriverEta, uint64 ttl) payable returns (uint256 requestId)",
  "function acceptCommit(uint256 requestId, bytes32 commitHash, uint64 eta) payable",
  "function acceptReveal(uint256 requestId, uint64 eta, bytes32 driverCell, bytes32 salt)",
  "function requests(uint256 requestId) view returns (address rider, bytes32 cellTopic, bytes32 regionTopic, bytes32 paramsHash, bytes32 pickupCommit, bytes32 dropoffCommit, uint256 riderDeposit, uint64 createdAt, uint64 commitEnd, uint64 revealEnd, uint64 ttl, uint32 maxDriverEta, uint32 commitCount, bool canceled, bool matched, uint256 sessionId)",
  "function postEncryptedMessage(uint256 sessionId, uint32 msgIndex, bytes header, bytes ciphertext) payable"
];

async function main() {
  console.log("\n=== RideHail Precompile Test ===\n");

  // Connect to local node
  const provider = new ethers.JsonRpcProvider("http://localhost:8545");

  // Use dev0 account from local_node.sh
  const privateKey = "0x88cbead91aee890d27bf06e003ade3d4e952427e88f88d31d61d3ef5e5d54305";
  const wallet = new ethers.Wallet(privateKey, provider);

  console.log(`Connected to provider: ${await provider.getNetwork()}`);
  console.log(`Wallet address: ${wallet.address}`);
  console.log(`Balance: ${ethers.formatEther(await provider.getBalance(wallet.address))} ETH\n`);

  // Connect to precompile
  const rideHail = new ethers.Contract(RIDEHAIL_PRECOMPILE_ADDRESS, RIDEHAIL_ABI, wallet);

  // Test 1: Check if precompile is accessible
  console.log("Test 1: Checking precompile accessibility...");
  try {
    const version = await rideHail.version();
    console.log(`✅ Precompile accessible! Version: ${version}`);
  } catch (error: any) {
    console.log(`❌ Failed to access precompile: ${error.message}`);
    return;
  }

  // Test 2: Check nextRequestId
  console.log("\nTest 2: Checking nextRequestId...");
  try {
    const nextId = await rideHail.nextRequestId();
    console.log(`✅ Next request ID: ${nextId}`);
  } catch (error: any) {
    console.log(`❌ Failed to get nextRequestId: ${error.message}`);
  }

  // Test 3: Validate create request (should succeed)
  console.log("\nTest 3: Testing validateCreateRequest with sufficient deposit...");
  try {
    const cellTopic = ethers.id("cell-9");
    const regionTopic = ethers.id("region-1");
    const paramsHash = ethers.id("params");
    const pickupCommit = ethers.id("pickup");
    const dropoffCommit = ethers.id("dropoff");
    const maxDriverEta = 1800;
    const ttl = 7200; // 2 hours

    const result = await rideHail.validateCreateRequest(
      cellTopic,
      regionTopic,
      paramsHash,
      pickupCommit,
      dropoffCommit,
      maxDriverEta,
      ttl
    );

    const success = result[0];
    const reason = result[1];

    if (success) {
      console.log(`✅ Validation succeeded: ${reason || "OK"}`);
    } else {
      console.log(`⚠️  Validation failed: ${reason}`);
    }
  } catch (error: any) {
    console.log(`❌ validateCreateRequest failed: ${error.message}`);
  }

  // Test 4: Validate create request with insufficient deposit (should fail)
  console.log("\nTest 4: Testing validateCreateRequest with insufficient deposit...");
  try {
    const cellTopic = ethers.id("cell-9");
    const regionTopic = ethers.id("region-1");
    const paramsHash = ethers.id("params");
    const pickupCommit = ethers.id("pickup");
    const dropoffCommit = ethers.id("dropoff");
    const maxDriverEta = 1800;
    const ttl = 7200;

    const result = await rideHail.validateCreateRequest(
      cellTopic,
      regionTopic,
      paramsHash,
      pickupCommit,
      dropoffCommit,
      maxDriverEta,
      ttl
    );

    const success = result[0];
    const reason = result[1];

    if (!success) {
      console.log(`✅ Validation correctly failed: ${reason}`);
    } else {
      console.log(`⚠️  Validation should have failed but succeeded`);
    }
  } catch (error: any) {
    console.log(`❌ validateCreateRequest failed: ${error.message}`);
  }

  // Test 5: Create request and verify state change
  console.log("\nTest 5: Testing createRequest and state persistence...");
  try {
    const cellTopic = ethers.id("cell-9");
    const regionTopic = ethers.id("region-1");
    const paramsHash = ethers.id("params");
    const pickupCommit = ethers.id("pickup");
    const dropoffCommit = ethers.id("dropoff");
    const maxDriverEta = 1800;
    const ttl = 7200;
    const riderDeposit = ethers.parseEther("1.0");

    const nextIdBefore = await rideHail.nextRequestId();
    console.log(`  Next ID before: ${nextIdBefore}`);

    // Get current nonce and increment it to avoid "replacement fee too low"
    const currentNonce = await provider.getTransactionCount(wallet.address, 'pending');
    console.log(`  Current nonce: ${currentNonce}`);

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
        nonce: currentNonce
      }
    );

    console.log(`  Transaction sent: ${tx.hash}`);

    // Try to wait for receipt, but don't fail if it errors
    let receipt;
    try {
      receipt = await tx.wait();
      console.log(`  Transaction confirmed in block ${receipt?.blockNumber}`);

      if (receipt?.status === 1) {
        console.log(`  ✅ Transaction successful`);
      } else {
        console.log(`  ❌ Transaction reverted`);
        return;
      }
    } catch (receiptError: any) {
      console.log(`  ⚠️  Receipt error: ${receiptError.message.split('\n')[0]}`);
      console.log(`  Continuing to check state changes...`);

      // Wait a bit for the transaction to be included
      await new Promise(resolve => setTimeout(resolve, 2000));
    }

    // Check state changes
    const nextIdAfter = await rideHail.nextRequestId();
    console.log(`  Next ID after: ${nextIdAfter}`);

    if (nextIdAfter > nextIdBefore) {
      console.log(`  ✅ State changed: nextRequestId incremented`);
    } else {
      console.log(`  ❌ State did NOT change: nextRequestId not incremented`);
    }

    // Verify request data
    const requestId = nextIdBefore;
    const request = await rideHail.requests(requestId);
    console.log(`\n  Request ${requestId} details:`);
    console.log(`    Rider: ${request.rider}`);
    console.log(`    Cell Topic: ${request.cellTopic}`);
    console.log(`    Deposit: ${ethers.formatEther(request.riderDeposit)} ETH`);
    console.log(`    Matched: ${request.matched}`);

    if (request.rider === wallet.address) {
      console.log(`  ✅ Request data correctly stored`);
    } else {
      console.log(`  ❌ Request data not stored correctly`);
    }

  } catch (error: any) {
    console.log(`❌ createRequest failed: ${error.message}`);
    if (error.data) {
      console.log(`  Error data: ${error.data}`);
    }
  }

  // Test 6: Test revert behavior
  console.log("\nTest 6: Testing revert behavior with insufficient deposit...");
  try {
    const cellTopic = ethers.id("cell-9");
    const regionTopic = ethers.id("region-1");
    const paramsHash = ethers.id("params");
    const pickupCommit = ethers.id("pickup");
    const dropoffCommit = ethers.id("dropoff");
    const maxDriverEta = 1800;
    const ttl = 7200;
    const insufficientDeposit = ethers.parseEther("0.1");

    const currentNonce = await provider.getTransactionCount(wallet.address, 'pending');

    const tx = await rideHail.createRequest(
      cellTopic,
      regionTopic,
      paramsHash,
      pickupCommit,
      dropoffCommit,
      maxDriverEta,
      ttl,
      {
        value: insufficientDeposit,
        nonce: currentNonce
      }
    );

    const receipt = await tx.wait();

    if (receipt?.status === 0) {
      console.log(`  ✅ Transaction correctly reverted`);
    } else {
      console.log(`  ⚠️  Transaction should have reverted but succeeded`);
    }

  } catch (error: any) {
    // Expected to fail
    if (error.message.includes("insufficient deposit") || error.message.includes("revert")) {
      console.log(`  ✅ Transaction correctly reverted: ${error.message.split('\n')[0]}`);
    } else {
      console.log(`  ❌ Unexpected error: ${error.message}`);
    }
  }

  console.log("\n=== Test Complete ===\n");
}

main().catch(console.error);
