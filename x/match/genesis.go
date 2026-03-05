package match

import (
	"fmt"

	"github.com/cosmos/evm/x/match/keeper"
	"github.com/cosmos/evm/x/match/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// InitGenesis initializes x/match state from genesis.
func InitGenesis(ctx sdk.Context, k keeper.Keeper, data types.GenesisState) {
	for _, replay := range data.Replays {
		k.SetReplay(ctx, replay.PoolId, replay.IntentId, replay.MatchId)
		if replay.Requester != "" && replay.Responder != "" {
			k.SetReplayParties(ctx, replay.PoolId, replay.IntentId, replay.Requester, replay.Responder)
		}
	}
}

// ExportGenesis exports x/match state to genesis.
func ExportGenesis(ctx sdk.Context, k keeper.Keeper) *types.GenesisState {
	replays, err := k.GetAllReplayEntries(ctx)
	if err != nil {
		panic(fmt.Errorf("failed to export %s replay index: %w", types.ModuleName, err))
	}

	return &types.GenesisState{
		Replays: replays,
	}
}
