/**
 * Check the status of a pending transaction
 */

import { ethers } from "ethers";

async function main() {
  const provider = new ethers.JsonRpcProvider("http://localhost:8545");

  const txHash = process.argv[2] || "0xde5911bd3ee6752d0a8ee820ae042b989e5704ed42bc04fcc81a9966fe856cac";

  console.log(`\nChecking transaction: ${txHash}\n`);

  try {
    // Check if transaction exists in mempool/blockchain
    console.log("1. Getting transaction...");
    const tx = await provider.getTransaction(txHash);
    if (tx) {
      console.log(`  ✅ Transaction found in ${tx.blockNumber ? 'blockchain' : 'mempool'}`);
      console.log(`  From: ${tx.from}`);
      console.log(`  To: ${tx.to}`);
      console.log(`  Value: ${ethers.formatEther(tx.value)} ETH`);
      console.log(`  Gas Limit: ${tx.gasLimit}`);
      console.log(`  Nonce: ${tx.nonce}`);
      console.log(`  Block Number: ${tx.blockNumber}`);
      console.log(`  Block Hash: ${tx.blockHash}`);
    } else {
      console.log(`  ❌ Transaction not found`);
      return;
    }

    // Check receipt
    console.log("\n2. Getting receipt...");
    const receipt = await provider.getTransactionReceipt(txHash);
    if (receipt) {
      console.log(`  ✅ Receipt found`);
      console.log(`  Block: ${receipt.blockNumber}`);
      console.log(`  Status: ${receipt.status === 1 ? 'SUCCESS' : 'FAILED'}`);
      console.log(`  Gas Used: ${receipt.gasUsed}`);
      console.log(`  Logs: ${receipt.logs.length}`);

      if (receipt.logs.length > 0) {
        console.log("\n  Event logs:");
        receipt.logs.forEach((log, i) => {
          console.log(`    Log ${i}:`);
          console.log(`      Address: ${log.address}`);
          console.log(`      Topics: ${log.topics.length}`);
          console.log(`      Topic[0]: ${log.topics[0]}`);
        });
      }
    } else {
      console.log(`  ⏳ Receipt not yet available (transaction in mempool)`);
    }

    // Check current block
    console.log("\n3. Current blockchain state...");
    const blockNumber = await provider.getBlockNumber();
    console.log(`  Current block: ${blockNumber}`);

    // Check pending transaction count
    const pendingNonce = await provider.getTransactionCount(tx.from, 'pending');
    const confirmedNonce = await provider.getTransactionCount(tx.from, 'latest');
    console.log(`  Sender nonce: ${confirmedNonce} (confirmed), ${pendingNonce} (pending)`);
    console.log(`  Pending transactions: ${pendingNonce - confirmedNonce}`);

  } catch (error: any) {
    console.log(`\n❌ Error: ${error.message}`);
    if (error.code) {
      console.log(`  Error code: ${error.code}`);
    }
  }

  console.log();
}

main().catch(console.error);
