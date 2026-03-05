package types

import (
	"fmt"
	"strings"
)

// ReplayIndexEntry stores replay index data for genesis import/export.
type ReplayIndexEntry struct {
	PoolId   string `json:"pool_id" yaml:"pool_id"`
	IntentId string `json:"intent_id" yaml:"intent_id"`
	MatchId  string `json:"match_id" yaml:"match_id"`
}

// GenesisState defines x/match genesis state.
type GenesisState struct {
	Replays []ReplayIndexEntry `json:"replays" yaml:"replays"`
}

// DefaultGenesisState returns the default x/match genesis state.
func DefaultGenesisState() *GenesisState {
	return &GenesisState{
		Replays: []ReplayIndexEntry{},
	}
}

// Validate checks whether the provided x/match genesis state is valid.
func (gs GenesisState) Validate() error {
	seen := make(map[string]struct{}, len(gs.Replays))

	for i, replay := range gs.Replays {
		if strings.TrimSpace(replay.PoolId) == "" {
			return fmt.Errorf("replays[%d].pool_id must not be empty", i)
		}
		if strings.TrimSpace(replay.IntentId) == "" {
			return fmt.Errorf("replays[%d].intent_id must not be empty", i)
		}
		if strings.TrimSpace(replay.MatchId) == "" {
			return fmt.Errorf("replays[%d].match_id must not be empty", i)
		}

		key := ReplayKeyString(replay.PoolId, replay.IntentId)
		if _, ok := seen[key]; ok {
			return fmt.Errorf("replays[%d] duplicate replay key: %s", i, key)
		}
		seen[key] = struct{}{}
	}

	return nil
}
