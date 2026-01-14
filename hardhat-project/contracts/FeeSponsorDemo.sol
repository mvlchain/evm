// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface IFeeSponsor {
    function createSponsorship(
        address beneficiary,
        uint64 maxGasPerTx,
        uint64 totalGasBudget,
        int64 expirationHeight
    ) external returns (bytes32);

    function isSponsored(
        address beneficiary,
        uint64 gasEstimate
    ) external view returns (bool sponsored, bytes32 sponsorshipId);

    function getSponsorship(
        bytes32 sponsorshipId
    ) external view returns (
        address sponsor,
        address beneficiary,
        uint64 maxGasPerTx,
        uint64 totalGasBudget,
        int64 expirationHeight,
        bool isActive,
        uint64 gasUsed,
        uint64 transactionCount
    );
}

contract FeeSponsorDemo {
    IFeeSponsor constant SPONSOR = IFeeSponsor(0x0000000000000000000000000000000000000900);

    event SponsorshipCreated(bytes32 indexed sponsorshipId, address indexed beneficiary);

    // Sponsor a user for gasless transactions
    function sponsorUser(address user) external returns (bytes32) {
        bytes32 sponsorshipId = SPONSOR.createSponsorship(
            user,
            1_000_000,      // Max 1M gas per tx
            100_000_000,    // Total budget 100M gas
            int64(uint64(block.number + 1_000_000)) // Expires in ~1M blocks
        );

        emit SponsorshipCreated(sponsorshipId, user);
        return sponsorshipId;
    }

    // Check if user is sponsored
    function checkSponsorship(address user) external view returns (bool, bytes32) {
        return SPONSOR.isSponsored(user, 100_000);
    }

    // Get full sponsorship details
    function getSponsorshipDetails(bytes32 sponsorshipId) external view returns (
        address sponsor,
        address beneficiary,
        uint64 maxGasPerTx,
        uint64 totalGasBudget,
        int64 expirationHeight,
        bool isActive,
        uint64 gasUsed,
        uint64 transactionCount
    ) {
        return SPONSOR.getSponsorship(sponsorshipId);
    }
}
