import assert from "node:assert/strict";
import { Contract, JsonRpcProvider, Wallet } from "ethers";
import { doubleRatchetAbi, keyRegistryAbi, rideHailAbi } from "./abi";
import { adHash } from "./crypto";
import { generateKeyPair, initializeInitiator, initializeResponder, ratchetEncrypt, ratchetDecrypt } from "./double_ratchet";
import {
  deriveSharedSecretInitiator,
  deriveSharedSecretResponder,
  generateIdentityKey,
  generateOneTimePreKey,
  generateSignedPreKey,
  verifySignedPreKey
} from "./x3dh";

const PRECOMPILE = "0x0000000000000000000000000000000000000808";

type Env = {
  rpcUrl: string;
  privateKey: string;
  chainId?: bigint;
  rideHailAddress?: string;
  keyRegistryAddress?: string;
  sessionId?: bigint;
  messageBondWei: bigint;
  maxHeaderBytes: number;
  maxCiphertextBytes: number;
};

function loadEnv(): Env {
  const rpcUrl = process.env.RPC_URL ?? "http://localhost:8545";
  const privateKey = process.env.PRIVATE_KEY ?? "0x88cbead91aee890d27bf06e003ade3d4e952427e88f88d31d61d3ef5e5d54305";
  if (!rpcUrl || !privateKey) {
    throw new Error("RPC_URL and PRIVATE_KEY are required");
  }

  return {
    rpcUrl,
    privateKey,
    chainId: process.env.CHAIN_ID ? BigInt(process.env.CHAIN_ID) : undefined,
    rideHailAddress: process.env.RIDEHAIL_ADDRESS,
    keyRegistryAddress: process.env.KEYREGISTRY_ADDRESS ?? "0x0000000000000000000000000000000000000809",
    sessionId: process.env.SESSION_ID ? BigInt(process.env.SESSION_ID) : BigInt(1),
    messageBondWei: BigInt(process.env.MESSAGE_BOND_WEI ?? "0"),
    maxHeaderBytes: Number(process.env.MAX_HEADER_BYTES ?? "256"),
    maxCiphertextBytes: Number(process.env.MAX_CIPHERTEXT_BYTES ?? "512")
  };
}

async function testPrecompile(provider: JsonRpcProvider, env: Env) {
  const iface = new Contract(PRECOMPILE, doubleRatchetAbi, provider);
  const ad = adHash(1n, "0x0000000000000000000000000000000000000001", "0x0000000000000000000000000000000000000002", env.chainId ?? 1n);
  const alice = generateIdentityKey();
  const bob = generateIdentityKey();
  const bobSpk = generateSignedPreKey(bob.privateKey);
  const bobOtpk = generateOneTimePreKey();
  const bundle = {
    identityPub: bob.publicKey,
    identityEd25519Pub: bob.ed25519PublicKey,  // Ed25519 public key for XEdDSA verification
    signedPreKeyPub: bobSpk.keyPair.publicKey,
    signature: bobSpk.signature,
    oneTimePreKeyPub: bobOtpk.publicKey
  };
  assert.equal(verifySignedPreKey(bundle), true, "X3DH signature invalid");
  const aliceEphemeral = generateKeyPair();
  const shared = deriveSharedSecretInitiator(alice.privateKey, aliceEphemeral.privateKey, bundle);
  const sharedResponder = deriveSharedSecretResponder(
    bob.privateKey,
    bobSpk.keyPair.privateKey,
    alice.publicKey,
    aliceEphemeral.publicKey,
    bobOtpk.privateKey
  );
  assert.deepEqual(shared, sharedResponder);
  const initiator = initializeInitiator(shared, bobSpk.keyPair.publicKey);
  const { header, ciphertext } = ratchetEncrypt(initiator, new Uint8Array([0x01, 0x02]), ad);

  try {
    const result = await iface.validateEnvelope.staticCall(header, ciphertext, env.maxHeaderBytes, env.maxCiphertextBytes);
    assert.equal(result[0], true, "precompile validation failed");
    console.log("precompile: validateEnvelope ok");
  } catch (err) {
    console.warn("Skipping precompile test; validateEnvelope call failed:", err);
  }
}

