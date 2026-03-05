package match

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	_ "embed"

	cmn "github.com/cosmos/evm/precompiles/common"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	// GasHasReplay defines the gas cost for a replay existence query.
	GasHasReplay = 2_000
	// GasGetReplay defines the gas cost for a replay lookup query.
	GasGetReplay = 2_500
	// GasGetReplayParties defines the gas cost for replay requester/responder lookups.
	GasGetReplayParties = 3_000
)

var _ vm.PrecompiledContract = &Precompile{}

var (
	// Embed abi json file to the executable binary. Needed when importing as dependency.
	//
	//go:embed abi.json
	f   []byte
	ABI abi.ABI
)

func init() {
	var err error
	ABI, err = abi.JSON(bytes.NewReader(f))
	if err != nil {
		panic(err)
	}
}

// Precompile defines the match precompile.
type Precompile struct {
	cmn.Precompile

	abi.ABI
	matchKeeper cmn.MatchKeeper
}

// NewPrecompile creates a new match precompile instance.
func NewPrecompile(matchKeeper cmn.MatchKeeper) *Precompile {
	return &Precompile{
		Precompile: cmn.Precompile{
			// Keep zero-cost KV gas config to avoid double charging and rely on EVM gas schedule.
			KvGasConfig:          storetypes.GasConfig{},
			TransientKVGasConfig: storetypes.GasConfig{},
			ContractAddress:      common.HexToAddress(evmtypes.MatchPrecompileAddress),
		},
		ABI:         ABI,
		matchKeeper: matchKeeper,
	}
}

// RequiredGas calculates the precompiled contract's base gas rate.
func (p Precompile) RequiredGas(input []byte) uint64 {
	// NOTE: this check avoids panics when trying to decode the method ID.
	if len(input) < 4 {
		return 0
	}

	methodID := input[:4]
	method, err := p.MethodById(methodID)
	if err != nil {
		// This should never happen since this method is going to fail during Run.
		return 0
	}

	switch method.Name {
	case HasReplayMethod:
		return GasHasReplay
	case GetReplayMethod:
		return GasGetReplay
	case GetReplayPartiesMethod:
		return GasGetReplayParties
	default:
		return 0
	}
}

func (p Precompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	return p.RunNativeAction(evm, contract, func(ctx sdk.Context) ([]byte, error) {
		return p.Execute(ctx, contract, readonly)
	})
}

// Execute executes the precompiled contract match query methods defined in the ABI.
func (p Precompile) Execute(ctx sdk.Context, contract *vm.Contract, readOnly bool) ([]byte, error) {
	method, args, err := cmn.SetupABI(p.ABI, contract, readOnly, p.IsTransaction)
	if err != nil {
		return nil, err
	}

	switch method.Name {
	case HasReplayMethod:
		return p.HasReplay(ctx, method, args)
	case GetReplayMethod:
		return p.GetReplay(ctx, method, args)
	case GetReplayPartiesMethod:
		return p.GetReplayParties(ctx, method, args)
	default:
		return nil, fmt.Errorf(cmn.ErrUnknownMethod, method.Name)
	}
}

// IsTransaction checks if the given method name corresponds to a transaction or query.
// All currently exposed match precompile methods are queries.
func (Precompile) IsTransaction(_ *abi.Method) bool {
	return false
}
