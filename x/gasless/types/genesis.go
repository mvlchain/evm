package types

import (
	"encoding/json"
)

// GenesisState defines the gasless module genesis state.
type GenesisState struct {
	Params Params `json:"params" yaml:"params"`
}

func DefaultGenesisState() *GenesisState {
	return &GenesisState{
		Params: DefaultParams(),
	}
}

func (gs GenesisState) Validate() error {
	return gs.Params.Validate()
}

// MarshalJSON implements json.Marshaler
func (gs GenesisState) MarshalJSON() ([]byte, error) {
	type Alias GenesisState
	return json.Marshal(&struct{ *Alias }{Alias: (*Alias)(&gs)})
}

// UnmarshalJSON implements json.Unmarshaler
func (gs *GenesisState) UnmarshalJSON(data []byte) error {
	type Alias GenesisState
	aux := &struct{ *Alias }{Alias: (*Alias)(gs)}
	return json.Unmarshal(data, &aux)
}
