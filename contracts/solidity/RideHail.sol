// SPDX-License-Identifier: MIT
pragma solidity ^0.8.20;

interface ISlashingHook {
    function onDriverSlashed(address driver, uint256 amount, uint256 sessionId) external;
}

contract RideHail {
    address public constant DOUBLE_RATCHET_PRECOMPILE = 0x0000000000000000000000000000000000000808;

    enum SessionState {
        None,
        Matched,
        RiderCheckedIn,
        DriverCheckedIn,
        BothCheckedIn,
        RideStarted,
        RideEnded,
        Canceled
    }

    struct Request {
        address rider;
        bytes32 cellTopic;
        bytes32 regionTopic;
        bytes32 paramsHash;
        bytes32 pickupCommit;
        bytes32 dropoffCommit;
        uint256 riderDeposit;
        uint64 createdAt;
        uint64 commitEnd;
        uint64 revealEnd;
        uint64 ttl;
        uint32 maxDriverEta;
        uint32 commitCount;
        bool canceled;
        bool matched;
        uint256 sessionId;
    }

    struct Commit {
        bytes32 commitHash;
        uint64 committedAt;
        uint64 eta;
        uint256 bond;
        bytes32 driverCell;
        bool revealed;
    }

    struct Session {
        address rider;
        address driver;
        uint256 requestId;
        uint256 riderDeposit;
        uint256 driverBond;
        uint64 createdAt;
        uint64 updatedAt;
        bytes32 lastCoarseCell;
        bool riderCheckedIn;
        bool driverCheckedIn;
        SessionState state;
    }

    struct RateLimit {
        uint64 windowStart;
        uint16 count;
    }

    struct Message {
        uint32 msgIndex;
        bytes header;
        bytes ciphertext;
    }

    event RideRequested(
        bytes32 indexed cellTopic,
        bytes32 indexed regionTopic,
        uint256 indexed requestId,
        address rider,
        bytes32 paramsHash,
        bytes32 pickupCommit,
        bytes32 dropoffCommit,
        uint64 commitEnd,
        uint64 revealEnd,
        uint256 riderDeposit
    );
    event DriverAcceptCommitted(
        uint256 indexed requestId,
        address indexed driver,
        bytes32 commitHash,
        uint64 eta,
        uint256 bond
    );
    event DriverAcceptRevealed(
        uint256 indexed requestId,
        address indexed driver,
        bytes32 revealHash,
        uint64 eta,
        bytes32 driverCell
    );
    event Matched(
        uint256 indexed sessionId,
        uint256 indexed requestId,
        address indexed rider,
        address driver,
        uint64 eta
    );
    event EncryptedMessage(
        uint256 indexed sessionId,
        address indexed sender,
        uint32 msgIndex,
        bytes header,
        bytes ciphertext
    );
    event StateChanged(uint256 indexed sessionId, SessionState newState, uint64 timestamp);
    event CoarseLocationUpdated(uint256 indexed sessionId, address indexed sender, bytes32 cellTopic);

    error NotRider();
    error NotDriver();
    error NotParticipant();
    error InvalidState();
    error InvalidRequest();
    error InvalidCommit();
    error CommitPhaseOver();
    error RevealPhaseInactive();
    error RateLimited();
    error MessageTooLarge();
    error InvalidMsgIndex();
    error InsufficientDeposit();
    error InvalidCancel();
    error InvalidEnvelope();

    uint256 public nextRequestId = 1;
    uint256 public nextSessionId = 1;

    uint256 public immutable minRiderDeposit;
    uint256 public immutable minDriverBond;
    uint64 public immutable commitDuration;
    uint64 public immutable revealDuration;
    uint64 public immutable requestWindow;
    uint16 public immutable maxRequestsPerWindow;
    uint64 public immutable messageWindow;
    uint16 public immutable maxMessagesPerWindow;
    uint32 public immutable maxHeaderBytes;
    uint32 public immutable maxCiphertextBytes;
    uint256 public immutable messageBond;
    uint16 public immutable cancelFeeBps;
    address public immutable slashingHook;

    mapping(uint256 => Request) public requests;
    mapping(uint256 => mapping(address => Commit)) public commits;
    mapping(uint256 => Session) public sessions;
    mapping(address => RateLimit) public riderRequestRate;
    mapping(uint256 => mapping(address => RateLimit)) public messageRate;
    mapping(uint256 => mapping(address => uint32)) public lastMsgIndex;
    mapping(uint256 => mapping(address => uint256)) public messageBondEscrow;
    mapping(uint256 => mapping(address => mapping(uint32 => Message))) public messages;

    constructor(
        uint256 _minRiderDeposit,
        uint256 _minDriverBond,
        uint64 _commitDuration,
        uint64 _revealDuration,
        uint64 _requestWindow,
        uint16 _maxRequestsPerWindow,
        uint64 _messageWindow,
        uint16 _maxMessagesPerWindow,
        uint32 _maxHeaderBytes,
        uint32 _maxCiphertextBytes,
        uint256 _messageBond,
        uint16 _cancelFeeBps,
        address _slashingHook
    ) {
        minRiderDeposit = _minRiderDeposit;
        minDriverBond = _minDriverBond;
        commitDuration = _commitDuration;
        revealDuration = _revealDuration;
        requestWindow = _requestWindow;
        maxRequestsPerWindow = _maxRequestsPerWindow;
        messageWindow = _messageWindow;
        maxMessagesPerWindow = _maxMessagesPerWindow;
        maxHeaderBytes = _maxHeaderBytes;
        maxCiphertextBytes = _maxCiphertextBytes;
        messageBond = _messageBond;
        cancelFeeBps = _cancelFeeBps;
        slashingHook = _slashingHook;
    }

    function createRequest(
        bytes32 cellTopic,
        bytes32 regionTopic,
        bytes32 paramsHash,
        bytes32 pickupCommit,
        bytes32 dropoffCommit,
        uint32 maxDriverEta,
        uint64 ttl
    ) external payable returns (uint256 requestId) {
        if (msg.value < minRiderDeposit) {
            revert InsufficientDeposit();
        }
        _checkRiderRateLimit(msg.sender);

        requestId = nextRequestId++;
        uint64 createdAt = uint64(block.timestamp);
        uint64 commitEnd = createdAt + commitDuration;
        uint64 revealEnd = commitEnd + revealDuration;

        requests[requestId] = Request({
            rider: msg.sender,
            cellTopic: cellTopic,
            regionTopic: regionTopic,
            paramsHash: paramsHash,
            pickupCommit: pickupCommit,
            dropoffCommit: dropoffCommit,
            riderDeposit: msg.value,
            createdAt: createdAt,
            commitEnd: commitEnd,
            revealEnd: revealEnd,
            ttl: ttl,
            maxDriverEta: maxDriverEta,
            commitCount: 0,
            canceled: false,
            matched: false,
            sessionId: 0
        });

        emit RideRequested(
            cellTopic,
            regionTopic,
            requestId,
            msg.sender,
            paramsHash,
            pickupCommit,
            dropoffCommit,
            commitEnd,
            revealEnd,
            msg.value
        );
    }

    function acceptCommit(
        uint256 requestId,
        bytes32 commitHash,
        uint64 eta
    ) external payable {
        Request storage request = requests[requestId];
        if (request.rider == address(0) || request.canceled || request.matched || _isExpired(request)) {
            revert InvalidRequest();
        }
        if (block.timestamp >= request.commitEnd) {
            revert CommitPhaseOver();
        }
        if (msg.value < minDriverBond) {
            revert InsufficientDeposit();
        }
        Commit storage commit = commits[requestId][msg.sender];
        if (commit.commitHash != bytes32(0)) {
            revert InvalidCommit();
        }

        commits[requestId][msg.sender] = Commit({
            commitHash: commitHash,
            committedAt: uint64(block.timestamp),
            eta: eta,
            bond: msg.value,
            driverCell: bytes32(0),
            revealed: false
        });

        request.commitCount += 1;

        emit DriverAcceptCommitted(requestId, msg.sender, commitHash, eta, msg.value);
    }

    function acceptReveal(
        uint256 requestId,
        uint64 eta,
        bytes32 driverCell,
        bytes32 salt
    ) external {
        Request storage request = requests[requestId];
        if (request.rider == address(0) || request.canceled || _isExpired(request)) {
            revert InvalidRequest();
        }
        if (block.timestamp < request.commitEnd || block.timestamp > request.revealEnd) {
            revert RevealPhaseInactive();
        }
        Commit storage commit = commits[requestId][msg.sender];
        if (commit.commitHash == bytes32(0) || commit.revealed) {
            revert InvalidCommit();
        }
        bytes32 revealHash = keccak256(abi.encode(requestId, msg.sender, eta, driverCell, salt));
        if (revealHash != commit.commitHash) {
            revert InvalidCommit();
        }
        if (eta > request.maxDriverEta) {
            revert InvalidCommit();
        }
        if (driverCell != request.cellTopic && driverCell != request.regionTopic) {
            revert InvalidCommit();
        }
        // TODO: Replace equality check with an H3 distance/radius precompile or lookup when available.

        commit.revealed = true;
        commit.eta = eta;
        commit.driverCell = driverCell;

        emit DriverAcceptRevealed(requestId, msg.sender, revealHash, eta, driverCell);

        if (!request.matched) {
            _matchSession(requestId, msg.sender, eta);
        } else {
            _refund(msg.sender, commit.bond);
            commit.bond = 0;
        }
    }

    function cancelRequest(uint256 requestId) external {
        Request storage request = requests[requestId];
        if (request.rider != msg.sender) {
            revert NotRider();
        }
        if (request.matched || request.canceled) {
            revert InvalidRequest();
        }
        bool beforeCommitEnd = block.timestamp < request.commitEnd;
        bool afterRevealEnd = block.timestamp > request.revealEnd;
        if (_isExpired(request)) {
            // Allow refund once TTL has elapsed.
        } else if (beforeCommitEnd) {
            if (request.commitCount != 0) {
                revert InvalidCancel();
            }
        } else if (!afterRevealEnd) {
            revert InvalidCancel();
        }

        request.canceled = true;
        _refund(msg.sender, request.riderDeposit);
        request.riderDeposit = 0;
    }

    function claimUnrevealedBond(uint256 requestId, address driver) external {
        Request storage request = requests[requestId];
        if (request.rider == address(0)) {
            revert InvalidRequest();
        }
        if (block.timestamp <= request.revealEnd) {
            revert RevealPhaseInactive();
        }
        Commit storage commit = commits[requestId][driver];
        if (commit.commitHash == bytes32(0) || commit.revealed || commit.bond == 0) {
            revert InvalidCommit();
        }
        uint256 bond = commit.bond;
        commit.bond = 0;
        _refund(request.rider, bond);
        _callSlashingHook(driver, bond, 0);
    }

    function riderCheckIn(uint256 sessionId) external {
        Session storage session = _requireSession(sessionId);
        if (msg.sender != session.rider) {
            revert NotRider();
        }
        if (session.state == SessionState.RideStarted || session.state == SessionState.RideEnded || session.state == SessionState.Canceled) {
            revert InvalidState();
        }
        session.riderCheckedIn = true;
        session.updatedAt = uint64(block.timestamp);
        _updateCheckInState(sessionId, session);
    }

    function driverCheckIn(uint256 sessionId) external {
        Session storage session = _requireSession(sessionId);
        if (msg.sender != session.driver) {
            revert NotDriver();
        }
        if (session.state == SessionState.RideStarted || session.state == SessionState.RideEnded || session.state == SessionState.Canceled) {
            revert InvalidState();
        }
        session.driverCheckedIn = true;
        session.updatedAt = uint64(block.timestamp);
        _updateCheckInState(sessionId, session);
    }

    function startRide(uint256 sessionId) external {
        Session storage session = _requireSession(sessionId);
        if (msg.sender != session.rider && msg.sender != session.driver) {
            revert NotParticipant();
        }
        if (!session.riderCheckedIn || !session.driverCheckedIn) {
            revert InvalidState();
        }
        if (session.state == SessionState.RideStarted || session.state == SessionState.RideEnded || session.state == SessionState.Canceled) {
            revert InvalidState();
        }
        session.state = SessionState.RideStarted;
        session.updatedAt = uint64(block.timestamp);
        emit StateChanged(sessionId, session.state, session.updatedAt);
    }

    function updateCoarseLocation(uint256 sessionId, bytes32 cellTopic) external {
        Session storage session = _requireSession(sessionId);
        if (msg.sender != session.rider && msg.sender != session.driver) {
            revert NotParticipant();
        }
        if (session.state == SessionState.RideEnded || session.state == SessionState.Canceled) {
            revert InvalidState();
        }
        session.lastCoarseCell = cellTopic;
        session.updatedAt = uint64(block.timestamp);
        emit CoarseLocationUpdated(sessionId, msg.sender, cellTopic);
    }

    function postEncryptedMessage(
        uint256 sessionId,
        uint32 msgIndex,
        bytes calldata header,
        bytes calldata ciphertext
    ) external payable {
        Session storage session = _requireSession(sessionId);
        if (msg.sender != session.rider && msg.sender != session.driver) {
            revert NotParticipant();
        }
        if (session.state == SessionState.RideEnded || session.state == SessionState.Canceled) {
            revert InvalidState();
        }
        if (header.length > maxHeaderBytes || ciphertext.length > maxCiphertextBytes) {
            revert MessageTooLarge();
        }
        if (msgIndex <= lastMsgIndex[sessionId][msg.sender]) {
            revert InvalidMsgIndex();
        }
        if (msg.value != messageBond) {
            revert InsufficientDeposit();
        }
        _validateEnvelope(header, ciphertext);

        _checkMessageRateLimit(sessionId, msg.sender);
        lastMsgIndex[sessionId][msg.sender] = msgIndex;
        messageBondEscrow[sessionId][msg.sender] += msg.value;
        messages[sessionId][msg.sender][msgIndex] = Message({
            msgIndex: msgIndex,
            header: header,
            ciphertext: ciphertext
        });

        emit EncryptedMessage(sessionId, msg.sender, msgIndex, header, ciphertext);
    }

    function endRide(uint256 sessionId) external {
        Session storage session = _requireSession(sessionId);
        if (msg.sender != session.rider && msg.sender != session.driver) {
            revert NotParticipant();
        }
        if (session.state != SessionState.RideStarted) {
            revert InvalidState();
        }
        session.state = SessionState.RideEnded;
        session.updatedAt = uint64(block.timestamp);
        emit StateChanged(sessionId, session.state, session.updatedAt);

        uint256 riderDeposit = session.riderDeposit;
        uint256 driverBond = session.driverBond;
        session.riderDeposit = 0;
        session.driverBond = 0;

        _refundMessageBonds(sessionId, session.rider, session.driver);
        _refund(session.driver, riderDeposit + driverBond);
    }

    function cancelSession(uint256 sessionId) external {
        Session storage session = _requireSession(sessionId);
        if (session.state == SessionState.RideEnded || session.state == SessionState.Canceled) {
            revert InvalidState();
        }
        if (session.state == SessionState.RideStarted) {
            revert InvalidCancel();
        }
        if (msg.sender != session.rider && msg.sender != session.driver) {
            revert NotParticipant();
        }
        session.state = SessionState.Canceled;
        session.updatedAt = uint64(block.timestamp);
        emit StateChanged(sessionId, session.state, session.updatedAt);

        uint256 riderDeposit = session.riderDeposit;
        uint256 driverBond = session.driverBond;
        session.riderDeposit = 0;
        session.driverBond = 0;

        _refundMessageBonds(sessionId, session.rider, session.driver);

        if (msg.sender == session.rider) {
            uint256 fee = (riderDeposit * cancelFeeBps) / 10000;
            _refund(session.rider, riderDeposit - fee);
            _refund(session.driver, fee + driverBond);
        } else {
            _refund(session.rider, riderDeposit + driverBond);
            _callSlashingHook(session.driver, driverBond, sessionId);
        }
    }

    function _matchSession(uint256 requestId, address driver, uint64 eta) internal {
        Request storage request = requests[requestId];
        if (request.matched) {
            revert InvalidRequest();
        }

        uint256 sessionId = nextSessionId++;
        request.matched = true;
        request.sessionId = sessionId;

        Session storage session = sessions[sessionId];
        session.rider = request.rider;
        session.driver = driver;
        session.requestId = requestId;
        session.riderDeposit = request.riderDeposit;
        session.driverBond = commits[requestId][driver].bond;
        session.createdAt = uint64(block.timestamp);
        session.updatedAt = uint64(block.timestamp);
        session.state = SessionState.Matched;

        emit Matched(sessionId, requestId, request.rider, driver, eta);
        emit StateChanged(sessionId, session.state, session.updatedAt);
    }

    function _updateCheckInState(uint256 sessionId, Session storage session) internal {
        if (session.riderCheckedIn && session.driverCheckedIn) {
            session.state = SessionState.BothCheckedIn;
        } else if (session.riderCheckedIn) {
            session.state = SessionState.RiderCheckedIn;
        } else if (session.driverCheckedIn) {
            session.state = SessionState.DriverCheckedIn;
        } else {
            session.state = SessionState.Matched;
        }
        emit StateChanged(sessionId, session.state, session.updatedAt);
    }

    function _checkRiderRateLimit(address rider) internal {
        RateLimit storage rate = riderRequestRate[rider];
        uint64 nowTs = uint64(block.timestamp);
        if (nowTs - rate.windowStart >= requestWindow) {
            rate.windowStart = nowTs;
            rate.count = 0;
        }
        if (rate.count >= maxRequestsPerWindow) {
            revert RateLimited();
        }
        rate.count += 1;
    }

    function _checkMessageRateLimit(uint256 sessionId, address sender) internal {
        RateLimit storage rate = messageRate[sessionId][sender];
        uint64 nowTs = uint64(block.timestamp);
        if (nowTs - rate.windowStart >= messageWindow) {
            rate.windowStart = nowTs;
            rate.count = 0;
        }
        if (rate.count >= maxMessagesPerWindow) {
            revert RateLimited();
        }
        rate.count += 1;
    }

    function _refundMessageBonds(uint256 sessionId, address rider, address driver) internal {
        uint256 riderBond = messageBondEscrow[sessionId][rider];
        if (riderBond > 0) {
            messageBondEscrow[sessionId][rider] = 0;
            _refund(rider, riderBond);
        }
        uint256 driverBond = messageBondEscrow[sessionId][driver];
        if (driverBond > 0) {
            messageBondEscrow[sessionId][driver] = 0;
            _refund(driver, driverBond);
        }
    }

    function _refund(address to, uint256 amount) internal {
        if (amount == 0) {
            return;
        }
        (bool ok, ) = payable(to).call{value: amount}("");
        require(ok, "refund failed");
    }

    function _validateEnvelope(bytes calldata header, bytes calldata ciphertext) internal view {
        if (DOUBLE_RATCHET_PRECOMPILE.code.length == 0) {
            return;
        }
        (bool ok, bytes memory ret) = DOUBLE_RATCHET_PRECOMPILE.staticcall(
            abi.encodeWithSignature(
                "validateEnvelope(bytes,bytes,uint32,uint32)",
                header,
                ciphertext,
                maxHeaderBytes,
                maxCiphertextBytes
            )
        );
        if (!ok || ret.length < 32) {
            revert InvalidEnvelope();
        }
        (bool valid, , , , , , ) = abi.decode(ret, (bool, bytes32, uint8, bytes32, uint32, uint32, bytes32));
        if (!valid) {
            revert InvalidEnvelope();
        }
    }

    function _callSlashingHook(address driver, uint256 amount, uint256 sessionId) internal {
        if (slashingHook == address(0) || amount == 0) {
            return;
        }
        try ISlashingHook(slashingHook).onDriverSlashed(driver, amount, sessionId) {} catch {}
    }

    function _requireSession(uint256 sessionId) internal view returns (Session storage session) {
        session = sessions[sessionId];
        if (session.rider == address(0)) {
            revert InvalidRequest();
        }
    }

    function _isExpired(Request storage request) internal view returns (bool) {
        if (request.ttl == 0) {
            return false;
        }
        return block.timestamp > uint256(request.createdAt + request.ttl);
    }
}
