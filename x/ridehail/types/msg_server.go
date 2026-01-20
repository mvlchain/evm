package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// MsgServer defines the ridehail Msg service
type MsgServer interface {
	CreateRequest(sdk.Context, *MsgCreateRequest) (*MsgCreateRequestResponse, error)
	SubmitDriverCommit(sdk.Context, *MsgSubmitDriverCommit) (*MsgSubmitDriverCommitResponse, error)
	RevealPickup(sdk.Context, *MsgRevealPickup) (*MsgRevealPickupResponse, error)
	RevealDropoff(sdk.Context, *MsgRevealDropoff) (*MsgRevealDropoffResponse, error)
}

// RegisterMsgServer registers the MsgServer implementation
func RegisterMsgServer(server interface{}, impl MsgServer) {
	// This will be implemented by Cosmos SDK's gRPC server registration
	// For now, we keep it as a placeholder
}
