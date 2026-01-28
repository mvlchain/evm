const hre = require("hardhat");
const { ethers } = require("hardhat");

async function main() {
  console.log("=== Testing Sponsored Transaction with Explicit Gas Price ===\n");

  // Load saved accounts
  const fs = require("fs");
  const accountDetails = JSON.parse(fs.readFileSync("./demo-accounts.json", "utf8"));

  // Reconstruct beneficiary wallet
  const beneficiaryWallet = new ethers.Wallet(accountDetails.beneficiary.privateKey, ethers.provider);

  console.log("Beneficiary:", beneficiaryWallet.address);
  const balance = await ethers.provider.getBalance(beneficiaryWallet.address);
  console.log("Balance:", ethers.formatEther(balance), "ETH");
  console.log();

  // Get fee data
  const feeData = await ethers.provider.getFeeData();
  console.log("Network fee data:");
  console.log("  gasPrice:", feeData.gasPrice?.toString());
  console.log("  maxFeePerGas:", feeData.maxFeePerGas?.toString());
  console.log("  maxPriorityFeePerGas:", feeData.maxPriorityFeePerGas?.toString());
  console.log();

  // Minimum gas price is 1 gwei = 1,000,000,000 wei
  const minGasPrice = ethers.parseUnits("1", "gwei");
  console.log("Using minGasPrice:", minGasPrice.toString(), "wei (1 gwei)");
  console.log();

  // Get counter contract
  const counterAbi = [
    "function getCount() external view returns (uint256)",
    "function increment() external"
  ];
  const counter = new ethers.Contract(accountDetails.counterAddress, counterAbi, beneficiaryWallet);

  console.log("Counter address:", accountDetails.counterAddress);
  const currentCount = await counter.getCount();
  console.log("Current count:", currentCount.toString());
  console.log();

  // Test with different gas price settings
  const testCases = [
    {
      name: "Type 2 (EIP-1559) with maxFeePerGas = minGasPrice",
      options: {
        maxFeePerGas: minGasPrice,
        maxPriorityFeePerGas: 1n, // minimum tip
        type: 2
      }
    },
    {
      name: "Legacy with gasPrice = minGasPrice",
      options: {
        gasPrice: minGasPrice,
        type: 0
      }
    },
    {
      name: "Type 2 with higher maxFeePerGas (2 gwei)",
      options: {
        maxFeePerGas: minGasPrice * 2n,
        maxPriorityFeePerGas: 1n,
        type: 2
      }
    }
  ];

  for (const testCase of testCases) {
    console.log(`\n--- Test: ${testCase.name} ---`);

    try {
      console.log("Preparing transaction...");
      const tx = await counter.increment.populateTransaction();

      // Add gas price options
      Object.assign(tx, testCase.options);

      console.log("Transaction details:");
      console.log("  to:", tx.to);
      console.log("  data:", tx.data);
      console.log("  type:", tx.type);
      if (tx.gasPrice) console.log("  gasPrice:", tx.gasPrice.toString());
      if (tx.maxFeePerGas) console.log("  maxFeePerGas:", tx.maxFeePerGas.toString());
      if (tx.maxPriorityFeePerGas) console.log("  maxPriorityFeePerGas:", tx.maxPriorityFeePerGas.toString());
      console.log();

      console.log("Sending transaction...");
      const sentTx = await beneficiaryWallet.sendTransaction(tx);
      console.log(`✅ Transaction sent! Hash: ${sentTx.hash}`);

      console.log("Waiting for confirmation...");
      const receipt = await sentTx.wait();
      console.log(`✅ Confirmed in block ${receipt.blockNumber}`);
      console.log(`   Gas used: ${receipt.gasUsed.toString()}`);
      console.log(`   Effective gas price: ${receipt.effectiveGasPrice?.toString()}`);

      const cost = receipt.gasUsed * (receipt.effectiveGasPrice || 0n);
      console.log(`   Total cost: ${ethers.formatEther(cost)} ETH`);

      const newCount = await counter.getCount();
      console.log(`   New count: ${newCount.toString()}`);

      // Check if beneficiary balance changed
      const newBalance = await ethers.provider.getBalance(beneficiaryWallet.address);
      console.log(`   Beneficiary balance after: ${ethers.formatEther(newBalance)} ETH`);

      if (newBalance === balance) {
        console.log("   ✅ BENEFICIARY DIDN'T PAY - SPONSORSHIP WORKED!");
      } else {
        console.log(`   ❌ Beneficiary balance changed by ${ethers.formatEther(balance - newBalance)} ETH`);
      }

      console.log("\n✅ TEST PASSED!");
      break; // If one succeeds, stop testing

    } catch (error) {
      console.log(`❌ TEST FAILED: ${error.message}`);
      if (error.shortMessage) {
        console.log(`   Short message: ${error.shortMessage}`);
      }
      if (error.data) {
        console.log(`   Error data: ${error.data}`);
      }
    }
  }

  console.log("\n=== Test Complete ===");
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error("Error:", error);
    process.exit(1);
  });
