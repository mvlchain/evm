package ridehail

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"

	sdk "github.com/cosmos/cosmos-sdk/types"
	cmn "github.com/cosmos/evm/precompiles/common"
)

const (
	VersionMethod            = "version"
	ValidateCreateRequestMethod = "validateCreateRequest"
	NextRequestIdMethod      = "nextRequestId"
	NextSessionIdMethod      = "nextSessionId"
	CreateRequestMethod      = "createRequest"
	AcceptCommitMethod       = "acceptCommit"
	AcceptRevealMethod       = "acceptReveal"
	RequestsMethod           = "requests"
	PostEncryptedMessageMethod = "postEncryptedMessage"

	EventRideRequested        = "RideRequested"
	EventDriverAcceptCommitted = "DriverAcceptCommitted"
	EventDriverAcceptRevealed  = "DriverAcceptRevealed"
	EventMatched              = "Matched"
	EventEncryptedMessage     = "EncryptedMessage"
	EventStateChanged         = "StateChanged"
)

const (
	minRiderDeposit = 1_000_000_000_000_000_000
	minDriverBond   = 200_000_000_000_000_000
	messageBond     = 10_000_000_000_000_000
	commitDuration  = 3600
	revealDuration  = 3600
	maxHeaderBytes  = 256
	maxCipherBytes  = 512
)

const (
	sessionStateMatched uint8 = 1
)

const rideHailVersion = 2

func (p Precompile) Version(method *abi.Method) ([]byte, error) {
	return method.Outputs.Pack(big.NewInt(rideHailVersion))
}

func (p Precompile) ValidateCreateRequest(method *abi.Method, ctx sdk.Context, evm *vm.EVM, contract *vm.Contract, args []interface{}) ([]byte, error) {
	if len(args) != 7 {
		return method.Outputs.Pack(false, "invalid args")
	}
	_, err := asBytes32(args[0])
	if err != nil {
		return method.Outputs.Pack(false, "invalid cellTopic")
	}
	_, err = asBytes32(args[1])
	if err != nil {
		return method.Outputs.Pack(false, "invalid regionTopic")
	}
	_, err = asBytes32(args[2])
	if err != nil {
		return method.Outputs.Pack(false, "invalid paramsHash")
	}
	_, err = asBytes32(args[3])
	if err != nil {
		return method.Outputs.Pack(false, "invalid pickupCommit")
	}
	_, err = asBytes32(args[4])
	if err != nil {
		return method.Outputs.Pack(false, "invalid dropoffCommit")
	}
	_, err = asUint32(args[5])
	if err != nil {
		return method.Outputs.Pack(false, "invalid maxDriverEta")
	}
	_, err = asUint64(args[6])
	if err != nil {
		return method.Outputs.Pack(false, "invalid ttl")
	}
	// Note: validateCreateRequest is a view function and cannot check msg.value
	// Deposit validation is done in createRequest
	return method.Outputs.Pack(true, "")
}

func (p Precompile) NextRequestId(method *abi.Method, ctx sdk.Context) ([]byte, error) {
	value := p.rideHailKeeper.GetNextRequestId(ctx)
	return method.Outputs.Pack(new(big.Int).SetUint64(value))
}

func (p Precompile) NextSessionId(method *abi.Method, ctx sdk.Context) ([]byte, error) {
	value := p.rideHailKeeper.GetNextSessionId(ctx)
	return method.Outputs.Pack(new(big.Int).SetUint64(value))
}

