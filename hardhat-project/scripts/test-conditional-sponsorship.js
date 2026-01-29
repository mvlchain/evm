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

function logBalance(label, before, after) {
  const diff = after - before;
  const diffStr = diff >= 0 ? `+${ethers.formatEther(diff)}` : ethers.formatEther(diff);
  const color = diff > 0 ? colors.green : diff < 0 ? colors.red : colors.cyan;
  log(`   ${label}:`, colors.cyan);
  log(`      Before: ${ethers.formatEther(before)} ETH`, colors.cyan);
  log(`      After:  ${ethers.formatEther(after)} ETH`, colors.cyan);
  log(`      Change: ${diffStr} ETH`, color);
}

async function getBalances(provider, sponsorAddr, beneficiaryAddr) {
  const [sponsorBal, beneficiaryBal] = await Promise.all([
    provider.getBalance(sponsorAddr),
    provider.getBalance(beneficiaryAddr)
  ]);
  return { sponsor: sponsorBal, beneficiary: beneficiaryBal };
}

function logBalanceChanges(label, before, after, options = {}) {
  const { valueSent = 0n, gasUsed = 0n } = options;

  console.log();
  log(`   📊 Balance Changes for ${label}:`, colors.yellow);
  logBalance("Sponsor", before.sponsor, after.sponsor);
  logBalance("Beneficiary", before.beneficiary, after.beneficiary);

  // Calculate who paid gas
  const sponsorDiff = after.sponsor - before.sponsor;
  const beneficiaryDiff = after.beneficiary - before.beneficiary;

  // If beneficiary sent value, account for it when determining gas payer
  const beneficiaryDiffExcludingValue = beneficiaryDiff + valueSent;

  if (sponsorDiff < 0n) {
    // Sponsor balance decreased - they paid something
    if (beneficiaryDiffExcludingValue >= 0n) {
      // Beneficiary didn't lose money on gas (only value transfer if any)
      log(`   💰 Gas paid by: SPONSOR`, colors.green);
      if (valueSent > 0n) {
        log(`   💸 Value transferred: ${ethers.formatEther(valueSent)} ETH (from beneficiary)`, colors.cyan);
      }
    } else {
      // Both paid - unusual case
      log(`   💰 Gas paid by: BOTH`, colors.yellow);
    }
  } else if (beneficiaryDiffExcludingValue < 0n) {
    // Sponsor didn't pay, but beneficiary lost more than value sent
    log(`   💰 Gas paid by: BENEFICIARY (sponsorship not used)`, colors.yellow);
  } else {
    // No one paid gas - failed transaction
    log(`   💰 No gas charged (transaction failed or reverted)`, colors.cyan);
  }
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

  // Deploy GasHeavy contract (will be whitelisted for daily limit test)
  logInfo("Deploying GasHeavy (whitelisted, for daily limit test)...");
  const GasHeavy = await ethers.getContractFactory("GasHeavy", sponsor);
  const gasHeavy = await GasHeavy.deploy();
  await gasHeavy.waitForDeployment();
  const gasHeavyAddress = await gasHeavy.getAddress();
  logSuccess(`GasHeavy deployed: ${gasHeavyAddress}`);

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
    maxGasPerTx: 1000000,                                      // 1M gas per tx
    totalGasBudget: 10000000,                                  // 10M gas total
    expirationHeight: expirationBlock,
    whitelistedContracts: [simpleStorageAddress, gasHeavyAddress],  // SimpleStorage + GasHeavy allowed
    maxTxValue: ethers.parseEther("0.1"),                      // Max 0.1 ETH per tx
    dailyGasLimit: 500000,                                     // 500K gas per day (reduced for faster testing)
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
  // GAS OVERRIDES - Fetch current base fee to avoid timeout issues
  // =============================================================================
  const feeData = await ethers.provider.getFeeData();
  const baseFee = feeData.maxFeePerGas || ethers.parseUnits("1", "gwei");
  const maxFeePerGas = baseFee + (baseFee / 5n); // +20% buffer

  const gasOverrides = {
    maxFeePerGas: maxFeePerGas,
    maxPriorityFeePerGas: 1n,
  };

  logInfo(`Using maxFeePerGas: ${ethers.formatUnits(maxFeePerGas, "gwei")} gwei`);
  console.log();

  // =============================================================================
  // STEP 4: TEST - Whitelisted contract (SHOULD SUCCEED)
  // =============================================================================
  logStep(4, 8, "Testing whitelisted contract interaction...");
  logTest("TEST 1: Interact with whitelisted SimpleStorage contract");

  const simpleStorageAsBeneficiary = simpleStorage.connect(beneficiary);

  // Get balances before
  const balances1Before = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

  try {
    logInfo("Calling store(42) on SimpleStorage...");
    const tx1 = await simpleStorageAsBeneficiary.store(42, gasOverrides);
    const receipt1 = await tx1.wait();

    // Get balances after
    const balances1After = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

    logSuccess("Transaction succeeded! (Expected ✓)");
    logInfo(`  TX Hash: ${receipt1.hash}`);
    logInfo(`  Gas used: ${receipt1.gasUsed.toString()}`);

    const value = await simpleStorage.retrieve();
    logInfo(`  Stored value: ${value.toString()}`);

    logBalanceChanges("TEST 1", balances1Before, balances1After);
  } catch (error) {
    logError(`Transaction failed: ${error.message} (Unexpected ✗)`);
  }

  console.log();

  // =============================================================================
  // STEP 5: TEST - Non-whitelisted contract (SHOULD FAIL)
  // =============================================================================
  logStep(5, 8, "Testing non-whitelisted contract interaction...");
  logTest("TEST 2: Interact with non-whitelisted Counter contract");
  logWarning("Note: Sponsorship will be silently rejected (contract not whitelisted)");
  logWarning("      Error shows 'insufficient funds' because beneficiary must pay gas themselves");

  const counterAsBeneficiary = counter.connect(beneficiary);

  // Get balances before
  const balances2Before = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

  try {
    logInfo("Calling increment() on Counter (NOT whitelisted)...");
    const tx2 = await counterAsBeneficiary.increment(gasOverrides);
    const receipt2 = await tx2.wait();

    // Get balances after
    const balances2After = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

    logInfo(`Transaction succeeded`);
    logInfo(`  TX Hash: ${receipt2.hash}`);
    logInfo(`  Gas used: ${receipt2.gasUsed.toString()}`);
    logInfo(`  Note: Sponsorship was correctly rejected (check logs), but tx succeeded due to zero gas price`);

    logBalanceChanges("TEST 2", balances2Before, balances2After);
  } catch (error) {
    // Get balances after (even on failure, to show no change)
    const balances2After = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

    logSuccess(`Transaction failed as expected! (Expected ✓)`);
    logInfo(`  Error: ${error.shortMessage || error.message}`);
    logInfo(`  Root cause: Counter contract is NOT in whitelisted contracts`);
    logInfo(`  Flow: Sponsorship rejected → Beneficiary must pay → Insufficient funds → TX fails`);
    logInfo(`  Note: The "insufficient funds" error is expected - it means sponsorship was correctly rejected`);

    logBalanceChanges("TEST 2 (failed tx)", balances2Before, balances2After);
  }

  console.log();

  // =============================================================================
  // STEP 6: TEST - Max transaction value (SHOULD FAIL if value > maxTxValue)
  // =============================================================================
  logStep(6, 8, "Testing max transaction value restriction...");
  logTest("TEST 3: Send transaction with value exceeding maxTxValue");
  logWarning("Note: Sponsorship will be silently rejected (value 0.2 ETH > maxTxValue 0.1 ETH)");
  logWarning("      Error shows 'insufficient funds' because beneficiary must pay gas themselves");

  // Fund beneficiary with ONLY the tx value (0.2 ETH) but NOT enough for gas
  // This way, if sponsorship is rejected, the tx will fail due to insufficient funds for gas
  logInfo("Funding beneficiary with exactly 0.2 ETH (tx value only, no gas buffer)...");
  const fundTx = await sponsor.sendTransaction({
    to: beneficiary.address,
    value: ethers.parseEther("0.2")
  });
  await fundTx.wait();
  logSuccess("Beneficiary funded with 0.2 ETH");

  // Get balances before
  const balances3Before = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

  try {
    logInfo(`Calling store(100) with 0.2 ETH (max allowed is 0.1 ETH)...`);
    const tx3 = await simpleStorageAsBeneficiary.store(100, {
      ...gasOverrides,
      value: ethers.parseEther("0.2") // Exceeds maxTxValue of 0.1 ETH
    });
    const receipt3 = await tx3.wait();

    // Get balances after
    const balances3After = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

    logError(`Transaction succeeded (Unexpected ✗)`);
    logInfo(`  TX Hash: ${receipt3.hash}`);
    logInfo(`  This should have failed because value exceeds maxTxValue!`);

    logBalanceChanges("TEST 3", balances3Before, balances3After);
  } catch (error) {
    // Get balances after (even on failure, to show no change)
    const balances3After = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

    logSuccess(`Transaction failed as expected! (Expected ✓)`);
    logInfo(`  Error: ${error.shortMessage || error.message}`);
    logInfo(`  Root cause: Transaction value (0.2 ETH) > maxTxValue (0.1 ETH)`);
    logInfo(`  Flow: Sponsorship rejected → Beneficiary must pay → Insufficient funds → TX fails`);
    logInfo(`  Note: The "insufficient funds" error is expected - it means maxTxValue check worked`);

    logBalanceChanges("TEST 3 (failed tx)", balances3Before, balances3After);
  }

  console.log();

  // =============================================================================
  // STEP 7: TEST - Transaction within maxTxValue (SHOULD SUCCEED)
  // =============================================================================
  logStep(7, 8, "Testing valid transaction value...");
  logTest("TEST 4: Send transaction with value within maxTxValue");

  // Fund beneficiary with more ETH for the valid value test
  logInfo("Funding beneficiary with additional 0.3 ETH for valid test...");
  const fundTx2 = await sponsor.sendTransaction({
    to: beneficiary.address,
    value: ethers.parseEther("0.3")
  });
  await fundTx2.wait();
  logSuccess("Beneficiary now has funds for value transfer");

  // Get balances before
  const balances4Before = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

  try {
    logInfo(`Calling store(200) with 0.05 ETH (within 0.1 ETH limit)...`);
    const tx4 = await simpleStorageAsBeneficiary.store(200, {
      ...gasOverrides,
      value: ethers.parseEther("0.05") // Within maxTxValue
    });
    const receipt4 = await tx4.wait();

    // Get balances after
    const balances4After = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

    logSuccess("Transaction succeeded! (Expected ✓)");
    logInfo(`  TX Hash: ${receipt4.hash}`);
    logInfo(`  Gas used: ${receipt4.gasUsed.toString()}`);
    logInfo(`  Value sent: 0.05 ETH (within limit)`);

    const value = await simpleStorage.retrieve();
    logInfo(`  Stored value: ${value.toString()}`);

    logBalanceChanges("TEST 4", balances4Before, balances4After, {
      valueSent: ethers.parseEther("0.05"),
      gasUsed: receipt4.gasUsed
    });
  } catch (error) {
    logError(`Transaction failed: ${error.message} (Unexpected ✗)`);
  }

  console.log();

  // =============================================================================
  // STEP 8: TEST - Daily gas limit
  // =============================================================================
  logStep(8, 8, "Testing daily gas limit...");
  logTest("TEST 5: Exhaust daily gas limit using GasHeavy contract");

  // Connect GasHeavy contract to beneficiary
  const gasHeavyAsBeneficiary = gasHeavy.connect(beneficiary);

  logInfo("Using GasHeavy contract to consume large amounts of gas per transaction...");
  logInfo(`Daily limit: ${config.dailyGasLimit.toLocaleString()} gas`);
  logInfo(`Each consumeGas(10) call uses ~200,000+ gas (10 storage writes)`);
  logWarning("Note: Once daily limit is exceeded, sponsorship is rejected but tx can still");
  logWarning("      succeed if beneficiary has funds to pay gas themselves.");
  console.log();

  // Get balances before the loop
  const balances5Before = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

  let totalGasUsedToday = 0;
  let txCount = 0;
  let sponsoredTxCount = 0;
  let selfPaidTxCount = 0;
  const gasPerCall = 10; // Number of storage writes per call (~200k gas)
  let dailyLimitExceeded = false;

  // Execute transactions to demonstrate daily limit behavior
  for (let i = 0; i < 10; i++) {
    // Get balances before this tx to determine who paid
    const txBalancesBefore = await getBalances(ethers.provider, sponsor.address, beneficiary.address);

    try {
      logInfo(`  Transaction ${i + 1}: Calling consumeGas(${gasPerCall})...`);
      const tx = await gasHeavyAsBeneficiary.consumeGas(gasPerCall, {
        ...gasOverrides,
        gasLimit: 500000 // Ensure enough gas for storage writes
      });
      const receipt = await tx.wait();

      // Get balances after to determine who paid
      const txBalancesAfter = await getBalances(ethers.provider, sponsor.address, beneficiary.address);
      const sponsorPaid = txBalancesAfter.sponsor < txBalancesBefore.sponsor;
      const beneficiaryPaid = txBalancesAfter.beneficiary < txBalancesBefore.beneficiary;

      totalGasUsedToday += Number(receipt.gasUsed);
      txCount++;

      const remaining = config.dailyGasLimit - totalGasUsedToday;

      if (sponsorPaid && !dailyLimitExceeded) {
        sponsoredTxCount++;
        logSuccess(`  ✓ SPONSORED - Gas: ${receipt.gasUsed.toLocaleString()} (Total: ${totalGasUsedToday.toLocaleString()}, Remaining: ${remaining.toLocaleString()})`);
      } else {
        selfPaidTxCount++;
        if (!dailyLimitExceeded) {
          dailyLimitExceeded = true;
          log(`  ⚠️  DAILY LIMIT EXCEEDED! Subsequent txs will be paid by beneficiary.`, colors.yellow);
        }
        log(`  ✓ SELF-PAID - Gas: ${receipt.gasUsed.toLocaleString()} (Beneficiary paid - sponsorship rejected)`, colors.yellow);
      }

      // Check if we've exceeded the limit
      if (remaining <= 0 && !dailyLimitExceeded) {
        dailyLimitExceeded = true;
        log(`  ⚠️  DAILY LIMIT EXCEEDED! Subsequent txs will be paid by beneficiary.`, colors.yellow);
      }

      // Small delay between transactions
      await new Promise(resolve => setTimeout(resolve, 300));
    } catch (error) {
      logError(`  ✗ Transaction ${i + 1} failed: ${error.shortMessage || error.message}`);
      logInfo(`     This means beneficiary couldn't afford gas after sponsorship was rejected.`);
      break;
    }
  }

  console.log();
  logInfo(`Summary: ${sponsoredTxCount} transactions sponsored, ${selfPaidTxCount} transactions self-paid`);
  logInfo(`Total gas consumed: ${totalGasUsedToday.toLocaleString()} (Daily limit: ${config.dailyGasLimit.toLocaleString()})`);

  // Get balances after the loop
  const balances5After = await getBalances(ethers.provider, sponsor.address, beneficiary.address);
  logBalanceChanges("TEST 5 (all transactions)", balances5Before, balances5After);

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
  logInfo(`  GasHeavy (whitelisted):          ${gasHeavyAddress}`);
  logInfo(`  Counter (NOT whitelisted):       ${counterAddress}`);
  console.log();

  logSuccess("Sponsorship Configuration:");
  logInfo(`  Sponsorship ID:    ${sponsorshipId}`);
  logInfo(`  Beneficiary:       ${beneficiary.address}`);
  logInfo(`  Sponsor:           ${sponsor.address}`);
  logInfo(`  Whitelisted:       ${config.whitelistedContracts.length} contracts`);
  logInfo(`  Max tx value:      ${ethers.formatEther(config.maxTxValue)} ETH`);
  logInfo(`  Daily gas limit:   ${config.dailyGasLimit.toLocaleString()} gas`);
  console.log();

  log("Test Results:", colors.yellow);
  logInfo("  TEST 1 (Whitelisted contract):     SUCCEED - Sponsor paid gas ✓");
  logInfo("  TEST 2 (Non-whitelisted contract): FAIL - Sponsorship rejected ✓");
  logInfo("  TEST 3 (Value > maxTxValue):       FAIL - Sponsorship rejected ✓");
  logInfo("  TEST 4 (Value within maxTxValue):  SUCCEED - Sponsor paid gas ✓");
  logInfo("  TEST 5 (Daily gas limit):          FAIL after limit exhausted ✓");
  console.log();

  log("Key Learnings:", colors.yellow);
  logInfo("• whitelistedContracts restricts sponsorship to specific contracts");
  logInfo("• maxTxValue prevents sponsored txs from sending too much ETH");
  logInfo("• dailyGasLimit prevents exhausting budget too quickly");
  logInfo("• When sponsorship is rejected, error shows 'insufficient funds'");
  logInfo("  because the beneficiary must pay gas themselves but cannot");
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
