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
	k.SetReplayParties(ctx, poolID, intentID, req.Certificate.Payload.Initiator, req.Certificate.Payload.Responder)

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

// SubmitMatchCertificateBatch applies certificate operations atomically.
// If any certificate fails validation/replay checks, the whole batch is rolled back.
func (k Keeper) SubmitMatchCertificateBatch(
	ctx sdk.Context,
	submitter string,
	certificates []types.MatchCertificate,
) ([]*types.MsgSubmitMatchCertificateResponse, error) {
	if strings.TrimSpace(submitter) == "" {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "submitter is required")
	}
	if len(certificates) == 0 {
		return nil, errorsmod.Wrap(types.ErrInvalidRequest, "certificates must not be empty")
	}

	cachedCtx, writeFn := ctx.CacheContext()
	responses := make([]*types.MsgSubmitMatchCertificateResponse, 0, len(certificates))
	for idx := range certificates {
		resp, err := k.SubmitMatchCertificate(cachedCtx, &types.MsgSubmitMatchCertificate{
			Submitter:   submitter,
			Certificate: certificates[idx],
		})
		if err != nil {
			return nil, errorsmod.Wrapf(types.ErrChainRejected, "batch certificate %d rejected: %v", idx, err)
		}
		responses = append(responses, resp)
	}

	writeFn()
	return responses, nil
}