func (p Precompile) CreateRequest(method *abi.Method, ctx sdk.Context, evm *vm.EVM, contract *vm.Contract, args []interface{}) ([]byte, error) {
	fmt.Printf("[RideHail] ========== CreateRequest (Thin Proxy) ==========\n")
	fmt.Printf("[RideHail] Caller: %s\n", contract.Caller().Hex())

	if len(args) != 7 {
		return nil, fmt.Errorf("invalid args")
	}

	// Parse EVM arguments
	cellTopic, err := asBytes32(args[0])
	if err != nil {
		return nil, err
	}
	regionTopic, err := asBytes32(args[1])
	if err != nil {
		return nil, err
	}
	paramsHash, err := asBytes32(args[2])
	if err != nil {
		return nil, err
	}
	pickupCommit, err := asBytes32(args[3])
	if err != nil {
		return nil, err
	}
	dropoffCommit, err := asBytes32(args[4])
	if err != nil {
		return nil, err
	}
	maxDriverEta, err := asUint32(args[5])
	if err != nil {
		return nil, err
	}
	ttl, err := asUint64(args[6])
	if err != nil {
		return nil, err
	}

	// Convert EVM address to Cosmos bech32 address
	riderAddr := sdk.AccAddress(contract.Caller().Bytes())
	fmt.Printf("[RideHail] Rider (Cosmos): %s\n", riderAddr.String())

	fmt.Printf("[RideHail] Calling core Keeper.CreateRequest...\n")

	// Call core keeper method
	requestIdU64, err := p.rideHailKeeper.CreateRequest(
		ctx,
		riderAddr.String(),
		cellTopic[:],
		regionTopic[:],
		paramsHash[:],
		pickupCommit[:],
		dropoffCommit[:],
		maxDriverEta,
		uint32(ttl),
		"0",
	)
	if err != nil {
		fmt.Printf("[RideHail] ERROR: Keeper.CreateRequest failed: %v\n", err)
		return nil, err
	}

	requestId := new(big.Int).SetUint64(requestIdU64)
	fmt.Printf("[RideHail] ✅ Core request created! RequestId: %s\n", requestId.String())

	// Emit EVM event for compatibility with existing clients
	now := evm.Context.Time
	commitEnd := now + commitDuration
	revealEnd := commitEnd + revealDuration

	if err := p.emitRideRequested(
		evm,
		requestId,
		contract.Caller(),
		cellTopic,
		regionTopic,
		paramsHash,
		pickupCommit,
		dropoffCommit,
		commitEnd,
		revealEnd,
		big.NewInt(0),
	); err != nil {
		fmt.Printf("[RideHail] WARNING: Failed to emit EVM event: %v\n", err)
	}

	return method.Outputs.Pack(requestId)
}

func (p Precompile) AcceptCommit(method *abi.Method, ctx sdk.Context, evm *vm.EVM, contract *vm.Contract, args []interface{}) ([]byte, error) {
	fmt.Printf("[RideHail] ========== AcceptCommit (Thin Proxy) ==========\n")
	fmt.Printf("[RideHail] Driver: %s\n", contract.Caller().Hex())

	if len(args) != 3 {
		return nil, fmt.Errorf("invalid args")
	}

	// Parse EVM arguments
	requestId := args[0].(*big.Int)
	commitHash, err := asBytes32(args[1])
	if err != nil {
		return nil, err
	}
	eta, err := asUint64(args[2])
	if err != nil {
		return nil, err
	}

	// Convert EVM address to Cosmos bech32 address
	driverAddr := sdk.AccAddress(contract.Caller().Bytes())
	fmt.Printf("[RideHail] Driver (Cosmos): %s, RequestId: %s, ETA: %d\n", driverAddr.String(), requestId.String(), eta)

	fmt.Printf("[RideHail] Calling core Keeper.SubmitDriverCommit...\n")

	// Call core keeper method
	err = p.rideHailKeeper.SubmitDriverCommit(
		ctx,
		driverAddr.String(),
		requestId.Uint64(),
		commitHash[:],
		uint32(eta),
	)
	if err != nil {
		fmt.Printf("[RideHail] ERROR: Keeper.SubmitDriverCommit failed: %v\n", err)
		return nil, err
	}

	fmt.Printf("[RideHail] ✅ Driver commit submitted to core!\n")

	// Emit EVM event for compatibility
	if err := p.emitDriverAcceptCommitted(evm, requestId, contract.Caller(), commitHash, eta, big.NewInt(0)); err != nil {
		fmt.Printf("[RideHail] WARNING: Failed to emit EVM event: %v\n", err)
	}

	return method.Outputs.Pack()
}

