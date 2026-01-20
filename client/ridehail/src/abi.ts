export const rideHailAbi = [
  "function postEncryptedMessage(uint256 sessionId, uint32 msgIndex, bytes header, bytes ciphertext) payable",
  "function maxHeaderBytes() view returns (uint32)",
  "function maxCiphertextBytes() view returns (uint32)"
];

export const keyRegistryAbi = [
  "function publishKeys(bytes32 identityKey, bytes32 signedPreKey, bytes signature, uint64 expiresAt)",
  "function publishKeysV2(bytes32 identityDhKey, bytes32 identitySignKey, bytes32 signedPreKey, bytes signature, uint64 expiresAt)",
  "function publishOneTimePreKeys(bytes32[] preKeys)",
  "function consumeOneTimePreKey(address owner) returns (bytes32)",
  "function oneTimePreKeyCount(address owner) view returns (uint256)",
  "function getKeys(address owner) view returns (tuple(bytes32 identityDhKey, bytes32 identitySignKey, bytes32 signedPreKey, bytes signature, uint64 expiresAt, uint64 updatedAt))"
];

export const doubleRatchetAbi = [
  "function validateEnvelope(bytes header, bytes ciphertext, uint32 maxHeaderBytes, uint32 maxCiphertextBytes) returns (bool valid, bytes32 envelopeHash, uint8 version, bytes32 dhPub, uint32 pn, uint32 n, bytes32 adHash)"
];
