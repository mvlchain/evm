/**
 * Direct state reading test - bypass transaction and read state directly
 */

import { ethers } from "ethers";

const RIDEHAIL_PRECOMPILE_ADDRESS = "0x000000000000000000000000000000000000080a";

async function main() {
  console.log("\n=== Direct State Reading Test ===\n");

  const provider = new ethers.JsonRpcProvider("http://localhost:8545");

  // Calculate the storage slot for nextRequestId
  // slot = keccak256("rh.nextRequestId")
  const slot = ethers.keccak256(ethers.toUtf8Bytes("rh.nextRequestId"));
  console.log(`Storage slot for nextRequestId: ${slot}`);

  // Read storage directly
  const value = await provider.getStorage(RIDEHAIL_PRECOMPILE_ADDRESS, slot);
  console.log(`Raw storage value: ${value}`);
  console.log(`Decoded value: ${BigInt(value)}`);

  // Also test with getStorageAt at slot 0
  const slot0 = await provider.getStorage(RIDEHAIL_PRECOMPILE_ADDRESS, 0);
  console.log(`\nStorage at slot 0: ${slot0}`);
  console.log(`Decoded: ${BigInt(slot0)}`);

  console.log("\n=== Test Complete ===\n");
}

main().catch(console.error);
