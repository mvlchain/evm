// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface KeyRegistryI {
    function publishKeysV2(
        bytes32 identityDhKey,
        bytes32 identitySignKey,
        bytes32 signedPreKey,
        bytes calldata signature,
        uint64 expiresAt
    ) external;

    function getKeys(address owner)
        external
        view
        returns (
            bytes32 identityDhKey,
            bytes32 identitySignKey,
            bytes32 signedPreKey,
            bytes memory signature,
            uint64 expiresAt,
            uint64 updatedAt
        );
}
