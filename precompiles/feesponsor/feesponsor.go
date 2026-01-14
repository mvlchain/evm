package feesponsor

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	_ "embed"

	cmn "github.com/cosmos/evm/precompiles/common"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	"cosmossdk.io/math"
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

// Precompile defines the Fee Sponsor precompile contract
type Precompile struct {
	cmn.Precompile
	abi.ABI
	vmKeeper VMKeeper
}

// VMKeeper defines the expected interface for the VM keeper
type VMKeeper interface {
	CreateSponsorship(
		ctx sdk.Context,
		sponsor common.Address,
		beneficiary common.Address,
		maxGasPerTx uint64,
		totalGasBudget uint64,
		expirationHeight int64,
		conditions *evmtypes.SponsorshipConditions,
	) (string, error)

	CancelSponsorship(
		ctx sdk.Context,
		sponsorshipID string,
		caller common.Address,
	) (common.Address, uint64, error)

	GetSponsorship(
		ctx sdk.Context,
		sponsorshipID string,
	) (*evmtypes.FeeSponsor, error)

	GetSponsorshipsForBeneficiary(
		ctx sdk.Context,
		beneficiary common.Address,
	) []*evmtypes.FeeSponsor

	GetActiveSponsorshipFor(
		ctx sdk.Context,
		beneficiary common.Address,
		gasLimit uint64,
		targetContract *common.Address,
		txValue *math.Int,
	) (*evmtypes.FeeSponsor, error)
}

// NewPrecompile creates a new Fee Sponsor Precompile instance
func NewPrecompile(vmKeeper VMKeeper) *Precompile {
	return &Precompile{
		Precompile: cmn.Precompile{
			KvGasConfig:          storetypes.KVGasConfig(),
			TransientKVGasConfig: storetypes.TransientGasConfig(),
			ContractAddress:      common.HexToAddress(evmtypes.FeeSponsorPrecompileAddress),
		},
		ABI:      ABI,
		vmKeeper: vmKeeper,
	}
}

// RequiredGas returns the required gas for the precompile
func (p Precompile) RequiredGas(input []byte) uint64 {
	// Base cost for sponsorship operations
	if len(input) < 4 {
		return 0
	}

	methodID := input[:4]
	method, err := p.MethodById(methodID)
	if err != nil {
		return 0
	}

	return p.Precompile.RequiredGas(input, p.IsTransaction(method))
}

// Run executes the Fee Sponsor precompile
func (p Precompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	return p.RunNativeAction(evm, contract, func(ctx sdk.Context) ([]byte, error) {
		return p.Execute(ctx, contract, readonly)
	})
}

// Execute performs the precompile execution
func (p Precompile) Execute(ctx sdk.Context, contract *vm.Contract, readonly bool) ([]byte, error) {
	method, args, err := cmn.SetupABI(p.ABI, contract, readonly, p.IsTransaction)
	if err != nil {
		return nil, err
	}

	switch method.Name {
	case "createSponsorship":
		return p.createSponsorship(ctx, contract, args)
	case "createSponsorshipWithConditions":
		return p.createSponsorshipWithConditions(ctx, contract, args)
	case "cancelSponsorship":
		return p.cancelSponsorship(ctx, contract, args)
	case "getSponsorship":
		return p.getSponsorship(ctx, args)
	case "getSponsorshipsFor":
		return p.getSponsorshipsFor(ctx, args)
	case "isSponsored":
		return p.isSponsored(ctx, args)
	default:
		return nil, fmt.Errorf("unknown method: %s", method.Name)
	}
}

// createSponsorship creates a new sponsorship
func (p Precompile) createSponsorship(ctx sdk.Context, contract *vm.Contract, args []interface{}) ([]byte, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("invalid arguments: expected 4, got %d", len(args))
	}

	beneficiary := args[0].(common.Address)
	maxGasPerTx := args[1].(uint64)
	totalGasBudget := args[2].(uint64)
	expirationHeight := args[3].(int64)

	sponsor := contract.Caller()

	sponsorshipID, err := p.vmKeeper.CreateSponsorship(
		ctx,
		sponsor,
		beneficiary,
		maxGasPerTx,
		totalGasBudget,
		expirationHeight,
		nil, // No conditions
	)
	if err != nil {
		return nil, err
	}

	// Convert sponsorship ID to bytes32
	var sponsorshipIDBytes [32]byte
	copy(sponsorshipIDBytes[:], common.HexToHash(sponsorshipID).Bytes())

	return p.ABI.Methods["createSponsorship"].Outputs.Pack(sponsorshipIDBytes)
}

