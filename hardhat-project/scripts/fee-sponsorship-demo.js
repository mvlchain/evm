const hre = require("hardhat");
const { ethers } = require("hardhat");

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
  log("   Fee Sponsorship Complete Demo (Hardhat)", colors.blue);
  log("================================================", colors.blue);
  console.log();

  const FEE_SPONSOR_ADDRESS = "0x0000000000000000000000000000000000000900";

  // =============================================================================
  // STEP 1: Setup accounts
  // =============================================================================
  logStep(1, 6, "Setting up accounts...");

  // Get sponsor account (from hardhat config)
  const [sponsor] = await ethers.getSigners();
  logInfo(`Sponsor address: ${sponsor.address}`);

  const sponsorBalance = await ethers.provider.getBalance(sponsor.address);
  logInfo(`Sponsor balance: ${ethers.formatEther(sponsorBalance)} ETH`);

  if (sponsorBalance < ethers.parseEther("1")) {
    log("⚠️  Sponsor balance too low! Attempting to fund from pre-funded account...", colors.yellow);
    console.log();

    try {
      // Use the pre-funded dev0 account from local_node.sh
      // dev0 address: 0xC6Fe5D33615a1C52c08018c47E8Bc53646A0E101
      const PREFUNDED_PRIVATE_KEY = process.env.PREFUNDED_PRIVATE_KEY;
      const prefundedWallet = new ethers.Wallet(PREFUNDED_PRIVATE_KEY, ethers.provider);

      logInfo(`Pre-funded account: ${prefundedWallet.address}`);
      const prefundedBalance = await ethers.provider.getBalance(prefundedWallet.address);
      logInfo(`Pre-funded balance: ${ethers.formatEther(prefundedBalance)} ETH`);

      if (prefundedBalance < ethers.parseEther("100")) {
        logError("Pre-funded account also has insufficient balance!");
        logError("Please ensure your local chain is running with funded accounts.");
        process.exit(1);
      }

      // Send 1000 ETH to sponsor
      logInfo("Sending 10 ETH to sponsor account...");
      const fundTx = await prefundedWallet.sendTransaction({
        to: sponsor.address,
        value: ethers.parseEther("10")
      });

      logInfo(`Transaction hash: ${fundTx.hash}`);
      logInfo("Waiting for confirmation...");
      await fundTx.wait();

      const newBalance = await ethers.provider.getBalance(sponsor.address);
      logSuccess(`Sponsor funded! New balance: ${ethers.formatEther(newBalance)} ETH`);
      console.log();
    } catch (error) {
      logError(`Failed to fund sponsor account: ${error.message}`);
      logError("Please manually fund the sponsor account or check if the chain is running.");
      process.exit(1);
    }
  }

  logSuccess("Sponsor account ready");
  console.log();

  // =============================================================================
  // STEP 2: Create beneficiary account (0 balance)
  // =============================================================================
  logStep(2, 6, "Creating beneficiary account...");

  // Create new wallet with random private key
  const beneficiaryWallet = ethers.Wallet.createRandom();
  const beneficiary = beneficiaryWallet.connect(ethers.provider);

  logInfo(`Beneficiary address: ${beneficiary.address}`);
  logInfo(`Beneficiary private key: ${beneficiaryWallet.privateKey}`);

  const beneficiaryBalance = await ethers.provider.getBalance(beneficiary.address);
  logInfo(`Beneficiary balance: ${ethers.formatEther(beneficiaryBalance)} ETH`);

  if (beneficiaryBalance === 0n) {
    logSuccess("Beneficiary has 0 balance (perfect for demo!)");

    // IMPORTANT: In Cosmos SDK chains, accounts must exist on-chain before they can be used
    // We need to send a tiny amount (1 wei) to initialize the account
    log("   Note: Initializing account on-chain (Cosmos SDK requirement)...", colors.yellow);
    try {
      const initTx = await sponsor.sendTransaction({
        to: beneficiary.address,
        value: 1n // Send 1 wei to initialize the account
      });
      logInfo(`   Initialization tx: ${initTx.hash}`);
      await initTx.wait();

      const newBalance = await ethers.provider.getBalance(beneficiary.address);
      logSuccess(`Account initialized! Balance: ${newBalance.toString()} wei (essentially 0)`);
    } catch (error) {
      logError(`Failed to initialize account: ${error.message}`);
      process.exit(1);
    }
  } else {
    log(`   Note: Beneficiary has some balance, but that's okay`, colors.yellow);
  }

  // Save account details
  const accountDetails = {
    sponsor: {
      address: sponsor.address,
    },
    beneficiary: {
      address: beneficiary.address,
      privateKey: beneficiaryWallet.privateKey,
    },
  };

  console.log();

  // =============================================================================
  // STEP 3: Create fee sponsorship
  // =============================================================================
  logStep(3, 6, "Creating fee sponsorship...");

  // Get Fee Sponsor precompile
  const feeSponsorAbi = [
    "function createSponsorship(address beneficiary, uint64 maxGasPerTx, uint64 totalGasBudget, int64 expirationHeight) external returns (bytes32)",
    "function isSponsored(address beneficiary, uint64 gasEstimate) external view returns (bool, bytes32)",
    "function getSponsorship(bytes32 sponsorshipId) external view returns (address, address, uint64, uint64, int64, bool, uint64, uint64)",
  ];

  const feeSponsor = new ethers.Contract(FEE_SPONSOR_ADDRESS, feeSponsorAbi, sponsor);

  const currentBlock = await ethers.provider.getBlockNumber();
  const expirationBlock = currentBlock + 100000;

  logInfo("Creating sponsorship with:");
  logInfo("- Max gas per tx: 1,000,000");
  logInfo("- Total budget: 100,000,000 gas");
  logInfo(`- Expiration: block ${expirationBlock}`);
  console.log();

  try {
    const tx = await feeSponsor.createSponsorship(
      beneficiary.address,
      1000000,
      100000000,
      expirationBlock
    );

    logInfo(`Transaction hash: ${tx.hash}`);
    logInfo("Waiting for confirmation...");

    const receipt = await tx.wait();
    logSuccess("Sponsorship created!");
    logInfo(`Transaction confirmed in block ${receipt.blockNumber}`);
    logInfo(`Gas used: ${receipt.gasUsed.toString()}`);
  } catch (error) {
    logError(`Failed to create sponsorship: ${error.message}`);
    process.exit(1);
  }

  console.log();

  // Verify sponsorship
  logInfo("Verifying sponsorship...");
  const [isSponsored, sponsorshipId] = await feeSponsor.isSponsored(beneficiary.address, 500000);

  if (isSponsored) {
    logSuccess("Beneficiary is sponsored!");
    logInfo(`Sponsorship ID: ${sponsorshipId}`);
    accountDetails.sponsorshipId = sponsorshipId;
  } else {
    logError("Sponsorship verification failed!");
    process.exit(1);
  }

  console.log();

  // Get sponsorship details
  logInfo("Getting sponsorship details...");
  const details = await feeSponsor.getSponsorship(sponsorshipId);
  logSuccess("Sponsorship details:");
  logInfo(`   Sponsor: ${details[0]}`);
  logInfo(`   Beneficiary: ${details[1]}`);
  logInfo(`   Max gas/tx: ${details[2].toString()}`);
  logInfo(`   Total budget: ${details[3].toString()}`);
  logInfo(`   Expiration: ${details[4].toString()}`);
  logInfo(`   Active: ${details[5]}`);
  logInfo(`   Gas used: ${details[6].toString()}`);
  logInfo(`   TX count: ${details[7].toString()}`);

  console.log();

  // =============================================================================
  // STEP 4: Deploy Counter contract
  // =============================================================================
  logStep(4, 6, "Deploying Counter contract...");

  logInfo("Compiling contracts...");
  const Counter = await ethers.getContractFactory("Counter", sponsor);

  logInfo("Deploying contract (sponsor pays gas)...");
  const counter = await Counter.deploy();
  await counter.waitForDeployment();

  const counterAddress = await counter.getAddress();
  logSuccess("Contract deployed!");
  logInfo(`Contract address: ${counterAddress}`);
  accountDetails.counterAddress = counterAddress;

  console.log();

  // =============================================================================
  // STEP 5: Execute gasless transactions as beneficiary
  // =============================================================================
  logStep(5, 6, "Executing gasless transactions as beneficiary...");

  let beneficiaryBalanceBefore = await ethers.provider.getBalance(beneficiary.address);
  let sponsorBalanceBefore = await ethers.provider.getBalance(sponsor.address);

  logInfo(`Beneficiary balance before: ${ethers.formatEther(beneficiaryBalanceBefore)} ETH (${beneficiaryBalanceBefore.toString()} wei)`);
  logInfo(`Sponsor balance before: ${ethers.formatEther(sponsorBalanceBefore)} ETH`);
  console.log();

  // Connect contract to beneficiary wallet
  const counterAsBeneficiary = counter.connect(beneficiary);

  // Fetch current fee data to ensure we meet the base fee requirement
  const feeData = await ethers.provider.getFeeData();
  const baseFee = feeData.maxFeePerGas || 100000000n;
  // Add 20% buffer to handle base fee increases between blocks
  const maxFeePerGas = baseFee + (baseFee / 5n);

  logInfo(`Current base fee: ${ethers.formatUnits(baseFee, "gwei")} gwei`);
  logInfo(`Using maxFeePerGas: ${ethers.formatUnits(maxFeePerGas, "gwei")} gwei`);

  const gasOverrides = {
    maxFeePerGas: maxFeePerGas,
    maxPriorityFeePerGas: 1n,   // matches --evm.min-tip=1
  };

  const transactions = [];
  const balanceLog = [];

  // Helper: execute a sponsored tx and log balance changes
  async function executeSponsoredTx(label, txPromise) {
    log(`   ${label}...`, colors.blue);
    const bBefore = await ethers.provider.getBalance(beneficiary.address);
    const sBefore = await ethers.provider.getBalance(sponsor.address);

    const tx = await txPromise();
    const receipt = await tx.wait();

    const bAfter = await ethers.provider.getBalance(beneficiary.address);
    const sAfter = await ethers.provider.getBalance(sponsor.address);

    const bDiff = bAfter - bBefore;
    const sDiff = sAfter - sBefore;

    logSuccess(`${label} successful!`);
    logInfo(`   TX Hash: ${receipt.hash}`);
    logInfo(`   Gas used: ${receipt.gasUsed.toString()}`);

    transactions.push({ name: label, gasUsed: receipt.gasUsed });
    balanceLog.push({
      name: label,
      gasUsed: receipt.gasUsed,
      beneficiary: { before: bBefore, after: bAfter, diff: bDiff },
      sponsor: { before: sBefore, after: sAfter, diff: sDiff },
    });
    console.log();
  }

  // Transaction 1: Increment
  try {
    await executeSponsoredTx("Transaction 1: Increment", () => counterAsBeneficiary.increment(gasOverrides));
    const count1 = await counter.getCount();
    logInfo(`   Counter value: ${count1.toString()}`);
  } catch (error) {
    logError(`Transaction 1 failed: ${error.message}`);
  }

  await new Promise(resolve => setTimeout(resolve, 1000));

  // Transaction 2: Increment again
  try {
    await executeSponsoredTx("Transaction 2: Increment", () => counterAsBeneficiary.increment(gasOverrides));
    const count2 = await counter.getCount();
    logInfo(`   Counter value: ${count2.toString()}`);
  } catch (error) {
    logError(`Transaction 2 failed: ${error.message}`);
  }

  await new Promise(resolve => setTimeout(resolve, 1000));

  // Transaction 3: Decrement
  try {
    await executeSponsoredTx("Transaction 3: Decrement", () => counterAsBeneficiary.decrement(gasOverrides));
    const count3 = await counter.getCount();
    logInfo(`   Counter value: ${count3.toString()}`);
  } catch (error) {
    logError(`Transaction 3 failed: ${error.message}`);
  }

  await new Promise(resolve => setTimeout(resolve, 1000));

  // Transaction 4: Reset
  try {
    await executeSponsoredTx("Transaction 4: Reset", () => counterAsBeneficiary.reset(gasOverrides));
    const count4 = await counter.getCount();
    logInfo(`   Counter value: ${count4.toString()}`);
  } catch (error) {
    logError(`Transaction 4 failed: ${error.message}`);
  }

  // Check final balances
  const beneficiaryBalanceAfter = await ethers.provider.getBalance(beneficiary.address);
  const sponsorBalanceAfter = await ethers.provider.getBalance(sponsor.address);

  const totalGasUsed = transactions.reduce((sum, tx) => sum + BigInt(tx.gasUsed), 0n);

  // =============================================================================
  // Balance Change Log
  // =============================================================================
  console.log();
  log("================================================", colors.yellow);
  log("          Balance Change Log", colors.yellow);
  log("================================================", colors.yellow);
  console.log();

  log("Per-Transaction Breakdown:", colors.bright);
  console.log();

  for (const entry of balanceLog) {
    log(`  ${entry.name}`, colors.blue);
    log(`  Gas used: ${entry.gasUsed.toString()}`, colors.reset);
    log(`  Sponsor:      ${ethers.formatEther(entry.sponsor.before)} -> ${ethers.formatEther(entry.sponsor.after)} ETH  (${entry.sponsor.diff >= 0n ? "+" : ""}${entry.sponsor.diff.toString()} wei)`, colors.reset);
    log(`  Beneficiary:  ${ethers.formatEther(entry.beneficiary.before)} -> ${ethers.formatEther(entry.beneficiary.after)} ETH  (${entry.beneficiary.diff >= 0n ? "+" : ""}${entry.beneficiary.diff.toString()} wei)`, colors.reset);
    console.log();
  }

  log("------------------------------------------------", colors.yellow);
  log("  Overall Summary:", colors.bright);
  log(`  Sponsor:      ${ethers.formatEther(sponsorBalanceBefore)} -> ${ethers.formatEther(sponsorBalanceAfter)} ETH`, colors.reset);
  log(`                Change: ${(sponsorBalanceAfter - sponsorBalanceBefore).toString()} wei`, colors.reset);
  log(`  Beneficiary:  ${ethers.formatEther(beneficiaryBalanceBefore)} -> ${ethers.formatEther(beneficiaryBalanceAfter)} ETH`, colors.reset);
  log(`                Change: +${(beneficiaryBalanceAfter - beneficiaryBalanceBefore).toString()} wei`, colors.reset);
  log(`  Total gas used: ${totalGasUsed.toString()}`, colors.reset);
  log("------------------------------------------------", colors.yellow);
  console.log();

  if (beneficiaryBalanceAfter > beneficiaryBalanceBefore) {
    log("  NOTE: Beneficiary balance INCREASED. This is because the EVM gas", colors.yellow);
    log("  refund (leftover gas) is sent to msg.From (beneficiary) rather than", colors.yellow);
    log("  the sponsor. The sponsor pays gasLimit*gasPrice upfront, but the", colors.yellow);
    log("  refund of (gasLimit - gasUsed)*gasPrice goes to the beneficiary.", colors.yellow);
    log("  This is a known issue to fix in RefundGas for sponsored txs.", colors.yellow);
    console.log();
  }

  logSuccess(`Total gas consumed: ${totalGasUsed.toString()}`);
  console.log();

  // =============================================================================
  // STEP 6: Verify sponsorship usage
  // =============================================================================
  logStep(6, 6, "Verifying sponsorship usage...");

  const detailsAfter = await feeSponsor.getSponsorship(sponsorshipId);
  logSuccess("Sponsorship usage updated:");
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

  logSuccess("Chain: Running");
  logSuccess("Sponsor: Created and funded");
  logSuccess("Beneficiary: Created with 0 balance");
  logSuccess("Sponsorship: Active");
  logSuccess("Contract: Deployed");
  logSuccess(`Transactions: ${transactions.length} gasless transactions executed`);
  console.log();

  log("Key Achievements:", colors.yellow);
  logInfo(`• Beneficiary executed ${transactions.length} transactions with 0 balance`);
  logInfo("• Sponsor paid all gas fees automatically");
  logInfo(`• Total gas consumed: ${totalGasUsed.toString()}`);
  logInfo(`• Beneficiary balance: ${ethers.formatEther(beneficiaryBalanceAfter)} ETH (gained gas refunds)`);
  console.log();

  log("Account Details:", colors.blue);
  logInfo(`Sponsor: ${accountDetails.sponsor.address}`);
  logInfo(`Beneficiary: ${accountDetails.beneficiary.address}`);
  logInfo(`Contract: ${accountDetails.counterAddress}`);
  logInfo(`Sponsorship ID: ${accountDetails.sponsorshipId}`);
  console.log();

  log("Next Steps:", colors.yellow);
  logInfo("1. Execute more transactions as beneficiary");
  logInfo("2. Monitor sponsorship usage");
  logInfo("3. Cancel sponsorship when done (to get refund)");
  console.log();

  logSuccess("🎉 Fee Sponsorship Demo Complete!");
  console.log();
  log("================================================", colors.blue);
  console.log();

  // Save account details to file
  const fs = require("fs");
  fs.writeFileSync(
    "./demo-accounts.json",
    JSON.stringify(accountDetails, null, 2)
  );
  logSuccess("💾 Account details saved to demo-accounts.json");
  console.log();

  log("Demo completed successfully! 🚀", colors.green);
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    logError("Error running demo:");
    console.error(error);
    process.exit(1);
  });
