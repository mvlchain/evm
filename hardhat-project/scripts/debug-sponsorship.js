const hre = require("hardhat");
const { ethers } = require("hardhat");

async function main() {
  console.log("=== Fee Sponsorship Debugging ===\n");

  const FEE_SPONSOR_ADDRESS = "0x0000000000000000000000000000000000000900";

  // Load saved accounts
  const fs = require("fs");
  const accountDetails = JSON.parse(fs.readFileSync("./demo-accounts.json", "utf8"));

  console.log("Loaded accounts:");
  console.log(`  Sponsor: ${accountDetails.sponsor.address}`);
  console.log(`  Beneficiary: ${accountDetails.beneficiary.address}`);
  console.log(`  Sponsorship ID: ${accountDetails.sponsorshipId}`);
  console.log();

  // Get sponsor account
  const [sponsor] = await ethers.getSigners();
  console.log("Current signer:", sponsor.address);
  console.log();

  // Check balances
  const sponsorBalance = await ethers.provider.getBalance(sponsor.address);
  const beneficiaryBalance = await ethers.provider.getBalance(accountDetails.beneficiary.address);

  console.log("=== Balances ===");
  console.log(`Sponsor: ${ethers.formatEther(sponsorBalance)} ETH`);
  console.log(`Beneficiary: ${ethers.formatEther(beneficiaryBalance)} ETH`);
  console.log();

  // Get Fee Sponsor precompile
  const feeSponsorAbi = [
    "function getSponsorship(bytes32 sponsorshipId) external view returns (address, address, uint64, uint64, int64, bool, uint64, uint64)",
    "function isSponsored(address beneficiary, uint64 gasEstimate) external view returns (bool, bytes32)",
  ];

  const feeSponsor = new ethers.Contract(FEE_SPONSOR_ADDRESS, feeSponsorAbi, sponsor);

  // Check sponsorship details
  console.log("=== Sponsorship Details ===");
  try {
    const details = await feeSponsor.getSponsorship(accountDetails.sponsorshipId);
    console.log(`Sponsor: ${details[0]}`);
    console.log(`Beneficiary: ${details[1]}`);
    console.log(`Max gas per tx: ${details[2].toString()}`);
    console.log(`Total budget: ${details[3].toString()}`);
    console.log(`Expiration: ${details[4].toString()}`);
    console.log(`Active: ${details[5]}`);
    console.log(`Gas used: ${details[6].toString()}`);
    console.log(`TX count: ${details[7].toString()}`);
    console.log();

    // Check if addresses match
    if (details[0].toLowerCase() !== accountDetails.sponsor.address.toLowerCase()) {
      console.log("⚠️  WARNING: Sponsor address mismatch!");
      console.log(`  Expected: ${accountDetails.sponsor.address}`);
      console.log(`  Actual: ${details[0]}`);
      console.log();
    }
  } catch (error) {
    console.log(`❌ Failed to get sponsorship details: ${error.message}`);
    console.log();
  }

  // Check if beneficiary is sponsored with different gas estimates
  console.log("=== Sponsorship Status Checks ===");
  const gasEstimates = [21000, 50000, 100000, 300000, 500000, 1000000];

  for (const gasEstimate of gasEstimates) {
    try {
      const [isSponsored, sponsorshipId] = await feeSponsor.isSponsored(
        accountDetails.beneficiary.address,
        gasEstimate
      );
      console.log(`Gas ${gasEstimate.toString().padStart(7)}: ${isSponsored ? "✅ SPONSORED" : "❌ NOT SPONSORED"} (ID: ${sponsorshipId})`);
    } catch (error) {
      console.log(`Gas ${gasEstimate.toString().padStart(7)}: ❌ ERROR - ${error.message}`);
    }
  }
  console.log();

  // Get current block and gas price info
  console.log("=== Network Info ===");
  const blockNumber = await ethers.provider.getBlockNumber();
  const block = await ethers.provider.getBlock(blockNumber);
  const feeData = await ethers.provider.getFeeData();

  console.log(`Current block: ${blockNumber}`);
  console.log(`Base fee: ${feeData.gasPrice ? ethers.formatUnits(feeData.gasPrice, "gwei") : "N/A"} gwei`);

  // Calculate cost for different gas amounts
  if (feeData.gasPrice) {
    console.log();
    console.log("=== Estimated Costs (at current gas price) ===");
    for (const gas of gasEstimates) {
      const cost = feeData.gasPrice * BigInt(gas);
      console.log(`${gas.toString().padStart(7)} gas: ${ethers.formatEther(cost)} ETH`);
    }
  }
  console.log();

  // Check if sponsor has enough for estimated costs
  console.log("=== Sponsor Balance Check ===");
  if (feeData.gasPrice) {
    for (const gas of gasEstimates) {
      const cost = feeData.gasPrice * BigInt(gas);
      const hasEnough = sponsorBalance >= cost;
      console.log(`${gas.toString().padStart(7)} gas: ${hasEnough ? "✅" : "❌"} (needs ${ethers.formatEther(cost)} ETH)`);
    }
  }
  console.log();

  // Try to get counter contract and check its state
  if (accountDetails.counterAddress) {
    console.log("=== Counter Contract ===");
    const counterAbi = [
      "function getCount() external view returns (uint256)",
      "function increment() external",
      "function decrement() external",
      "function reset() external"
    ];

    try {
      const counter = new ethers.Contract(accountDetails.counterAddress, counterAbi, sponsor);
      const count = await counter.getCount();
      console.log(`Current count: ${count.toString()}`);
      console.log();

      // Estimate gas for increment
      console.log("=== Gas Estimates for Counter Operations ===");
      try {
        const incrementGas = await counter.increment.estimateGas();
        console.log(`increment(): ${incrementGas.toString()} gas`);
      } catch (e) {
        console.log(`increment(): ${e.message}`);
      }

      try {
        const resetGas = await counter.reset.estimateGas();
        console.log(`reset(): ${resetGas.toString()} gas`);
      } catch (e) {
        console.log(`reset(): ${e.message}`);
      }
      console.log();
    } catch (error) {
      console.log(`Failed to interact with counter: ${error.message}`);
      console.log();
    }
  }

  // Reconstruct beneficiary wallet and check if it can be used
  console.log("=== Beneficiary Wallet Test ===");
  const beneficiaryWallet = new ethers.Wallet(accountDetails.beneficiary.privateKey, ethers.provider);
  console.log(`Reconstructed address: ${beneficiaryWallet.address}`);
  console.log(`Matches saved: ${beneficiaryWallet.address.toLowerCase() === accountDetails.beneficiary.address.toLowerCase()}`);
  console.log();

  // Check if there are any other sponsorships
  console.log("=== Testing Transaction as Beneficiary ===");
  if (accountDetails.counterAddress) {
    const counterAbi = ["function increment() external"];
    const counter = new ethers.Contract(accountDetails.counterAddress, counterAbi, beneficiaryWallet);

    try {
      console.log("Attempting to estimate gas for increment...");
      const gasEstimate = await counter.increment.estimateGas();
      console.log(`✅ Gas estimate: ${gasEstimate.toString()}`);

      // Check if this amount is sponsored
      const [isSponsored, sponsorshipId] = await feeSponsor.isSponsored(
        beneficiaryWallet.address,
        Number(gasEstimate)
      );
      console.log(`✅ Is sponsored for this gas amount: ${isSponsored}`);
      console.log(`   Sponsorship ID: ${sponsorshipId}`);
    } catch (error) {
      console.log(`❌ Gas estimation failed: ${error.message}`);
      if (error.data) {
        console.log(`   Error data: ${error.data}`);
      }
    }
  }
  console.log();

  console.log("=== Debugging Complete ===");
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error("Error:", error);
    process.exit(1);
  });
