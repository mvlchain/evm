package types

import (
	"encoding/json"
	"fmt"
	"strconv"

	"cosmossdk.io/math"
)

type Params struct {
	Enabled            bool       `json:"enabled" yaml:"enabled"`
	AllowedContracts   []string   `json:"allowed_contracts" yaml:"allowed_contracts"`
	DefaultSponsor     string     `json:"default_sponsor" yaml:"default_sponsor"`
	MaxGasPerTx        uint64     `json:"max_gas_per_tx" yaml:"max_gas_per_tx"`
	MaxSubsidyPerBlock math.Int   `json:"max_subsidy_per_block" yaml:"max_subsidy_per_block"`
}

func DefaultParams() Params {
	return Params{
		Enabled:            false,
		AllowedContracts:   nil,
		DefaultSponsor:     "",
		MaxGasPerTx:        500_000,
		MaxSubsidyPerBlock: math.NewInt(0),
	}
}

func (p Params) Validate() error {
	if p.MaxGasPerTx == 0 {
		return fmt.Errorf("max_gas_per_tx must be > 0")
	}
	// TODO: validate AllowedContracts as hex addresses and DefaultSponsor as bech32
	return nil
}

// UnmarshalJSON implements json.Unmarshaler to handle string-encoded uint64
func (p *Params) UnmarshalJSON(data []byte) error {
	type Alias Params
	aux := &struct {
		MaxGasPerTx interface{} `json:"max_gas_per_tx"`
		*Alias
	}{
		Alias: (*Alias)(p),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Handle MaxGasPerTx as either string or number
	switch v := aux.MaxGasPerTx.(type) {
	case string:
		val, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			return fmt.Errorf("invalid max_gas_per_tx: %w", err)
		}
		p.MaxGasPerTx = val
	case float64:
		p.MaxGasPerTx = uint64(v)
	case nil:
		p.MaxGasPerTx = 0
	default:
		return fmt.Errorf("invalid max_gas_per_tx type: %T", v)
	}

	return nil
}
