import { hkdf } from "@noble/hashes/hkdf";
import { sha256 } from "@noble/hashes/sha2";
import { hmac } from "@noble/hashes/hmac";
import { x25519 } from "@noble/curves/ed25519";
import { chacha20poly1305 } from "@noble/ciphers/chacha";

import { buildHeader } from "./crypto";

const INFO_RK = new TextEncoder().encode("DR_RK");
const INFO_CK = new TextEncoder().encode("DR_CK");

export type KeyPair = {
  privateKey: Uint8Array;
  publicKey: Uint8Array;
};

export type RatchetState = {
  rootKey: Uint8Array;
  sendChainKey: Uint8Array | null;
  recvChainKey: Uint8Array | null;
  dhPair: KeyPair;
  remoteDhPub: Uint8Array;
  pn: number;
  ns: number;
  nr: number;
  skippedMessageKeys: Map<string, Uint8Array>;
};

export type RatchetMessage = {
  header: Uint8Array;
  ciphertext: Uint8Array;
  pn: number;
  n: number;
};

export function generateKeyPair(): KeyPair {
  const privateKey = x25519.utils.randomPrivateKey();
  const publicKey = x25519.getPublicKey(privateKey);
  return { privateKey, publicKey };
}

export function initializeInitiator(
  sharedSecret: Uint8Array,
  remoteDhPub: Uint8Array
): RatchetState {
  const dhPair = generateKeyPair();
  const dhOut = x25519.getSharedSecret(dhPair.privateKey, remoteDhPub);
  const { rootKey, chainKey } = kdfRoot(sharedSecret, dhOut);
  return {
    rootKey,
    sendChainKey: chainKey,
    recvChainKey: null,
    dhPair,
    remoteDhPub,
    pn: 0,
    ns: 0,
    nr: 0,
    skippedMessageKeys: new Map()
  };
}

export function initializeResponder(
  sharedSecret: Uint8Array,
  localDhPair: KeyPair,
  remoteDhPub: Uint8Array
): RatchetState {
  const dhOut = x25519.getSharedSecret(localDhPair.privateKey, remoteDhPub);
  const { rootKey, chainKey } = kdfRoot(sharedSecret, dhOut);
  return {
    rootKey,
    sendChainKey: null,
    recvChainKey: chainKey,
    dhPair: localDhPair,
    remoteDhPub,
    pn: 0,
    ns: 0,
    nr: 0,
    skippedMessageKeys: new Map()
  };
}

export function ratchetEncrypt(
  state: RatchetState,
  plaintext: Uint8Array,
  ad: Uint8Array
): RatchetMessage {
  if (!state.sendChainKey) {
    state.dhPair = generateKeyPair();
    const dhOut = x25519.getSharedSecret(state.dhPair.privateKey, state.remoteDhPub);
    const next = kdfRoot(state.rootKey, dhOut);
    state.rootKey = next.rootKey;
    state.sendChainKey = next.chainKey;
    state.pn = state.ns;
    state.ns = 0;
  }

  const { messageKey, chainKey } = kdfChain(state.sendChainKey);
  state.sendChainKey = chainKey;
  const n = state.ns;
  state.ns += 1;

  const header = buildHeader(state.dhPair.publicKey, state.pn, n, ad);
  const nonce = nonceFromCounter(n);
  const aead = chacha20poly1305(messageKey, nonce, ad);
  const ciphertext = aead.encrypt(plaintext);

  return { header, ciphertext, pn: state.pn, n };
}

