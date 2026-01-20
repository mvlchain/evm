package keeper

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/evm/x/ridehail/types"
)

type msgServer struct {
	Keeper
}

// NewMsgServerImpl returns an implementation of the ridehail MsgServer interface
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{Keeper: keeper}
}

var _ types.MsgServer = msgServer{}

// CreateRequest (method for Keeper to satisfy interface)
func (k Keeper) CreateRequest(ctx sdk.Context, rider string, cellTopic, regionTopic, paramsHash, pickupCommit, dropoffCommit []byte, maxDriverEta uint32, ttl uint32, deposit string) (uint64, error) {
	msg := &types.MsgCreateRequest{
		Rider:         rider,
		CellTopic:     cellTopic,
		RegionTopic:   regionTopic,
		ParamsHash:    paramsHash,
		PickupCommit:  pickupCommit,
		DropoffCommit: dropoffCommit,
		MaxDriverEta:  maxDriverEta,
		Ttl:           ttl,
		Deposit:       deposit,
	}

	msgServer := NewMsgServerImpl(k)
	resp, err := msgServer.CreateRequest(ctx, msg)
	if err != nil {
		return 0, err
	}
	return resp.RequestId, nil
}

// SubmitDriverCommit (method for Keeper to satisfy interface)
func (k Keeper) SubmitDriverCommit(ctx sdk.Context, driver string, requestId uint64, driverCommit []byte, eta uint32) error {
	msg := &types.MsgSubmitDriverCommit{
		Driver:       driver,
		RequestId:    requestId,
		DriverCommit: driverCommit,
		Eta:          eta,
	}

	msgServer := NewMsgServerImpl(k)
	_, err := msgServer.SubmitDriverCommit(ctx, msg)
	return err
}

// CreateRequest handles ride request creation at core level
func (m msgServer) CreateRequest(goCtx sdk.Context, msg *types.MsgCreateRequest) (*types.MsgCreateRequestResponse, error) {
	ctx := goCtx

	// Get next request ID
	requestId := m.GetNextRequestId(ctx)

	// Create pending request
	pendingReq := &types.PendingRequest{
		RequestId:     requestId,
		Rider:         msg.Rider,
		CellTopic:     msg.CellTopic,
		RegionTopic:   msg.RegionTopic,
		ParamsHash:    msg.ParamsHash,
		PickupCommit:  msg.PickupCommit,
		DropoffCommit: msg.DropoffCommit,
		MaxDriverEta:  msg.MaxDriverEta,
		Ttl:           msg.Ttl,
		CreatedAt:     ctx.BlockTime().Unix(),
		ExpiresAt:     ctx.BlockTime().Unix() + int64(msg.Ttl),
		Deposit:       msg.Deposit,
	}

	// Store pending request
	m.StorePendingRequest(ctx, pendingReq)

	// Increment request ID
	m.SetNextRequestId(ctx, requestId+1)

	// Emit event for immediate driver detection
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"ridehail_request_created",
			sdk.NewAttribute("request_id", fmt.Sprintf("%d", requestId)),
			sdk.NewAttribute("rider", msg.Rider),
			sdk.NewAttribute("cell_topic", fmt.Sprintf("%x", msg.CellTopic)),
			sdk.NewAttribute("max_eta", fmt.Sprintf("%d", msg.MaxDriverEta)),
			sdk.NewAttribute("expires_at", fmt.Sprintf("%d", pendingReq.ExpiresAt)),
		),
	)

	m.Logger(ctx).Info(
		"Ride request created",
		"request_id", requestId,
		"rider", msg.Rider,
		"expires_at", pendingReq.ExpiresAt,
	)

	return &types.MsgCreateRequestResponse{RequestId: requestId}, nil
}

// SubmitDriverCommit handles driver commitment at core level
func (m msgServer) SubmitDriverCommit(goCtx sdk.Context, msg *types.MsgSubmitDriverCommit) (*types.MsgSubmitDriverCommitResponse, error) {
	ctx := goCtx

	// Verify request exists and is not expired
	pendingReq, found := m.GetPendingRequest(ctx, msg.RequestId)
	if !found {
		return nil, types.ErrRequestNotFound
	}

	currentTime := ctx.BlockTime().Unix()
	if currentTime > pendingReq.ExpiresAt {
		return nil, types.ErrRequestExpired
	}

	// Store driver commit
	commit := &types.DriverCommit{
		RequestId:    msg.RequestId,
		Driver:       msg.Driver,
		DriverCommit: msg.DriverCommit,
		Eta:          msg.Eta,
		SubmittedAt:  currentTime,
	}

	m.StoreDriverCommit(ctx, commit)

	// Emit event for immediate processing
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"driver_commit_submitted",
			sdk.NewAttribute("request_id", fmt.Sprintf("%d", msg.RequestId)),
			sdk.NewAttribute("driver", msg.Driver),
			sdk.NewAttribute("eta", fmt.Sprintf("%d", msg.Eta)),
		),
	)

	m.Logger(ctx).Info(
		"Driver commit submitted",
		"request_id", msg.RequestId,
		"driver", msg.Driver,
		"eta", msg.Eta,
	)

	return &types.MsgSubmitDriverCommitResponse{Success: true}, nil
}

// RevealPickup handles pickup location reveal
func (m msgServer) RevealPickup(goCtx sdk.Context, msg *types.MsgRevealPickup) (*types.MsgRevealPickupResponse, error) {
	ctx := goCtx

	// Validate reveal
	valid, err := m.ValidatePickupReveal(ctx, msg.SessionId, msg.PickupCoord, msg.PickupSalt)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, types.ErrInvalidReveal
	}

	// Get session
	session, err := m.GetSessionByID(ctx, msg.SessionId)
	if err != nil {
		return nil, err
	}

	// Update session
	session.PickupRevealed = true
	session.PickupCoord = msg.PickupCoord
	m.UpdateSession(ctx, session)

	// Emit event
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"pickup_revealed",
			sdk.NewAttribute("session_id", fmt.Sprintf("%d", msg.SessionId)),
			sdk.NewAttribute("rider", msg.Rider),
		),
	)

	m.Logger(ctx).Info("Pickup revealed", "session_id", msg.SessionId)

	return &types.MsgRevealPickupResponse{Success: true}, nil
}

// RevealDropoff handles dropoff location reveal
func (m msgServer) RevealDropoff(goCtx sdk.Context, msg *types.MsgRevealDropoff) (*types.MsgRevealDropoffResponse, error) {
	ctx := goCtx

	// Validate reveal
	valid, err := m.ValidateDropoffReveal(ctx, msg.SessionId, msg.DropoffCoord, msg.DropoffSalt)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, types.ErrInvalidReveal
	}

	// Get session
	session, err := m.GetSessionByID(ctx, msg.SessionId)
	if err != nil {
		return nil, err
	}

	// Update session
	session.DropoffRevealed = true
	session.DropoffCoord = msg.DropoffCoord
	session.Status = types.SessionStatusActive
	m.UpdateSession(ctx, session)

	// Emit event
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"dropoff_revealed",
			sdk.NewAttribute("session_id", fmt.Sprintf("%d", msg.SessionId)),
			sdk.NewAttribute("rider", msg.Rider),
		),
	)

	m.Logger(ctx).Info("Dropoff revealed - ride active", "session_id", msg.SessionId)

	return &types.MsgRevealDropoffResponse{Success: true}, nil
}
