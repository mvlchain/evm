// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

/// @dev The IFeeSponsor contract's address.
address constant FEE_SPONSOR_PRECOMPILE_ADDRESS = 0x0000000000000000000000000000000000000900;

/// @dev The IFeeSponsor contract's instance.
IFeeSponsor constant FEE_SPONSOR_CONTRACT = IFeeSponsor(FEE_SPONSOR_PRECOMPILE_ADDRESS);

/// @title IFeeSponsor
/// @author Cosmos EVM
/// @notice This interface enables fee sponsorship functionality where one account can pay
/// gas fees for another account's transactions.
/// @dev The precompile contract's address is 0x0000000000000000000000000000000000000900.
interface IFeeSponsor {
    /// @dev Emitted when a new sponsorship is created
    event SponsorshipCreated(
        bytes32 indexed sponsorshipId,
        address indexed sponsor,
        address indexed beneficiary,
        uint64 totalBudget
    );

    /// @dev Emitted when a sponsorship is used
    event SponsorshipUsed(
        bytes32 indexed sponsorshipId,
        address indexed beneficiary,
        uint64 gasUsed
    );

    /// @dev Emitted when a sponsorship is cancelled
    event SponsorshipCancelled(
        bytes32 indexed sponsorshipId,
        uint64 refundedAmount
    );

    /// @notice Creates a basic sponsorship
    function createSponsorship(
        address beneficiary,
        uint64 maxGasPerTx,
        uint64 totalGasBudget,
        int64 expirationHeight
    ) external returns (bytes32 sponsorshipId);

    /// @notice Creates a sponsorship with conditions
    function createSponsorshipWithConditions(
        address beneficiary,
        uint64 maxGasPerTx,
        uint64 totalGasBudget,
        int64 expirationHeight,
        address[] calldata whitelistedContracts,
        uint256 maxTxValue,
        uint64 dailyGasLimit
    ) external returns (bytes32 sponsorshipId);

    /// @notice Cancels a sponsorship
    function cancelSponsorship(bytes32 sponsorshipId) external returns (uint64 refundedAmount);

    /// @notice Gets sponsorship details
    function getSponsorship(bytes32 sponsorshipId)
        external
        view
        returns (
            address sponsor,
            address beneficiary,
            uint64 maxGasPerTx,
            uint64 totalGasBudget,
            int64 expirationHeight,
            bool isActive,
            uint64 gasUsed,
            uint64 transactionCount
        );

    /// @notice Gets all sponsorships for a beneficiary
    function getSponsorshipsFor(address beneficiary)
        external
        view
        returns (bytes32[] memory sponsorshipIds);

    /// @notice Checks if a beneficiary is sponsored
    function isSponsored(address beneficiary, uint64 gasEstimate)
        external
        view
        returns (bool sponsored, bytes32 sponsorshipId);
}
