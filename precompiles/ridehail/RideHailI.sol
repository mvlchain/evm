// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface RideHailI {
    function version() external view returns (uint256);
    function validateCreateRequest(
        bytes32 cellTopic,
        bytes32 regionTopic,
        bytes32 paramsHash,
        bytes32 pickupCommit,
        bytes32 dropoffCommit,
        uint32 maxDriverEta,
        uint64 ttl
    ) external view returns (bool ok, string memory reason);
    function nextRequestId() external view returns (uint256);
    function nextSessionId() external view returns (uint256);

    function createRequest(
        bytes32 cellTopic,
        bytes32 regionTopic,
        bytes32 paramsHash,
        bytes32 pickupCommit,
        bytes32 dropoffCommit,
        uint32 maxDriverEta,
        uint64 ttl
    ) external returns (uint256 requestId);

    function acceptCommit(uint256 requestId, bytes32 commitHash, uint64 eta) external;

    function acceptReveal(uint256 requestId, uint64 eta, bytes32 driverCell, bytes32 salt) external;

    function requests(uint256 requestId)
        external
        view
        returns (
            address rider,
            bytes32 cellTopic,
            bytes32 regionTopic,
            bytes32 paramsHash,
            bytes32 pickupCommit,
            bytes32 dropoffCommit,
            uint256 riderDeposit,
            uint64 createdAt,
            uint64 commitEnd,
            uint64 revealEnd,
            uint64 ttl,
            uint32 maxDriverEta,
            uint32 commitCount,
            bool canceled,
            bool matched,
            uint256 sessionId
        );

    function postEncryptedMessage(
        uint256 sessionId,
        uint32 msgIndex,
        bytes calldata header,
        bytes calldata ciphertext
    ) external;
}
