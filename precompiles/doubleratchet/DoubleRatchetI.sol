// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface DoubleRatchetI {
    function validateEnvelope(
        bytes calldata header,
        bytes calldata ciphertext,
        uint32 maxHeaderBytes,
        uint32 maxCiphertextBytes
    )
        external
        returns (
            bool valid,
            bytes32 envelopeHash,
            uint8 version,
            bytes32 dhPub,
            uint32 pn,
            uint32 n,
            bytes32 adHash
        );
}
