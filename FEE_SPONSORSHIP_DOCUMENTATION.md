# Fee Sponsorship Documentation

## Table of Contents
1. [What is Fee Sponsorship?](#1-what-is-fee-sponsorship)
2. [Implementation in This Codebase](#2-implementation-in-this-codebase)
3. [Features](#3-features)
4. [Major File Changes](#4-major-file-changes)
5. [Workflow Process](#5-workflow-process)
6. [Different Fee Sponsorship Methods](#6-different-fee-sponsorship-methods)
7. [Pros & Cons](#7-pros--cons)

---

## 1. What is Fee Sponsorship?

**Fee Sponsorship** is a mechanism that enables one account (the **sponsor**) to pay transaction gas fees on behalf of another account (the **beneficiary**). This creates gasless transaction experiences for end users.

### Key Concept
- **Sponsor**: Account that pays gas fees for others
- **Beneficiary**: Account whose transaction fees are covered
- **Sponsorship**: A funded budget with conditions that determines when fees are paid

### Use Cases
- **User Onboarding**: DApps can onboard new users without requiring them to acquire gas tokens first
- **Better UX**: Protocols improve user experience by absorbing transaction costs
- **Subscription Models**: Users pay upfront for a package of sponsored transactions
- **Conditional Access**: Limit sponsorship to specific contracts, actions, or value limits

---

## 2. Implementation in This Codebase

This implementation uses a **Precompile-based approach** integrated directly into the EVM execution layer.

### Architecture Overview

```
┌─────────────────────────────────────────────────────────────┐
│                    Transaction Flow                         │
│                                                              │
│  User Tx ──> Ante Handler ──> Sponsorship Check ──> EVM    │
│                     │                │                       │
│                     │                ▼                       │
│                     │         VM Keeper                      │
│                     │         (Storage)                      │
│                     ▼                                        │
│              Fee Deduction                                   │
│           (Sponsor/Beneficiary)                              │
└─────────────────────────────────────────────────────────────┘
```

### Core Components

1. **Precompile Contract** (`precompiles/feesponsor/`)
   - Address: `0x0000000000000000000000000000000000000900`
   - Exposes Solidity interface for sponsorship management
   - Handles: Create, Cancel, Query sponsorships

2. **VM Keeper** (`x/vm/keeper/sponsorship.go`)
   - Backend state management
   - Validates sponsorship conditions
   - Tracks usage and budget deductions

3. **Ante Handler** (`ante/evm/mono_decorator.go`)
   - Intercepts transactions before execution
   - Checks for active sponsorship
   - Redirects fee payment to sponsor

4. **Storage Layer** (Proto definitions)
   - Persistent storage of sponsorships
   - Indexing by beneficiary for fast lookups
   - Daily usage tracking for rate limits

---

## 3. Features

### 3.1 Basic Sponsorship
Create simple sponsorships with core parameters:
- **Beneficiary address**: Who gets sponsored
- **Max gas per tx**: Prevents abuse (per-transaction cap)
- **Total gas budget**: Total sponsorship pool
- **Expiration height**: Automatic expiry at block height

### 3.2 Conditional Sponsorship
Advanced sponsorships with restrictions:
- **Contract Whitelist**: Only sponsor calls to specific contracts
- **Max Transaction Value**: Limit the ETH value of sponsored transactions
- **Daily Gas Limits**: Cap gas usage per day
- **Signature Requirements**: Sponsor can require co-signing (future feature)

### 3.3 Sponsorship Management
- **Create**: Set up new sponsorships via Solidity interface
- **Cancel**: Sponsor can cancel and recover unused budget
- **Query**: Check sponsorship status, remaining budget, usage stats
- **Auto-expiry**: Sponsorships automatically deactivate at expiration height

### 3.4 Automatic Fee Routing
- Transparent to beneficiary (normal transaction flow)
- Ante handler automatically detects active sponsorship
- Fees deducted from sponsor's account instead of sender
- Usage tracked and budget decremented

### 3.5 Multi-Sponsorship Support
- Beneficiaries can have multiple active sponsorships
- System selects first valid sponsorship matching criteria
- Prioritization by creation order

---

## 4. Major File Changes

### 4.1 Precompile Layer

**`precompiles/feesponsor/feesponsor.go`** (322 lines)
- Implements the Fee Sponsor precompile contract
- Bridges Solidity calls to VM keeper functions
- Methods: `createSponsorship`, `createSponsorshipWithConditions`, `cancelSponsorship`, `getSponsorship`, `getSponsorshipsFor`, `isSponsored`

**`precompiles/feesponsor/abi.json`** (239 lines)
- ABI definition for the precompile interface
- Enables Solidity contracts to interact with sponsorships

**`contracts/solidity/precompiles/feesponsor/IFeeSponsor.sol`** (129 lines)
- Solidity interface definition
- Events: `SponsorshipCreated`, `SponsorshipUsed`, `SponsorshipCancelled`
- Developer-facing API

### 4.2 State Management

**`x/vm/keeper/sponsorship.go`** (488 lines)
- **Core Functions**:
  - `CreateSponsorship()`: Creates new sponsorship with validation
  - `GetActiveSponsorshipFor()`: Finds valid sponsorship for a transaction
  - `UseSponsorshipForTransaction()`: Deducts gas from budget
  - `CancelSponsorship()`: Deactivates and refunds
  - `isSponsorshipValid()`: Validates all conditions (expiry, balance, whitelist, limits)
- **Storage helpers**: CRUD operations for sponsorships
- **Indexing**: Fast beneficiary lookups
- **Daily usage tracking**: For rate limiting

**`proto/cosmos/evm/vm/v1/sponsorship.proto`** (97 lines)
- Protobuf definitions for data structures:
  - `FeeSponsor`: Main sponsorship record
  - `SponsorshipConditions`: Advanced conditions
  - `BeneficiarySponsorshipIndex`: Lookup index
  - `DailyUsage`: Daily gas tracking

### 4.3 Transaction Processing

**`ante/evm/mono_decorator.go`** (87 insertions)
- **Lines 178-188**: Early sponsorship check before balance validation
- **Lines 196-238**: Skip gas balance check for sponsored transactions (only validate value transfer)
- **Lines 267-280**: Redirect fee payment to sponsor address
- **Line 276**: Track sponsorship usage after transaction

**Changes brought**:
- Non-intrusive insertion into existing transaction flow
- Maintains backward compatibility with non-sponsored transactions
- Sponsor balance validation added to prevent failed transactions

### 4.4 Type System

**`x/vm/types/sponsorship.pb.go`** (2,230 lines - generated)
- Auto-generated Go code from protobuf definitions
- Marshaling/unmarshaling for storage
- Type safety for sponsorship structures

**`precompiles/types/static_precompiles.go`** (+10 lines)
- Registers Fee Sponsor precompile at address `0x0900`
- Makes it available at chain initialization

### 4.5 Demo & Testing

**`hardhat-project/`** (Complete setup)
- **Scripts**:
  - `fee-sponsorship-demo.js`: End-to-end demo (419 lines)
  - `sponsor-contract.js`: Contract-based sponsorship (350 lines)
  - `test-conditional-sponsorship.js`: Tests conditions (407 lines)
- **Contracts**:
  - `FeeSponsorDemo.sol`: Demo contract showcasing usage
  - `Counter.sol`: Simple sponsored interaction
  - `SimpleStorage.sol`: Storage interaction demo
- **Configuration**: Complete Hardhat setup with local node support

---

## 5. Workflow Process

### 5.1 Creating a Sponsorship

```solidity
// Sponsor calls the precompile
IFeeSponsor feeSponsor = IFeeSponsor(0x0000000000000000000000000000000000000900);

bytes32 sponsorshipId = feeSponsor.createSponsorship(
    beneficiaryAddress,  // Who to sponsor
    500000,              // Max 500k gas per tx
    5000000,             // Total 5M gas budget
    block.number + 50000 // Expires in ~7 days
);
```

**Flow**:
1. Sponsor calls precompile via Solidity interface
2. Precompile validates parameters (non-zero, future expiry)
3. VM Keeper generates unique sponsorship ID (hash of sponsor + beneficiary + height)
4. Sponsorship stored with `isActive = true`
5. Beneficiary index updated for fast lookups
6. Event `SponsorshipCreated` emitted

### 5.2 Transaction Execution (Beneficiary Sends Tx)

```
1. Beneficiary submits normal transaction (doesn't know about sponsorship)
        ↓
2. Transaction enters Ante Handler (mono_decorator.go)
        ↓
3. Sponsorship Check (line 178-188):
   - Extract beneficiary, gas limit, target contract, tx value
   - Call vmKeeper.GetActiveSponsorshipFor()
        ↓
4. VM Keeper searches for valid sponsorship:
   - Get all sponsorships for beneficiary (indexed lookup)
   - For each sponsorship, validate:
     ✓ Is active
     ✓ Not expired (currentHeight < expirationHeight)
     ✓ Gas limit within per-tx cap (gas ≤ maxGasPerTx)
     ✓ Sufficient budget (totalGasBudget ≥ gas)
     ✓ Sponsor has balance (sponsor balance ≥ gas * baseFee)
     ✓ If conditions exist:
       - Target in whitelist (or empty list)
       - Tx value ≤ maxTxValue
       - Daily usage + gas ≤ dailyGasLimit
   - Return first valid sponsorship
        ↓
5. Balance Validation (line 196-238):
   - If sponsored: Skip gas balance check, only verify value transfer
   - If not sponsored: Normal balance check (gas + value)
        ↓
6. Fee Deduction (line 267-280):
   - If sponsored: Use sponsor address as fee payer
   - If not sponsored: Use beneficiary address
        ↓
7. Update Sponsorship State (line 276):
   - Deduct gas from totalGasBudget
   - Increment gasUsed and transactionCount
   - Track daily usage (if daily limit set)
   - Deactivate if budget exhausted
   - Emit event SponsorshipUsed
        ↓
8. Execute transaction normally (EVM processes the transaction)
```

### 5.3 Cancelling a Sponsorship

```solidity
uint64 refunded = feeSponsor.cancelSponsorship(sponsorshipId);
```

**Flow**:
1. Caller invokes cancel method
2. VM Keeper validates caller is the sponsor
3. Set `isActive = false`, capture remaining budget
4. Remove from beneficiary index
5. Refund remaining budget (gas not consumed)
6. Event `SponsorshipCancelled` emitted

### 5.4 Querying Sponsorships

```solidity
// Check if beneficiary is sponsored for specific gas amount
(bool isSponsored, bytes32 id) = feeSponsor.isSponsored(beneficiary, 100000);

// Get sponsorship details
(address sponsor, address beneficiary, uint64 maxGas, ...) =
    feeSponsor.getSponsorship(sponsorshipId);

// Get all sponsorships for a beneficiary
bytes32[] memory ids = feeSponsor.getSponsorshipsFor(beneficiary);
```

---

## 6. Different Fee Sponsorship Methods

This implementation uses **Method 1: Precompile-Based Sponsorship**. Here's a comparison with alternative approaches:

### 6.1 **Precompile-Based Sponsorship** (Implemented)

**How it works**:
- Native precompile contract at fixed address (`0x0900`)
- Ante handler checks sponsorship before execution
- Direct integration with transaction lifecycle

**Advantages**:
- ✅ Native integration, no external dependencies
- ✅ Transparent to beneficiary (no special wallet support)
- ✅ Low overhead (single lookup per transaction)
- ✅ On-chain state ensures atomicity
- ✅ Works with any wallet/dApp

**Disadvantages**:
- ❌ Requires chain-level modifications
- ❌ Not portable across other EVMs without precompile support
- ❌ Sponsor must maintain balance proactively

**Best for**: Chain-native feature where full transparency and efficiency are priorities

---

### 6.2 Relayer-Based Flow

**How it works**:
- Off-chain relayer service monitors requests
- Beneficiary signs transaction with `gasPrice = 0`
- Relayer wraps and submits with their own gas payment
- Relayer tracks costs and manages reimbursement

**Advantages**:
- ✅ No chain modifications needed
- ✅ Works on any EVM chain
- ✅ Flexible business logic (off-chain pricing, subscriptions)

**Disadvantages**:
- ❌ Requires centralized relayer infrastructure
- ❌ Trust in relayer (censorship, uptime)
- ❌ Latency added by relayer
- ❌ Complex user flow (special wallet support)
- ❌ Relayer can become a single point of failure

**Best for**: Cross-chain applications where chain modification isn't possible

---

### 6.3 Sponsored Transactions (EIP-3074/Account Abstraction)

**How it works**:
- Uses `AUTH` and `AUTHCALL` opcodes (EIP-3074) or Account Abstraction (ERC-4337)
- User signs intent, sponsor creates invoker contract
- Invoker executes transaction on behalf of user, paying fees

**Advantages**:
- ✅ Standards-based approach
- ✅ Advanced features (batching, delegation)
- ✅ Future-proof (ecosystem alignment)

**Disadvantages**:
- ❌ Complex implementation (AA bundlers, EntryPoint contracts)
- ❌ Limited EIP-3074 support currently
- ❌ Requires wallet changes
- ❌ Higher gas overhead
- ❌ Fragmented standards still evolving

**Best for**: Next-generation dApps leveraging account abstraction

---

### 6.4 FeeGrant Integration (Cosmos SDK)

**How it works**:
- Uses Cosmos SDK's built-in `feegrant` module
- Granter authorizes another account to use allowance
- Works at Cosmos level, not EVM level

**Advantages**:
- ✅ Cosmos-native, no custom code
- ✅ Expiration and periodic allowances
- ✅ Proven module with existing tooling

**Disadvantages**:
- ❌ Cosmos transactions only (not EVM transactions)
- ❌ Not accessible from Solidity contracts
- ❌ Separate UX from EVM ecosystem
- ❌ Doesn't support conditional sponsorship based on target contract

**Best for**: Cosmos SDK applications with minimal EVM integration

---

### Comparison Table

| Feature | Precompile (Implemented) | Relayer-Based | Account Abstraction | FeeGrant |
|---------|--------------------------|---------------|---------------------|----------|
| **Chain Modification** | Required | Not Required | Optional | Not Required |
| **EVM Integration** | Native | External | Native | None |
| **Solidity Access** | ✅ Direct | ❌ Off-chain | ⚠️ Complex | ❌ Cosmos only |
| **Transparency** | ✅ Fully on-chain | ❌ Off-chain logic | ✅ On-chain | ✅ On-chain |
| **Gas Overhead** | Low | Medium | High | Low |
| **Complexity** | Medium | Low | High | Low |
| **Wallet Support** | Universal | Special | Special | Cosmos wallets |
| **Portability** | Low | High | High | Cosmos only |

---

## 7. Pros & Cons

### 7.1 Pros of This Implementation

#### User Experience
✅ **True Gasless Transactions**: Beneficiaries send transactions normally without special wallet support or awareness of sponsorships

✅ **Instant Activation**: No waiting for relayers or external services

✅ **Universal Compatibility**: Works with any wallet (MetaMask, WalletConnect, etc.) without modifications

#### Developer Experience
✅ **Simple Integration**: Easy Solidity interface - just call the precompile

✅ **Powerful Conditions**: Fine-grained control with whitelists, value limits, daily caps

✅ **Query Functions**: Real-time sponsorship status checks before sending transactions

#### Technical Benefits
✅ **Low Overhead**: Single KV store lookup per transaction, minimal gas increase

✅ **Atomic Operations**: Sponsorship checks and fee deductions in same transaction (no race conditions)

✅ **On-Chain Guarantees**: No trust in external relayers or infrastructure

✅ **Auto-Management**: Automatic expiry, budget tracking, and deactivation

✅ **Multi-Sponsor Support**: Beneficiaries can have multiple sponsorships (flexibility)

#### Security
✅ **Abuse Protection**: Per-tx caps, daily limits, contract whitelists prevent exploitation

✅ **Sponsor Control**: Only sponsor can cancel, guaranteed refunds for unused budget

✅ **Balance Validation**: Checks sponsor balance before accepting transaction (no failed transactions)

---

### 7.2 Cons of This Implementation

#### Portability
❌ **Chain-Specific**: Requires Cosmos EVM with precompile support - not portable to Ethereum mainnet or other EVMs

❌ **Upgrade Dependency**: Changes to sponsorship logic require chain upgrades

#### Sponsor Management
❌ **Proactive Funding**: Sponsors must maintain sufficient balance at all times, or sponsorships fail

❌ **No Automatic Refills**: Budget exhaustion requires manual creation of new sponsorship

❌ **Upfront Capital**: Sponsors must lock funds in gas budget (opportunity cost)

#### Limitations
❌ **Simple Selection Logic**: First-match sponsorship selection (no priority/weighting)

❌ **No Partial Sponsorship**: Can't sponsor 50% of fees - all or nothing

❌ **Limited Conditions**: Whitelisting by contract address only (not by function signature)

#### Operational
❌ **State Bloat**: Each sponsorship adds storage (though minimal with indexing)

❌ **No Transfer**: Sponsorships are non-transferable (locked to beneficiary/sponsor pair)

❌ **Expiry Management**: Expired sponsorships remain in storage (cleanup not automatic)

#### Developer Considerations
❌ **Testing Complexity**: Requires local chain with precompile (standard testnets won't work)

❌ **Monitoring**: Sponsors need off-chain monitoring to track budget usage and refill

❌ **Limited Tooling**: No existing dashboards/explorers for sponsorship visualization (yet)

---

## Summary

This **precompile-based fee sponsorship implementation** provides a production-ready solution for gasless transactions on Cosmos EVM chains. It balances:

- **Simplicity**: Easy integration for developers
- **Performance**: Low overhead and native execution
- **Flexibility**: Conditional sponsorships with multiple parameters
- **Security**: Abuse protection and on-chain guarantees

**Trade-off**: Chain-specific implementation (not portable to other EVMs) in exchange for superior performance and native integration.

**Ideal for**: Cosmos EVM-based applications prioritizing UX, where chain-level features can be leveraged for competitive advantage.

---

## Quick Start

### For Sponsors (DApp Developers)
```solidity
// 1. Deploy your DApp contract
// 2. Create sponsorship for your users
IFeeSponsor sponsor = IFeeSponsor(0x0000000000000000000000000000000000000900);

bytes32 id = sponsor.createSponsorshipWithConditions(
    userAddress,
    300000,                  // 300k gas per tx
    3000000,                 // 3M total budget
    block.number + 100000,   // ~2 weeks
    [address(myDAppContract)], // Only your contract
    0.01 ether,              // Max 0.01 ETH per tx
    100000                   // 100k gas per day
);
```

### For Beneficiaries (End Users)
```solidity
// Just send transactions normally - sponsorship is automatic!
myDApp.performAction(); // Fees paid by sponsor if sponsorship active
```

### Testing Locally
```bash
# 1. Start local node
./local_node.sh

# 2. Run demo
cd hardhat-project
npm install
npm run demo:fee-sponsorship
```

---

## References

- **Precompile Interface**: `contracts/solidity/precompiles/feesponsor/IFeeSponsor.sol`
- **Implementation**: `precompiles/feesponsor/feesponsor.go`
- **Storage Layer**: `x/vm/keeper/sponsorship.go`
- **Demo Scripts**: `hardhat-project/scripts/`
