# Fee Sponsorship Precompile

The Fee Sponsorship precompile enables one account to pay transaction fees for another account, creating gasless transaction experiences for end users.

## Address

```
0x0000000000000000000000000000000000000900
```

## Overview

Fee sponsorship allows:
- **DApps** to onboard new users without requiring them to acquire gas tokens first
- **Protocols** to improve UX by covering transaction costs for their users
- **Subscription services** where users pay upfront for sponsored transactions
- **Conditional sponsorship** limiting what actions can be sponsored

## Interface

The complete interface is defined in [IFeeSponsor.sol](./IFeeSponsor.sol).

### Core Functions

#### createSponsorship

Create a basic sponsorship with no additional conditions.

```solidity
function createSponsorship(
    address beneficiary,
    uint64 maxGasPerTx,
    uint64 totalGasBudget,
    int64 expirationHeight
) external returns (bytes32 sponsorshipId);
```

**Parameters:**
- `beneficiary`: Address whose fees will be sponsored
- `maxGasPerTx`: Maximum gas allowed per transaction (prevents abuse)
- `totalGasBudget`: Total gas budget for all transactions
- `expirationHeight`: Block height when sponsorship expires

**Returns:** Unique sponsorship ID

**Example:**
```solidity
// Sponsor a user with 5M gas total, max 500k per tx, expires in ~7 days
bytes32 id = FEE_SPONSOR_CONTRACT.createSponsorship(
    userAddress,
    500000,
    5000000,
    int64(uint64(block.number + 50000))
);
```

#### createSponsorshipWithConditions

Create a sponsorship with advanced conditions.

```solidity
function createSponsorshipWithConditions(
    address beneficiary,
    uint64 maxGasPerTx,
    uint64 totalGasBudget,
    int64 expirationHeight,
    address[] calldata whitelistedContracts,
    uint256 maxTxValue,
    uint64 dailyGasLimit
) external returns (bytes32 sponsorshipId);
```

**Additional Parameters:**
- `whitelistedContracts`: Array of allowed contract addresses (empty = any)
- `maxTxValue`: Maximum transaction value in wei
- `dailyGasLimit`: Maximum gas per day (0 = unlimited)

**Example:**
```solidity
address[] memory whitelist = new address[](2);
whitelist[0] = myDAppContract;
whitelist[1] = myTokenContract;

bytes32 id = FEE_SPONSOR_CONTRACT.createSponsorshipWithConditions(
    userAddress,
    200000,      // max gas per tx
    2000000,     // total budget
    int64(uint64(block.number + 50000)),
    whitelist,   // only these contracts
    0.01 ether,  // max tx value
    100000       // daily limit
);
```

#### cancelSponsorship

Cancel a sponsorship and refund unused gas budget.

```solidity
function cancelSponsorship(bytes32 sponsorshipId)
    external
    returns (uint64 refundedAmount);
```

**Note:** Only the original sponsor can cancel their sponsorship.

#### getSponsorship

Query sponsorship details.

```solidity
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
```

#### isSponsored

Check if a beneficiary has sufficient sponsorship for a transaction.

```solidity
function isSponsored(address beneficiary, uint64 gasEstimate)
    external
    view
    returns (bool sponsored, bytes32 sponsorshipId);
```

## Events

### SponsorshipCreated
```solidity
event SponsorshipCreated(
    bytes32 indexed sponsorshipId,
    address indexed sponsor,
    address indexed beneficiary,
    uint64 totalBudget
);
```

### SponsorshipUsed
```solidity
event SponsorshipUsed(
    bytes32 indexed sponsorshipId,
    address indexed beneficiary,
    uint64 gasUsed
);
```

### SponsorshipCancelled
```solidity
event SponsorshipCancelled(
    bytes32 indexed sponsorshipId,
    uint64 refundedAmount
);
```

## Usage Examples

### 1. Simple DApp Sponsorship

See [SimpleDAppSponsorship.sol](./examples/SimpleDAppSponsorship.sol) for a complete example of sponsoring new users.

```solidity
import "./IFeeSponsor.sol";

contract MyDApp {
    function sponsorNewUser(address user) external {
        bytes32 sponsorshipId = FEE_SPONSOR_CONTRACT.createSponsorship(
            user,
            500000,   // 500k gas per tx
            5000000,  // 5M total
            int64(uint64(block.number + 100000)) // ~14 days
        );
        // User can now send transactions without gas!
    }
}
```

