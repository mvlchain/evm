#!/usr/bin/env node

const { ethers } = require('ethers');

async function testGasless() {
    // Connect to local node
    const provider = new ethers.JsonRpcProvider('http://127.0.0.1:8545');

    // Use the validator's private key (exported from evmd keys)
    const VALIDATOR_PRIVATE_KEY = '0xC4447F6E030EF4613C42AF1A09C8C3B5559B59B81B2A7B0EAB6CBE9A6CCC80EC';
    const validatorWallet = new ethers.Wallet(VALIDATOR_PRIVATE_KEY, provider);
    console.log('Validator address:', validatorWallet.address);

    // Create a test wallet for sending
    const wallet = ethers.Wallet.createRandom().connect(provider);
    console.log('Test sender address:', wallet.address);

    // Fund the test account from validator
    console.log('\n1. Funding test account...');
    const fundTx = await validatorWallet.sendTransaction({
        to: wallet.address,
        value: ethers.parseEther('10'), // 10 tokens
        gasLimit: 21000,
        gasPrice: ethers.parseUnits('1', 'gwei')
    });
    console.log('Funding tx hash:', fundTx.hash);

    // Work around RPC bug - just wait for block time instead of using .wait()
    await new Promise(resolve => setTimeout(resolve, 6000));
    console.log('Funding tx should be confirmed!');

    const balance = await provider.getBalance(wallet.address);
    console.log('Test account balance:', ethers.formatEther(balance), 'tokens');

    // Test 1: Send to gasless-enabled address (properly checksummed)
    const gaslessAddr = ethers.getAddress('0xaa00000000000000000000000000000000000000');
    console.log('\n2. Testing GASLESS transaction to', gaslessAddr, '...');
    const gaslessTx = await wallet.sendTransaction({
        to: gaslessAddr,
        value: ethers.parseEther('0.001'),
        gasLimit: 21000,
        gasPrice: ethers.parseUnits('1', 'gwei') // 1 Gwei - required even for gasless
    });
    console.log('Gasless tx hash:', gaslessTx.hash);

    // Work around RPC bug
    await new Promise(resolve => setTimeout(resolve, 6000));

    const gaslessReceipt = await provider.send('eth_getTransactionReceipt', [gaslessTx.hash]);
    console.log('\nGasless Transaction Receipt:');
    console.log('  Status:', gaslessReceipt.status);
    console.log('  Block:', gaslessReceipt.blockNumber);
    console.log('  Gas used:', gaslessReceipt.gasUsed);
    console.log('  effectiveGasPrice:', gaslessReceipt.effectiveGasPrice);

    if (gaslessReceipt.effectiveGasPrice === '0x0') {
        console.log('  ✅ SUCCESS: effectiveGasPrice is 0x0 (gasless transaction worked!)');
    } else {
        console.log('  ❌ FAILED: effectiveGasPrice is', gaslessReceipt.effectiveGasPrice, '(expected 0x0)');
    }

    // Test 2: Send to regular address (should charge gas normally)
    console.log('\n3. Testing REGULAR transaction to random address...');
    const regularTx = await wallet.sendTransaction({
        to: '0x0000000000000000000000000000000000000001',
        value: ethers.parseEther('0.001'),
        gasLimit: 21000,
        gasPrice: ethers.parseUnits('1', 'gwei')
    });
    console.log('Regular tx hash:', regularTx.hash);

    // Work around RPC bug
    await new Promise(resolve => setTimeout(resolve, 6000));

    const regularReceipt = await provider.send('eth_getTransactionReceipt', [regularTx.hash]);
    console.log('\nRegular Transaction Receipt:');
    console.log('  Status:', regularReceipt.status);
    console.log('  Block:', regularReceipt.blockNumber);
    console.log('  Gas used:', regularReceipt.gasUsed);
    console.log('  effectiveGasPrice:', regularReceipt.effectiveGasPrice);

    if (regularReceipt.effectiveGasPrice !== '0x0') {
        console.log('  ✅ SUCCESS: effectiveGasPrice is', regularReceipt.effectiveGasPrice, '(regular transaction charged gas)');
    } else {
        console.log('  ❌ FAILED: effectiveGasPrice should not be 0x0 for regular transactions');
    }

    console.log('\n=== Test Complete ===');
}

testGasless().catch(console.error);
