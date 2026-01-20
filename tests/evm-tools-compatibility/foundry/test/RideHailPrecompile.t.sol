// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";

interface IRideHailPrecompile {
    function version() external view returns (uint256);

    function nextRequestId() external view returns (uint256);

    function nextSessionId() external view returns (uint256);

    function validateCreateRequest(
        bytes32 cellTopic,
        bytes32 regionTopic,
        bytes32 paramsHash,
        bytes32 pickupCommit,
        bytes32 dropoffCommit,
        uint32 maxDriverEta,
        uint64 ttl
    ) external payable returns (bool success, string memory reason);

    function createRequest(
        bytes32 cellTopic,
        bytes32 regionTopic,
        bytes32 paramsHash,
        bytes32 pickupCommit,
        bytes32 dropoffCommit,
        uint32 maxDriverEta,
        uint64 ttl
    ) external payable returns (uint256 requestId);

    function acceptCommit(
        uint256 requestId,
        bytes32 commitHash,
        uint64 eta
    ) external payable;

    function acceptReveal(
        uint256 requestId,
        uint64 eta,
        bytes32 driverCell,
        bytes32 salt
    ) external;

    function requests(uint256 requestId) external view returns (
        address rider,
        bytes32 cellTopic,
        bytes32 regionTopic,
        bytes32 paramsHash,
        bytes32 pickupCommit,
        bytes32 dropoffCommit,
        uint256 riderDeposit,
        uint64 createdAt,
        uint64 commitEnd,
        uint64 revealEnd,
        uint64 ttl,
        uint32 maxDriverEta,
        uint32 commitCount,
        bool canceled,
        bool matched,
        uint256 sessionId
    );

    function postEncryptedMessage(
        uint256 sessionId,
        uint32 msgIndex,
        bytes calldata header,
        bytes calldata ciphertext
    ) external payable;
}

