// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

import "../IFeeSponsor.sol";

/// @title ConditionalSponsorship
/// @notice Advanced example showing conditional sponsorship with whitelisted contracts
/// @dev This demonstrates how a protocol can sponsor users only for specific actions
contract ConditionalSponsorship {
    /// @notice Protocol owner
    address public owner;

    /// @notice Whitelist of contracts that can be called with sponsored transactions
    address[] public whitelistedContracts;

    /// @notice Mapping of user addresses to their sponsorship IDs
    mapping(address => bytes32) public userSponsorships;

    /// @notice Premium tier sponsorship mapping
    mapping(address => bytes32) public premiumSponsorships;

    event BasicSponsorshipCreated(address indexed user, bytes32 indexed sponsorshipId);
    event PremiumSponsorshipCreated(address indexed user, bytes32 indexed sponsorshipId);

    constructor(address[] memory _whitelistedContracts) {
        owner = msg.sender;
        whitelistedContracts = _whitelistedContracts;
    }

    /// @notice Create a basic sponsorship for a user (limited actions)
    /// @dev Only allows calling whitelisted contracts with limited gas and value
    /// @param user Address to sponsor
    function createBasicSponsorship(address user) external returns (bytes32 sponsorshipId) {
        require(msg.sender == owner, "Only owner");
        require(userSponsorships[user] == bytes32(0), "Already sponsored");

        // Basic tier:
        // - Can only call whitelisted contracts
        // - Max 200,000 gas per tx
        // - Max 0.01 ETH value per tx
        // - 2,000,000 total gas budget
        // - 100,000 gas per day limit
        sponsorshipId = FEE_SPONSOR_CONTRACT.createSponsorshipWithConditions(
            user,
            200000, // maxGasPerTx
            2000000, // totalGasBudget
            int64(uint64(block.number + 50000)), // ~7 days expiration
            whitelistedContracts, // only these contracts
            0.01 ether, // max tx value
            100000 // daily gas limit
        );

        userSponsorships[user] = sponsorshipId;
        emit BasicSponsorshipCreated(user, sponsorshipId);
    }

    /// @notice Create a premium sponsorship for a user (more flexibility)
    /// @dev Allows higher limits and any contract calls
    /// @param user Address to sponsor
    function createPremiumSponsorship(address user) external returns (bytes32 sponsorshipId) {
        require(msg.sender == owner, "Only owner");
        require(premiumSponsorships[user] == bytes32(0), "Already has premium");

        // Premium tier:
        // - Can call any contract (empty whitelist)
        // - Max 1,000,000 gas per tx
        // - Max 1 ETH value per tx
        // - 10,000,000 total gas budget
        // - No daily limit (0)
        address[] memory emptyWhitelist; // Empty = allow any contract

        sponsorshipId = FEE_SPONSOR_CONTRACT.createSponsorshipWithConditions(
            user,
            1000000, // maxGasPerTx
            10000000, // totalGasBudget
            int64(uint64(block.number + 200000)), // ~30 days expiration
            emptyWhitelist, // allow any contract
            1 ether, // max tx value
            0 // no daily limit
        );

        premiumSponsorships[user] = sponsorshipId;
        emit PremiumSponsorshipCreated(user, sponsorshipId);
    }

    /// @notice Check if user has any active sponsorship
    function hasActiveSponsorship(address user) external view returns (bool basic, bool premium) {
        bytes32 basicId = userSponsorships[user];
        bytes32 premiumId = premiumSponsorships[user];

        if (basicId != bytes32(0)) {
            (, , , , , basic, , ) = FEE_SPONSOR_CONTRACT.getSponsorship(basicId);
        }

        if (premiumId != bytes32(0)) {
            (, , , , , premium, , ) = FEE_SPONSOR_CONTRACT.getSponsorship(premiumId);
        }
    }

    /// @notice Upgrade user from basic to premium
    function upgradeToPremium(address user) external {
        require(msg.sender == owner, "Only owner");
        bytes32 basicId = userSponsorships[user];
        require(basicId != bytes32(0), "No basic sponsorship");
        require(premiumSponsorships[user] == bytes32(0), "Already premium");

        // Cancel basic sponsorship
        FEE_SPONSOR_CONTRACT.cancelSponsorship(basicId);
        delete userSponsorships[user];

        // Create premium sponsorship
        this.createPremiumSponsorship(user);
    }

    /// @notice Update the whitelist of allowed contracts
    function updateWhitelist(address[] calldata newWhitelist) external {
        require(msg.sender == owner, "Only owner");
        whitelistedContracts = newWhitelist;
    }

    /// @notice Get the current whitelist
    function getWhitelist() external view returns (address[] memory) {
        return whitelistedContracts;
    }

    /// @notice Cancel all sponsorships for a user
    function cancelAllSponsorships(address user) external {
        require(msg.sender == owner, "Only owner");

        bytes32 basicId = userSponsorships[user];
        if (basicId != bytes32(0)) {
            FEE_SPONSOR_CONTRACT.cancelSponsorship(basicId);
            delete userSponsorships[user];
        }

        bytes32 premiumId = premiumSponsorships[user];
        if (premiumId != bytes32(0)) {
            FEE_SPONSOR_CONTRACT.cancelSponsorship(premiumId);
            delete premiumSponsorships[user];
        }
    }
}