### 2. Conditional Sponsorship

See [ConditionalSponsorship.sol](./examples/ConditionalSponsorship.sol) for tiered sponsorship with whitelists.

```solidity
// Create basic tier with limited access
function createBasicTier(address user) external {
    address[] memory allowed = new address[](1);
    allowed[0] = myContract;

    FEE_SPONSOR_CONTRACT.createSponsorshipWithConditions(
        user,
        200000,
        2000000,
        int64(uint64(block.number + 50000)),
        allowed,      // only myContract
        0.01 ether,   // max 0.01 ETH per tx
        100000        // 100k gas per day
    );
}
```

### 3. Subscription Service

See [SubscriptionService.sol](./examples/SubscriptionService.sol) for a subscription-based model.

```solidity
// Users pay upfront for sponsored transactions
function subscribe(uint256 planId) external payable {
    require(msg.value >= plans[planId].price);

    bytes32 sponsorshipId = FEE_SPONSOR_CONTRACT.createSponsorship(
        msg.sender,
        plans[planId].maxGasPerTx,
        plans[planId].gasAllocation,
        int64(uint64(block.number + plans[planId].duration))
    );

    // User now has sponsored transactions for the duration
}
```

## Gas Costs

Approximate gas costs for operations:

| Operation | Gas Cost |
|-----------|----------|
| Create Basic Sponsorship | ~50,000 |
| Create Conditional Sponsorship | ~65,000 |
| Cancel Sponsorship | ~30,000 |
| Get Sponsorship (view) | ~3,000 |
| Is Sponsored (view) | ~5,000 |

## Best Practices

### 1. Set Reasonable Limits

Always set appropriate limits to prevent abuse:

```solidity
// ✅ Good: Reasonable limits
createSponsorship(user, 500000, 5000000, expirationHeight);

// ❌ Bad: No limits, vulnerable to abuse
createSponsorship(user, type(uint64).max, type(uint64).max, expirationHeight);
```

### 2. Use Whitelists for DApps

If sponsoring specific interactions, whitelist your contracts:

```solidity
// ✅ Good: Users can only interact with your protocol
address[] memory whitelist = [dappContract, tokenContract];
createSponsorshipWithConditions(
    user, maxGas, budget, expiration,
    whitelist,  // restricted
    maxValue, dailyLimit
);
```

### 3. Monitor Usage

Check sponsorship status periodically:

```solidity
function checkSponsorship(bytes32 id) public view {
    (, , , , , bool isActive, uint64 gasUsed, uint64 txCount) =
        FEE_SPONSOR_CONTRACT.getSponsorship(id);

    if (!isActive || gasUsed > warningThreshold) {
        // Take action: notify user, cancel, or renew
    }
}
```

### 4. Handle Expiration

Set reasonable expiration times and handle renewal:

```solidity
function renewSponsorshipIfNeeded(address user) external {
    (bool sponsored, bytes32 sponsorshipId) =
        FEE_SPONSOR_CONTRACT.isSponsored(user, 100000);

    if (!sponsored) {
        // Create new sponsorship
        createSponsorship(user, maxGas, budget, newExpiration);
    }
}
```

## Common Patterns

### Pattern 1: New User Onboarding

```solidity
// Give new users enough gas for initial interactions
function onboardUser(address newUser) external {
    require(!hasBeenSponsored[newUser], "Already onboarded");

    bytes32 id = FEE_SPONSOR_CONTRACT.createSponsorship(
        newUser,
        300000,   // enough for most transactions
        1500000,  // ~5 transactions
        int64(uint64(block.number + 28800)) // 2 days
    );

    hasBeenSponsored[newUser] = true;
}
```

### Pattern 2: Action-Specific Sponsorship

```solidity
// Sponsor only specific high-value actions
function sponsorPremiumAction(address user) external {
    address[] memory whitelist = new address[](1);
    whitelist[0] = premiumFeatureContract;

    FEE_SPONSOR_CONTRACT.createSponsorshipWithConditions(
        user,
        1000000,  // premium actions need more gas
        1000000,  // single use
        int64(uint64(block.number + 100)), // expires quickly
        whitelist,
        0,        // no value transfers
        0         // no daily limit
    );
}
```

