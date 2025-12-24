package keeper

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmos/evm/x/gasless/types"

	"github.com/cosmos/cosmos-sdk/codec"
	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// toLowerHex normalizes hex addresses to lowercase for case-insensitive comparison
func toLowerHex(addr string) string {
	return strings.ToLower(addr)
}

type Keeper struct {
	cdc      codec.JSONCodec
	storeKey storetypes.StoreKey

	bankKeeper    types.BankKeeper
	accountKeeper types.AccountKeeper
	evmKeeper     types.EVMKeeper
}

func NewKeeper(
	cdc codec.JSONCodec,
	storeKey storetypes.StoreKey,
	bankKeeper types.BankKeeper,
	accountKeeper types.AccountKeeper,
	evmKeeper types.EVMKeeper,
) Keeper {
	return Keeper{
		cdc:          cdc,
		storeKey:     storeKey,
		bankKeeper:   bankKeeper,
		accountKeeper: accountKeeper,
		evmKeeper:    evmKeeper,
	}
}

// Params

func (k Keeper) GetParams(ctx sdk.Context) types.Params {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get([]byte("params"))
	if bz == nil {
		return types.DefaultParams()
	}
	var params types.Params
	if err := json.Unmarshal(bz, &params); err != nil {
		return types.DefaultParams()
	}
	return params
}

func (k Keeper) SetParams(ctx sdk.Context, params types.Params) error {
	if err := params.Validate(); err != nil {
		return err
	}
	store := ctx.KVStore(k.storeKey)
	bz, err := json.Marshal(&params)
	if err != nil {
		return err
	}
	store.Set([]byte("params"), bz)
	return nil
}

// Policy helpers

// IsGaslessAllowed returns (allowed, sponsorAddr, error) for a given EVM "to" address in hex.
func (k Keeper) IsGaslessAllowed(ctx sdk.Context, ethTo string) (bool, sdk.AccAddress, error) {
	params := k.GetParams(ctx)
	if !params.Enabled {
		return false, nil, nil
	}

	allowed := false
	// Normalize addresses to lowercase for case-insensitive comparison (EIP-55)
	ethToLower := toLowerHex(ethTo)
	for _, c := range params.AllowedContracts {
		if toLowerHex(c) == ethToLower {
			allowed = true
			break
		}
	}
	if !allowed {
		return false, nil, nil
	}

	if params.DefaultSponsor == "" {
		return false, nil, nil
	}

	sponsor, err := sdk.AccAddressFromBech32(params.DefaultSponsor)
	if err != nil {
		return false, nil, err
	}

	return true, sponsor, nil
}

// ValidateGasLimit checks if the gas limit is within the allowed range for gasless txs.
func (k Keeper) ValidateGasLimit(ctx sdk.Context, gas uint64) error {
	params := k.GetParams(ctx)
	if gas > params.MaxGasPerTx {
		return fmt.Errorf("gasless tx exceeds max gas limit: %d > %d", gas, params.MaxGasPerTx)
	}
	return nil
}

// CheckBlockSubsidyLimit checks if adding a new fee would exceed the per-block subsidy limit.
// Returns error if limit would be exceeded.
func (k Keeper) CheckBlockSubsidyLimit(ctx sdk.Context, newFee sdk.Coins) error {
	params := k.GetParams(ctx)
	if params.MaxSubsidyPerBlock.IsZero() {
		// No limit configured
		return nil
	}

	// Track subsidy used in current block
	store := ctx.KVStore(k.storeKey)
	blockHeight := ctx.BlockHeight()
	key := []byte(fmt.Sprintf("subsidy/%d", blockHeight))

	bz := store.Get(key)
	var currentSubsidy sdk.Coins
	if bz != nil {
		if err := json.Unmarshal(bz, &currentSubsidy); err != nil {
			currentSubsidy = sdk.NewCoins()
		}
	} else {
		currentSubsidy = sdk.NewCoins()
	}

	// Add new fee to current subsidy
	totalSubsidy := currentSubsidy.Add(newFee...)

	// Check if total exceeds limit (assuming single denom for simplicity)
	totalAmount := totalSubsidy.AmountOf(newFee[0].Denom)
	if totalAmount.GT(params.MaxSubsidyPerBlock) {
		return fmt.Errorf("gasless subsidy limit exceeded for block %d: %s > %s",
			blockHeight, totalAmount.String(), params.MaxSubsidyPerBlock.String())
	}

	// Update stored subsidy for this block
	updatedBz, err := json.Marshal(&totalSubsidy)
	if err != nil {
		return err
	}
	store.Set(key, updatedBz)

	return nil
}

// ChargeSponsor charges the sponsor account and moves coins into the gasless module account.
func (k Keeper) ChargeSponsor(ctx sdk.Context, sponsor sdk.AccAddress, fee sdk.Coins) error {
	return k.bankKeeper.SendCoinsFromAccountToModule(ctx, sponsor, types.ModuleName, fee)
}
