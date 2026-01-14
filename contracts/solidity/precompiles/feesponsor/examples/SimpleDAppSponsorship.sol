// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

import "../IFeeSponsor.sol";

/// @title SimpleDAppSponsorship
/// @notice Example contract demonstrating how a DApp can sponsor user transactions
/// @dev This contract shows a simple pattern where a DApp sponsors new users' first transactions
contract SimpleDAppSponsorship {
    /// @notice Address of the DApp owner who will fund sponsorships
    address public owner;

    /// @notice Mapping to track which users have been sponsored
    mapping(address => bytes32) public userSponsorships;

    /// @notice Event emitted when a new user is sponsored
    event UserSponsored(address indexed user, bytes32 indexed sponsorshipId);

    constructor() {
        owner = msg.sender;
    }

    /// @notice Sponsor a new user with a basic sponsorship package
    /// @dev Creates a sponsorship that covers up to 10 transactions for the new user
    /// @param newUser Address of the user to sponsor
    /// @return sponsorshipId The created sponsorship ID
    function sponsorNewUser(address newUser) external returns (bytes32 sponsorshipId) {
        require(msg.sender == owner, "Only owner can sponsor users");
        require(userSponsorships[newUser] == bytes32(0), "User already sponsored");

        // Sponsor parameters:
        // - maxGasPerTx: 500,000 gas per transaction
        // - totalGasBudget: 5,000,000 gas total (enough for ~10 transactions)
        // - expirationHeight: current block + 100,000 blocks (~14 days at 12s blocks)
        sponsorshipId = FEE_SPONSOR_CONTRACT.createSponsorship(
            newUser,
            500000, // maxGasPerTx
            5000000, // totalGasBudget
            int64(uint64(block.number + 100000)) // expirationHeight
        );

        userSponsorships[newUser] = sponsorshipId;
        emit UserSponsored(newUser, sponsorshipId);

        return sponsorshipId;
    }

    /// @notice Check if a user is currently sponsored
    /// @param user Address to check
    /// @return True if the user has an active sponsorship
    function isUserSponsored(address user) external view returns (bool) {
        bytes32 sponsorshipId = userSponsorships[user];
        if (sponsorshipId == bytes32(0)) {
            return false;
        }

        // Check if sponsorship is still active
        (, , , , , bool isActive, , ) = FEE_SPONSOR_CONTRACT.getSponsorship(sponsorshipId);
        return isActive;
    }

    /// @notice Get sponsorship details for a user
    /// @param user Address to query
    function getUserSponsorshipDetails(address user)
        external
        view
        returns (
            address sponsor,
            uint64 maxGasPerTx,
            uint64 totalGasBudget,
            int64 expirationHeight,
            bool isActive,
            uint64 gasUsed,
            uint64 transactionCount
        )
    {
        bytes32 sponsorshipId = userSponsorships[user];
        require(sponsorshipId != bytes32(0), "User not sponsored");

        (sponsor, , maxGasPerTx, totalGasBudget, expirationHeight, isActive, gasUsed, transactionCount) =
            FEE_SPONSOR_CONTRACT.getSponsorship(sponsorshipId);
    }

    /// @notice Cancel a user's sponsorship (emergency function)
    /// @param user Address whose sponsorship to cancel
    function cancelUserSponsorship(address user) external returns (uint64 refundedAmount) {
        require(msg.sender == owner, "Only owner can cancel sponsorships");

        bytes32 sponsorshipId = userSponsorships[user];
        require(sponsorshipId != bytes32(0), "User not sponsored");

        refundedAmount = FEE_SPONSOR_CONTRACT.cancelSponsorship(sponsorshipId);
        delete userSponsorships[user];

        return refundedAmount;
    }

    /// @notice Withdraw any ETH sent to this contract
    function withdraw() external {
        require(msg.sender == owner, "Only owner can withdraw");
        payable(owner).transfer(address(this).balance);
    }

    /// @notice Allow contract to receive ETH
    receive() external payable {}
}
