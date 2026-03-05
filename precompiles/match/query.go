package match

import (
	"github.com/ethereum/go-ethereum/accounts/abi"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	// HasReplayMethod defines the ABI method name for replay existence queries.
	HasReplayMethod = "hasReplay"
	// GetReplayMethod defines the ABI method name for replay match_id queries.
	GetReplayMethod = "getReplay"
	// GetReplayPartiesMethod defines the ABI method name for replay requester/responder queries.
	GetReplayPartiesMethod = "getReplayParties"
)

// HasReplay returns whether replay index data exists for (pool_id, intent_id).
func (p Precompile) HasReplay(
	ctx sdk.Context,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	req, err := ParseReplayArgs(args)
	if err != nil {
		return nil, err
	}

	exists := p.matchKeeper.HasReplay(ctx, req.PoolID, req.IntentID)
	return method.Outputs.Pack(exists)
}

// GetReplay returns replay index data for (pool_id, intent_id).
func (p Precompile) GetReplay(
	ctx sdk.Context,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	req, err := ParseReplayArgs(args)
	if err != nil {
		return nil, err
	}

	matchID, found := p.matchKeeper.GetReplayMatchID(ctx, req.PoolID, req.IntentID)
	if !found {
		return method.Outputs.Pack(false, "")
	}

	return method.Outputs.Pack(true, matchID)
}

// GetReplayParties returns replay index data and requester/responder metadata for (pool_id, intent_id).
func (p Precompile) GetReplayParties(
	ctx sdk.Context,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	req, err := ParseReplayArgs(args)
	if err != nil {
		return nil, err
	}

	matchID, found := p.matchKeeper.GetReplayMatchID(ctx, req.PoolID, req.IntentID)
	if !found {
		return method.Outputs.Pack(false, "", "", "")
	}

	requester, responder, partiesFound := p.matchKeeper.GetReplayParties(ctx, req.PoolID, req.IntentID)
	if !partiesFound {
		return method.Outputs.Pack(true, matchID, "", "")
	}

	return method.Outputs.Pack(true, matchID, requester, responder)
}