export function ratchetDecrypt(
  state: RatchetState,
  header: Uint8Array,
  ciphertext: Uint8Array,
  ad: Uint8Array
): Uint8Array {
  const dhPub = header.slice(1, 33);
  const pn = readUint32(header, 33);
  const n = readUint32(header, 37);

  const skippedKeyId = makeSkippedKeyId(dhPub, n);
  const skippedKey = state.skippedMessageKeys.get(skippedKeyId);
  if (skippedKey) {
    state.skippedMessageKeys.delete(skippedKeyId);
    const nonce = nonceFromCounter(n);
    const aead = chacha20poly1305(skippedKey, nonce, ad);
    return aead.decrypt(ciphertext);
  }

  if (!equalBytes(dhPub, state.remoteDhPub)) {
    skipMessageKeys(state, pn);
    performDhRatchet(state, dhPub);
  }

  if (!state.recvChainKey) {
    throw new Error("missing recv chain key");
  }

  skipMessageKeys(state, n);

  const { messageKey, chainKey } = kdfChain(state.recvChainKey);
  state.recvChainKey = chainKey;
  state.nr = n + 1;

  const nonce = nonceFromCounter(n);
  const aead = chacha20poly1305(messageKey, nonce, ad);
  return aead.decrypt(ciphertext);
}

function performDhRatchet(state: RatchetState, remoteDhPub: Uint8Array) {
  state.pn = state.ns;
  state.ns = 0;
  state.nr = 0;
  state.remoteDhPub = remoteDhPub;

  const dhOutRecv = x25519.getSharedSecret(state.dhPair.privateKey, state.remoteDhPub);
  const recv = kdfRoot(state.rootKey, dhOutRecv);
  state.rootKey = recv.rootKey;
  state.recvChainKey = recv.chainKey;

  state.dhPair = generateKeyPair();
  const dhOutSend = x25519.getSharedSecret(state.dhPair.privateKey, state.remoteDhPub);
  const send = kdfRoot(state.rootKey, dhOutSend);
  state.rootKey = send.rootKey;
  state.sendChainKey = send.chainKey;
}

const MAX_SKIP = 1000;

function skipMessageKeys(state: RatchetState, until: number) {
  if (!state.recvChainKey) return;
  while (state.nr < until) {
    if (state.skippedMessageKeys.size >= MAX_SKIP) {
      throw new Error("skipped message key limit exceeded");
    }
    const { messageKey, chainKey } = kdfChain(state.recvChainKey);
    const keyId = makeSkippedKeyId(state.remoteDhPub, state.nr);
    state.skippedMessageKeys.set(keyId, messageKey);
    state.recvChainKey = chainKey;
    state.nr += 1;
  }
}

function makeSkippedKeyId(dhPub: Uint8Array, n: number): string {
  return `${Buffer.from(dhPub).toString("hex")}:${n}`;
}

function kdfRoot(rootKey: Uint8Array, dhOut: Uint8Array): { rootKey: Uint8Array; chainKey: Uint8Array } {
  const out = hkdf(sha256, dhOut, rootKey, INFO_RK, 64);
  return { rootKey: out.slice(0, 32), chainKey: out.slice(32, 64) };
}

function kdfChain(chainKey: Uint8Array): { messageKey: Uint8Array; chainKey: Uint8Array } {
  const messageKey = hmac(sha256, chainKey, INFO_CK);
  const nextChainKey = hmac(sha256, chainKey, new Uint8Array([0x01]));
  return { messageKey, chainKey: nextChainKey };
}

function nonceFromCounter(counter: number): Uint8Array {
  const nonce = new Uint8Array(12);
  nonce[8] = (counter >>> 24) & 0xff;
  nonce[9] = (counter >>> 16) & 0xff;
  nonce[10] = (counter >>> 8) & 0xff;
  nonce[11] = counter & 0xff;
  return nonce;
}

function readUint32(buf: Uint8Array, offset: number): number {
  return (
    (buf[offset] << 24) |
    (buf[offset + 1] << 16) |
    (buf[offset + 2] << 8) |
    buf[offset + 3]
  ) >>> 0;
}

function equalBytes(a: Uint8Array, b: Uint8Array): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i += 1) {
    if (a[i] !== b[i]) return false;
  }
  return true;
}
