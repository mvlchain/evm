package keeper

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/evm/x/ridehail/types"
)

// ProcessMatching runs the driver matching algorithm at the core level
// This is called from BeginBlocker to match pending requests with driver commits
func (k Keeper) ProcessMatching(ctx sdk.Context) error {
	// Get all pending requests
	pendingRequests := k.GetAllPendingRequests(ctx)

	currentTime := ctx.BlockTime().Unix()

	for _, req := range pendingRequests {
		// Check if request expired
		if currentTime > req.ExpiresAt {
			k.ExpireRequest(ctx, req.RequestId)
			continue
		}

		// Get driver commits for this request
		commits := k.GetDriverCommits(ctx, req.RequestId)

		if len(commits) == 0 {
			// No drivers yet, wait for next block
			continue
		}

		// Select best driver based on ETA
		matchedDriver := k.SelectBestDriver(ctx, req, commits)
		if matchedDriver == nil {
			// No valid drivers, wait for next block
			continue
		}

		// Create session
		sessionId := k.CreateMatchedSession(ctx, req, matchedDriver)

		// Emit match event for clients to detect immediately
		ctx.EventManager().EmitEvent(
			sdk.NewEvent(
				"ridehail_match",
				sdk.NewAttribute("request_id", fmt.Sprintf("%d", req.RequestId)),
				sdk.NewAttribute("session_id", fmt.Sprintf("%d", sessionId)),
				sdk.NewAttribute("rider", req.Rider),
				sdk.NewAttribute("driver", matchedDriver.Driver),
			),
		)

		// Clean up
		k.DeletePendingRequest(ctx, req.RequestId)
		k.DeleteDriverCommits(ctx, req.RequestId)

		k.Logger(ctx).Info(
			"Matched rider with driver",
			"request_id", req.RequestId,
			"session_id", sessionId,
			"rider", req.Rider,
			"driver", matchedDriver.Driver,
			"eta", matchedDriver.Eta,
		)
	}

	return nil
}

// SelectBestDriver chooses the best driver from commits
func (k Keeper) SelectBestDriver(ctx sdk.Context, req *types.PendingRequest, commits []*types.DriverCommit) *types.DriverCommit {
	var bestDriver *types.DriverCommit

	for _, commit := range commits {
		// Validate ETA is within acceptable range
		if commit.Eta > req.MaxDriverEta {
			continue
		}

		// Validate driver commit (basic check - full verification happens on reveal)
		if len(commit.DriverCommit) != 32 {
			continue
		}

		// Select driver with lowest ETA
		if bestDriver == nil || commit.Eta < bestDriver.Eta {
			bestDriver = commit
		}
	}

	return bestDriver
}

// CreateMatchedSession creates a session after matching
func (k Keeper) CreateMatchedSession(ctx sdk.Context, req *types.PendingRequest, driverCommit *types.DriverCommit) uint64 {
	sessionId := k.GetNextSessionId(ctx)

	session := &types.Session{
		SessionId:       sessionId,
		RequestId:       req.RequestId,
		Rider:           req.Rider,
		Driver:          driverCommit.Driver,
		PickupRevealed:  false,
		DropoffRevealed: false,
		Status:          types.SessionStatusPending,
		CreatedAt:       ctx.BlockTime().Unix(),
	}

	bz, err := json.Marshal(session)
	if err != nil {
		panic(err)
	}

	store := ctx.KVStore(k.storeKey)
	key := types.SessionKey(sessionId)
	store.Set(key, bz)

	k.SetNextSessionId(ctx, sessionId+1)

	return sessionId
}

// ExpireRequest handles expired requests
func (k Keeper) ExpireRequest(ctx sdk.Context, requestId uint64) {
	// Delete pending request
	k.DeletePendingRequest(ctx, requestId)

	// Delete all driver commits
	k.DeleteDriverCommits(ctx, requestId)

	// Emit expired event
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"ridehail_request_expired",
			sdk.NewAttribute("request_id", fmt.Sprintf("%d", requestId)),
		),
	)

	k.Logger(ctx).Info("Request expired", "request_id", requestId)
}

// ValidatePickupReveal validates pickup location reveal
func (k Keeper) ValidatePickupReveal(ctx sdk.Context, sessionId uint64, coord []byte, salt []byte) (bool, error) {
	session, err := k.GetSessionByID(ctx, sessionId)
	if err != nil {
		return false, err
	}

	req, found := k.GetPendingRequest(ctx, session.RequestId)
	if !found {
		return false, types.ErrRequestNotFound
	}

	// Verify commitment: hash(coord || salt) == pickupCommit
	hasher := sha256.New()
	hasher.Write(coord)
	hasher.Write(salt)
	computedCommit := hasher.Sum(nil)

	if !bytes.Equal(computedCommit, req.PickupCommit) {
		return false, types.ErrInvalidReveal
	}

	return true, nil
}

// ValidateDropoffReveal validates dropoff location reveal
func (k Keeper) ValidateDropoffReveal(ctx sdk.Context, sessionId uint64, coord []byte, salt []byte) (bool, error) {
	session, err := k.GetSessionByID(ctx, sessionId)
	if err != nil {
		return false, err
	}

	req, found := k.GetPendingRequest(ctx, session.RequestId)
	if !found {
		return false, types.ErrRequestNotFound
	}

	// Verify commitment: hash(coord || salt) == dropoffCommit
	hasher := sha256.New()
	hasher.Write(coord)
	hasher.Write(salt)
	computedCommit := hasher.Sum(nil)

	if !bytes.Equal(computedCommit, req.DropoffCommit) {
		return false, types.ErrInvalidReveal
	}

	return true, nil
}

// GetSessionByID retrieves a session by ID
func (k Keeper) GetSessionByID(ctx sdk.Context, sessionId uint64) (*types.Session, error) {
	store := ctx.KVStore(k.storeKey)
	key := types.SessionKey(sessionId)

	bz := store.Get(key)
	if bz == nil {
		return nil, types.ErrSessionNotFound
	}

	var session types.Session
	if err := json.Unmarshal(bz, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

// UpdateSession updates session state
func (k Keeper) UpdateSession(ctx sdk.Context, session *types.Session) {
	bz, err := json.Marshal(session)
	if err != nil {
		panic(err)
	}

	store := ctx.KVStore(k.storeKey)
	key := types.SessionKey(session.SessionId)
	store.Set(key, bz)
}
