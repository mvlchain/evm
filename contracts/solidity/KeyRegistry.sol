// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

contract KeyRegistry {
    struct KeyBundle {
        bytes32 identityDhKey;
        bytes32 identitySignKey;
        bytes32 signedPreKey;
        bytes signature;
        uint64 expiresAt;
        uint64 updatedAt;
    }

    event KeyPublished(
        address indexed owner,
        bytes32 identityDhKey,
        bytes32 identitySignKey,
        bytes32 signedPreKey,
        bytes signature,
        uint64 expiresAt
    );
    event KeyRevoked(address indexed owner);
    event OneTimePreKeyPublished(address indexed owner, uint256 count);
    event OneTimePreKeyConsumed(address indexed owner, bytes32 preKey);

    error InvalidExpiry();
    error EmptyKey();

    mapping(address => KeyBundle) public bundles;
    mapping(address => bytes32[]) private oneTimePreKeys;

    function publishKeys(
        bytes32 identityKey,
        bytes32 signedPreKey,
        bytes calldata signature,
        uint64 expiresAt
    ) external {
        if (identityKey == bytes32(0) || signedPreKey == bytes32(0)) {
            revert EmptyKey();
        }
        if (expiresAt != 0 && expiresAt <= block.timestamp) {
            revert InvalidExpiry();
        }

        bundles[msg.sender] = KeyBundle({
            identityDhKey: identityKey,
            identitySignKey: identityKey,
            signedPreKey: signedPreKey,
            signature: signature,
            expiresAt: expiresAt,
            updatedAt: uint64(block.timestamp)
        });

        emit KeyPublished(msg.sender, identityKey, identityKey, signedPreKey, signature, expiresAt);
    }

    function publishKeysV2(
        bytes32 identityDhKey,
        bytes32 identitySignKey,
        bytes32 signedPreKey,
        bytes calldata signature,
        uint64 expiresAt
    ) external {
        if (identityDhKey == bytes32(0) || identitySignKey == bytes32(0) || signedPreKey == bytes32(0)) {
            revert EmptyKey();
        }
        if (expiresAt != 0 && expiresAt <= block.timestamp) {
            revert InvalidExpiry();
        }

        bundles[msg.sender] = KeyBundle({
            identityDhKey: identityDhKey,
            identitySignKey: identitySignKey,
            signedPreKey: signedPreKey,
            signature: signature,
            expiresAt: expiresAt,
            updatedAt: uint64(block.timestamp)
        });

        emit KeyPublished(msg.sender, identityDhKey, identitySignKey, signedPreKey, signature, expiresAt);
    }

    function revokeKeys() external {
        delete bundles[msg.sender];
        delete oneTimePreKeys[msg.sender];
        emit KeyRevoked(msg.sender);
    }

    function getKeys(address owner) external view returns (KeyBundle memory) {
        return bundles[owner];
    }

    function publishOneTimePreKeys(bytes32[] calldata preKeys) external {
        if (preKeys.length == 0) {
            revert EmptyKey();
        }
        bytes32[] storage store = oneTimePreKeys[msg.sender];
        for (uint256 i = 0; i < preKeys.length; i++) {
            if (preKeys[i] == bytes32(0)) {
                revert EmptyKey();
            }
            store.push(preKeys[i]);
        }
        emit OneTimePreKeyPublished(msg.sender, preKeys.length);
    }

    function consumeOneTimePreKey(address owner) external returns (bytes32 preKey) {
        bytes32[] storage store = oneTimePreKeys[owner];
        uint256 len = store.length;
        if (len == 0) {
            return bytes32(0);
        }
        preKey = store[len - 1];
        store.pop();
        emit OneTimePreKeyConsumed(owner, preKey);
    }

    function oneTimePreKeyCount(address owner) external view returns (uint256) {
        return oneTimePreKeys[owner].length;
    }
}
