package keeper

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/cosmos/evm/x/match/types"

	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// MsgServer is the message server implementation for x/match.
type MsgServer struct {
	Keeper
}

// NewMsgServerImpl creates a new MsgServer.
func NewMsgServerImpl(k Keeper) MsgServer {
	return MsgServer{Keeper: k}
}

// SubmitMatchCertificate executes certificate validation, replay protection and event emission.
func (ms MsgServer) SubmitMatchCertificate(
	goCtx context.Context,
	req *types.MsgSubmitMatchCertificate,
) (*types.MsgSubmitMatchCertificateResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)
	return ms.Keeper.SubmitMatchCertificate(ctx, req)
}

// SubmitMatchCertificate validates and records a match certificate.
func (k Keeper) SubmitMatchCertificate(
	ctx sdk.Context,
	req *types.MsgSubmitMatchCertificate,
) (*types.MsgSubmitMatchCertificateResponse, error) {
	if req == nil {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "request is required")
	}

	nowUnix := ctx.BlockTime().Unix()
	if err := req.ValidateForSubmission(nowUnix); err != nil {
		return nil, err
	}

	poolID := req.Certificate.Payload.PoolId
	intentID := req.Certificate.Payload.IntentId
	replayKey := types.ReplayKeyString(poolID, intentID)
	if k.HasReplay(ctx, poolID, intentID) {
		return nil, errorsmod.Wrapf(types.ErrReplayDetected, "replay key already exists: %s", replayKey)
	}

	certificateHash := req.Certificate.CertificateHash()
	matchID := req.Certificate.MatchID()
	if strings.TrimSpace(matchID) == "" {
		matchID = types.BuildMatchID(poolID, intentID, hex.EncodeToString(certificateHash))
	}
	k.SetReplay(ctx, poolID, intentID, matchID)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			types.EventTypeSubmitMatchCertificate,
			sdk.NewAttribute(types.AttributeKeySubmitter, req.Submitter),
			sdk.NewAttribute(types.AttributeKeyPoolID, poolID),
			sdk.NewAttribute(types.AttributeKeyIntentID, intentID),
			sdk.NewAttribute(types.AttributeKeyResponseID, req.Certificate.Payload.ResponseId),
			sdk.NewAttribute(types.AttributeKeyFinalizeID, req.Certificate.Payload.FinalizeId),
			sdk.NewAttribute(types.AttributeKeyCertificateHash, hex.EncodeToString(certificateHash)),
			sdk.NewAttribute(types.AttributeKeyReplayKey, replayKey),
			sdk.NewAttribute(types.AttributeKeyMatchID, matchID),
		),
	)

	return &types.MsgSubmitMatchCertificateResponse{
		MatchId:         matchID,
		CertificateHash: certificateHash,
		ReplayKey:       replayKey,
	}, nil
}
