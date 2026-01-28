const { ethers } = require("hardhat");

async function main() {
  const FEE_SPONSOR_ADDRESS = "0x0000000000000000000000000000000000000900";
  const PREFUNDED_PRIVATE_KEY = process.env.PREFUNDED_PRIVATE_KEY;

  // Setup
  const [sponsor] = await ethers.getSigners();
  const prefundedWallet = new ethers.Wallet(PREFUNDED_PRIVATE_KEY, ethers.provider);

  // Fund sponsor if needed
  const sponsorBal = await ethers.provider.getBalance(sponsor.address);
  if (sponsorBal < ethers.parseEther("1")) {
    console.log("Funding sponsor...");
    const tx = await prefundedWallet.sendTransaction({
      to: sponsor.address,
      value: ethers.parseEther("10"),
    });
    await tx.wait();
    console.log("Sponsor funded");
  }

  // Create beneficiary
  const beneficiaryWallet = ethers.Wallet.createRandom().connect(ethers.provider);
  console.log(`Beneficiary: ${beneficiaryWallet.address}`);

  // Initialize beneficiary account on-chain
  const initTx = await sponsor.sendTransaction({
    to: beneficiaryWallet.address,
    value: 1n,
  });
  await initTx.wait();
  console.log("Beneficiary initialized with 1 wei");

  // Create sponsorship
  const feeSponsorAbi = [
    "function createSponsorship(address beneficiary, uint64 maxGasPerTx, uint64 totalGasBudget, int64 expirationHeight) external returns (bytes32)",
    "function isSponsored(address beneficiary, uint64 gasEstimate) external view returns (bool, bytes32)",
  ];
  const feeSponsor = new ethers.Contract(FEE_SPONSOR_ADDRESS, feeSponsorAbi, sponsor);

  const currentBlock = await ethers.provider.getBlockNumber();
  console.log(`Current block: ${currentBlock}`);

  const createTx = await feeSponsor.createSponsorship(
    beneficiaryWallet.address,
    1000000,
    100000000,
    currentBlock + 100000,
  );
  const createReceipt = await createTx.wait();
  console.log(`Sponsorship created in block ${createReceipt.blockNumber}`);

  // Verify sponsorship
  const [isSponsored] = await feeSponsor.isSponsored(beneficiaryWallet.address, 500000);
  console.log(`Is sponsored: ${isSponsored}`);

  if (!isSponsored) {
    console.log("ERROR: Sponsorship not detected!");
    process.exit(1);
  }

  // Wait a few blocks to make sure the mempool has picked up the state change
  console.log("\nWaiting 5 seconds for chain state to propagate to mempool...");
  await new Promise((resolve) => setTimeout(resolve, 5000));

  const blockAfterWait = await ethers.provider.getBlockNumber();
  console.log(`Block after wait: ${blockAfterWait}`);

  // Re-verify sponsorship
  const [isStillSponsored] = await feeSponsor.isSponsored(beneficiaryWallet.address, 500000);
  console.log(`Is still sponsored: ${isStillSponsored}`);

  // Deploy counter
  const Counter = await ethers.getContractFactory("Counter", sponsor);
  const counter = await Counter.deploy();
  await counter.waitForDeployment();
  const counterAddr = await counter.getAddress();
  console.log(`Counter deployed at: ${counterAddr}`);

  // Try sending with explicit gas params
  const incrementData = counter.interface.encodeFunctionData("increment");
  const nonce = await ethers.provider.getTransactionCount(beneficiaryWallet.address);
  console.log(`\nBeneficiary nonce: ${nonce}`);
  console.log(`Beneficiary balance: ${await ethers.provider.getBalance(beneficiaryWallet.address)} wei`);

  // Check what gas fees ethers would auto-detect
  const feeData = await ethers.provider.getFeeData();
  console.log("\nFee data from chain:");
  console.log(`  gasPrice: ${feeData.gasPrice}`);
  console.log(`  maxFeePerGas: ${feeData.maxFeePerGas}`);
  console.log(`  maxPriorityFeePerGas: ${feeData.maxPriorityFeePerGas}`);

  // Try eth_estimateGas first
  console.log("\nTesting eth_estimateGas...");
  try {
    const gasEstimate = await ethers.provider.estimateGas({
      from: beneficiaryWallet.address,
      to: counterAddr,
      data: incrementData,
    });
    console.log(`Gas estimate: ${gasEstimate}`);
  } catch (e) {
    console.log(`Gas estimate FAILED: ${e.message}`);
  }

  // Send tx with explicit fees
  console.log("\nSending tx with explicit gas params...");
  try {
    const tx = await beneficiaryWallet.sendTransaction({
      to: counterAddr,
      data: incrementData,
      gasLimit: 100000,
      maxFeePerGas: 100000000n,
      maxPriorityFeePerGas: 1n,
      nonce: nonce,
    });
    console.log(`TX sent! Hash: ${tx.hash}`);

    // Check txpool status via RPC
    console.log("\nChecking txpool status...");
    try {
      const pending = await ethers.provider.send("txpool_content", []);
      console.log("Txpool pending addresses:", Object.keys(pending.pending || {}));
      console.log("Txpool queued addresses:", Object.keys(pending.queued || {}));
    } catch (e) {
      console.log(`txpool_content not available: ${e.message}`);
    }

    console.log("Waiting for receipt (20s timeout)...");
    const receipt = await Promise.race([
      tx.wait(),
      new Promise((_, reject) =>
        setTimeout(() => reject(new Error("Timeout after 20s")), 20000),
      ),
    ]);
    console.log(`TX mined! Block: ${receipt.blockNumber}, Gas used: ${receipt.gasUsed}`);

    const count = await counter.getCount();
    console.log(`Counter value: ${count}`);
  } catch (e) {
    console.log(`FAILED: ${e.message}`);

    // Check if tx is still in mempool
    console.log("\nDiagnostics after failure:");
    const currentBlock2 = await ethers.provider.getBlockNumber();
    console.log(`Current block: ${currentBlock2}`);

    try {
      const pending = await ethers.provider.send("txpool_content", []);
      const pendingAddrs = Object.keys(pending.pending || {});
      const queuedAddrs = Object.keys(pending.queued || {});
      console.log(`Txpool pending: ${pendingAddrs.length} addresses`);
      console.log(`Txpool queued: ${queuedAddrs.length} addresses`);
      for (const addr of pendingAddrs) {
        console.log(`  Pending: ${addr}: ${Object.keys(pending.pending[addr]).length} txs`);
      }
      for (const addr of queuedAddrs) {
        console.log(`  Queued: ${addr}: ${Object.keys(pending.queued[addr]).length} txs`);
      }
    } catch (e2) {
      console.log(`txpool_content: ${e2.message}`);
    }

    // Try txpool_status
    try {
      const status = await ethers.provider.send("txpool_status", []);
      console.log(`Txpool status - pending: ${status.pending}, queued: ${status.queued}`);
    } catch (e2) {
      console.log(`txpool_status: ${e2.message}`);
    }
  }

  console.log("\nDone.");
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