func (p Precompile) AcceptReveal(method *abi.Method, ctx sdk.Context, evm *vm.EVM, contract *vm.Contract, args []interface{}) ([]byte, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("invalid args")
	}
	requestId := args[0].(*big.Int)
	eta, err := asUint64(args[1])
	if err != nil {
		return nil, err
	}
	driverCell, err := asBytes32(args[2])
	if err != nil {
		return nil, err
	}
	salt, err := asBytes32(args[3])
	if err != nil {
		return nil, err
	}

	// Get request data from Keeper
	requestData := p.rideHailKeeper.GetRequest(ctx, requestId.Uint64())
	if len(requestData) == 0 {
		return nil, fmt.Errorf("invalid request")
	}

	// Deserialize request data
	rider := common.BytesToAddress(requestData[0:20])
	cellTopic := common.BytesToHash(requestData[20:52])
	regionTopic := common.BytesToHash(requestData[52:84])
	riderDeposit := new(big.Int).SetBytes(requestData[180:212])
	commitEnd := sdk.BigEndianToUint64(requestData[220:228])
	revealEnd := sdk.BigEndianToUint64(requestData[228:236])
	maxDriverEta := uint64(requestData[244])<<24 | uint64(requestData[245])<<16 | uint64(requestData[246])<<8 | uint64(requestData[247])
	canceled := requestData[256] != 0

	if rider == (common.Address{}) {
		return nil, fmt.Errorf("invalid request")
	}
	if canceled {
		return nil, fmt.Errorf("invalid request")
	}
	if evm.Context.Time < commitEnd || evm.Context.Time > revealEnd {
		return nil, fmt.Errorf("reveal window closed")
	}

	stateDB := evm.StateDB
	commitBase := nestedCommitSlot(requestId, contract.Caller())
	commitHash := getHash(stateDB, p.Address(), addSlot(commitBase, 0))
	if commitHash == (common.Hash{}) || getBool(stateDB, p.Address(), addSlot(commitBase, 5)) {
		return nil, fmt.Errorf("invalid commit")
	}

	revealHash, err := computeRevealHash(requestId, contract.Caller(), eta, driverCell, salt)
	if err != nil {
		return nil, err
	}
	if revealHash != commitHash {
		return nil, fmt.Errorf("invalid reveal")
	}

	if eta > maxDriverEta {
		return nil, fmt.Errorf("eta too high")
	}

	if common.BytesToHash(driverCell[:]) != cellTopic && common.BytesToHash(driverCell[:]) != regionTopic {
		return nil, fmt.Errorf("invalid cell")
	}

	setBool(stateDB, p.Address(), addSlot(commitBase, 5), true)
	setUint64(stateDB, p.Address(), addSlot(commitBase, 2), eta)
	setHash(stateDB, p.Address(), addSlot(commitBase, 4), common.BytesToHash(driverCell[:]))

	if err := p.emitDriverAcceptRevealed(evm, requestId, contract.Caller(), revealHash, eta, driverCell); err != nil {
		return nil, err
	}

	// Check if already matched from deserialized data
	matched := requestData[257] != 0
	if !matched {
		// Get and increment sessionId using Keeper
		sessionIdU64 := p.rideHailKeeper.GetNextSessionId(ctx)
		sessionId := new(big.Int).SetUint64(sessionIdU64)
		p.rideHailKeeper.SetNextSessionId(ctx, sessionIdU64+1)

		// Update request to mark as matched and store sessionId
		// We need to update bytes 248-257 (sessionId + matched flag)
		copy(requestData[248:256], sdk.Uint64ToBigEndian(sessionIdU64))
		requestData[257] = 1 // set matched to true
		p.rideHailKeeper.SetRequest(ctx, requestId.Uint64(), requestData)

		// Get driver deposit from stateDB (temporary commit storage)
		driverDeposit := getUint256(stateDB, p.Address(), addSlot(commitBase, 3))

		// Create session data
		// Format: rider(20) + driver(20) + requestId(32) + riderDeposit(32) + driverDeposit(32)
		//         + createdAt(8) + lastUpdate(8) + lastMessageHash(32) + riderComplete(1) + driverComplete(1) + state(8)
		sessionData := make([]byte, 0, 194)
		sessionData = append(sessionData, rider.Bytes()...)                                  // 20 bytes
		sessionData = append(sessionData, contract.Caller().Bytes()...)                      // 20 bytes
		sessionData = append(sessionData, common.LeftPadBytes(requestId.Bytes(), 32)...)     // 32 bytes
		sessionData = append(sessionData, common.LeftPadBytes(riderDeposit.Bytes(), 32)...)  // 32 bytes
		sessionData = append(sessionData, common.LeftPadBytes(driverDeposit.Bytes(), 32)...) // 32 bytes
		sessionData = append(sessionData, sdk.Uint64ToBigEndian(evm.Context.Time)...)        // 8 bytes - createdAt
		sessionData = append(sessionData, sdk.Uint64ToBigEndian(evm.Context.Time)...)        // 8 bytes - lastUpdate
		sessionData = append(sessionData, make([]byte, 32)...)                               // 32 bytes - lastMessageHash (empty)
		sessionData = append(sessionData, 0)                                                 // 1 byte - riderComplete (false)
		sessionData = append(sessionData, 0)                                                 // 1 byte - driverComplete (false)
		sessionData = append(sessionData, sdk.Uint64ToBigEndian(uint64(sessionStateMatched))...) // 8 bytes - state

		// Store session data using Keeper
		p.rideHailKeeper.SetSession(ctx, sessionIdU64, sessionData)

		if err := p.emitMatched(evm, sessionId, requestId, rider, contract.Caller(), eta); err != nil {
			return nil, err
		}
		if err := p.emitStateChanged(evm, sessionId, sessionStateMatched, evm.Context.Time); err != nil {
			return nil, err
		}
	}

	return method.Outputs.Pack()
}

