# Fee Sponsorship Documentation

## Table of Contents
1. [What is Fee Sponsorship?](#1-what-is-fee-sponsorship)
2. [Architecture](#2-architecture)
3. [Features](#3-features)
4. [Key Files](#4-key-files)
5. [Workflow](#5-workflow)
6. [Important Behaviors](#6-important-behaviors)
7. [Testing](#7-testing)
8. [Alternative Approaches](#8-alternative-approaches)
9. [Pros & Cons](#9-pros--cons)

---

## 1. What is Fee Sponsorship?

Enables one account (**sponsor**) to pay gas fees for another (**beneficiary**), creating gasless transaction experiences.

**Use Cases**: User onboarding, better UX, subscription models, conditional access to dApps.

---

## 2. Architecture

```
User Tx → Ante Handler → Sponsorship Check → Fee Deduction → EVM Execution
                              ↓
                         VM Keeper
                         (Storage)
```

**Core Components**:
| Component | Location | Purpose |
|-----------|----------|---------|
| Precompile | `precompiles/feesponsor/` | Solidity interface at `0x0...0900` |
| VM Keeper | `x/vm/keeper/sponsorship.go` | State management & validation |
| Ante Handler | `ante/evm/mono_decorator.go` | Intercepts tx, routes fees |
| Proto | `proto/.../sponsorship.proto` | Data structures |

---

## 3. Features

### Basic Sponsorship
- Beneficiary address, max gas/tx, total budget, expiration height

### Conditional Sponsorship
- **Contract Whitelist**: Only sponsor specific contracts
- **Max Transaction Value**: Cap ETH value per tx
- **Daily Gas Limit**: Rate limiting per day

### Management
- Create/Cancel via Solidity interface
- Query status, budget, usage stats
- Auto-expiry at block height

---

## 4. Key Files

| File | Lines | Purpose |
|------|-------|---------|
| `precompiles/feesponsor/feesponsor.go` | ~322 | Precompile implementation |
| `x/vm/keeper/sponsorship.go` | ~515 | Core logic & storage |
| `ante/evm/mono_decorator.go` | +87 | Transaction interception |
| `proto/.../sponsorship.proto` | ~97 | Data structures |

**Test Files** (`hardhat-project/`):
- `fee-sponsorship-demo.js` - Basic demo with balance tracking
- `test-conditional-sponsorship.js` - Conditional tests
- `GasHeavy.sol` - High gas contract for daily limit testing

---

## 5. Workflow

### Creating Sponsorship
```solidity
IFeeSponsor fs = IFeeSponsor(0x0000000000000000000000000000000000000900);

bytes32 id = fs.createSponsorshipWithConditions(
    beneficiary,
    500000,              // max gas/tx
    5000000,             // total budget
    block.number + 50000,// expiration
    [contractAddr],      // whitelist
    0.1 ether,           // max tx value
    100000               // daily gas limit
);
```

### Transaction Flow
1. Beneficiary submits normal transaction
2. Ante handler calls `GetActiveSponsorshipFor()`
3. VM Keeper validates: active, not expired, within limits, sponsor has balance, conditions met
4. **If valid**: Sponsor pays fees, usage tracked
5. **If invalid**: Falls back to beneficiary paying (see [Important Behaviors](#6-important-behaviors))

### Cancelling
```solidity
uint64 refunded = fs.cancelSponsorship(sponsorshipId);
```

---

## 6. Important Behaviors

### Fallback on Rejection
**Critical**: Sponsorship rejection does NOT block transactions.

```
Condition Failed → Sponsorship Rejected → Beneficiary Must Pay
                                              ↓
                            Has funds? → TX succeeds (self-paid)
                            No funds?  → TX fails ("insufficient funds")
```

**Implication**: To test condition enforcement, beneficiaries must have minimal funds so rejection causes failure.

### EIP-1559 Gas Prices
Always fetch current base fee dynamically:
```javascript
const feeData = await ethers.provider.getFeeData();
const maxFeePerGas = feeData.maxFeePerGas + (feeData.maxFeePerGas / 5n); // +20% buffer
```

**Common Error**: `max fee per gas less than block base fee` - means hardcoded gas price is too low.

### Gas Refunds
Unused gas is refunded to the **sponsor** (not beneficiary) for sponsored transactions.

---

## 7. Testing

### Run Tests
```bash
cd hardhat-project
npm install
npm run demo              # Basic sponsorship demo
npm run test-conditional  # Conditional sponsorship tests
```

### Test Results Interpretation

| Scenario | Sponsorship | Transaction | Who Pays |
|----------|-------------|-------------|----------|
| Whitelisted contract | ✅ Valid | ✅ Success | Sponsor |
| Non-whitelisted | ❌ Rejected | ❌ Fails* | N/A |
| Value > maxTxValue | ❌ Rejected | ❌ Fails* | N/A |
| Value ≤ maxTxValue | ✅ Valid | ✅ Success | Sponsor |
| Daily limit exceeded | ❌ Rejected | ✅/❌** | Beneficiary |

\* Fails if beneficiary lacks funds; succeeds (self-paid) if they have funds.
\** Depends on beneficiary balance.

### Output Labels
- **SPONSORED**: Sponsor paid gas
- **SELF-PAID**: Beneficiary paid (sponsorship rejected but had funds)
- **FAILED**: TX rejected (sponsorship rejected, no funds)

---

## 8. Alternative Approaches

| Method | Chain Mod | EVM Native | Wallet Support | Best For |
|--------|-----------|------------|----------------|----------|
| **Precompile** (this) | Required | ✅ | Universal | Chain-native UX |
| Relayer | No | External | Special | Cross-chain apps |
| Account Abstraction | Optional | ✅ | Special | Next-gen dApps |
| FeeGrant (Cosmos) | No | ❌ | Cosmos | Non-EVM apps |

---

## 9. Pros & Cons

### Pros
- ✅ True gasless UX - works with any wallet
- ✅ Low overhead - single KV lookup per tx
- ✅ Atomic operations - no race conditions
- ✅ Powerful conditions - whitelist, value caps, daily limits
- ✅ Abuse protection - per-tx caps, sponsor balance validation

### Cons
- ❌ Chain-specific - not portable to other EVMs
- ❌ Proactive funding - sponsors must maintain balance
- ❌ No partial sponsorship - all or nothing
- ❌ Simple selection - first-match only, no priority

---

## Quick Reference

### Precompile Address
```
0x0000000000000000000000000000000000000900
```

### Key Functions
```solidity
// Create
createSponsorship(beneficiary, maxGasPerTx, totalBudget, expiration)
createSponsorshipWithConditions(..., whitelist, maxTxValue, dailyLimit)

// Manage
cancelSponsorship(sponsorshipId) returns (refundedGas)

// Query
isSponsored(beneficiary, gasEstimate) returns (bool, sponsorshipId)
getSponsorship(sponsorshipId) returns (details...)
getSponsorshipsFor(beneficiary) returns (ids[])
```

### Gas Price Template
```javascript
const feeData = await ethers.provider.getFeeData();
const gasOverrides = {
  maxFeePerGas: feeData.maxFeePerGas + (feeData.maxFeePerGas / 5n),
  maxPriorityFeePerGas: 1n
};
await contract.method(gasOverrides);
```
