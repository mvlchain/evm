package ridehail

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

var _ vm.PrecompiledContract = &Precompile{}

var (
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

// Precompile implements the RideHail precompiled contract.
type Precompile struct {
	cmn.Precompile
	abi.ABI
	baseGas        uint64
	rideHailKeeper RideHailKeeper
}

func NewPrecompile(baseGas uint64, keeper RideHailKeeper) (*Precompile, error) {
	if baseGas == 0 {
		return nil, fmt.Errorf("baseGas cannot be zero")
	}
	return &Precompile{
		Precompile: cmn.Precompile{
			KvGasConfig:          storetypes.GasConfig{},
			TransientKVGasConfig: storetypes.GasConfig{},
			ContractAddress:      common.HexToAddress(evmtypes.RideHailPrecompileAddress),
		},
		ABI:            ABI,
		baseGas:        baseGas,
		rideHailKeeper: keeper,
	}, nil
}

func (Precompile) Address() common.Address {
	return common.HexToAddress(evmtypes.RideHailPrecompileAddress)
}

func (p Precompile) RequiredGas(_ []byte) uint64 {
	return p.baseGas
}

func (p Precompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	fmt.Printf("[RideHail] ========== Run() called ==========\n")
	fmt.Printf("[RideHail] Caller: %s, Value: %s, Input length: %d, ReadOnly: %t\n",
		contract.Caller().Hex(), contract.Value().ToBig().String(), len(contract.Input), readonly)

	result, err := p.RunNativeAction(evm, contract, func(ctx sdk.Context) ([]byte, error) {
		return p.Execute(ctx, evm, contract, readonly)
	})

	if err != nil {
		fmt.Printf("[RideHail] ERROR in Run(): %v\n", err)
	} else {
		fmt.Printf("[RideHail] Run() success, result length: %d\n", len(result))
	}

	return result, err
}

func (p Precompile) Execute(ctx sdk.Context, evm *vm.EVM, contract *vm.Contract, readOnly bool) (bz []byte, err error) {
	if len(contract.Input) < 4 {
		return nil, vm.ErrExecutionReverted
	}
	methodID := contract.Input[:4]
	method, err := p.MethodById(methodID)
	if err != nil {
		return nil, err
	}
	argsBz := contract.Input[4:]
	args, err := method.Inputs.Unpack(argsBz)
	if err != nil {
		return nil, err
	}

	switch method.Name {
	case VersionMethod:
		bz, err = p.Version(method)
	case ValidateCreateRequestMethod:
		bz, err = p.ValidateCreateRequest(method, ctx, evm, contract, args)
	case NextRequestIdMethod:
		bz, err = p.NextRequestId(method, ctx)
	case NextSessionIdMethod:
		bz, err = p.NextSessionId(method, ctx)
	case CreateRequestMethod:
		bz, err = p.CreateRequest(method, ctx, evm, contract, args)
	case AcceptCommitMethod:
		bz, err = p.AcceptCommit(method, ctx, evm, contract, args)
	case AcceptRevealMethod:
		bz, err = p.AcceptReveal(method, ctx, evm, contract, args)
	case RequestsMethod:
		bz, err = p.Requests(method, ctx, evm, args)
	case PostEncryptedMessageMethod:
		bz, err = p.PostEncryptedMessage(method, ctx, evm, contract, args)
	default:
		return nil, vm.ErrExecutionReverted
	}

	if err != nil {
		return nil, err
	}
	return bz, nil
}
