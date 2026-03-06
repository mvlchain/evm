// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.0;

/// @title Match precompile interface
/// @dev The precompile is deployed at 0x0000000000000000000000000000000000000808.
interface MatchI {
    function hasReplay(string calldata poolId, string calldata intentId) external view returns (bool exists);

    function getReplay(string calldata poolId, string calldata intentId)
        external
        view
        returns (bool found, string memory matchId);

    function getReplayParties(string calldata poolId, string calldata intentId)
        external
        view
        returns (bool found, string memory matchId, string memory requester, string memory responder);

    function submitMatchCertificate(bytes calldata certificate)
        external
        returns (string memory matchId, string memory replayKey, bytes memory certificateHash);
}