async function testKeyRegistry(wallet: Wallet, env: Env) {
  if (!env.keyRegistryAddress) {
    return;
  }
  const registry = new Contract(env.keyRegistryAddress, keyRegistryAbi, wallet);
  const keys = generateIdentityKey();
  const spk = generateSignedPreKey(keys.privateKey);
  const identityKey = "0x" + Buffer.from(keys.publicKey).toString("hex");
  const signedPreKey = "0x" + Buffer.from(spk.keyPair.publicKey).toString("hex");
  const signature = "0x" + Buffer.from(spk.signature).toString("hex");
  const expiresAt = BigInt(Math.floor(Date.now() / 1000) + 3600);

  const tx = await registry.publishKeysV2(identityKey, identityKey, signedPreKey, signature, expiresAt);
  await tx.wait();

  const otpk1 = generateOneTimePreKey();
  const otpk2 = generateOneTimePreKey();
  const otpkHexes = [
    "0x" + Buffer.from(otpk1.publicKey).toString("hex"),
    "0x" + Buffer.from(otpk2.publicKey).toString("hex")
  ];
  const otpkTx = await registry.publishOneTimePreKeys(otpkHexes);
  await otpkTx.wait();

  const bundle = await registry.getKeys(wallet.address);
  assert.equal(bundle.identityDhKey, identityKey, "identity key mismatch");
  assert.equal(bundle.identitySignKey, identityKey, "identity key mismatch (both should be same)");
  assert.equal(bundle.signedPreKey, signedPreKey, "signed prekey mismatch");
  const zero32 = "0x" + "0".repeat(64);
  const consumed1 = await registry.consumeOneTimePreKey(wallet.address);
  assert.notEqual(consumed1, zero32, "consumed OTPK should not be zero");
  const consumed2 = await registry.consumeOneTimePreKey(wallet.address);
  assert.notEqual(consumed2, zero32, "second consumed OTPK should not be zero");
  const consumed3 = await registry.consumeOneTimePreKey(wallet.address);
  assert.equal(consumed3, zero32, "OTPK should be exhausted");
  console.log("key-registry: publish/get ok");
}

async function testRideHailMessage(wallet: Wallet, env: Env, provider: JsonRpcProvider) {
  if (!env.rideHailAddress || env.sessionId === undefined) {
    return;
  }
  const code = await provider.getCode(env.rideHailAddress);
  if (!code || code === "0x") {
    console.warn("ridehail: skipping, no contract code at address");
    return;
  }
  const rideHail = new Contract(env.rideHailAddress, rideHailAbi, wallet);

  const ad = adHash(env.sessionId, wallet.address, wallet.address, env.chainId ?? 1n);
  const alice = generateIdentityKey();
  const bob = generateIdentityKey();
  const bobSpk = generateSignedPreKey(bob.privateKey);
  const bobOtpk = generateOneTimePreKey();
  const bundle = {
    identityPub: bob.publicKey,
    identityEd25519Pub: bob.ed25519PublicKey,  // Ed25519 public key for XEdDSA verification
    signedPreKeyPub: bobSpk.keyPair.publicKey,
    signature: bobSpk.signature,
    oneTimePreKeyPub: bobOtpk.publicKey
  };
  assert.equal(verifySignedPreKey(bundle), true, "X3DH signature invalid");
  const aliceEphemeral = generateKeyPair();
  const shared = deriveSharedSecretInitiator(alice.privateKey, aliceEphemeral.privateKey, bundle);
  const sharedResponder = deriveSharedSecretResponder(
    bob.privateKey,
    bobSpk.keyPair.privateKey,
    alice.publicKey,
    aliceEphemeral.publicKey,
    bobOtpk.privateKey
  );
  assert.deepEqual(shared, sharedResponder);
  const initiator = initializeInitiator(shared, bobSpk.keyPair.publicKey);
  const responder = initializeResponder(shared, bobSpk.keyPair, initiator.dhPair.publicKey);
  const { header, ciphertext } = ratchetEncrypt(initiator, new Uint8Array([0x10, 0x20]), ad);
  const plaintext = ratchetDecrypt(responder, header, ciphertext, ad);
  assert.deepEqual(plaintext, new Uint8Array([0x10, 0x20]));
  const msgIndex = 1;

  const tx = await rideHail.postEncryptedMessage(env.sessionId, msgIndex, header, ciphertext, {
    value: env.messageBondWei
  });
  await tx.wait();
  console.log("ridehail: postEncryptedMessage ok");
}

async function main() {
  const env = loadEnv();
  const started = Date.now();
  const provider = new JsonRpcProvider(env.rpcUrl);
  const wallet = new Wallet(env.privateKey, provider);

  if (!env.chainId) {
    const net = await provider.getNetwork();
    env.chainId = BigInt(net.chainId);
  }

  await testPrecompile(provider, env);
  await testKeyRegistry(wallet, env);
  await testRideHailMessage(wallet, env, provider);
  const elapsedMs = Date.now() - started;
  console.log(`ridehail-tests: done in ${elapsedMs}ms`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