contract RideHailPrecompileTest is Test {
    IRideHailPrecompile internal rideHail;

    address internal constant RIDEHAIL_PRECOMPILE = 0x000000000000000000000000000000000000080a;

    address internal rider = address(0x100);
    address internal driver = address(0x200);

    bytes32 internal cellTopic = keccak256("cell-9");
    bytes32 internal regionTopic = keccak256("region-1");
    bytes32 internal paramsHash = keccak256("params");
    bytes32 internal pickupCommit = keccak256("pickup");
    bytes32 internal dropoffCommit = keccak256("dropoff");

    uint256 internal constant RIDER_DEPOSIT = 1 ether;
    uint256 internal constant DRIVER_BOND = 0.2 ether;
    uint256 internal constant MESSAGE_BOND = 0.01 ether;

    function setUp() public {
        rideHail = IRideHailPrecompile(RIDEHAIL_PRECOMPILE);

        vm.deal(rider, 10 ether);
        vm.deal(driver, 10 ether);
    }

    function test_version() public view {
        uint256 version = rideHail.version();
        console.log("RideHail precompile version:", version);
        assertEq(version, 2);
    }

    function test_nextRequestId() public view {
        uint256 nextId = rideHail.nextRequestId();
        console.log("Next request ID:", nextId);
        assertGe(nextId, 1);
    }

    function test_nextSessionId() public view {
        uint256 nextId = rideHail.nextSessionId();
        console.log("Next session ID:", nextId);
        assertGe(nextId, 1);
    }

    function test_validateCreateRequest_success() public {
        vm.deal(address(this), 10 ether);

        (bool success, string memory reason) = rideHail.validateCreateRequest{value: RIDER_DEPOSIT}(
            cellTopic,
            regionTopic,
            paramsHash,
            pickupCommit,
            dropoffCommit,
            1800,
            2 hours
        );

        console.log("Validation success:", success);
        console.log("Validation reason:", reason);
        assertTrue(success);
    }

    function test_validateCreateRequest_insufficientDeposit() public {
        vm.deal(address(this), 10 ether);

        (bool success, string memory reason) = rideHail.validateCreateRequest{value: 0.5 ether}(
            cellTopic,
            regionTopic,
            paramsHash,
            pickupCommit,
            dropoffCommit,
            1800,
            2 hours
        );

        console.log("Validation success:", success);
        console.log("Validation reason:", reason);
        assertFalse(success);
        assertEq(reason, "insufficient deposit");
    }

    function test_createRequest() public {
        uint256 nextIdBefore = rideHail.nextRequestId();

        vm.prank(rider);
        uint256 requestId = rideHail.createRequest{value: RIDER_DEPOSIT}(
            cellTopic,
            regionTopic,
            paramsHash,
            pickupCommit,
            dropoffCommit,
            1800,
            2 hours
        );

        console.log("Created request ID:", requestId);
        assertEq(requestId, nextIdBefore);

        uint256 nextIdAfter = rideHail.nextRequestId();
        assertEq(nextIdAfter, nextIdBefore + 1);

        (
            address riderAddr,
            bytes32 cell,
            bytes32 region,
            bytes32 params,
            bytes32 pickup,
            bytes32 dropoff,
            uint256 deposit,
            ,,,,,,,
            bool matched,
        ) = rideHail.requests(requestId);

        assertEq(riderAddr, rider);
        assertEq(cell, cellTopic);
        assertEq(region, regionTopic);
        assertEq(params, paramsHash);
        assertEq(pickup, pickupCommit);
        assertEq(dropoff, dropoffCommit);
        assertEq(deposit, RIDER_DEPOSIT);
        assertFalse(matched);
    }

    function test_fullFlow_createAcceptReveal() public {
        // Create request
        vm.prank(rider);
        uint256 requestId = rideHail.createRequest{value: RIDER_DEPOSIT}(
            cellTopic,
            regionTopic,
            paramsHash,
            pickupCommit,
            dropoffCommit,
            1800,
            2 hours
        );

        console.log("Created request ID:", requestId);

        // Get commit end time
        (,,,,,,, uint64 createdAt, uint64 commitEnd,,,,,,,) = rideHail.requests(requestId);
        console.log("Created at:", createdAt);
        console.log("Commit end:", commitEnd);

        // Driver commits
        bytes32 salt = keccak256("salt");
        uint64 eta = 600;
        bytes32 commitHash = keccak256(abi.encode(requestId, driver, eta, cellTopic, salt));

        vm.prank(driver);
        rideHail.acceptCommit{value: DRIVER_BOND}(requestId, commitHash, eta);

        console.log("Driver committed");

        // Warp to reveal phase
        vm.warp(commitEnd + 1);

        // Driver reveals
        vm.prank(driver);
        rideHail.acceptReveal(requestId, eta, cellTopic, salt);

        console.log("Driver revealed");

        // Check that session was created
        (,,,,,,,,,,,, bool matched, uint256 sessionId) = rideHail.requests(requestId);
        assertTrue(matched);
        assertGt(sessionId, 0);

        console.log("Session ID:", sessionId);
    }

    function test_postEncryptedMessage() public {
        // Create matched session
        vm.prank(rider);
        uint256 requestId = rideHail.createRequest{value: RIDER_DEPOSIT}(
            cellTopic,
            regionTopic,
            paramsHash,
            pickupCommit,
            dropoffCommit,
            1800,
            2 hours
        );

        (,,,,,,,uint64 commitEnd,,,,,,) = rideHail.requests(requestId);

        bytes32 salt = keccak256("salt");
        uint64 eta = 600;
        bytes32 commitHash = keccak256(abi.encode(requestId, driver, eta, cellTopic, salt));

        vm.prank(driver);
        rideHail.acceptCommit{value: DRIVER_BOND}(requestId, commitHash, eta);

        vm.warp(commitEnd + 1);

        vm.prank(driver);
        rideHail.acceptReveal(requestId, eta, cellTopic, salt);

        (,,,,,,,,,,,, bool matched, uint256 sessionId) = rideHail.requests(requestId);
        assertTrue(matched);

        // Post encrypted message
        bytes memory header = hex"0102030405060708";
        bytes memory ciphertext = hex"deadbeef";

        vm.prank(rider);
        rideHail.postEncryptedMessage{value: MESSAGE_BOND}(sessionId, 1, header, ciphertext);

        console.log("Posted encrypted message to session:", sessionId);
    }
}
