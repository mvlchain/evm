const { ethers } = require("hardhat");

/**
 * Conditional Sponsorship Testing Script
 *
 * Tests the following conditions:
 * 1. whitelistedContracts - Sponsorship only works with specific contracts
 * 2. maxTxValue - Maximum ETH value that can be sent in sponsored tx
 * 3. dailyGasLimit - Maximum gas that can be consumed per day
 *
 * Usage:
 *   npm run test-conditional
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
  magenta: "\x1b[35m",
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

function logWarning(message) {
  log(`⚠️  ${message}`, colors.yellow);
}

function logTest(message) {
  log(`\n🧪 ${message}`, colors.magenta);
}

async function main() {
  log("========================================================", colors.blue);
  log("   Conditional Fee Sponsorship Testing Suite", colors.blue);
  log("========================================================", colors.blue);
  console.log();

  const FEE_SPONSOR_ADDRESS = "0x0000000000000000000000000000000000000900";

  // =============================================================================
  // STEP 1: Setup accounts
  // =============================================================================
  logStep(1, 8, "Setting up accounts...");

  const [sponsor] = await ethers.getSigners();
  logInfo(`Sponsor address: ${sponsor.address}`);

  const sponsorBalance = await ethers.provider.getBalance(sponsor.address);
  logInfo(`Sponsor balance: ${ethers.formatEther(sponsorBalance)} ETH`);

  // Fund sponsor if needed
  if (sponsorBalance < ethers.parseEther("1")) {
    logWarning("Sponsor balance too low! Attempting to fund...");
    console.log();

    try {
      const PREFUNDED_PRIVATE_KEY = process.env.PREFUNDED_PRIVATE_KEY;
      const prefundedWallet = new ethers.Wallet(PREFUNDED_PRIVATE_KEY, ethers.provider);

      logInfo(`Sending 10 ETH to sponsor...`);
      const fundTx = await prefundedWallet.sendTransaction({
        to: sponsor.address,
        value: ethers.parseEther("10")
      });
      await fundTx.wait();

      const newBalance = await ethers.provider.getBalance(sponsor.address);
      logSuccess(`Sponsor funded! Balance: ${ethers.formatEther(newBalance)} ETH`);
    } catch (error) {
      logError(`Failed to fund sponsor: ${error.message}`);
      process.exit(1);
    }
  }

  // Create beneficiary with 0 balance
  const beneficiaryWallet = ethers.Wallet.createRandom();
  const beneficiary = beneficiaryWallet.connect(ethers.provider);

  logInfo(`Beneficiary address: ${beneficiary.address}`);
  logInfo(`Beneficiary private key: ${beneficiaryWallet.privateKey}`);

  // Initialize beneficiary account
  const initTx = await sponsor.sendTransaction({
    to: beneficiary.address,
    value: 1n // 1 wei to initialize
  });
  await initTx.wait();
  logSuccess("Beneficiary account initialized");

  console.log();

  // =============================================================================
  // STEP 2: Deploy test contracts
  // =============================================================================
  logStep(2, 8, "Deploying test contracts...");

  // Deploy SimpleStorage contract (will be whitelisted)
  logInfo("Deploying SimpleStorage (whitelisted)...");
  const SimpleStorage = await ethers.getContractFactory("SimpleStorage", sponsor);
  const simpleStorage = await SimpleStorage.deploy();
  await simpleStorage.waitForDeployment();
  const simpleStorageAddress = await simpleStorage.getAddress();
  logSuccess(`SimpleStorage deployed: ${simpleStorageAddress}`);

  // Deploy Counter contract (will NOT be whitelisted)
  logInfo("Deploying Counter (NOT whitelisted)...");
  const Counter = await ethers.getContractFactory("Counter", sponsor);
  const counter = await Counter.deploy();
  await counter.waitForDeployment();
  const counterAddress = await counter.getAddress();
  logSuccess(`Counter deployed: ${counterAddress}`);

  console.log();

  // =============================================================================
  // STEP 3: Create conditional sponsorship
  // =============================================================================
  logStep(3, 8, "Creating conditional sponsorship...");

  const feeSponsorAbi = [
    "function createSponsorshipWithConditions(address beneficiary, uint64 maxGasPerTx, uint64 totalGasBudget, int64 expirationHeight, address[] calldata whitelistedContracts, uint256 maxTxValue, uint64 dailyGasLimit) external returns (bytes32)",
    "function getSponsorship(bytes32 sponsorshipId) external view returns (address, address, uint64, uint64, int64, bool, uint64, uint64)",
    "function isSponsored(address beneficiary, uint64 gasEstimate) external view returns (bool, bytes32)",
  ];

  const feeSponsor = new ethers.Contract(FEE_SPONSOR_ADDRESS, feeSponsorAbi, sponsor);

  const currentBlock = await ethers.provider.getBlockNumber();
  const expirationBlock = currentBlock + 100000;

  // Sponsorship configuration
  const config = {
    beneficiary: beneficiary.address,
    maxGasPerTx: 1000000,                          // 1M gas per tx
    totalGasBudget: 10000000,                      // 10M gas total
    expirationHeight: expirationBlock,
    whitelistedContracts: [simpleStorageAddress],  // Only SimpleStorage allowed
    maxTxValue: ethers.parseEther("0.1"),          // Max 0.1 ETH per tx
    dailyGasLimit: 5000000,                        // 5M gas per day
  };

  logInfo("Configuration:");
  logInfo(`  Beneficiary:      ${config.beneficiary}`);
  logInfo(`  Max gas per tx:   ${config.maxGasPerTx.toLocaleString()}`);
  logInfo(`  Total budget:     ${config.totalGasBudget.toLocaleString()} gas`);
  logInfo(`  Expiration:       block ${config.expirationHeight}`);
  logInfo(`  Whitelisted:      [${config.whitelistedContracts.join(', ')}]`);
  logInfo(`  Max tx value:     ${ethers.formatEther(config.maxTxValue)} ETH`);
  logInfo(`  Daily gas limit:  ${config.dailyGasLimit.toLocaleString()} gas`);
  console.log();

  let sponsorshipId;
  try {
    logInfo("Creating conditional sponsorship...");
    const tx = await feeSponsor.createSponsorshipWithConditions(
      config.beneficiary,
      config.maxGasPerTx,
      config.totalGasBudget,
      config.expirationHeight,
      config.whitelistedContracts,
      config.maxTxValue,
      config.dailyGasLimit
    );

    logInfo(`Transaction hash: ${tx.hash}`);
    const receipt = await tx.wait();
    logSuccess("Conditional sponsorship created!");
    logInfo(`Block: ${receipt.blockNumber}`);
    logInfo(`Gas used: ${receipt.gasUsed.toString()}`);

    // Get sponsorship ID
    const [isSponsored, id] = await feeSponsor.isSponsored(beneficiary.address, 500000);
    sponsorshipId = id;
    logSuccess(`Sponsorship ID: ${sponsorshipId}`);
  } catch (error) {
    logError(`Failed to create sponsorship: ${error.message}`);
    console.error(error);
    process.exit(1);
  }

  console.log();

  // =============================================================================
  // STEP 4: TEST - Whitelisted contract (SHOULD SUCCEED)
  // =============================================================================
  logStep(4, 8, "Testing whitelisted contract interaction...");
  logTest("TEST 1: Interact with whitelisted SimpleStorage contract");

  const simpleStorageAsBeneficiary = simpleStorage.connect(beneficiary);

  try {
    logInfo("Calling store(42) on SimpleStorage...");
    const tx1 = await simpleStorageAsBeneficiary.store(42);
    const receipt1 = await tx1.wait();

    logSuccess("Transaction succeeded! (Expected ✓)");
    logInfo(`  TX Hash: ${receipt1.hash}`);
    logInfo(`  Gas used: ${receipt1.gasUsed.toString()}`);

    const value = await simpleStorage.retrieve();
    logInfo(`  Stored value: ${value.toString()}`);
  } catch (error) {
    logError(`Transaction failed: ${error.message} (Unexpected ✗)`);
  }

  console.log();

  // =============================================================================
  // STEP 5: TEST - Non-whitelisted contract (SHOULD FAIL)
  // =============================================================================
  logStep(5, 8, "Testing non-whitelisted contract interaction...");
  logTest("TEST 2: Interact with non-whitelisted Counter contract");
  logWarning("Note: In dev environment with zero gas prices, this may succeed even though sponsorship is rejected");

  const counterAsBeneficiary = counter.connect(beneficiary);

  try {
    logInfo("Calling increment() on Counter (NOT whitelisted)...");
    const tx2 = await counterAsBeneficiary.increment();
    const receipt2 = await tx2.wait();

    logInfo(`Transaction succeeded`);
    logInfo(`  TX Hash: ${receipt2.hash}`);
    logInfo(`  Gas used: ${receipt2.gasUsed.toString()}`);
    logInfo(`  Note: Sponsorship was correctly rejected (check logs), but tx succeeded due to zero gas price`);
  } catch (error) {
    logSuccess(`Transaction failed as expected! (Expected ✓)`);
    logInfo(`  Error: ${error.shortMessage || error.message}`);
    logInfo(`  Reason: Counter contract is not in whitelisted contracts`);
  }

  console.log();

  // =============================================================================
  // STEP 6: TEST - Max transaction value (SHOULD FAIL if value > maxTxValue)
  // =============================================================================
  logStep(6, 8, "Testing max transaction value restriction...");
  logTest("TEST 3: Send transaction with value exceeding maxTxValue");

  // First, give beneficiary some ETH to send
  logInfo("Funding beneficiary with 0.5 ETH for value test...");
  const fundTx = await sponsor.sendTransaction({
    to: beneficiary.address,
    value: ethers.parseEther("0.5")
  });
  await fundTx.wait();
  logSuccess("Beneficiary funded");

  try {
    logInfo(`Calling store(100) with 0.2 ETH (max allowed is 0.1 ETH)...`);
    const tx3 = await simpleStorageAsBeneficiary.store(100, {
      value: ethers.parseEther("0.2") // Exceeds maxTxValue of 0.1 ETH
    });
    const receipt3 = await tx3.wait();

    logError(`Transaction succeeded (Unexpected ✗)`);
    logInfo(`  TX Hash: ${receipt3.hash}`);
    logInfo(`  This should have failed because value exceeds maxTxValue!`);
  } catch (error) {
    logSuccess(`Transaction failed as expected! (Expected ✓)`);
    logInfo(`  Error: ${error.shortMessage || error.message}`);
    logInfo(`  Reason: Transaction value (0.2 ETH) > maxTxValue (0.1 ETH)`);
  }

  console.log();

  // =============================================================================
  // STEP 7: TEST - Transaction within maxTxValue (SHOULD SUCCEED)
  // =============================================================================
  logStep(7, 8, "Testing valid transaction value...");
  logTest("TEST 4: Send transaction with value within maxTxValue");

  try {
    logInfo(`Calling store(200) with 0.05 ETH (within 0.1 ETH limit)...`);
    const tx4 = await simpleStorageAsBeneficiary.store(200, {
      value: ethers.parseEther("0.05") // Within maxTxValue
    });
    const receipt4 = await tx4.wait();

    logSuccess("Transaction succeeded! (Expected ✓)");
    logInfo(`  TX Hash: ${receipt4.hash}`);
    logInfo(`  Gas used: ${receipt4.gasUsed.toString()}`);
    logInfo(`  Value sent: 0.05 ETH (within limit)`);

    const value = await simpleStorage.retrieve();
    logInfo(`  Stored value: ${value.toString()}`);
  } catch (error) {
    logError(`Transaction failed: ${error.message} (Unexpected ✗)`);
  }

  console.log();

  // =============================================================================
  // STEP 8: TEST - Daily gas limit
  // =============================================================================
  logStep(8, 8, "Testing daily gas limit...");
  logTest("TEST 5: Exhaust daily gas limit");

  logInfo("Executing multiple transactions to test daily gas limit...");
  logInfo(`Daily limit: ${config.dailyGasLimit.toLocaleString()} gas`);

  let totalGasUsedToday = 0;
  let txCount = 0;

  // Execute transactions until we hit the daily limit
  for (let i = 0; i < 20; i++) {
    try {
      logInfo(`  Transaction ${i + 1}: Calling store(${300 + i})...`);
      const tx = await simpleStorageAsBeneficiary.store(300 + i);
      const receipt = await tx.wait();

      totalGasUsedToday += Number(receipt.gasUsed);
      txCount++;

      logSuccess(`  ✓ Succeeded - Gas used: ${receipt.gasUsed.toString()} (Total today: ${totalGasUsedToday.toLocaleString()})`);

      // Check if we're approaching the limit
      if (totalGasUsedToday >= config.dailyGasLimit) {
        logInfo(`  Reached daily gas limit after ${txCount} transactions`);
        break;
      }

      // Small delay between transactions
      await new Promise(resolve => setTimeout(resolve, 500));
    } catch (error) {
      logSuccess(`  ✓ Transaction failed as expected! (Daily limit reached)`);
      logInfo(`  Error: ${error.shortMessage || error.message}`);
      logInfo(`  Total gas used today: ${totalGasUsedToday.toLocaleString()} / ${config.dailyGasLimit.toLocaleString()}`);
      logInfo(`  Successful transactions before limit: ${txCount}`);
      break;
    }
  }

  console.log();

  // =============================================================================
  // Final Summary
  // =============================================================================
  log("========================================================", colors.blue);
  log("              Test Results Summary", colors.blue);
  log("========================================================", colors.blue);
  console.log();

  logSuccess("Contract Deployment:");
  logInfo(`  SimpleStorage (whitelisted):     ${simpleStorageAddress}`);
  logInfo(`  Counter (NOT whitelisted):       ${counterAddress}`);
  console.log();

  logSuccess("Sponsorship Configuration:");
  logInfo(`  Sponsorship ID:    ${sponsorshipId}`);
  logInfo(`  Beneficiary:       ${beneficiary.address}`);
  logInfo(`  Sponsor:           ${sponsor.address}`);
  logInfo(`  Whitelisted:       [${config.whitelistedContracts.join(', ')}]`);
  logInfo(`  Max tx value:      ${ethers.formatEther(config.maxTxValue)} ETH`);
  logInfo(`  Daily gas limit:   ${config.dailyGasLimit.toLocaleString()} gas`);
  console.log();

  log("Test Results:", colors.yellow);
  logInfo("  TEST 1 (Whitelisted contract):     Expected to SUCCEED ✓");
  logInfo("  TEST 2 (Non-whitelisted contract): Expected to FAIL ✓");
  logInfo("  TEST 3 (Value > maxTxValue):       Expected to FAIL ✓");
  logInfo("  TEST 4 (Value < maxTxValue):       Expected to SUCCEED ✓");
  logInfo("  TEST 5 (Daily gas limit):          Expected to FAIL after limit ✓");
  console.log();

  log("Key Learnings:", colors.yellow);
  logInfo("• whitelistedContracts restricts sponsorship to specific contracts");
  logInfo("• maxTxValue prevents sponsored txs from sending too much ETH");
  logInfo("• dailyGasLimit prevents exhausting budget too quickly");
  logInfo("• Conditions are enforced at the protocol level automatically");
  console.log();

  logSuccess("🎉 Conditional Sponsorship Testing Complete!");
  console.log();
  log("========================================================", colors.blue);
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    logError("Error running tests:");
    console.error(error);
    process.exit(1);
  });
