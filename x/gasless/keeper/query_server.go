package keeper

import (
	"context"

	"github.com/cosmos/evm/x/gasless/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type QueryServer struct {
	Keeper
}

func NewQueryServerImpl(k Keeper) *QueryServer {
	return &QueryServer{Keeper: k}
}

// Params returns the current gasless module parameters.
func (q *QueryServer) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	params := q.Keeper.GetParams(sdkCtx)
	return &types.QueryParamsResponse{Params: params}, nil
}
