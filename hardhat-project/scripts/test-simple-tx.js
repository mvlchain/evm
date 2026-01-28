const hre = require("hardhat");
const { ethers } = require("hardhat");

async function main() {
  console.log("=== Testing Simple Transaction as Beneficiary ===\n");

  // Load saved accounts
  const fs = require("fs");
  const accountDetails = JSON.parse(fs.readFileSync("./demo-accounts.json", "utf8"));

  // Reconstruct beneficiary wallet
  const beneficiaryWallet = new ethers.Wallet(accountDetails.beneficiary.privateKey, ethers.provider);

  console.log("Beneficiary:", beneficiaryWallet.address);
  const balance = await ethers.provider.getBalance(beneficiaryWallet.address);
  console.log("Balance:", ethers.formatEther(balance), "ETH");
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

  // Try different transaction options
  const testCases = [
    {
      name: "Default (gasPrice: auto)",
      options: {}
    },
    {
      name: "Explicit gasPrice: 0",
      options: { gasPrice: 0 }
    },
    {
      name: "Explicit gasPrice: 1",
      options: { gasPrice: 1 }
    },
    {
      name: "Explicit gas limit: 100000, gasPrice: 0",
      options: { gasLimit: 100000, gasPrice: 0 }
    }
  ];

  for (const testCase of testCases) {
    console.log(`\n--- Test: ${testCase.name} ---`);

    try {
      console.log("Estimating gas...");
      const gasEstimate = await counter.increment.estimateGas(testCase.options);
      console.log(`✅ Gas estimate: ${gasEstimate.toString()}`);

      console.log("Preparing transaction...");
      const tx = await counter.increment.populateTransaction(testCase.options);
      console.log("Transaction:", {
        to: tx.to,
        data: tx.data,
        gasLimit: tx.gasLimit?.toString(),
        gasPrice: tx.gasPrice?.toString(),
        from: beneficiaryWallet.address
      });

      console.log("\nSending transaction...");
      const sentTx = await beneficiaryWallet.sendTransaction(tx);
      console.log(`✅ Transaction sent! Hash: ${sentTx.hash}`);

      console.log("Waiting for confirmation...");
      const receipt = await sentTx.wait();
      console.log(`✅ Confirmed in block ${receipt.blockNumber}`);
      console.log(`   Gas used: ${receipt.gasUsed.toString()}`);

      const newCount = await counter.getCount();
      console.log(`   New count: ${newCount.toString()}`);

      console.log("\n✅ TEST PASSED!");
      break; // If one succeeds, stop testing

    } catch (error) {
      console.log(`❌ TEST FAILED: ${error.message}`);
      if (error.data) {
        console.log(`   Error data: ${error.data}`);
      }
      if (error.transaction) {
        console.log("   Failed transaction:", {
          to: error.transaction.to,
          from: error.transaction.from,
          gasLimit: error.transaction.gasLimit?.toString(),
          gasPrice: error.transaction.gasPrice?.toString()
        });
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
