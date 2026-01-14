// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

/**
 * @title Counter
 * @dev Simple counter contract for testing fee sponsorship
 */
contract Counter {
    uint256 public count;
    address public lastCaller;

    event Incremented(address indexed caller, uint256 newCount);
    event Decremented(address indexed caller, uint256 newCount);
    event Reset(address indexed caller);
    event ValueSet(address indexed caller, uint256 newCount);

    /**
     * @dev Increment the counter by 1
     */
    function increment() public {
        count += 1;
        lastCaller = msg.sender;
        emit Incremented(msg.sender, count);
    }

    /**
     * @dev Decrement the counter by 1
     */
    function decrement() public {
        require(count > 0, "Count is already 0");
        count -= 1;
        lastCaller = msg.sender;
        emit Decremented(msg.sender, count);
    }

    /**
     * @dev Reset the counter to 0
     */
    function reset() public {
        count = 0;
        lastCaller = msg.sender;
        emit Reset(msg.sender);
    }

    /**
     * @dev Set the counter to a specific value
     * @param _value The new value for the counter
     */
    function setValue(uint256 _value) public {
        count = _value;
        lastCaller = msg.sender;
        emit ValueSet(msg.sender, _value);
    }

    /**
     * @dev Get the current counter value
     * @return The current count
     */
    function getCount() public view returns (uint256) {
        return count;
    }

    /**
     * @dev Get the address of the last caller
     * @return The address that last modified the counter
     */
    function getLastCaller() public view returns (address) {
        return lastCaller;
    }
}