func (p Precompile) Requests(method *abi.Method, ctx sdk.Context, evm *vm.EVM, args []interface{}) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("invalid args")
	}
	requestId := args[0].(*big.Int)

	// Get request data from Keeper
	requestData := p.rideHailKeeper.GetRequest(ctx, requestId.Uint64())
	if len(requestData) == 0 {
		// Return empty/zero values for non-existent request
		return method.Outputs.Pack(
			common.Address{},
			common.Hash{},
			common.Hash{},
			common.Hash{},
			common.Hash{},
			common.Hash{},
			big.NewInt(0),
			uint64(0),
			uint64(0),
			uint64(0),
			uint64(0),
			uint32(0),
			uint32(0),
			false,
			false,
			big.NewInt(0),
		)
	}

	// Deserialize request data
	// Format: rider(20) + cellTopic(32) + regionTopic(32) + paramsHash(32) + pickupCommit(32) + dropoffCommit(32)
	//         + deposit(32) + createdAt(8) + commitEnd(8) + revealEnd(8) + ttl(8) + maxDriverEta(4)
	//         + sessionId(8) + cancelled(1) + fulfilled(1) + sessionDeposit(32)
	rider := common.BytesToAddress(requestData[0:20])
	cellTopic := common.BytesToHash(requestData[20:52])
	regionTopic := common.BytesToHash(requestData[52:84])
	paramsHash := common.BytesToHash(requestData[84:116])
	pickupCommit := common.BytesToHash(requestData[116:148])
	dropoffCommit := common.BytesToHash(requestData[148:180])
	riderDeposit := new(big.Int).SetBytes(requestData[180:212])
	createdAt := sdk.BigEndianToUint64(requestData[212:220])
	commitEnd := sdk.BigEndianToUint64(requestData[220:228])
	revealEnd := sdk.BigEndianToUint64(requestData[228:236])
	ttl := sdk.BigEndianToUint64(requestData[236:244])
	maxDriverEta := uint32(requestData[244])<<24 | uint32(requestData[245])<<16 | uint32(requestData[246])<<8 | uint32(requestData[247])
	sessionIdU64 := sdk.BigEndianToUint64(requestData[248:256])
	sessionId := new(big.Int).SetUint64(sessionIdU64)
	canceled := requestData[256] != 0
	matched := requestData[257] != 0

	// Get commitCount from stateDB (temporary storage)
	stateDB := evm.StateDB
	base := mappingSlot(seedRequest(), requestId)
	commitCount := uint32(getUint64(stateDB, p.Address(), addSlot(base, 12)))

	return method.Outputs.Pack(
		rider,
		cellTopic,
		regionTopic,
		paramsHash,
		pickupCommit,
		dropoffCommit,
		riderDeposit,
		createdAt,
		commitEnd,
		revealEnd,
		ttl,
		maxDriverEta,
		commitCount,
		canceled,
		matched,
		sessionId,
	)
}

