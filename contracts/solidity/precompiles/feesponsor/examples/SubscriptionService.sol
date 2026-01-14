// SPDX-License-Identifier: LGPL-3.0-only
pragma solidity >=0.8.17;

import "../IFeeSponsor.sol";

/// @title SubscriptionService
/// @notice Example demonstrating subscription-based fee sponsorship
/// @dev Users pay upfront to receive sponsored transactions for a period
contract SubscriptionService {
    /// @notice Subscription plan details
    struct Plan {
        uint256 price; // Price in wei
        uint64 gasAllocation; // Total gas allocated
        uint64 maxGasPerTx; // Max gas per transaction
        uint64 duration; // Duration in blocks
        bool active; // Whether plan is available
    }

    /// @notice User subscription details
    struct Subscription {
        uint256 planId;
        bytes32 sponsorshipId;
        uint256 startBlock;
        uint256 endBlock;
        bool active;
    }

    /// @notice Service owner
    address public owner;

    /// @notice Available subscription plans
    mapping(uint256 => Plan) public plans;

    /// @notice User subscriptions
    mapping(address => Subscription) public subscriptions;

    /// @notice Next plan ID
    uint256 public nextPlanId;

    event PlanCreated(uint256 indexed planId, uint256 price, uint64 gasAllocation);
    event SubscriptionPurchased(address indexed user, uint256 indexed planId, bytes32 sponsorshipId);
    event SubscriptionCancelled(address indexed user, uint256 refund);

    constructor() {
        owner = msg.sender;

        // Create default plans
        createPlan(0.1 ether, 5000000, 500000, 50000); // Basic: 0.1 ETH for 5M gas
        createPlan(0.5 ether, 30000000, 1000000, 200000); // Premium: 0.5 ETH for 30M gas
        createPlan(1 ether, 100000000, 2000000, 500000); // Enterprise: 1 ETH for 100M gas
    }

    /// @notice Create a new subscription plan
    function createPlan(
        uint256 price,
        uint64 gasAllocation,
        uint64 maxGasPerTx,
        uint64 duration
    ) public returns (uint256 planId) {
        require(msg.sender == owner, "Only owner");

        planId = nextPlanId++;
        plans[planId] = Plan({
            price: price,
            gasAllocation: gasAllocation,
            maxGasPerTx: maxGasPerTx,
            duration: duration,
            active: true
        });

        emit PlanCreated(planId, price, gasAllocation);
    }

    /// @notice Purchase a subscription plan
    /// @param planId ID of the plan to purchase
    function subscribe(uint256 planId) external payable returns (bytes32 sponsorshipId) {
        Plan memory plan = plans[planId];
        require(plan.active, "Plan not active");
        require(msg.value >= plan.price, "Insufficient payment");
        require(!subscriptions[msg.sender].active, "Already subscribed");

        // Calculate expiration
        uint256 expirationBlock = block.number + plan.duration;

        // Create sponsorship for the user
        sponsorshipId = FEE_SPONSOR_CONTRACT.createSponsorship(
            msg.sender,
            plan.maxGasPerTx,
            plan.gasAllocation,
            int64(uint64(expirationBlock))
        );

        // Record subscription
        subscriptions[msg.sender] = Subscription({
            planId: planId,
            sponsorshipId: sponsorshipId,
            startBlock: block.number,
            endBlock: expirationBlock,
            active: true
        });

        emit SubscriptionPurchased(msg.sender, planId, sponsorshipId);

        // Refund excess payment
        if (msg.value > plan.price) {
            payable(msg.sender).transfer(msg.value - plan.price);
        }
    }

    /// @notice Cancel subscription and get partial refund
    /// @dev Refunds unused gas proportionally
    function cancelSubscription() external returns (uint256 refund) {
        Subscription storage sub = subscriptions[msg.sender];
        require(sub.active, "No active subscription");

        // Cancel the sponsorship and get refunded gas
        uint64 refundedGas = FEE_SPONSOR_CONTRACT.cancelSponsorship(sub.sponsorshipId);

        // Calculate proportional refund
        Plan memory plan = plans[sub.planId];
        refund = (uint256(refundedGas) * plan.price) / plan.gasAllocation;

        // Deactivate subscription
        sub.active = false;

        // Send refund
        if (refund > 0) {
            payable(msg.sender).transfer(refund);
        }

        emit SubscriptionCancelled(msg.sender, refund);
    }

    /// @notice Renew an expired subscription
    function renewSubscription() external payable {
        Subscription storage sub = subscriptions[msg.sender];
        require(!sub.active || block.number > sub.endBlock, "Subscription still active");

        // Cancel old subscription if exists
        if (sub.active && sub.sponsorshipId != bytes32(0)) {
            try FEE_SPONSOR_CONTRACT.cancelSponsorship(sub.sponsorshipId) {} catch {}
        }

        // Create new subscription with same plan
        uint256 planId = sub.planId;
        delete subscriptions[msg.sender]; // Clear old subscription

        this.subscribe{value: msg.value}(planId);
    }

    /// @notice Check if user has active subscription
    function hasActiveSubscription(address user) external view returns (bool) {
        Subscription memory sub = subscriptions[user];
        if (!sub.active) return false;
        if (block.number > sub.endBlock) return false;

        // Verify sponsorship is still active
        (, , , , , bool isActive, , ) = FEE_SPONSOR_CONTRACT.getSponsorship(sub.sponsorshipId);
        return isActive;
    }

    /// @notice Get subscription details
    function getSubscriptionDetails(address user)
        external
        view
        returns (
            uint256 planId,
            uint256 startBlock,
            uint256 endBlock,
            bool active,
            uint64 gasUsed,
            uint64 gasRemaining
        )
    {
        Subscription memory sub = subscriptions[user];
        require(sub.sponsorshipId != bytes32(0), "No subscription");

        planId = sub.planId;
        startBlock = sub.startBlock;
        endBlock = sub.endBlock;
        active = sub.active && block.number <= endBlock;

        (, , , uint64 totalBudget, , , gasUsed, ) = FEE_SPONSOR_CONTRACT.getSponsorship(sub.sponsorshipId);
        gasRemaining = totalBudget > gasUsed ? totalBudget - gasUsed : 0;
    }

    /// @notice Update plan availability
    function setPlanActive(uint256 planId, bool active) external {
        require(msg.sender == owner, "Only owner");
        plans[planId].active = active;
    }

    /// @notice Withdraw collected funds
    function withdraw() external {
        require(msg.sender == owner, "Only owner");
        payable(owner).transfer(address(this).balance);
    }

    /// @notice Get plan details
    function getPlan(uint256 planId)
        external
        view
        returns (
            uint256 price,
            uint64 gasAllocation,
            uint64 maxGasPerTx,
            uint64 duration,
            bool active
        )
    {
        Plan memory plan = plans[planId];
        return (plan.price, plan.gasAllocation, plan.maxGasPerTx, plan.duration, plan.active);
    }
}