// createSponsorshipWithConditions creates a sponsorship with advanced conditions
func (p Precompile) createSponsorshipWithConditions(ctx sdk.Context, contract *vm.Contract, args []interface{}) ([]byte, error) {
	if len(args) != 7 {
		return nil, fmt.Errorf("invalid arguments: expected 7, got %d", len(args))
	}

	beneficiary := args[0].(common.Address)
	maxGasPerTx := args[1].(uint64)
	totalGasBudget := args[2].(uint64)
	expirationHeight := args[3].(int64)
	whitelistedContracts := args[4].([]common.Address)
	maxTxValue := args[5].(*math.Int)
	dailyGasLimit := args[6].(uint64)

	sponsor := contract.Caller()

	// Convert whitelisted contracts to string array
	whitelistedStrings := make([]string, len(whitelistedContracts))
	for i, addr := range whitelistedContracts {
		whitelistedStrings[i] = addr.Hex()
	}

	conditions := &evmtypes.SponsorshipConditions{
		WhitelistedContracts: whitelistedStrings,
		MaxTxValue:          *maxTxValue,
		DailyGasLimit:       dailyGasLimit,
		RequireSignature:    false,
	}

	sponsorshipID, err := p.vmKeeper.CreateSponsorship(
		ctx,
		sponsor,
		beneficiary,
		maxGasPerTx,
		totalGasBudget,
		expirationHeight,
		conditions,
	)
	if err != nil {
		return nil, err
	}

	var sponsorshipIDBytes [32]byte
	copy(sponsorshipIDBytes[:], common.HexToHash(sponsorshipID).Bytes())

	return p.ABI.Methods["createSponsorshipWithConditions"].Outputs.Pack(sponsorshipIDBytes)
}

// cancelSponsorship cancels a sponsorship
func (p Precompile) cancelSponsorship(ctx sdk.Context, contract *vm.Contract, args []interface{}) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("invalid arguments: expected 1, got %d", len(args))
	}

	sponsorshipIDBytes := args[0].([32]byte)
	sponsorshipID := common.BytesToHash(sponsorshipIDBytes[:]).Hex()

	caller := contract.Caller()

	_, refundedAmount, err := p.vmKeeper.CancelSponsorship(ctx, sponsorshipID, caller)
	if err != nil {
		return nil, err
	}

	return p.ABI.Methods["cancelSponsorship"].Outputs.Pack(refundedAmount)
}

// getSponsorship retrieves sponsorship information
func (p Precompile) getSponsorship(ctx sdk.Context, args []interface{}) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("invalid arguments: expected 1, got %d", len(args))
	}

	sponsorshipIDBytes := args[0].([32]byte)
	sponsorshipID := common.BytesToHash(sponsorshipIDBytes[:]).Hex()

	sponsorship, err := p.vmKeeper.GetSponsorship(ctx, sponsorshipID)
	if err != nil {
		return nil, err
	}

	return p.ABI.Methods["getSponsorship"].Outputs.Pack(
		common.HexToAddress(sponsorship.Sponsor),
		common.HexToAddress(sponsorship.Beneficiary),
		sponsorship.MaxGasPerTx,
		sponsorship.TotalGasBudget,
		sponsorship.ExpirationHeight,
		sponsorship.IsActive,
		sponsorship.GasUsed,
		sponsorship.TransactionCount,
	)
}

// getSponsorshipsFor retrieves all sponsorships for a beneficiary
func (p Precompile) getSponsorshipsFor(ctx sdk.Context, args []interface{}) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("invalid arguments: expected 1, got %d", len(args))
	}

	beneficiary := args[0].(common.Address)

	sponsorships := p.vmKeeper.GetSponsorshipsForBeneficiary(ctx, beneficiary)

	sponsorshipIDs := make([][32]byte, len(sponsorships))
	for i, sponsorship := range sponsorships {
		copy(sponsorshipIDs[i][:], common.HexToHash(sponsorship.SponsorshipId).Bytes())
	}

	return p.ABI.Methods["getSponsorshipsFor"].Outputs.Pack(sponsorshipIDs)
}

// isSponsored checks if a beneficiary has active sponsorship
func (p Precompile) isSponsored(ctx sdk.Context, args []interface{}) ([]byte, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("invalid arguments: expected 2, got %d", len(args))
	}

	beneficiary := args[0].(common.Address)
	gasEstimate := args[1].(uint64)

	sponsorship, err := p.vmKeeper.GetActiveSponsorshipFor(ctx, beneficiary, gasEstimate, nil, nil)
	if err != nil {
		return nil, err
	}

	if sponsorship == nil {
		var emptyID [32]byte
		return p.ABI.Methods["isSponsored"].Outputs.Pack(false, emptyID)
	}

	var sponsorshipIDBytes [32]byte
	copy(sponsorshipIDBytes[:], common.HexToHash(sponsorship.SponsorshipId).Bytes())

	return p.ABI.Methods["isSponsored"].Outputs.Pack(true, sponsorshipIDBytes)
}

// IsTransaction returns whether the method is a transaction
func (p Precompile) IsTransaction(method *abi.Method) bool {
	switch method.Name {
	case "createSponsorship", "createSponsorshipWithConditions", "cancelSponsorship":
		return true
	default:
		return false
	}
}
