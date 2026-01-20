/**
 * Test Offchain Matching Flow
 * 1. Rider creates request
 * 2. Check if request is stored in Core
 * 3. Driver submits commit
 * 4. Check BeginBlocker matching
 */

import { ethers } from "ethers";

const RIDEHAIL_ABI = [
  "function createRequest(bytes32 cellTopic, bytes32 regionTopic, bytes32 paramsHash, bytes32 pickupCommit, bytes32 dropoffCommit, uint32 maxDriverEta, uint64 ttl) returns (uint256)",
  "function acceptCommit(uint256 requestId, bytes32 commitHash, uint64 eta)",
  "function nextRequestId() view returns (uint256)",
  "function requests(uint256 requestId) view returns (address rider, bytes32 cellTopic, bytes32 regionTopic, bytes32 paramsHash, bytes32 pickupCommit, bytes32 dropoffCommit, uint256 riderDeposit, uint64 createdAt, uint64 commitEnd, uint64 revealEnd, uint64 ttl, uint32 maxDriverEta, uint32 commitCount, bool canceled, bool matched, uint256 sessionId)",
];

const RIDEHAIL_ADDRESS = "0x000000000000000000000000000000000000080a";
const RPC_URL = "http://localhost:8545";
const CHAIN_ID = 262144;

async function main() {
  console.log("🧪 Testing Offchain Matching Flow\n");

  const provider = new ethers.JsonRpcProvider(RPC_URL, CHAIN_ID);

  // Rider wallet
  const riderWallet = new ethers.Wallet(
    "0xE9B1D63E8ACD7FE676ACB43AFB390D4B0202DAB61ABEC9CF2A561E4BECB147DE",
    provider
  );

  // Driver wallet
  const driverWallet = new ethers.Wallet(
    "0x88CBEAD91AEE890D27BF06E003ADE3D4E952427E88F88D31D61D3EF5E5D54305",
    provider
  );

  console.log(`Rider: ${riderWallet.address}`);
  console.log(`Driver: ${driverWallet.address}\n`);

  const contract = new ethers.Contract(RIDEHAIL_ADDRESS, RIDEHAIL_ABI, riderWallet) as any;

  // Step 1: Get current request ID
  console.log("📋 Step 1: Check current request ID");
  const currentRequestId = await contract.nextRequestId();
  console.log(`   Current nextRequestId: ${currentRequestId.toString()}\n`);

  // Step 2: Rider creates request
  console.log("🚕 Step 2: Rider creates request");
  const cellTopic = ethers.hexlify(ethers.randomBytes(32));
  const regionTopic = ethers.hexlify(ethers.randomBytes(32));
  const paramsHash = ethers.hexlify(ethers.randomBytes(32));
  const pickupCommit = ethers.hexlify(ethers.randomBytes(32));
  const dropoffCommit = ethers.hexlify(ethers.randomBytes(32));
  const maxDriverEta = 300;
  const ttl = 600;

  const tx1 = await contract.createRequest(
    cellTopic,
    regionTopic,
    paramsHash,
    pickupCommit,
    dropoffCommit,
    maxDriverEta,
    ttl,
    { gasLimit: 5000000 }
  );
  console.log(`   TX sent: ${tx1.hash}`);

  const receipt1 = await tx1.wait();
  if (!receipt1) throw new Error("Receipt is null");

  console.log(`   ✅ Request created! Block: ${receipt1.blockNumber}, Status: ${receipt1.status}\n`);

  // Step 3: Query the request from Core
  console.log("🔍 Step 3: Query request from Core");
  const requestId = currentRequestId;
  const requestData = await contract.requests(requestId);
  console.log(`   Request ID: ${requestId.toString()}`);
  console.log(`   Rider: ${requestData.rider}`);
  console.log(`   Cell Topic: ${requestData.cellTopic}`);
  console.log(`   Created At: ${requestData.createdAt.toString()}`);
  console.log(`   Max Driver ETA: ${requestData.maxDriverEta}`);
  console.log(`   Matched: ${requestData.matched}`);
  console.log(`   Canceled: ${requestData.canceled}\n`);

  if (requestData.rider === ethers.ZeroAddress) {
    console.log("❌ Request was NOT stored in Core!");
    return;
  }
  console.log("✅ Request IS stored in Core!\n");

  // Step 4: Driver submits commit
  console.log("🚗 Step 4: Driver submits commit");

  // Generate commit: hash(driverCell || salt)
  const driverCell = ethers.hexlify(ethers.randomBytes(32));
  const salt = ethers.hexlify(ethers.randomBytes(32));
  const commitHash = ethers.keccak256(
    ethers.solidityPacked(["bytes32", "bytes32"], [driverCell, salt])
  );
  const eta = 180;

  const driverContract = contract.connect(driverWallet);
  const tx2 = await driverContract.acceptCommit(
    requestId,
    commitHash,
    eta,
    { gasLimit: 5000000 }
  );
  console.log(`   TX sent: ${tx2.hash}`);

  const receipt2 = await tx2.wait();
  if (!receipt2) throw new Error("Receipt is null");

  console.log(`   ✅ Commit submitted! Block: ${receipt2.blockNumber}, Status: ${receipt2.status}\n`);

  // Step 5: Wait for BeginBlocker to run (next block)
  console.log("⏳ Step 5: Waiting for BeginBlocker to process matching...");
  await new Promise((resolve) => setTimeout(resolve, 3000));

  // Step 6: Check if matched
  console.log("\n🔍 Step 6: Check if request was matched");
  const updatedRequest = await contract.requests(requestId);
  console.log(`   Matched: ${updatedRequest.matched}`);
  console.log(`   Session ID: ${updatedRequest.sessionId.toString()}`);

  if (updatedRequest.matched) {
    console.log("\n✅ SUCCESS! Offchain matching worked!");
    console.log(`   Session ID: ${updatedRequest.sessionId.toString()}`);
  } else {
    console.log("\n⚠️  Request not matched yet (might need more time or reveal phase)");
  }
}

main()
  .then(() => {
    console.log("\n✅ Test completed!");
    process.exit(0);
  })
  .catch((error) => {
    console.error("\n❌ Test failed:", error);
    process.exit(1);
  });
