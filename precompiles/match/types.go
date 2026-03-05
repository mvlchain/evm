package match

import (
	"fmt"
	"strings"

	cmn "github.com/cosmos/evm/precompiles/common"
)

// ReplayArgs represents method inputs that identify a replay index entry.
type ReplayArgs struct {
	PoolID   string
	IntentID string
}

// ParseReplayArgs parses common replay query arguments.
func ParseReplayArgs(args []interface{}) (ReplayArgs, error) {
	if len(args) != 2 {
		return ReplayArgs{}, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 2, len(args))
	}

	poolID, ok := args[0].(string)
	if !ok {
		return ReplayArgs{}, fmt.Errorf(cmn.ErrInvalidType, "poolId", "", args[0])
	}
	if strings.TrimSpace(poolID) == "" {
		return ReplayArgs{}, fmt.Errorf("poolId cannot be empty")
	}

	intentID, ok := args[1].(string)
	if !ok {
		return ReplayArgs{}, fmt.Errorf(cmn.ErrInvalidType, "intentId", "", args[1])
	}
	if strings.TrimSpace(intentID) == "" {
		return ReplayArgs{}, fmt.Errorf("intentId cannot be empty")
	}

	return ReplayArgs{
		PoolID:   poolID,
		IntentID: intentID,
	}, nil
}
