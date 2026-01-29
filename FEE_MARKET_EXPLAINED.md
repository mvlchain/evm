# Understanding the Fee Market: A Simple Guide

This document explains how gas fees work in Cosmos EVM, using simple language and examples.

---

## Table of Contents

1. [Basic Concepts](#basic-concepts)
2. [EIP-1559: The Fee Model](#eip-1559-the-fee-model)
3. [How Base Fee Changes](#how-base-fee-changes)
4. [Transaction Fee Calculation](#transaction-fee-calculation)
5. [Fee Sponsorship](#fee-sponsorship)
6. [Configuration Parameters](#configuration-parameters)
7. [Common Errors & Solutions](#common-errors--solutions)

---

## Basic Concepts

### What is Gas?

**Gas** is a unit that measures computational work. Every operation in the EVM costs gas:

| Operation | Gas Cost |
|-----------|----------|
| Simple transfer | 21,000 |
| Storage write | 20,000 |
| Addition | 3 |
| Contract creation | 32,000+ |

**Example:** Sending 1 ETH to someone costs 21,000 gas. Calling a smart contract might cost 50,000-500,000 gas depending on complexity.

### What is Gas Price?

**Gas Price** is how much you pay per unit of gas, measured in **wei** (the smallest unit).

```
Fee = Gas Used × Gas Price

Example:
- Gas used: 21,000
- Gas price: 100 gwei (100,000,000,000 wei)
- Fee: 21,000 × 100 gwei = 2,100,000 gwei = 0.0021 ETH
```

### Unit Conversions

```
1 ETH = 1,000,000,000 gwei = 1,000,000,000,000,000,000 wei

Common gas prices:
- 1 gwei = 1,000,000,000 wei (10^9)
- 100 gwei = 100,000,000,000 wei
```

---

## EIP-1559: The Fee Model

Cosmos EVM uses **EIP-1559**, Ethereum's modern fee system. Instead of a single gas price, there are three components:

### 1. Base Fee

- Set by the **network**, not by users
- Changes every block based on demand
- **Burned** (removed from circulation)
- You MUST pay at least this amount

### 2. Priority Fee (Tip)

- Set by the **user**
- Goes to the block producer (validator)
- Higher tip = faster inclusion
- Optional but recommended

### 3. Max Fee

- The **maximum** you're willing to pay per gas
- Must be >= base fee + priority fee
- Protects you from sudden fee spikes

### Visual Representation

```
Your Transaction:
┌─────────────────────────────────────────┐
│  Max Fee Per Gas: 100 gwei              │  ← Maximum you'll pay
├─────────────────────────────────────────┤
│  Base Fee: 50 gwei (network sets this)  │  ← Burned
│  Priority Fee: 2 gwei (your tip)        │  ← To validator
│  ─────────────────────────────────────  │
│  Effective Price: 52 gwei               │  ← What you actually pay
│  Refund: 48 gwei                        │  ← Returned to you
└─────────────────────────────────────────┘
```

### Formula

```
Effective Gas Price = min(maxFeePerGas, baseFee + priorityFee)

Actual Fee = Effective Gas Price × Gas Used
Refund = (maxFeePerGas - Effective Gas Price) × Gas Used
```

---

## How Base Fee Changes

The base fee adjusts automatically to target **50% block utilization**.

### The Algorithm

```
Gas Target = Block Gas Limit ÷ 2

If block is MORE than 50% full:
    Base fee INCREASES (up to 12.5% per block)

If block is LESS than 50% full:
    Base fee DECREASES (up to 12.5% per block)

If block is exactly 50% full:
    Base fee stays the SAME
```

### Example Scenario

```
Block Gas Limit: 30,000,000
Gas Target: 15,000,000 (50%)

Block 1: Gas used = 20,000,000 (67% full)
         Base fee: 100 gwei → increases

Block 2: Gas used = 25,000,000 (83% full)
         Base fee: 110 gwei → increases more

Block 3: Gas used = 10,000,000 (33% full)
         Base fee: 120 gwei → decreases

Block 4: Gas used = 5,000,000 (17% full)
         Base fee: 105 gwei → decreases more
```

### Why This Matters

- **High network activity** = Higher base fee = More expensive transactions
- **Low network activity** = Lower base fee = Cheaper transactions
- **Predictable fees** = You can estimate costs reliably

---

## Transaction Fee Calculation

### Step-by-Step Example

**Scenario:** You want to send tokens using a smart contract.

```
Your transaction parameters:
- Gas Limit: 100,000 (maximum gas you're willing to use)
- Max Fee Per Gas: 150 gwei
- Priority Fee Per Gas: 2 gwei

Current network state:
- Base Fee: 80 gwei

Calculation:
1. Effective Gas Price = min(150, 80 + 2) = 82 gwei
2. Transaction executes, uses 65,000 gas
3. Actual Fee = 82 gwei × 65,000 = 5,330,000 gwei = 0.00533 ETH
4. Unused gas refund = (100,000 - 65,000) × 82 gwei = 2,870,000 gwei
```

### What You Pay vs What's Deducted

```
Initially reserved: Gas Limit × Max Fee = 100,000 × 150 gwei = 15,000,000 gwei
Actually paid: 65,000 × 82 gwei = 5,330,000 gwei
Refunded: 15,000,000 - 5,330,000 = 9,670,000 gwei
```

---

## Fee Sponsorship

Fee sponsorship allows one account (sponsor) to pay gas fees for another account (beneficiary).

### How It Works

```
Normal Transaction:
┌──────────┐     ┌──────────┐     ┌──────────┐
│  User    │────▶│  Network │────▶│ Contract │
│ pays fee │     │          │     │          │
└──────────┘     └──────────┘     └──────────┘

Sponsored Transaction:
┌──────────┐     ┌──────────┐     ┌──────────┐
│  User    │────▶│  Network │────▶│ Contract │
│ (no fee) │     │          │     │          │
└──────────┘     └──────────┘     └──────────┘
      ▲                │
      │                │ fee charged to
      │                ▼
      │          ┌──────────┐
      └──────────│ Sponsor  │
   sponsorship   │ pays fee │
                 └──────────┘
```

### Sponsorship Parameters

When creating a sponsorship:

| Parameter | Description | Example |
|-----------|-------------|---------|
| `beneficiary` | Address that gets free transactions | `0xABC...` |
| `maxGasPerTx` | Maximum gas per single transaction | 1,000,000 |
| `totalGasBudget` | Total gas the sponsor will cover | 100,000,000 |
| `expiration` | Block number when sponsorship ends | 1,000,000 |

### Fee Flow with Sponsorship

```
1. Beneficiary submits transaction (has 0 balance)
2. Ante handler checks: "Is this address sponsored?"
3. Found sponsorship → Fee charged to SPONSOR, not beneficiary
4. Sponsorship budget reduced by gas used
5. Beneficiary's transaction executes successfully
```

### Conditional Sponsorship

Sponsors can add conditions:

```
conditions: {
    whitelistedContracts: ["0x123...", "0x456..."],  // Only these contracts
    maxTransactionValue: 1000000000000000000,        // Max 1 ETH value
    dailyGasLimit: 10000000                          // Max 10M gas per day
}
```

---

## Configuration Parameters

These parameters control fee market behavior:

### Core Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `baseFee` | 1 gwei | Starting base fee |
| `noBaseFee` | false | Disable EIP-1559 (use legacy) |
| `enableHeight` | 0 | Block height to enable base fee |
| `baseFeeChangeDenominator` | 8 | Controls how fast base fee changes (max 12.5% per block) |
| `elasticityMultiplier` | 2 | Target = block_limit / this (50% target) |

### Minimum Fee Parameters

| Parameter | Default | Description |
|-----------|---------|-------------|
| `minGasPrice` | 0 | Global minimum gas price |
| `minGasMultiplier` | 0.5 | Minimum gas counted for base fee calculation |

### What `minGasMultiplier` Does

Prevents manipulation where someone submits high gas limit but uses little:

```
Transaction: gasLimit = 1,000,000, actually uses = 100,000
Without protection: Only 100,000 counted for base fee calculation
With minGasMultiplier = 0.5: At least 500,000 counted

countedGas = max(gasUsed, gasLimit × minGasMultiplier)
           = max(100,000, 1,000,000 × 0.5)
           = max(100,000, 500,000)
           = 500,000
```

---

## Common Errors & Solutions

### Error: "max fee per gas less than block base fee"

**Cause:** Your `maxFeePerGas` is lower than the current base fee.

```
Your maxFeePerGas: 100,000,000 (0.1 gwei)
Current baseFee: 263,725,748 (0.26 gwei)
Result: REJECTED
```

**Solution:** Query current base fee and add a buffer:

```javascript
const feeData = await provider.getFeeData();
const baseFee = feeData.maxFeePerGas;
const maxFeePerGas = baseFee + (baseFee / 5n); // +20% buffer
```

### Error: "insufficient fee"

**Cause:** Total fee doesn't meet minimum requirements.

**Solution:** Increase `maxFeePerGas` or check `minGasPrice` parameter.

### Error: "insufficient funds for gas * price + value"

**Cause:** Your account doesn't have enough balance.

```
Required: gas × maxFeePerGas + transfer amount
Your balance: Less than required
```

**Solution:** Add funds or use fee sponsorship.

### Error: "nonce too low"

**Cause:** Transaction nonce doesn't match account's expected nonce.

**Solution:** Query current nonce: `provider.getTransactionCount(address)`

---

## Quick Reference

### Fee Calculation Cheat Sheet

```
┌─────────────────────────────────────────────────────────────┐
│ INPUTS                                                      │
│   gasLimit: Maximum gas willing to use                      │
│   maxFeePerGas: Maximum price per gas                       │
│   priorityFeePerGas: Tip for validator                      │
│   baseFee: Current network base fee (from block)            │
├─────────────────────────────────────────────────────────────┤
│ CALCULATION                                                 │
│   effectivePrice = min(maxFeePerGas, baseFee + priorityFee) │
│   actualFee = effectivePrice × gasUsed                      │
│   refund = (gasLimit - gasUsed) × effectivePrice            │
├─────────────────────────────────────────────────────────────┤
│ VALIDATION                                                  │
│   maxFeePerGas >= baseFee           ← REQUIRED              │
│   balance >= gasLimit × maxFeePerGas + value                │
└─────────────────────────────────────────────────────────────┘
```

### Code Locations

| Component | File |
|-----------|------|
| Fee market module | `x/feemarket/` |
| Base fee calculation | `x/feemarket/keeper/eip1559.go` |
| Fee ante handler | `ante/evm/mono_decorator.go` |
| Fee checker | `ante/evm/fee_checker.go` |
| Sponsorship logic | `x/vm/keeper/sponsorship.go` |
| Parameters | `x/feemarket/types/params.go` |

---

## Summary

1. **Gas** measures computational work; **gas price** is cost per unit
2. **EIP-1559** splits fees into **base fee** (burned) and **priority fee** (tip)
3. **Base fee** adjusts automatically to target 50% block utilization
4. **Max fee** protects you from overpaying; unused portion is refunded
5. **Fee sponsorship** lets one account pay fees for another
6. Always query current base fee and add a buffer to avoid rejections
