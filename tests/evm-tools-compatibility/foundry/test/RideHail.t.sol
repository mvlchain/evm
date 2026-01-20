// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

import "forge-std/Test.sol";
import "repo/contracts/solidity/RideHail.sol";

contract RideHailTest is Test {
    RideHail internal rideHail;

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
        rideHail = new RideHail(
            RIDER_DEPOSIT,
            DRIVER_BOND,
            1 hours,
            1 hours,
            1 hours,
            2,
            60,
            3,
            256,
            512,
            MESSAGE_BOND,
            500,
            address(0)
        );

        vm.deal(rider, 10 ether);
        vm.deal(driver, 10 ether);
    }

    function test_driverScenario_eventSubscribeFlow() public {
        uint256 requestId = _createRequest();
        uint64 commitEnd = _getRequest(requestId).commitEnd;

        bytes32 salt = keccak256("salt");
        uint64 eta = 600;
        bytes32 commitHash = keccak256(abi.encode(requestId, driver, eta, cellTopic, salt));

        vm.prank(driver);
        rideHail.acceptCommit{value: DRIVER_BOND}(requestId, commitHash, eta);

        vm.warp(commitEnd + 1);

        vm.prank(driver);
        rideHail.acceptReveal(requestId, eta, cellTopic, salt);

        RideHail.Request memory request = _getRequest(requestId);
        uint256 sessionId = request.sessionId;
        assertTrue(sessionId != 0);

        bytes memory header = hex"01";
        bytes memory ciphertext = hex"0203";

        vm.prank(rider);
        rideHail.postEncryptedMessage{value: MESSAGE_BOND}(sessionId, 1, header, ciphertext);

        vm.prank(driver);
        rideHail.postEncryptedMessage{value: MESSAGE_BOND}(sessionId, 1, hex"04", hex"0506");

        vm.prank(rider);
        rideHail.riderCheckIn(sessionId);

        vm.prank(driver);
        rideHail.driverCheckIn(sessionId);

        vm.prank(rider);
        rideHail.startRide(sessionId);

        vm.prank(driver);
        rideHail.updateCoarseLocation(sessionId, cellTopic);

        uint256 riderBalanceBefore = rider.balance;
        uint256 driverBalanceBefore = driver.balance;

        vm.prank(driver);
        rideHail.endRide(sessionId);

        RideHail.Session memory session = _getSession(sessionId);
        assertEq(uint8(session.state), uint8(RideHail.SessionState.RideEnded));

        (uint32 msgIndex, bytes memory storedHeader, bytes memory storedCipher) =
            rideHail.messages(sessionId, rider, 1);
        assertEq(msgIndex, 1);
        assertEq(storedHeader, header);
        assertEq(storedCipher, ciphertext);

        assertEq(rider.balance, riderBalanceBefore + MESSAGE_BOND);
        assertEq(driver.balance, driverBalanceBefore + RIDER_DEPOSIT + DRIVER_BOND + MESSAGE_BOND);
    }

    function test_fuzzInvalidRevealReverts(bytes32 wrongSalt) public {
        bytes32 correctSalt = keccak256("correct");
        vm.assume(wrongSalt != correctSalt);

        uint256 requestId = _createRequest();
        uint64 commitEnd = _getRequest(requestId).commitEnd;

        uint64 eta = 300;
        bytes32 commitHash = keccak256(abi.encode(requestId, driver, eta, cellTopic, correctSalt));

        vm.prank(driver);
        rideHail.acceptCommit{value: DRIVER_BOND}(requestId, commitHash, eta);

        vm.warp(commitEnd + 1);

        vm.prank(driver);
        vm.expectRevert(RideHail.InvalidCommit.selector);
        rideHail.acceptReveal(requestId, eta, cellTopic, wrongSalt);
    }

    function test_fuzzMessageRateLimit(uint8 extra) public {
        vm.assume(extra > 0);
        uint256 sessionId = _createMatchedSession();

        uint16 limit = rideHail.maxMessagesPerWindow();
        for (uint16 i = 1; i <= limit; i++) {
            vm.prank(rider);
            rideHail.postEncryptedMessage{value: MESSAGE_BOND}(sessionId, i, hex"aa", hex"bb");
        }

        vm.prank(rider);
        vm.expectRevert(RideHail.RateLimited.selector);
        rideHail.postEncryptedMessage{value: MESSAGE_BOND}(sessionId, limit + 1, hex"aa", hex"bb");
    }

    function test_fuzzRequestRateLimit(uint8 extra) public {
        vm.assume(extra > 0);
        vm.deal(rider, 100 ether);

        uint16 limit = rideHail.maxRequestsPerWindow();
        for (uint16 i = 0; i < limit; i++) {
            _createRequest();
        }

        vm.prank(rider);
        vm.expectRevert(RideHail.RateLimited.selector);
        rideHail.createRequest{value: RIDER_DEPOSIT}(
            cellTopic,
            regionTopic,
            paramsHash,
            pickupCommit,
            dropoffCommit,
            1800,
            2 hours
        );
    }

    function _createMatchedSession() internal returns (uint256 sessionId) {
        uint256 requestId = _createRequest();
        uint64 commitEnd = _getRequest(requestId).commitEnd;

        bytes32 salt = keccak256("salt");
        uint64 eta = 120;
        bytes32 commitHash = keccak256(abi.encode(requestId, driver, eta, cellTopic, salt));

        vm.prank(driver);
        rideHail.acceptCommit{value: DRIVER_BOND}(requestId, commitHash, eta);

        vm.warp(commitEnd + 1);

        vm.prank(driver);
        rideHail.acceptReveal(requestId, eta, cellTopic, salt);

        sessionId = _getRequest(requestId).sessionId;
    }

    function _createRequest() internal returns (uint256 requestId) {
        vm.prank(rider);
        requestId = rideHail.createRequest{value: RIDER_DEPOSIT}(
            cellTopic,
            regionTopic,
            paramsHash,
            pickupCommit,
            dropoffCommit,
            1800,
            2 hours
        );
    }

    function _getRequest(uint256 requestId) internal view returns (RideHail.Request memory request) {
        (
            request.rider,
            request.cellTopic,
            request.regionTopic,
            request.paramsHash,
            request.pickupCommit,
            request.dropoffCommit,
            request.riderDeposit,
            request.createdAt,
            request.commitEnd,
            request.revealEnd,
            request.ttl,
            request.maxDriverEta,
            request.commitCount,
            request.canceled,
            request.matched,
            request.sessionId
        ) = rideHail.requests(requestId);
    }

    function _getSession(uint256 sessionId) internal view returns (RideHail.Session memory session) {
        (
            session.rider,
            session.driver,
            session.requestId,
            session.riderDeposit,
            session.driverBond,
            session.createdAt,
            session.updatedAt,
            session.lastCoarseCell,
            session.riderCheckedIn,
            session.driverCheckedIn,
            session.state
        ) = rideHail.sessions(sessionId);
    }
}
