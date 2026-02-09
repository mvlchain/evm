package keeper

import (
	"context"
	"encoding/hex"
	"strings"

	"github.com/cosmos/evm/x/auction/types"

	errorsmod "cosmossdk.io/errors"

	cmttypes "github.com/cometbft/cometbft/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func (k Keeper) ConfirmAuction(goCtx context.Context, msg *types.MsgConfirmAuction) (*types.MsgConfirmAuctionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if k.HasAuction(ctx, msg.AuctionId) {
		return nil, types.ErrAuctionExists
	}
	if msg.EndHeight < ctx.BlockHeight() {
		return nil, errorsmod.Wrapf(types.ErrInvalidEndHeight, "end_height %d < current height %d", msg.EndHeight, ctx.BlockHeight())
	}

	auction := types.Auction{
		AuctionId:       msg.AuctionId,
		Seller:          strings.ToLower(msg.Seller),
		Winner:          strings.ToLower(msg.Winner),
		Price:           msg.Price,
		Denom:           msg.Denom,
		EndHeight:       msg.EndHeight,
		AskSig:          msg.AskSig,
		BidSig:          msg.BidSig,
		SellerSig:       msg.SellerSig,
		ConfirmedHeight: ctx.BlockHeight(),
	}

	if len(ctx.TxBytes()) > 0 {
		txHash := cmttypes.Tx(ctx.TxBytes()).Hash()
		auction.TxHash = hex.EncodeToString(txHash)
	}

	k.SetAuction(ctx, auction)

	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			types.EventTypeAuctionConfirmed,
			sdk.NewAttribute(types.AttributeKeyAuctionID, msg.AuctionId),
			sdk.NewAttribute(types.AttributeKeySeller, strings.ToLower(msg.Seller)),
			sdk.NewAttribute(types.AttributeKeyWinner, strings.ToLower(msg.Winner)),
			sdk.NewAttribute(types.AttributeKeyPrice, msg.Price),
		),
	})

	return &types.MsgConfirmAuctionResponse{}, nil
}