### Pattern 3: Subscription Tiers

```solidity
enum Tier { Basic, Premium, Enterprise }

function createTierSponsorship(address user, Tier tier) external {
    if (tier == Tier.Basic) {
        // Limited sponsorship
        createSponsorship(user, 200000, 2000000, shortExpiry);
    } else if (tier == Tier.Premium) {
        // More generous
        createSponsorship(user, 500000, 10000000, mediumExpiry);
    } else {
        // Enterprise: unlimited
        createSponsorship(user, 2000000, 100000000, longExpiry);
    }
}
```

## Security Considerations

### 1. Prevent Sponsorship Abuse

- Always set `maxGasPerTx` to prevent single expensive transactions
- Use `totalGasBudget` to limit total exposure
- Set expiration heights to avoid indefinite sponsorships

### 2. Access Control

- Only authorized addresses should create sponsorships
- Implement proper role management for sponsorship managers

```solidity
modifier onlyAuthorized() {
    require(isAuthorized[msg.sender], "Not authorized");
    _;
}

function sponsorUser(address user) external onlyAuthorized {
    // Create sponsorship
}
```

### 3. Whitelist Management

- Carefully manage whitelisted contracts
- Audit contracts before adding to whitelist
- Implement multi-sig for whitelist updates

### 4. Monitor Budget Depletion

- Track sponsorship usage
- Alert when budgets are running low
- Implement automatic refunding or cancellation

## Integration Guide

### Step 1: Import Interface

```solidity
import "path/to/IFeeSponsor.sol";
```

### Step 2: Use Constant

```solidity
// Interface provides convenient constant
bytes32 id = FEE_SPONSOR_CONTRACT.createSponsorship(...);
```

### Step 3: Handle Events

```solidity
// Listen for events in your frontend
const filter = feeSponsorContract.filters.SponsorshipCreated(null, sponsorAddress);
feeSponsorContract.on(filter, (sponsorshipId, sponsor, beneficiary, budget) => {
    console.log(`Sponsorship created: ${sponsorshipId}`);
});
```

## Testing

See the test suite in `/tests/solidity/suites/precompiles/feesponsor/` for comprehensive examples.

## CLI Usage

```bash
# Query sponsorship
evmd query evm call-precompile 0x0000000000000000000000000000000000000900 \
  "getSponsorship(bytes32)" <sponsorship-id>

# Check if user is sponsored
evmd query evm call-precompile 0x0000000000000000000000000000000000000900 \
  "isSponsored(address,uint64)" <user-address> 100000
```

## Frontend Integration

```typescript
import { ethers } from 'ethers';
import FeeSponsorABI from './IFeeSponsor.json';

const FEE_SPONSOR_ADDRESS = '0x0000000000000000000000000000000000000900';
const feeSponsor = new ethers.Contract(
  FEE_SPONSOR_ADDRESS,
  FeeSponsorABI,
  provider
);

// Check if user is sponsored before sending transaction
const [isSponsored, sponsorshipId] = await feeSponsor.isSponsored(
  userAddress,
  estimatedGas
);

if (isSponsored) {
  // User transaction will be sponsored, show "Free transaction" in UI
  console.log(`Transaction sponsored by ${sponsorshipId}`);
}
```

## FAQ

**Q: Can a user have multiple sponsorships?**
A: Yes! Users can have multiple active sponsorships. The protocol will use the first matching one.

**Q: What happens if sponsorship runs out mid-transaction?**
A: The transaction will fail if there's insufficient sponsored gas. The user would need to have their own gas.

**Q: Can I update a sponsorship after creation?**
A: No. You must cancel and create a new one.

**Q: How do refunds work?**
A: When canceling, unused gas is refunded proportionally based on the remaining budget.

**Q: Can I sponsor specific function calls?**
A: Use `whitelistedContracts` to restrict which contracts can be called. For function-level control, implement checks in your contract.

## Further Reading

- [Fee Sponsorship Implementation Guide](../../../../FEE_SPONSORSHIP_IMPLEMENTATION.md)
- [Precompile Development](../../../../precompiles/README.md)
- [EVM Module Documentation](../../../../x/vm/README.md)

## Support

For questions or issues:
- GitHub: https://github.com/cosmos/evm/issues
- Discord: #cosmos-evm