func (p Precompile) PostEncryptedMessage(method *abi.Method, ctx sdk.Context, evm *vm.EVM, contract *vm.Contract, args []interface{}) ([]byte, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("invalid args")
	}
	sessionId := args[0].(*big.Int)
	msgIndex := args[1].(uint32)
	header := args[2].([]byte)
	ciphertext := args[3].([]byte)

	if len(header) > maxHeaderBytes || len(ciphertext) > maxCipherBytes {
		return nil, fmt.Errorf("message too large")
	}

	stateDB := evm.StateDB
	sessionBase := mappingSlot(seedSession(), sessionId)
	rider := getAddress(stateDB, p.Address(), addSlot(sessionBase, 0))
	driver := getAddress(stateDB, p.Address(), addSlot(sessionBase, 1))
	if rider == (common.Address{}) || driver == (common.Address{}) {
		return nil, fmt.Errorf("invalid session")
	}
	if contract.Caller() != rider && contract.Caller() != driver {
		return nil, fmt.Errorf("not participant")
	}

	lastIdx := getUint64(stateDB, p.Address(), nestedMsgIndexSlot(sessionId, contract.Caller()))
	if msgIndex <= uint32(lastIdx) {
		return nil, fmt.Errorf("invalid msg index")
	}
	setUint64(stateDB, p.Address(), nestedMsgIndexSlot(sessionId, contract.Caller()), uint64(msgIndex))

	msgBase := messageSlot(sessionId, contract.Caller(), msgIndex)
	storeBytes(stateDB, p.Address(), msgBase, header)
	storeBytes(stateDB, p.Address(), addSlot(msgBase, 1), ciphertext)

	if err := p.emitEncryptedMessage(evm, sessionId, contract.Caller(), msgIndex, header, ciphertext); err != nil {
		return nil, err
	}

	return method.Outputs.Pack()
}

func (p Precompile) emitRideRequested(
	evm *vm.EVM,
	requestId *big.Int,
	rider common.Address,
	cellTopic [32]byte,
	regionTopic [32]byte,
	paramsHash [32]byte,
	pickupCommit [32]byte,
	dropoffCommit [32]byte,
	commitEnd uint64,
	revealEnd uint64,
	riderDeposit *big.Int,
) error {
	event := p.Events[EventRideRequested]
	topics := []common.Hash{
		event.ID,
		common.BytesToHash(cellTopic[:]),
		common.BytesToHash(regionTopic[:]),
		common.BigToHash(requestId),
	}
	arguments := abi.Arguments{event.Inputs[3], event.Inputs[4], event.Inputs[5], event.Inputs[6], event.Inputs[7], event.Inputs[8], event.Inputs[9]}
	data, err := arguments.Pack(rider, common.BytesToHash(paramsHash[:]), common.BytesToHash(pickupCommit[:]), common.BytesToHash(dropoffCommit[:]), commitEnd, revealEnd, riderDeposit)
	if err != nil {
		return err
	}
	evm.StateDB.AddLog(&ethtypes.Log{Address: p.Address(), Topics: topics, Data: data, BlockNumber: uint64(evm.Context.BlockNumber.Int64())})
	return nil
}

