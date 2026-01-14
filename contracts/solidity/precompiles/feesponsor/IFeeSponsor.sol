// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

/// @dev The IFeeSponsor contract's address.
address constant FEE_SPONSOR_PRECOMPILE_ADDRESS = 0x0000000000000000000000000000000000000900;

/// @dev The IFeeSponsor contract's instance.
IFeeSponsor constant FEE_SPONSOR_CONTRACT = IFeeSponsor(FEE_SPONSOR_PRECOMPILE_ADDRESS);

/// @title IFeeSponsor
/// @author Cosmos EVM
/// @notice This interface enables fee sponsorship functionality where one account can pay
/// gas fees for another account's transactions. This is useful for onboarding new users,
/// DApp user acquisition, and gasless transaction experiences.
/// @dev The precompile contract's address is 0x0000000000000000000000000000000000000900.
interface IFeeSponsor {
    /// @dev Emitted when a new sponsorship is created
    /// @param sponsorshipId Unique identifier for the sponsorship
    /// @param sponsor Address of the account sponsoring fees
    /// @param beneficiary Address of the account whose fees will be sponsored
    /// @param totalBudget Total gas budget allocated for this sponsorship
    event SponsorshipCreated(
        bytes32 indexed sponsorshipId,
        address indexed sponsor,
        address indexed beneficiary,
        uint64 totalBudget
    );

    /// @dev Emitted when a sponsorship is used to pay for transaction fees
    /// @param sponsorshipId Unique identifier for the sponsorship
    /// @param beneficiary Address of the beneficiary whose transaction was sponsored
    /// @param gasUsed Amount of gas consumed from the sponsorship budget
    event SponsorshipUsed(
        bytes32 indexed sponsorshipId,
        address indexed beneficiary,
        uint64 gasUsed
    );

    /// @dev Emitted when a sponsorship is cancelled
    /// @param sponsorshipId Unique identifier for the sponsorship
    /// @param refundedAmount Amount of gas refunded to the sponsor
    event SponsorshipCancelled(
        bytes32 indexed sponsorshipId,
        uint64 refundedAmount
    );

    /// @notice Creates a basic sponsorship with no additional conditions
    /// @dev The caller will be the sponsor and will pay for the beneficiary's transaction fees
    /// @param beneficiary Address of the account whose fees will be sponsored
    /// @param maxGasPerTx Maximum gas allowed per transaction (prevents abuse)
    /// @param totalGasBudget Total gas budget allocated for all transactions
    /// @param expirationHeight Block height when this sponsorship expires
    /// @return sponsorshipId Unique identifier for the created sponsorship
    function createSponsorship(
        address beneficiary,
        uint64 maxGasPerTx,
        uint64 totalGasBudget,
        int64 expirationHeight
    ) external returns (bytes32 sponsorshipId);

    /// @notice Creates a sponsorship with advanced conditions
    /// @dev Allows specifying which contracts can be called, value limits, and daily gas limits
    /// @param beneficiary Address of the account whose fees will be sponsored
    /// @param maxGasPerTx Maximum gas allowed per transaction
    /// @param totalGasBudget Total gas budget allocated for all transactions
    /// @param expirationHeight Block height when this sponsorship expires
    /// @param whitelistedContracts Array of contract addresses that can be called (empty = any)
    /// @param maxTxValue Maximum transaction value allowed (in wei)
    /// @param dailyGasLimit Maximum gas that can be used per day (0 = unlimited)
    /// @return sponsorshipId Unique identifier for the created sponsorship
    function createSponsorshipWithConditions(
        address beneficiary,
        uint64 maxGasPerTx,
        uint64 totalGasBudget,
        int64 expirationHeight,
        address[] calldata whitelistedContracts,
        uint256 maxTxValue,
        uint64 dailyGasLimit
    ) external returns (bytes32 sponsorshipId);

    /// @notice Cancels an existing sponsorship and refunds remaining gas budget
    /// @dev Only the sponsor can cancel their own sponsorship
    /// @param sponsorshipId The ID of the sponsorship to cancel
    /// @return refundedAmount Amount of gas refunded to the sponsor
    function cancelSponsorship(bytes32 sponsorshipId) external returns (uint64 refundedAmount);

    /// @notice Retrieves detailed information about a sponsorship
    /// @param sponsorshipId The ID of the sponsorship to query
    /// @return sponsor Address of the sponsor
    /// @return beneficiary Address of the beneficiary
    /// @return maxGasPerTx Maximum gas per transaction
    /// @return totalGasBudget Total gas budget
    /// @return expirationHeight Block height when sponsorship expires
    /// @return isActive Whether the sponsorship is currently active
    /// @return gasUsed Amount of gas already consumed
    /// @return transactionCount Number of transactions sponsored so far
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

    /// @notice Gets all sponsorship IDs for a given beneficiary
    /// @param beneficiary Address to check for sponsorships
    /// @return sponsorshipIds Array of sponsorship IDs
    function getSponsorshipsFor(address beneficiary)
        external
        view
        returns (bytes32[] memory sponsorshipIds);

    /// @notice Checks if a beneficiary has active sponsorship for a given gas amount
    /// @dev This is a convenience function for wallets/frontends to check before sending transactions
    /// @param beneficiary Address to check
    /// @param gasEstimate Estimated gas for the transaction
    /// @return sponsored True if the beneficiary has sufficient sponsorship
    /// @return sponsorshipId The ID of the sponsorship that will be used (if sponsored is true)
    function isSponsored(address beneficiary, uint64 gasEstimate)
        external
        view
        returns (bool sponsored, bytes32 sponsorshipId);
}
