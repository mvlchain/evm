/**
 * Check transaction receipt using raw RPC
 */

import { ethers } from "ethers";

async function main() {
  const provider = new ethers.JsonRpcProvider("http://localhost:8545");

  const txHash = process.argv[2] || "0x01955d1b995083c923a0217cb46f33409dac31cdad5ae59911ab6626ad46bfb8";

  console.log(`\nChecking transaction: ${txHash}\n`);

  try {
    // Try multiple methods to get receipt
    console.log("1. Using provider.getTransactionReceipt()...");
    try {
      const receipt = await provider.getTransactionReceipt(txHash);
      if (receipt) {
        console.log(`  ✅ Receipt found:`);
        console.log(`     Block: ${receipt.blockNumber}`);
        console.log(`     Status: ${receipt.status} (1=success, 0=failure)`);
        console.log(`     Gas Used: ${receipt.gasUsed}`);
        console.log(`     Logs: ${receipt.logs.length}`);
        receipt.logs.forEach((log, i) => {
          console.log(`       Log ${i}: ${log.topics[0]} (${log.topics.length} topics)`);
        });
      } else {
        console.log(`  ❌ Receipt not found`);
      }
    } catch (error: any) {
      console.log(`  ❌ Error: ${error.message}`);
    }

    console.log("\n2. Using raw RPC call...");
    try {
      const response = await provider.send("eth_getTransactionReceipt", [txHash]);
      console.log(`  Response: ${JSON.stringify(response, null, 2)}`);
    } catch (error: any) {
      console.log(`  ❌ Error: ${error.message}`);
    }

    console.log("\n3. Getting transaction details...");
    const tx = await provider.getTransaction(txHash);
    if (tx) {
      console.log(`  From: ${tx.from}`);
      console.log(`  To: ${tx.to}`);
      console.log(`  Value: ${ethers.formatEther(tx.value)} ETH`);
      console.log(`  Data: ${tx.data.substring(0, 66)}...`);
      console.log(`  Gas Limit: ${tx.gasLimit}`);
      console.log(`  Block: ${tx.blockNumber}`);
    }

    console.log("\n4. Getting block details...");
    if (tx && tx.blockNumber) {
      const block = await provider.getBlock(tx.blockNumber);
      if (block) {
        console.log(`  Block Number: ${block.number}`);
        console.log(`  Block Hash: ${block.hash}`);
        console.log(`  Transactions: ${block.transactions.length}`);
        console.log(`  Timestamp: ${block.timestamp}`);
      }
    }

  } catch (error: any) {
    console.log(`\n❌ Error: ${error.message}`);
  }
}

main().catch(console.error);
