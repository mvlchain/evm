const hre = require("hardhat");
const { ethers } = require("hardhat");

async function main() {
  console.log("=== Verifying Sponsor Payment for Gas Fees ===\n");

  // Load saved accounts
  const fs = require("fs");
  const accountDetails = JSON.parse(fs.readFileSync("./demo-accounts.json", "utf8"));

  const sponsor = accountDetails.sponsor.address;
  const beneficiary = accountDetails.beneficiary.address;

  console.log("Sponsor:", sponsor);
  console.log("Beneficiary:", beneficiary);
  console.log();

  // Get current balances
  const sponsorBalance = await ethers.provider.getBalance(sponsor);
  const beneficiaryBalance = await ethers.provider.getBalance(beneficiary);

  console.log("=== Current Balances ===");
  console.log(`Sponsor: ${ethers.formatEther(sponsorBalance)} ETH`);
  console.log(`Beneficiary: ${ethers.formatEther(beneficiaryBalance)} ETH`);
  console.log();

  // Get the transactions from the sponsorship
  console.log("=== Getting Transaction Details ===");

  // Query recent blocks to find beneficiary's transactions
  const currentBlock = await ethers.provider.getBlockNumber();
  console.log(`Current block: ${currentBlock}`);
  console.log("Searching recent blocks for beneficiary transactions...\n");

  const txDetails = [];
  const blocksToCheck = 50; // Check last 50 blocks

  for (let i = 0; i < blocksToCheck; i++) {
    const blockNum = currentBlock - i;
    if (blockNum < 0) break;

    try {
      const block = await ethers.provider.getBlock(blockNum, true);
      if (block && block.transactions) {
        for (const tx of block.transactions) {
          // Check if transaction is from beneficiary
          if (tx.from && tx.from.toLowerCase() === beneficiary.toLowerCase()) {
            const receipt = await ethers.provider.getTransactionReceipt(tx.hash);

            txDetails.push({
              hash: tx.hash,
              blockNumber: blockNum,
              from: tx.from,
              to: tx.to,
              gasLimit: tx.gasLimit ? tx.gasLimit.toString() : 'N/A',
              gasPrice: tx.gasPrice ? tx.gasPrice.toString() : 'N/A',
              gasPriceGwei: tx.gasPrice ? ethers.formatUnits(tx.gasPrice, "gwei") : 'N/A',
              gasUsed: receipt.gasUsed.toString(),
              effectiveGasPrice: receipt.effectiveGasPrice ? receipt.effectiveGasPrice.toString() : 'N/A',
              effectiveGasPriceGwei: receipt.effectiveGasPrice ? ethers.formatUnits(receipt.effectiveGasPrice, "gwei") : 'N/A',
              actualCost: receipt.gasUsed && receipt.effectiveGasPrice ?
                receipt.gasUsed * receipt.effectiveGasPrice : 0n,
              status: receipt.status
            });
          }
        }
      }
    } catch (error) {
      // Skip blocks that can't be fetched
    }
  }

  if (txDetails.length === 0) {
    console.log("❌ No transactions found from beneficiary in recent blocks");
    console.log("   This might mean transactions were too long ago or never sent.\n");
  } else {
    console.log(`Found ${txDetails.length} transactions from beneficiary:\n`);

    let totalGasUsed = 0n;
    let totalCost = 0n;

    txDetails.reverse(); // Show oldest first

    for (let i = 0; i < txDetails.length; i++) {
      const tx = txDetails[i];
      console.log(`Transaction ${i + 1}:`);
      console.log(`  Hash: ${tx.hash}`);
      console.log(`  Block: ${tx.blockNumber}`);
      console.log(`  To: ${tx.to}`);
      console.log(`  Gas Price: ${tx.gasPriceGwei} gwei (${tx.gasPrice} wei)`);
      console.log(`  Effective Gas Price: ${tx.effectiveGasPriceGwei} gwei (${tx.effectiveGasPrice} wei)`);
      console.log(`  Gas Used: ${tx.gasUsed}`);
      console.log(`  Actual Cost: ${ethers.formatEther(tx.actualCost)} ETH`);
      console.log(`  Status: ${tx.status === 1 ? '✅ Success' : '❌ Failed'}`);
      console.log();

      totalGasUsed += BigInt(tx.gasUsed);
      totalCost += BigInt(tx.actualCost);
    }

    console.log("=== Summary ===");
    console.log(`Total gas used: ${totalGasUsed.toString()}`);
    console.log(`Total cost: ${ethers.formatEther(totalCost)} ETH`);
    console.log();
  }

  // Check gas price from network
  console.log("=== Network Gas Price Info ===");
  const feeData = await ethers.provider.getFeeData();
  console.log(`Current gas price: ${feeData.gasPrice ? ethers.formatUnits(feeData.gasPrice, "gwei") + " gwei" : "N/A"}`);
  console.log(`Max fee per gas: ${feeData.maxFeePerGas ? ethers.formatUnits(feeData.maxFeePerGas, "gwei") + " gwei" : "N/A"}`);
  console.log(`Max priority fee: ${feeData.maxPriorityFeePerGas ? ethers.formatUnits(feeData.maxPriorityFeePerGas, "gwei") + " gwei" : "N/A"}`);
  console.log();

  // Calculate expected cost if gas price was 0
  if (txDetails.length > 0) {
    const totalGasUsed = txDetails.reduce((sum, tx) => sum + BigInt(tx.gasUsed), 0n);

    console.log("=== Analysis ===");

    // Check if gas price was actually 0
    const allZeroGasPrice = txDetails.every(tx => tx.gasPrice === "0" || tx.effectiveGasPrice === "0");

    if (allZeroGasPrice) {
      console.log("✅ ALL TRANSACTIONS HAD 0 GAS PRICE!");
      console.log("   This means NO fees were charged to anyone.");
      console.log(`   Total gas used: ${totalGasUsed.toString()}`);
      console.log("   Total cost: 0 ETH (because gas price = 0)");
      console.log();
      console.log("📝 Explanation:");
      console.log("   Your chain is configured with 0 gas price (baseFeePerGas = 0).");
      console.log("   This means transactions are FREE for everyone.");
      console.log("   Fee sponsorship still tracks gas usage, but no actual funds are deducted.");
    } else {
      console.log("Gas prices were non-zero:");
      txDetails.forEach((tx, i) => {
        console.log(`  TX ${i + 1}: ${tx.effectiveGasPriceGwei} gwei`);
      });
    }
  }

  console.log("\n=== Complete ===");
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error("Error:", error);
    process.exit(1);
  });