func (p Precompile) emitDriverAcceptCommitted(evm *vm.EVM, requestId *big.Int, driver common.Address, commitHash [32]byte, eta uint64, bond *big.Int) error {
	event := p.Events[EventDriverAcceptCommitted]
	topics := []common.Hash{event.ID, common.BigToHash(requestId)}
	driverTopic, err := cmn.MakeTopic(driver)
	if err != nil {
		return err
	}
	topics = append(topics, driverTopic)
	arguments := abi.Arguments{event.Inputs[2], event.Inputs[3], event.Inputs[4]}
	data, err := arguments.Pack(common.BytesToHash(commitHash[:]), eta, bond)
	if err != nil {
		return err
	}
	evm.StateDB.AddLog(&ethtypes.Log{Address: p.Address(), Topics: topics, Data: data, BlockNumber: uint64(evm.Context.BlockNumber.Int64())})
	return nil
}

func (p Precompile) emitDriverAcceptRevealed(evm *vm.EVM, requestId *big.Int, driver common.Address, revealHash common.Hash, eta uint64, driverCell [32]byte) error {
	event := p.Events[EventDriverAcceptRevealed]
	topics := []common.Hash{event.ID, common.BigToHash(requestId)}
	driverTopic, err := cmn.MakeTopic(driver)
	if err != nil {
		return err
	}
	topics = append(topics, driverTopic)
	arguments := abi.Arguments{event.Inputs[2], event.Inputs[3], event.Inputs[4]}
	data, err := arguments.Pack(revealHash, eta, common.BytesToHash(driverCell[:]))
	if err != nil {
		return err
	}
	evm.StateDB.AddLog(&ethtypes.Log{Address: p.Address(), Topics: topics, Data: data, BlockNumber: uint64(evm.Context.BlockNumber.Int64())})
	return nil
}

func (p Precompile) emitMatched(evm *vm.EVM, sessionId, requestId *big.Int, rider, driver common.Address, eta uint64) error {
	event := p.Events[EventMatched]
	topics := []common.Hash{event.ID, common.BigToHash(sessionId), common.BigToHash(requestId)}
	riderTopic, err := cmn.MakeTopic(rider)
	if err != nil {
		return err
	}
	topics = append(topics, riderTopic)
	arguments := abi.Arguments{event.Inputs[3], event.Inputs[4]}
	data, err := arguments.Pack(driver, eta)
	if err != nil {
		return err
	}
	evm.StateDB.AddLog(&ethtypes.Log{Address: p.Address(), Topics: topics, Data: data, BlockNumber: uint64(evm.Context.BlockNumber.Int64())})
	return nil
}

func (p Precompile) emitEncryptedMessage(evm *vm.EVM, sessionId *big.Int, sender common.Address, msgIndex uint32, header, ciphertext []byte) error {
	event := p.Events[EventEncryptedMessage]
	topics := []common.Hash{event.ID, common.BigToHash(sessionId)}
	senderTopic, err := cmn.MakeTopic(sender)
	if err != nil {
		return err
	}
	topics = append(topics, senderTopic)
	arguments := abi.Arguments{event.Inputs[2], event.Inputs[3], event.Inputs[4]}
	data, err := arguments.Pack(msgIndex, header, ciphertext)
	if err != nil {
		return err
	}
	evm.StateDB.AddLog(&ethtypes.Log{Address: p.Address(), Topics: topics, Data: data, BlockNumber: uint64(evm.Context.BlockNumber.Int64())})
	return nil
}

