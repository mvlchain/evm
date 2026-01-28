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
  const createTx = await feeSponsor.createSponsorship(
    beneficiaryWallet.address,
    1000000,
    100000000,
    currentBlock + 100000
  );
  await createTx.wait();
  console.log("Sponsorship created");

  const [isSponsored] = await feeSponsor.isSponsored(beneficiaryWallet.address, 500000);
  console.log(`Is sponsored: ${isSponsored}`);

  // Deploy counter
  const Counter = await ethers.getContractFactory("Counter", sponsor);
  const counter = await Counter.deploy();
  await counter.waitForDeployment();
  const counterAddr = await counter.getAddress();
  console.log(`Counter deployed at: ${counterAddr}`);

  const incrementData = counter.interface.encodeFunctionData("increment");

  console.log("\n--- Testing with explicit gas fees (to meet min-tip requirement) ---");
  try {
    const nonce = await ethers.provider.getTransactionCount(beneficiaryWallet.address);
    console.log(`Nonce: ${nonce}`);

    // Set explicit gas fees to meet the chain's --evm.min-tip=1 requirement
    // The sponsor will pay these fees, but the tx must declare them
    const tx = await beneficiaryWallet.sendTransaction({
      to: counterAddr,
      data: incrementData,
      gasLimit: 100000,
      maxFeePerGas: 100000000n, // 0.1 gwei - matches --minimum-gas-prices
      maxPriorityFeePerGas: 1n,  // matches --evm.min-tip=1
      nonce: nonce,
    });
    console.log(`TX sent! Hash: ${tx.hash}`);
    console.log("Waiting for receipt (15s timeout)...");

    const receipt = await Promise.race([
      tx.wait(),
      new Promise((_, reject) => setTimeout(() => reject(new Error("Timeout")), 15000)),
    ]);
    console.log(`TX mined! Block: ${receipt.blockNumber}, Gas used: ${receipt.gasUsed}`);

    // Check balances
    const benefBal = await ethers.provider.getBalance(beneficiaryWallet.address);
    console.log(`Beneficiary balance after: ${benefBal} wei (should still be ~1 if sponsor paid)`);

    const count = await counter.getCount();
    console.log(`Counter value: ${count}`);
  } catch (e) {
    console.log(`FAILED: ${e.message}`);
  }

  console.log("\nDone.");
}

main()
  .then(() => process.exit(0))
  .catch((error) => {
    console.error(error);
    process.exit(1);
  });
