const hre = require("hardhat");

async function main() {
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("🚀 Deploying Contracts to Cosmos EVM");
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("");

  // Get deployer account
  const [deployer] = await hre.ethers.getSigners();
  console.log("Deployer address:", deployer.address);

  const balance = await hre.ethers.provider.getBalance(deployer.address);
  console.log("Deployer balance:", hre.ethers.formatEther(balance), "ETH");
  console.log("");

  // Deploy SimpleStorage
  console.log("📦 Deploying SimpleStorage...");
  const SimpleStorage = await hre.ethers.getContractFactory("SimpleStorage");
  const simpleStorage = await SimpleStorage.deploy();
  await simpleStorage.waitForDeployment();
  const simpleStorageAddress = await simpleStorage.getAddress();
  console.log("✅ SimpleStorage deployed to:", simpleStorageAddress);
  console.log("");

  // Deploy FeeSponsorDemo
  console.log("📦 Deploying FeeSponsorDemo...");
  const FeeSponsorDemo = await hre.ethers.getContractFactory("FeeSponsorDemo");
  const feeSponsorDemo = await FeeSponsorDemo.deploy();
  await feeSponsorDemo.waitForDeployment();
  const feeSponsorDemoAddress = await feeSponsorDemo.getAddress();
  console.log("✅ FeeSponsorDemo deployed to:", feeSponsorDemoAddress);
  console.log("");

  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("✨ Deployment Complete!");
  console.log("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
  console.log("");
  console.log("Contract Addresses:");
  console.log("  SimpleStorage:    ", simpleStorageAddress);
  console.log("  FeeSponsorDemo:   ", feeSponsorDemoAddress);
  console.log("");
  console.log("🔍 View in block explorer:");
  console.log(`  http://localhost:3000/account/${simpleStorageAddress}`);
  console.log(`  http://localhost:3000/account/${feeSponsorDemoAddress}`);
  console.log("");

  // Save deployment info
  const deploymentInfo = {
    network: hre.network.name,
    deployer: deployer.address,
    timestamp: new Date().toISOString(),
    contracts: {
      SimpleStorage: simpleStorageAddress,
      FeeSponsorDemo: feeSponsorDemoAddress
    }
  };

  const fs = require("fs");
  fs.writeFileSync(
    "deployments.json",
    JSON.stringify(deploymentInfo, null, 2)
  );
  console.log("💾 Deployment info saved to deployments.json");
  console.log("");
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
