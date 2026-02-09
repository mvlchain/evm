package auction

import (
	"github.com/cosmos/evm/x/auction/keeper"
	"github.com/cosmos/evm/x/auction/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func InitGenesis(ctx sdk.Context, k keeper.Keeper, genState types.GenesisState) {
	for _, a := range genState.Auctions {
		k.SetAuction(ctx, a)
	}
}

func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	return &types.GenesisState{
		Auctions: k.GetAllAuctions(ctx),
	}
}
