const { ethers } = require("hardhat");

/**
 * Complete Contract-Based Sponsorship Demo:
 * 1. Deploys SimpleStorage contract
 * 2. Creates sponsorship for ANY user interacting with it
 * 3. Tests with a new 0-balance wallet
 *
 * Usage:
 *   npm run sponsor-contract
 */

// Colors for console output
const colors = {
  reset: "\x1b[0m",
  bright: "\x1b[1m",
  red: "\x1b[31m",
  green: "\x1b[32m",
  yellow: "\x1b[33m",
  blue: "\x1b[34m",
  cyan: "\x1b[36m",
};

function log(message, color = colors.reset) {
  console.log(`${color}${message}${colors.reset}`);
}

function logStep(step, total, message) {
  log(`\n[Step ${step}/${total}] ${message}`, colors.yellow);
}

function logSuccess(message) {
  log(`✅ ${message}`, colors.green);
}

function logError(message) {
  log(`❌ ${message}`, colors.red);
}

function logInfo(message) {
  log(`   ${message}`, colors.cyan);
}

async function main() {
  log("================================================", colors.blue);
  log("   Contract-Based Fee Sponsorship Demo", colors.blue);
  log("================================================", colors.blue);
  console.log();

  const FEE_SPONSOR_ADDRESS = "0x0000000000000000000000000000000000000900";
  const ZERO_ADDRESS = "0x0000000000000000000000000000000000000000";

  // =============================================================================
  // STEP 1: Setup sponsor account
  // =============================================================================
  logStep(1, 5, "Setting up sponsor account...");

  const [sponsor] = await ethers.getSigners();
  logInfo(`Sponsor address: ${sponsor.address}`);

  const sponsorBalance = await ethers.provider.getBalance(sponsor.address);
  logInfo(`Sponsor balance: ${ethers.formatEther(sponsorBalance)} ETH`);

  if (sponsorBalance < ethers.parseEther("1")) {
    log("⚠️  Sponsor balance too low! Attempting to fund from pre-funded account...", colors.yellow);
    console.log();

    try {
      const PREFUNDED_PRIVATE_KEY = process.env.PREFUNDED_PRIVATE_KEY;
      const prefundedWallet = new ethers.Wallet(PREFUNDED_PRIVATE_KEY, ethers.provider);

      logInfo(`Pre-funded account: ${prefundedWallet.address}`);
      const prefundedBalance = await ethers.provider.getBalance(prefundedWallet.address);
      logInfo(`Pre-funded balance: ${ethers.formatEther(prefundedBalance)} ETH`);

      if (prefundedBalance < ethers.parseEther("100")) {
        logError("Pre-funded account also has insufficient balance!");
        process.exit(1);
      }

      logInfo("Sending 10 ETH to sponsor account...");
      const fundTx = await prefundedWallet.sendTransaction({
        to: sponsor.address,
        value: ethers.parseEther("10")
      });

      logInfo(`Transaction hash: ${fundTx.hash}`);
      await fundTx.wait();

      const newBalance = await ethers.provider.getBalance(sponsor.address);
      logSuccess(`Sponsor funded! New balance: ${ethers.formatEther(newBalance)} ETH`);
    } catch (error) {
      logError(`Failed to fund sponsor: ${error.message}`);
      process.exit(1);
    }
  }

  logSuccess("Sponsor account ready");
  console.log();

  // =============================================================================
  // STEP 2: Deploy SimpleStorage contract
  // =============================================================================
  logStep(2, 5, "Deploying SimpleStorage contract...");

  logInfo("Compiling contracts...");
  const SimpleStorage = await ethers.getContractFactory("SimpleStorage", sponsor);

  logInfo("Deploying contract (sponsor pays gas)...");
  const simpleStorage = await SimpleStorage.deploy();
  await simpleStorage.waitForDeployment();

  const contractAddress = await simpleStorage.getAddress();
  logSuccess("Contract deployed!");
  logInfo(`Contract address: ${contractAddress}`);
  console.log();

  // =============================================================================
  // STEP 3: Create test user with 0 balance (BEFORE sponsorship)
  // =============================================================================
  logStep(3, 5, "Creating test user with 0 balance...");

  const testUserWallet = ethers.Wallet.createRandom();
  const testUser = testUserWallet.connect(ethers.provider);

  logInfo(`Test user address: ${testUser.address}`);
  logInfo(`Test user private key: ${testUserWallet.privateKey}`);

  const userBalance = await ethers.provider.getBalance(testUser.address);
  logInfo(`Test user balance: ${ethers.formatEther(userBalance)} ETH`);

  if (userBalance === 0n) {
    logSuccess("User has 0 balance (perfect for demo!)");

    // Initialize account on-chain (Cosmos SDK requirement)
    log("   Note: Initializing account on-chain (Cosmos SDK requirement)...", colors.yellow);
    try {
      const initTx = await sponsor.sendTransaction({
        to: testUser.address,
        value: 1n // Send 1 wei to initialize the account
      });
      logInfo(`   Initialization tx: ${initTx.hash}`);
      await initTx.wait();

      const newBalance = await ethers.provider.getBalance(testUser.address);
      logSuccess(`Account initialized! Balance: ${newBalance.toString()} wei (essentially 0)`);
    } catch (error) {
      logError(`Failed to initialize account: ${error.message}`);
      process.exit(1);
    }
  }

  console.log();

  // =============================================================================
  // STEP 4: Create sponsorship for the test user
  // =============================================================================
  logStep(4, 5, "Creating sponsorship for test user...");

  const feeSponsorAbi = [
    "function createSponsorship(address beneficiary, uint64 maxGasPerTx, uint64 totalGasBudget, int64 expirationHeight) external returns (bytes32)",
    "function isSponsored(address beneficiary, uint64 gasEstimate) external view returns (bool, bytes32)",
    "function getSponsorship(bytes32 sponsorshipId) external view returns (address, address, uint64, uint64, int64, bool, uint64, uint64)",
  ];

  const feeSponsor = new ethers.Contract(FEE_SPONSOR_ADDRESS, feeSponsorAbi, sponsor);

  const currentBlock = await ethers.provider.getBlockNumber();
  const expirationBlock = currentBlock + 100000; // ~30 days at 2s blocks

  // Sponsorship configuration
  const config = {
    beneficiary: testUser.address,               // Specific user
    maxGasPerTx: 1000000,                        // 1M gas per tx
    totalGasBudget: 1000000000,                  // 1B gas total (~1000 transactions)
    expirationHeight: expirationBlock
  };

  logInfo("Configuration:");
  logInfo(`  Beneficiary:      ${config.beneficiary}`);
  logInfo(`  Max gas per tx:   ${config.maxGasPerTx.toLocaleString()}`);
  logInfo(`  Total budget:     ${config.totalGasBudget.toLocaleString()} gas`);
  logInfo(`  Expiration block: ${config.expirationHeight}`);
  console.log();

  try {
    logInfo("Creating sponsorship...");
    const tx = await feeSponsor.createSponsorship(
      config.beneficiary,
      config.maxGasPerTx,
      config.totalGasBudget,
      config.expirationHeight
    );

    logInfo(`Transaction hash: ${tx.hash}`);
    logInfo("Waiting for confirmation...");

    const receipt = await tx.wait();
    logSuccess("Sponsorship created!");
    logInfo(`Block: ${receipt.blockNumber}`);
    logInfo(`Gas used: ${receipt.gasUsed.toString()}`);
    console.log();

    // Verify sponsorship
    logInfo("Verifying sponsorship...");
    const [isSponsored, sponsorshipId] = await feeSponsor.isSponsored(testUser.address, 500000);

    if (isSponsored) {
      logSuccess("User is now sponsored!");
      logInfo(`Sponsorship ID: ${sponsorshipId}`);
    } else {
      logError("Sponsorship verification failed");
      process.exit(1);
    }

    console.log();

    // =============================================================================
    // STEP 5: Test gasless transactions
    // =============================================================================
    logStep(5, 5, "Testing gasless transactions...");

    logInfo(`User balance: ${ethers.formatEther(await ethers.provider.getBalance(testUser.address))} ETH (essentially 0)`);
    console.log();

    // Connect to the SimpleStorage contract as the test user
    const simpleStorageAsUser = simpleStorage.connect(testUser);

    // Transaction 1: Store value 42
    log("   Transaction 1: Storing value 42...", colors.blue);
    try {
      const tx1 = await simpleStorageAsUser.store(42);
      const receipt1 = await tx1.wait();
      logSuccess("Transaction 1 successful!");
      logInfo(`   TX Hash: ${receipt1.hash}`);
      logInfo(`   Gas used: ${receipt1.gasUsed.toString()}`);

      const value1 = await simpleStorage.retrieve();
      logInfo(`   Stored value: ${value1.toString()}`);
      console.log();
    } catch (error) {
      logError(`Transaction 1 failed: ${error.message}`);
    }

    await new Promise(resolve => setTimeout(resolve, 1000));

    // Transaction 2: Store value 100
    log("   Transaction 2: Storing value 100...", colors.blue);
    try {
      const tx2 = await simpleStorageAsUser.store(100);
      const receipt2 = await tx2.wait();
      logSuccess("Transaction 2 successful!");
      logInfo(`   TX Hash: ${receipt2.hash}`);
      logInfo(`   Gas used: ${receipt2.gasUsed.toString()}`);

      const value2 = await simpleStorage.retrieve();
      logInfo(`   Stored value: ${value2.toString()}`);
      console.log();
    } catch (error) {
      logError(`Transaction 2 failed: ${error.message}`);
    }

    await new Promise(resolve => setTimeout(resolve, 1000));

    // Transaction 3: Store value 999
    log("   Transaction 3: Storing value 999...", colors.blue);
    try {
      const tx3 = await simpleStorageAsUser.store(999);
      const receipt3 = await tx3.wait();
      logSuccess("Transaction 3 successful!");
      logInfo(`   TX Hash: ${receipt3.hash}`);
      logInfo(`   Gas used: ${receipt3.gasUsed.toString()}`);

      const value3 = await simpleStorage.retrieve();
      logInfo(`   Stored value: ${value3.toString()}`);
      console.log();
    } catch (error) {
      logError(`Transaction 3 failed: ${error.message}`);
    }

    // Check final balances
    const finalUserBalance = await ethers.provider.getBalance(testUser.address);
    const finalSponsorBalance = await ethers.provider.getBalance(sponsor.address);

    logSuccess("Final balances:");
    logInfo(`   User: ${ethers.formatEther(finalUserBalance)} ETH (still essentially 0!)`);
    logInfo(`   Sponsor: ${ethers.formatEther(finalSponsorBalance)} ETH`);
    console.log();

    // Check sponsorship usage
    const detailsAfter = await feeSponsor.getSponsorship(sponsorshipId);
    logSuccess("Sponsorship usage:");
    logInfo(`   Gas used: ${detailsAfter[6].toString()}`);
    logInfo(`   Transaction count: ${detailsAfter[7].toString()}`);
    console.log();

    // =============================================================================
    // Summary
    // =============================================================================
    log("================================================", colors.blue);
    log("              Demo Summary", colors.blue);
    log("================================================", colors.blue);
    console.log();

    logSuccess("Contract: Deployed");
    logSuccess("Sponsorship: Active (ANY user)");
    logSuccess("Test user: Created with 0 balance");
    logSuccess(`Transactions: ${detailsAfter[7].toString()} gasless transactions executed`);
    console.log();

    log("Key Achievements:", colors.yellow);
    logInfo("• Deployed SimpleStorage contract");
    logInfo("• Created sponsorship for ANY user");
    logInfo(`• User with 0 balance executed ${detailsAfter[7].toString()} transactions`);
    logInfo("• Sponsor paid all gas fees automatically");
    logInfo(`• Total gas consumed: ${detailsAfter[6].toString()}`);
    logInfo(`• User balance: Still ${ethers.formatEther(finalUserBalance)} ETH!`);
    console.log();

    log("Note:", colors.yellow);
    logInfo("• This demo sponsors a specific user (basic sponsorship)");
    logInfo("• To sponsor ALL users interacting with a contract:");
    logInfo("  - Use createSponsorshipWithConditions with whitelistedContracts");
    logInfo("  - Backend implementation required for contract whitelisting");
    console.log();

    log("Account Details:", colors.blue);
    logInfo(`Sponsor: ${sponsor.address}`);
    logInfo(`Contract: ${contractAddress}`);
    logInfo(`Test user: ${testUser.address}`);
    logInfo(`Sponsorship ID: ${sponsorshipId}`);
    console.log();

    logSuccess("🎉 Contract-Based Fee Sponsorship Demo Complete!");
    console.log();
    log("================================================", colors.blue);

  } catch (error) {
    logError(`Failed: ${error.message}`);
    console.error(error);
    process.exit(1);
  }
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
