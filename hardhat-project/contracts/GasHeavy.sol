// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title GasHeavy
 * @notice Contract designed to consume a lot of gas for testing daily gas limits
 */
contract GasHeavy {
    uint256[] public data;
    uint256 public counter;

    event GasConsumed(uint256 iterations, uint256 gasUsed);

    /**
     * @notice Consumes approximately `iterations * 20000` gas by writing to storage
     * @param iterations Number of storage writes to perform
     */
    function consumeGas(uint256 iterations) external {
        uint256 startGas = gasleft();

        for (uint256 i = 0; i < iterations; i++) {
            // Each SSTORE to a new slot costs ~20,000 gas
            data.push(block.timestamp + i);
        }
        counter += iterations;

        uint256 gasUsed = startGas - gasleft();
        emit GasConsumed(iterations, gasUsed);
    }

    /**
     * @notice Resets the data array to free up storage
     */
    function reset() external {
        delete data;
        counter = 0;
    }

    /**
     * @notice Returns the length of the data array
     */
    function dataLength() external view returns (uint256) {
        return data.length;
    }
}
