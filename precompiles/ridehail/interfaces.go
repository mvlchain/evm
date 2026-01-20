package ridehail

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// RideHailKeeper defines the expected interface for the RideHail keeper
type RideHailKeeper interface {
	GetNextRequestId(ctx sdk.Context) uint64
	SetNextRequestId(ctx sdk.Context, id uint64)
	GetNextSessionId(ctx sdk.Context) uint64
	SetNextSessionId(ctx sdk.Context, id uint64)
	SetRequest(ctx sdk.Context, requestId uint64, data []byte)
	GetRequest(ctx sdk.Context, requestId uint64) []byte
	SetSession(ctx sdk.Context, sessionId uint64, data []byte)
	GetSession(ctx sdk.Context, sessionId uint64) []byte

	// Core message processing methods
	CreateRequest(ctx sdk.Context, rider string, cellTopic, regionTopic, paramsHash, pickupCommit, dropoffCommit []byte, maxDriverEta uint32, ttl uint32, deposit string) (uint64, error)
	SubmitDriverCommit(ctx sdk.Context, driver string, requestId uint64, driverCommit []byte, eta uint32) error
}
