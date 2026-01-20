import { Contract, JsonRpcProvider, Wallet, AbiCoder, keccak256, type TransactionRequest } from "ethers";
import { adHash } from "./crypto";
import {
  generateIdentityKey,
  generateOneTimePreKey,
  generateSignedPreKey,
  verifySignedPreKey,
  deriveSharedSecretInitiator,
  deriveSharedSecretResponder
} from "./x3dh";
import { generateKeyPair, initializeInitiator, initializeResponder, ratchetEncrypt, ratchetDecrypt } from "./double_ratchet";

const KEY_REGISTRY = "0x0000000000000000000000000000000000000809";

const rideHailAbi = [
  "function nextRequestId() view returns (uint256)",
  "function createRequest(bytes32 cellTopic, bytes32 regionTopic, bytes32 paramsHash, bytes32 pickupCommit, bytes32 dropoffCommit, uint32 maxDriverEta, uint64 ttl) payable returns (uint256)",
  "function acceptCommit(uint256 requestId, bytes32 commitHash, uint64 eta) payable",
  "function acceptReveal(uint256 requestId, uint64 eta, bytes32 driverCell, bytes32 salt)",
  "function requests(uint256 requestId) view returns (address rider, bytes32 cellTopic, bytes32 regionTopic, bytes32 paramsHash, bytes32 pickupCommit, bytes32 dropoffCommit, uint256 riderDeposit, uint64 createdAt, uint64 commitEnd, uint64 revealEnd, uint64 ttl, uint32 maxDriverEta, uint32 commitCount, bool canceled, bool matched, uint256 sessionId)",
  "function postEncryptedMessage(uint256 sessionId, uint32 msgIndex, bytes header, bytes ciphertext) payable"
];

const keyRegistryAbi = [
  "function publishKeysV2(bytes32 identityDhKey, bytes32 identitySignKey, bytes32 signedPreKey, bytes signature, uint64 expiresAt)",
  "function publishOneTimePreKeys(bytes32[] preKeys)",
  "function consumeOneTimePreKey(address owner) returns (bytes32)",
  "function oneTimePreKeyCount(address owner) view returns (uint256)",
  "function getKeys(address owner) view returns (tuple(bytes32 identityDhKey, bytes32 identitySignKey, bytes32 signedPreKey, bytes signature, uint64 expiresAt, uint64 updatedAt))"
];

type Env = {
  rpcUrl: string;
  riderKey: string;
  driverKey: string;
  rideHailAddress: string;
  messageBondWei: bigint;
  gasPriceWei: bigint;
  gasLimit: bigint;
};

function loadEnv(): Env {
  const rpcUrl = process.env.RPC_URL ?? "http://localhost:8545";
  const riderKey = process.env.RIDER_KEY ?? "0x741de4f8988ea941d3ff0287911ca4074e62b7d45c991a51186455366f10b544";
  const driverKey = process.env.DRIVER_KEY ?? "0x3b7955d25189c99a7468192fcbc6429205c158834053ebe3f78f4512ab432db9";
  const rideHailAddress = process.env.RIDEHAIL_ADDRESS ?? "0x000000000000000000000000000000000000080a";
  if (!riderKey || !driverKey || !rideHailAddress) {
    throw new Error("RIDER_KEY, DRIVER_KEY, and RIDEHAIL_ADDRESS are required");
  }
  if (rideHailAddress.toLowerCase() === "0x0000000000000000000000000000000000000808") {
    throw new Error("RIDEHAIL_ADDRESS must be the RideHail contract, not the Double Ratchet precompile");
  }
  if (rideHailAddress.toLowerCase() === "0x0000000000000000000000000000000000000809") {
    throw new Error("RIDEHAIL_ADDRESS must be the RideHail contract, not the Key Registry precompile");
  }
  return {
    rpcUrl,
    riderKey,
    driverKey,
    rideHailAddress,
    messageBondWei: BigInt(process.env.MESSAGE_BOND_WEI ?? "10000000000000000"),
    gasPriceWei: BigInt(process.env.GAS_PRICE_WEI ?? "10000000000"),
    gasLimit: BigInt(process.env.GAS_LIMIT ?? "600000")
  };
}

