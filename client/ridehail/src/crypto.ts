import { keccak256, toBeHex } from "ethers";

const HEADER_VERSION = 1;

// AES/ChaCha AEAD의 associated data(AD)로 쓰는 32바이트 해시입니다.
export function adHash(
  sessionId: bigint,
  rider: string,
  driver: string,
  chainId: bigint
): Uint8Array {
  const encoded = new TextEncoder().encode(
    `${sessionId.toString()}|${rider.toLowerCase()}|${driver.toLowerCase()}|${chainId.toString()}`
  );
  return Uint8Array.from(Buffer.from(keccak256(encoded).slice(2), "hex"));
}

// AES/ChaCha AEAD의 header입니다.
export function buildHeader(
  dhPub: Uint8Array,
  pn: number,
  n: number,
  ad: Uint8Array
): Uint8Array {
  if (dhPub.length !== 32 || ad.length !== 32) {
    throw new Error("invalid dhPub/adHash length");
  }
  const pnHex = toBeHex(pn, 4);
  const nHex = toBeHex(n, 4);
  const header = new Uint8Array(73);
  header[0] = HEADER_VERSION;
  header.set(dhPub, 1);
  header.set(hexToBytes(pnHex), 33);
  header.set(hexToBytes(nHex), 37);
  header.set(ad, 41);
  return header;
}

// hex string을 uint8array로 변환
function hexToBytes(hex: string): Uint8Array {
  const clean = hex.startsWith("0x") ? hex.slice(2) : hex;
  return Uint8Array.from(Buffer.from(clean, "hex"));
}