func (p Precompile) emitStateChanged(evm *vm.EVM, sessionId *big.Int, newState uint8, timestamp uint64) error {
	event := p.Events[EventStateChanged]
	topics := []common.Hash{event.ID, common.BigToHash(sessionId)}
	arguments := abi.Arguments{event.Inputs[1], event.Inputs[2]}
	data, err := arguments.Pack(newState, timestamp)
	if err != nil {
		return err
	}
	evm.StateDB.AddLog(&ethtypes.Log{Address: p.Address(), Topics: topics, Data: data, BlockNumber: uint64(evm.Context.BlockNumber.Int64())})
	return nil
}

func computeRevealHash(requestId *big.Int, driver common.Address, eta uint64, driverCell [32]byte, salt [32]byte) (common.Hash, error) {
	uint256Type, _ := abi.NewType("uint256", "", nil)
	addressType, _ := abi.NewType("address", "", nil)
	uint64Type, _ := abi.NewType("uint64", "", nil)
	bytes32Type, _ := abi.NewType("bytes32", "", nil)
	args := abi.Arguments{
		{Type: uint256Type},
		{Type: addressType},
		{Type: uint64Type},
		{Type: bytes32Type},
		{Type: bytes32Type},
	}
	bz, err := args.Pack(requestId, driver, eta, common.BytesToHash(driverCell[:]), common.BytesToHash(salt[:]))
	if err != nil {
		return common.Hash{}, err
	}
	return crypto.Keccak256Hash(bz), nil
}

func slot(name string) common.Hash {
	return crypto.Keccak256Hash([]byte(name))
}

func seedRequest() common.Hash { return slot("rh.request") }
func seedSession() common.Hash { return slot("rh.session") }
func seedCommit() common.Hash  { return slot("rh.commit") }
func seedMsgIndex() common.Hash { return slot("rh.msgIndex") }
func seedMessage() common.Hash { return slot("rh.message") }

func mappingSlot(seed common.Hash, key *big.Int) common.Hash {
	keyBytes := common.LeftPadBytes(key.Bytes(), 32)
	slotBytes := common.LeftPadBytes(seed.Bytes(), 32)
	return crypto.Keccak256Hash(append(keyBytes, slotBytes...))
}

func nestedCommitSlot(requestId *big.Int, driver common.Address) common.Hash {
	outer := mappingSlot(seedCommit(), requestId)
	keyBytes := common.LeftPadBytes(driver.Bytes(), 32)
	slotBytes := common.LeftPadBytes(outer.Bytes(), 32)
	return crypto.Keccak256Hash(append(keyBytes, slotBytes...))
}

func nestedMsgIndexSlot(sessionId *big.Int, sender common.Address) common.Hash {
	outer := mappingSlot(seedMsgIndex(), sessionId)
	keyBytes := common.LeftPadBytes(sender.Bytes(), 32)
	slotBytes := common.LeftPadBytes(outer.Bytes(), 32)
	return crypto.Keccak256Hash(append(keyBytes, slotBytes...))
}

func messageSlot(sessionId *big.Int, sender common.Address, msgIndex uint32) common.Hash {
	outer := mappingSlot(seedMessage(), sessionId)
	senderSlot := crypto.Keccak256Hash(append(common.LeftPadBytes(sender.Bytes(), 32), common.LeftPadBytes(outer.Bytes(), 32)...))
	idx := new(big.Int).SetUint64(uint64(msgIndex))
	return crypto.Keccak256Hash(append(common.LeftPadBytes(idx.Bytes(), 32), common.LeftPadBytes(senderSlot.Bytes(), 32)...))
}

func addSlot(base common.Hash, offset uint64) common.Hash {
	value := new(big.Int).SetBytes(base.Bytes())
	return common.BigToHash(value.Add(value, new(big.Int).SetUint64(offset)))
}

func setHash(stateDB vm.StateDB, addr common.Address, slot common.Hash, value common.Hash) {
	stateDB.SetState(addr, slot, value)
}

