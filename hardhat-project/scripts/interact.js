const hre = require("hardhat");

async function main() {
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("🔧 Interacting with Contracts");
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("");

  // Load deployment info
  const fs = require("fs");
  let deployments;
  try {
    deployments = JSON.parse(fs.readFileSync("deployments.json", "utf8"));
  } catch (error) {
    console.error("❌ No deployments.json found. Run 'npm run deploy' first.");
    process.exit(1);
  }

  const [signer] = await hre.ethers.getSigners();
  console.log("Signer address:", signer.address);
  console.log("");

  // Interact with SimpleStorage
  console.log("📝 SimpleStorage Contract");
  const SimpleStorage = await hre.ethers.getContractFactory("SimpleStorage");
  const simpleStorage = SimpleStorage.attach(deployments.contracts.SimpleStorage);

  console.log("  Contract:", deployments.contracts.SimpleStorage);

  // Store a value
  console.log("  Storing value 45...");
  const tx1 = await simpleStorage.store(45);
  await tx1.wait();
  console.log("  ✅ Transaction:", tx1.hash);

  // Retrieve the value
  const value = await simpleStorage.retrieve();
  console.log("  Retrieved value:", value.toString());
  console.log("");

  // Interact with FeeSponsorDemo
  console.log("🎉 FeeSponsorDemo Contract");
  const FeeSponsorDemo = await hre.ethers.getContractFactory("FeeSponsorDemo");
  const feeSponsorDemo = FeeSponsorDemo.attach(deployments.contracts.FeeSponsorDemo);

  console.log("  Contract:", deployments.contracts.FeeSponsorDemo);

  // Example: Check if an address is sponsored
  const testAddress = "0x64Ca845aA902214b2Baf6Fc2f8f7b4D89ad337d6";
  console.log("  Checking if", testAddress, "is sponsored...");
  const [isSponsored, sponsorshipId] = await feeSponsorDemo.checkSponsorship(testAddress);
  console.log("  Is sponsored:", isSponsored);
  if (isSponsored) {
    console.log("  Sponsorship ID:", sponsorshipId);
  }
  console.log("");

  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("✨ Interaction Complete!");
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