async function waitForRevealWindow(provider: JsonRpcProvider, commitEnd: bigint) {
  while (true) {
    const block = await provider.getBlock("latest");
    if (block && BigInt(block.timestamp) > commitEnd) {
      break;
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
}

async function waitForReceipt(provider: JsonRpcProvider, txHash: string, label: string) {
  console.log(`waiting for receipt: ${label} (polling)`);
  const tryMineBlock = async () => {
    try {
      await provider.send("evm_mine", []);
    } catch {
      // Ignore if not supported by the node.
    }
  };
  const safeSend = async (method: string, params: unknown[]) => {
    try {
      return await provider.send(method, params);
    } catch (err: any) {
      const msg = err?.shortMessage ?? err?.message ?? "";
      if (msg.includes("request timed out")) {
        return null;
      }
      throw err;
    }
  };
  let seenTx = false;
  for (let i = 0; i < 60; i += 1) {
    try {
      const receipt = await safeSend("eth_getTransactionReceipt", [txHash]);
      if (receipt) {
        if (receipt.status === "0x0" || receipt.status === 0) {
          throw new Error(`transaction reverted: ${label} (${txHash})`);
        }
        return receipt;
      }
    } catch (err) {
      // Some Cosmos-EVM nodes intermittently fail to fetch receipts; fall back to tx inclusion.
    }
    try {
      const tx = await safeSend("eth_getTransactionByHash", [txHash]);
      if (tx) {
        seenTx = true;
      }
      if (tx && tx.blockNumber) {
        return tx;
      }
    } catch (err) {
      if (i === 59) {
        throw err;
      }
    }
    if (i % 10 === 0) {
      console.log(`waiting for receipt: ${label} (${i + 1}/60)`);
    }
    if (i % 5 === 0) {
      await tryMineBlock();
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  if (!seenTx) {
    throw new Error(`tx not found via eth_getTransactionByHash: ${label} (${txHash})`);
  }
  throw new Error(`timeout waiting for receipt: ${label}`);
}

async function txOverrides(env: Env, provider: JsonRpcProvider) {
  const feeData = await provider.getFeeData();
  const networkGas = feeData.gasPrice ?? env.gasPriceWei;
  const gasPrice = networkGas > env.gasPriceWei ? networkGas : env.gasPriceWei;
  return { gasPrice, gasLimit: env.gasLimit };
}

async function sendPopulatedTx(
  signer: Wallet,
  provider: JsonRpcProvider,
  txReq: TransactionRequest,
  overrides: Record<string, unknown>,
  label: string
): Promise<string> {
  const network = await provider.getNetwork();
  const latestNonce = await provider.getTransactionCount(signer.address, "latest");
  const pendingNonce = await provider.getTransactionCount(signer.address, "pending");
  let nonce = pendingNonce;
  let gasPrice = overrides.gasPrice as bigint | undefined;
  if (pendingNonce > latestNonce) {
    nonce = latestNonce;
    if (gasPrice) {
      gasPrice = gasPrice * 2n;
    }
    console.warn(`pending nonce gap detected for ${label}; replacing at nonce ${nonce.toString()}`);
  }

  const tx: TransactionRequest = {
    ...txReq,
    ...overrides,
    chainId: network.chainId,
    nonce,
    type: 0
  };
  if (gasPrice) {
    tx.gasPrice = gasPrice;
  }
  delete (tx as Record<string, unknown>).maxFeePerGas;
  delete (tx as Record<string, unknown>).maxPriorityFeePerGas;
  const signed = await signer.signTransaction(tx);
  const hash = await provider.send("eth_sendRawTransaction", [signed]);
  console.log(`${label} tx: ${hash}`);
  return hash;
}

async function waitForRequest(rideHail: Contract, requestId: bigint) {
  for (let i = 0; i < 120; i += 1) {
    try {
      const block = await rideHail.runner?.provider?.getBlock("latest");
      if (block) {
        console.log(`waiting for request ${requestId} at block ${block.number}`);
      }
      const nextId = await rideHail.nextRequestId();
      if (nextId > requestId) {
        return;
      }
      const req = await rideHail.requests(requestId);
      if (req && req.rider && req.rider !== "0x0000000000000000000000000000000000000000") {
        return req;
      }
    } catch (err) {
      // Ignore transient call errors and retry.
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`timeout waiting for request ${requestId}`);
}

async function waitForNextRequestId(rideHail: Contract, expected: bigint, txHash: string) {
  for (let i = 0; i < 120; i += 1) {
    try {
      const nextId = await rideHail.nextRequestId();
      if (nextId > expected) {
        return nextId;
      }
      if (i % 10 === 0) {
        console.log(`nextRequestId still ${nextId}, waiting for > ${expected}`);
      }
    } catch (err) {
      // Ignore transient call errors and retry.
    }
    await new Promise((resolve) => setTimeout(resolve, 1000));
  }
  throw new Error(`timeout waiting for nextRequestId > ${expected}; tx may have reverted (${txHash})`);
}

async function ensureBlocksAdvancing(provider: JsonRpcProvider) {
  const first = await provider.getBlockNumber();
  await new Promise((resolve) => setTimeout(resolve, 3000));
  const second = await provider.getBlockNumber();
  if (second <= first) {
    throw new Error("chain is not producing blocks; restart local node");
  }
}

async function main() {
  const env = loadEnv();
  const provider = new JsonRpcProvider(env.rpcUrl);
  const rider = new Wallet(env.riderKey, provider);
  const driver = new Wallet(env.driverKey, provider);
  const coder = AbiCoder.defaultAbiCoder();

  await ensureBlocksAdvancing(provider);

  const rideHailRider = new Contract(env.rideHailAddress, rideHailAbi, rider);
  const rideHailDriver = new Contract(env.rideHailAddress, rideHailAbi, driver);
  const keyRegistryDriver = new Contract(KEY_REGISTRY, keyRegistryAbi, driver);

  let requestId: bigint;
  try {
    requestId = await rideHailRider.nextRequestId();
  } catch (err) {
    console.error("RideHail precompile call failed. Ensure evmd is rebuilt and local_node.sh restarted with 0x...080A active.");
    throw err;
  }
  const cellTopic = keccak256(coder.encode(["string"], ["cell-9"]));
  const paramsHash = keccak256(coder.encode(["string"], ["params"]));
  const pickupCommit = keccak256(coder.encode(["string"], ["pickup"]));
  const dropoffCommit = keccak256(coder.encode(["string"], ["dropoff"]));

  const createReq = await rideHailRider.createRequest.populateTransaction(
    cellTopic,
    cellTopic,
    paramsHash,
    pickupCommit,
    dropoffCommit,
    1800,
    7200,
    { value: BigInt(process.env.RIDER_DEPOSIT_WEI ?? "1000000000000000000") }
  );
  const createHash = await sendPopulatedTx(
    rider,
    provider,
    createReq,
    await txOverrides(env, provider),
    "createRequest"
  );
  await waitForReceipt(provider, createHash, "createRequest");
  console.log(`step1: rider created request ${requestId}`);

  await waitForNextRequestId(rideHailRider, requestId, createHash);
  await waitForRequest(rideHailRider, requestId);

  const eta = 600;
  const salt = keccak256(coder.encode(["string"], ["salt"]));
  const commitHash = keccak256(coder.encode(["uint256", "address", "uint64", "bytes32", "bytes32"], [requestId, driver.address, eta, cellTopic, salt]));
  const commitReq = await rideHailDriver.acceptCommit.populateTransaction(requestId, commitHash, eta, {
    value: BigInt(process.env.DRIVER_BOND_WEI ?? "200000000000000000")
  });
  const commitHashTx = await sendPopulatedTx(
    driver,
    provider,
    commitReq,
    await txOverrides(env, provider),
    "acceptCommit"
  );
  await waitForReceipt(provider, commitHashTx, "acceptCommit");
  console.log("step2: driver committed");

  const req = await rideHailRider.requests(requestId);
  await waitForRevealWindow(provider, BigInt(req.commitEnd));
  const revealReq = await rideHailDriver.acceptReveal.populateTransaction(requestId, eta, cellTopic, salt);
  const revealHashTx = await sendPopulatedTx(
    driver,
    provider,
    revealReq,
    await txOverrides(env, provider),
    "acceptReveal"
  );
  await waitForReceipt(provider, revealHashTx, "acceptReveal");
  const sessionId = (await rideHailRider.requests(requestId)).sessionId;
  console.log(`step3: matched session ${sessionId}`);

  const driverKeys = generateIdentityKey();
  const driverSpk = generateSignedPreKey(driverKeys.privateKey);
  const driverOtpk1 = generateOneTimePreKey();
  const driverOtpk2 = generateOneTimePreKey();
  const publishReq = await keyRegistryDriver.publishKeysV2.populateTransaction(
    "0x" + Buffer.from(driverKeys.publicKey).toString("hex"),
    "0x" + Buffer.from(driverKeys.publicKey).toString("hex"),  // Same key for both DH and signing
    "0x" + Buffer.from(driverSpk.keyPair.publicKey).toString("hex"),
    "0x" + Buffer.from(driverSpk.signature).toString("hex"),
    BigInt(Math.floor(Date.now() / 1000) + 3600)
  );
  const publishHash = await sendPopulatedTx(
    driver,
    provider,
    publishReq,
    await txOverrides(env, provider),
    "publishKeysV2"
  );
  await waitForReceipt(provider, publishHash, "publishKeysV2");
  console.log("step3.1: driver published prekey bundle");

  const otpkHexes = [
    "0x" + Buffer.from(driverOtpk1.publicKey).toString("hex"),
    "0x" + Buffer.from(driverOtpk2.publicKey).toString("hex")
  ];
  const otpkPrivMap = new Map<string, Uint8Array>([
    [otpkHexes[0].toLowerCase(), driverOtpk1.privateKey],
    [otpkHexes[1].toLowerCase(), driverOtpk2.privateKey]
  ]);
  const publishOtpkReq = await keyRegistryDriver.publishOneTimePreKeys.populateTransaction(otpkHexes);
  const publishOtpkHash = await sendPopulatedTx(
    driver,
    provider,
    publishOtpkReq,
    await txOverrides(env, provider),
    "publishOneTimePreKeys"
  );
  await waitForReceipt(provider, publishOtpkHash, "publishOneTimePreKeys");
  console.log("step3.1.1: driver published one-time prekeys");

  const bundle = await keyRegistryDriver.getKeys(driver.address);
  const consumedOtpkHex = await keyRegistryDriver.consumeOneTimePreKey(driver.address);
  const zero32 = "0x" + "0".repeat(64);
  let consumedOtpkPub: Uint8Array | undefined;
  let consumedOtpkPriv: Uint8Array | undefined;
  if (consumedOtpkHex !== zero32) {
    consumedOtpkPub = Buffer.from(consumedOtpkHex.slice(2), "hex");
    const consumedHexLower = consumedOtpkHex.toLowerCase();
    const priv = otpkPrivMap.get(consumedHexLower);
    if (!priv) {
      throw new Error("consumed OTPK does not match local private keys");
    }
    consumedOtpkPriv = priv;
  }
  const x3dhBundle = {
    identityPub: Buffer.from(bundle.identityDhKey.slice(2), "hex"),  // X25519 public key for DH
    identityEd25519Pub: driverKeys.ed25519PublicKey,  // Ed25519 public key for XEdDSA verification
    signedPreKeyPub: Buffer.from(bundle.signedPreKey.slice(2), "hex"),
    signature: Buffer.from(bundle.signature.slice(2), "hex"),
    oneTimePreKeyPub: consumedOtpkPub
  };
  if (!verifySignedPreKey(x3dhBundle)) {
    throw new Error("invalid signed prekey");
  }

  const riderIdentity = generateIdentityKey();
  const riderEphemeral = generateKeyPair();
  const shared = deriveSharedSecretInitiator(riderIdentity.privateKey, riderEphemeral.privateKey, x3dhBundle);
  const sharedResponder = deriveSharedSecretResponder(
    driverKeys.privateKey,
    driverSpk.keyPair.privateKey,
    riderIdentity.publicKey,
    riderEphemeral.publicKey,
    consumedOtpkPriv
  );
  console.log("step3.2: X3DH shared secret match", Buffer.from(shared).equals(Buffer.from(sharedResponder)));

  const ad = adHash(sessionId, rider.address, driver.address, BigInt((await provider.getNetwork()).chainId));
  const initiator = initializeInitiator(shared, driverSpk.keyPair.publicKey);
  const responder = initializeResponder(shared, driverSpk.keyPair, initiator.dhPair.publicKey);
  const encoder = new TextEncoder();
  const decoder = new TextDecoder();
  const packets = [
    ratchetEncrypt(initiator, encoder.encode("pickup details 1"), ad),
    ratchetEncrypt(initiator, encoder.encode("pickup details 2"), ad),
    ratchetEncrypt(initiator, encoder.encode("pickup details 3"), ad)
  ];

  const p3 = ratchetDecrypt(responder, packets[2].header, packets[2].ciphertext, ad);
  const p2 = ratchetDecrypt(responder, packets[1].header, packets[1].ciphertext, ad);
  const p1 = ratchetDecrypt(responder, packets[0].header, packets[0].ciphertext, ad);
  console.log("step4: driver decrypted", decoder.decode(p3), decoder.decode(p2), decoder.decode(p1));

  for (let i = 0; i < packets.length; i += 1) {
    const msgReq = await rideHailRider.postEncryptedMessage.populateTransaction(sessionId, i + 1, packets[i].header, packets[i].ciphertext, {
      value: env.messageBondWei
    });
    const msgHash = await sendPopulatedTx(
      rider,
      provider,
      msgReq,
      await txOverrides(env, provider),
      `postEncryptedMessage-${i + 1}`
    );
    await waitForReceipt(provider, msgHash, `postEncryptedMessage-${i + 1}`);
  }
  console.log("step5: rider posted encrypted details");

  const replyPacket = ratchetEncrypt(responder, encoder.encode("driver reply"), ad);
  const replyPlain = ratchetDecrypt(initiator, replyPacket.header, replyPacket.ciphertext, ad);
  console.log("step6: rider decrypted", decoder.decode(replyPlain));

  const replyReq = await rideHailDriver.postEncryptedMessage.populateTransaction(sessionId, packets.length + 1, replyPacket.header, replyPacket.ciphertext, {
    value: env.messageBondWei
  });
  const replyHash = await sendPopulatedTx(
    driver,
    provider,
    replyReq,
    await txOverrides(env, provider),
    "postEncryptedMessage-reply"
  );
  await waitForReceipt(provider, replyHash, "postEncryptedMessage-reply");
  console.log("step7: driver posted encrypted reply");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