func getHash(stateDB vm.StateDB, addr common.Address, slot common.Hash) common.Hash {
	return stateDB.GetState(addr, slot)
}

func setUint256(stateDB vm.StateDB, addr common.Address, slot common.Hash, value *big.Int) {
	stateDB.SetState(addr, slot, common.BigToHash(value))
}

func getUint256(stateDB vm.StateDB, addr common.Address, slot common.Hash) *big.Int {
	return stateDB.GetState(addr, slot).Big()
}

func setUint64(stateDB vm.StateDB, addr common.Address, slot common.Hash, value uint64) {
	stateDB.SetState(addr, slot, common.BigToHash(new(big.Int).SetUint64(value)))
}

func getUint64(stateDB vm.StateDB, addr common.Address, slot common.Hash) uint64 {
	return stateDB.GetState(addr, slot).Big().Uint64()
}

func setBool(stateDB vm.StateDB, addr common.Address, slot common.Hash, value bool) {
	var v uint64
	if value {
		v = 1
	}
	setUint64(stateDB, addr, slot, v)
}

func getBool(stateDB vm.StateDB, addr common.Address, slot common.Hash) bool {
	return getUint64(stateDB, addr, slot) != 0
}

func setAddress(stateDB vm.StateDB, addr common.Address, slot common.Hash, value common.Address) {
	stateDB.SetState(addr, slot, common.BytesToHash(common.LeftPadBytes(value.Bytes(), 32)))
}

func getAddress(stateDB vm.StateDB, addr common.Address, slot common.Hash) common.Address {
	value := stateDB.GetState(addr, slot)
	return common.BytesToAddress(value.Bytes())
}

func storeBytes(stateDB vm.StateDB, addr common.Address, slot common.Hash, data []byte) {
	setUint64(stateDB, addr, slot, uint64(len(data)))
	base := crypto.Keccak256Hash(slot.Bytes())
	for i := 0; i < len(data); i += 32 {
		chunk := data[i:]
		if len(chunk) > 32 {
			chunk = chunk[:32]
		}
		stateDB.SetState(addr, addSlot(base, uint64(i/32)), common.BytesToHash(common.RightPadBytes(chunk, 32)))
	}
}

func asBytes32(value interface{}) ([32]byte, error) {
	switch v := value.(type) {
	case [32]byte:
		return v, nil
	case common.Hash:
		var out [32]byte
		copy(out[:], v.Bytes())
		return out, nil
	case []byte:
		if len(v) != 32 {
			return [32]byte{}, fmt.Errorf("invalid bytes32 length")
		}
		var out [32]byte
		copy(out[:], v)
		return out, nil
	default:
		return [32]byte{}, fmt.Errorf("invalid bytes32 type")
	}
}

func asUint64(value interface{}) (uint64, error) {
	switch v := value.(type) {
	case uint64:
		return v, nil
	case uint32:
		return uint64(v), nil
	case int64:
		if v < 0 {
			return 0, fmt.Errorf("invalid uint64")
		}
		return uint64(v), nil
	case *big.Int:
		if v.Sign() < 0 {
			return 0, fmt.Errorf("invalid uint64")
		}
		return v.Uint64(), nil
	default:
		return 0, fmt.Errorf("invalid uint64 type")
	}
}

func asUint32(value interface{}) (uint32, error) {
	switch v := value.(type) {
	case uint32:
		return v, nil
	case uint64:
		if v > uint64(^uint32(0)) {
			return 0, fmt.Errorf("uint32 overflow")
		}
		return uint32(v), nil
	case int64:
		if v < 0 || v > int64(^uint32(0)) {
			return 0, fmt.Errorf("uint32 overflow")
		}
		return uint32(v), nil
	case *big.Int:
		if v.Sign() < 0 || v.BitLen() > 32 {
			return 0, fmt.Errorf("uint32 overflow")
		}
		return uint32(v.Uint64()), nil
	default:
		return 0, fmt.Errorf("invalid uint32 type")
	}
}
