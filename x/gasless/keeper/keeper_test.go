package keeper_test

import (
	"testing"

	tmproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	"cosmossdk.io/store"
	storetypes "cosmossdk.io/store/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/evm/x/gasless/keeper"
	"github.com/cosmos/evm/x/gasless/types"
)

// mockBankKeeper for testing
type mockBankKeeper struct{}

func (m mockBankKeeper) SendCoinsFromAccountToModule(ctx sdk.Context, sender sdk.AccAddress, recipientModule string, amt sdk.Coins) error {
	return nil
}

func (m mockBankKeeper) SendCoinsFromModuleToAccount(ctx sdk.Context, senderModule string, recipient sdk.AccAddress, amt sdk.Coins) error {
	return nil
}

// mockAccountKeeper for testing
type mockAccountKeeper struct{}

func (m mockAccountKeeper) GetModuleAddress(moduleName string) sdk.AccAddress {
	return sdk.AccAddress{}
}

// mockEVMKeeper for testing
type mockEVMKeeper struct{}

func setupKeeper(t *testing.T) (keeper.Keeper, sdk.Context) {
	storeKey := storetypes.NewKVStoreKey(types.StoreKey)

	db := dbm.NewMemDB()
	stateStore := store.NewCommitMultiStore(db, log.NewNopLogger(), nil)
	stateStore.MountStoreWithDB(storeKey, storetypes.StoreTypeIAVL, db)
	require.NoError(t, stateStore.LoadLatestVersion())

	registry := codectypes.NewInterfaceRegistry()
	cdc := codec.NewProtoCodec(registry)

	ctx := sdk.NewContext(stateStore, tmproto.Header{ChainID: "test-chain"}, false, log.NewNopLogger())

	k := keeper.NewKeeper(
		cdc,
		storeKey,
		mockBankKeeper{},
		mockAccountKeeper{},
		mockEVMKeeper{},
	)

	return k, ctx
}

func TestKeeper_GetSetParams(t *testing.T) {
	k, ctx := setupKeeper(t)

	// First set some params to initialize store
	defaultParams := types.DefaultParams()
	err := k.SetParams(ctx, defaultParams)
	require.NoError(t, err)

	// Set custom params
	params := types.Params{
		Enabled:            true,
		AllowedContracts:   []string{"0xAa00000000000000000000000000000000000000"},
		DefaultSponsor:     "cosmos1test123",
		MaxGasPerTx:        1000000,
		MaxSubsidyPerBlock: math.NewInt(5000000),
	}

	err = k.SetParams(ctx, params)
	require.NoError(t, err)

	// Get params
	retrieved := k.GetParams(ctx)
	require.Equal(t, params.Enabled, retrieved.Enabled)
	require.Equal(t, params.AllowedContracts, retrieved.AllowedContracts)
	require.Equal(t, params.DefaultSponsor, retrieved.DefaultSponsor)
	require.Equal(t, params.MaxGasPerTx, retrieved.MaxGasPerTx)
	require.True(t, params.MaxSubsidyPerBlock.Equal(retrieved.MaxSubsidyPerBlock))
}

func TestKeeper_ValidateGasLimit(t *testing.T) {
	k, ctx := setupKeeper(t)

	// Set params with max gas 500000
	params := types.DefaultParams()
	params.MaxGasPerTx = 500000
	err := k.SetParams(ctx, params)
	require.NoError(t, err)

	tests := []struct {
		name      string
		gas       uint64
		expectErr bool
	}{
		{"within limit", 100000, false},
		{"at limit", 500000, false},
		{"exceeds limit", 600000, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := k.ValidateGasLimit(ctx, tt.gas)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestKeeper_IsGaslessAllowed(t *testing.T) {
	k, ctx := setupKeeper(t)

	// Setup params
	params := types.Params{
		Enabled: true,
		AllowedContracts: []string{
			"0xAa00000000000000000000000000000000000000",
			"0xBb11111111111111111111111111111111111111",
		},
		DefaultSponsor:     "cosmos1sponsor123",
		MaxGasPerTx:        500000,
		MaxSubsidyPerBlock: math.NewInt(0),
	}
	err := k.SetParams(ctx, params)
	require.NoError(t, err)

	tests := []struct {
		name          string
		toAddress     string
		expectAllowed bool
	}{
		{"allowed address 1", "0xAa00000000000000000000000000000000000000", true},
		{"allowed address 2", "0xBb11111111111111111111111111111111111111", true},
		{"not allowed", "0xCc22222222222222222222222222222222222222", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, sponsor, err := k.IsGaslessAllowed(ctx, tt.toAddress)
			require.NoError(t, err)
			require.Equal(t, tt.expectAllowed, allowed)
			if tt.expectAllowed {
				require.NotNil(t, sponsor)
			}
		})
	}
}

func TestKeeper_IsGaslessAllowed_Disabled(t *testing.T) {
	k, ctx := setupKeeper(t)

	// Setup params with disabled
	params := types.DefaultParams()
	params.Enabled = false
	params.AllowedContracts = []string{"0xAa00000000000000000000000000000000000000"}
	err := k.SetParams(ctx, params)
	require.NoError(t, err)

	allowed, _, err := k.IsGaslessAllowed(ctx, "0xAa00000000000000000000000000000000000000")
	require.NoError(t, err)
	require.False(t, allowed, "gasless should be disabled")
}
