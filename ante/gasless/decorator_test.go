package gasless

import (
	"context"
	"fmt"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	evmtypes "github.com/cosmos/evm/x/vm/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/protobuf/proto"
)

// mockGaslessKeeper is a simple in-memory implementation of GaslessKeeperI
// used to test the decorator logic.
type mockGaslessKeeper struct {
	allowed       bool
	returnSponsor sdk.AccAddress
	isAllowedErr  error

	chargedSponsor sdk.AccAddress
	chargedFee     sdk.Coins

	// Optional function overrides for testing
	validateGasLimitFn      func(ctx sdk.Context, gas uint64) error
	checkBlockSubsidyFn     func(ctx sdk.Context, newFee sdk.Coins) error
}

func (m *mockGaslessKeeper) IsGaslessAllowed(ctx sdk.Context, ethTo string) (bool, sdk.AccAddress, error) {
	return m.allowed, m.returnSponsor, m.isAllowedErr
}

func (m *mockGaslessKeeper) ChargeSponsor(ctx sdk.Context, sponsor sdk.AccAddress, fee sdk.Coins) error {
	m.chargedSponsor = sponsor
	m.chargedFee = fee
	return nil
}

func (m *mockGaslessKeeper) ValidateGasLimit(ctx sdk.Context, gas uint64) error {
	if m.validateGasLimitFn != nil {
		return m.validateGasLimitFn(ctx, gas)
	}
	// Default: reject gas > 1_000_000 in tests
	if gas > 1_000_000 {
		return fmt.Errorf("gas limit too high: %d", gas)
	}
	return nil
}

func (m *mockGaslessKeeper) CheckBlockSubsidyLimit(ctx sdk.Context, newFee sdk.Coins) error {
	if m.checkBlockSubsidyFn != nil {
		return m.checkBlockSubsidyFn(ctx, newFee)
	}
	// Default: no limit check
	return nil
}

// testTx is a minimal sdk.Tx implementation used for testing.
type testTx struct {
	msgs []sdk.Msg
}

func (t testTx) GetMsgs() []sdk.Msg { return t.msgs }
func (t testTx) GetMsgsV2() ([]proto.Message, error) {
	msgs := make([]proto.Message, len(t.msgs))
	for i, m := range t.msgs {
		if pm, ok := m.(proto.Message); ok {
			msgs[i] = pm
		}
	}
	return msgs, nil
}
func (t testTx) ValidateBasic() error { return nil }

// newEthMsgTx builds a simple MsgEthereumTx wrapped in an sdk.Tx for testing.
func newEthMsgTx(to common.Address, gas uint64, gasPrice *big.Int) sdk.Tx {
	// Build a simple legacy Ethereum transaction
	legacy := &ethtypes.LegacyTx{
		Nonce:    0,
		GasPrice: gasPrice,
		Gas:      gas,
		To:       &to,
		Value:    big.NewInt(0),
		Data:     nil,
	}
	tx := ethtypes.NewTx(legacy)

	var msg evmtypes.MsgEthereumTx
	msg.FromEthereumTx(tx)
	// From 필드는 decorator에서 안 쓰므로 비워둬도 무방

	return testTx{msgs: []sdk.Msg{&msg}}
}

func TestGaslessDecorator_ChargesSponsorWhenAllowed(t *testing.T) {
	// Initialize EVM coin info for testing
	evmtypes.SetDefaultEvmCoinInfo(evmtypes.EvmCoinInfo{
		Denom:         "aevmos",
		Decimals:      18,
		ExtendedDenom: "aevmos",
		DisplayDenom:  "evmos",
	})

	to := common.HexToAddress("0xAa00000000000000000000000000000000000000")
	gas := uint64(21_000)
	gasPrice := big.NewInt(1_000_000_000) // 1 gwei

	tx := newEthMsgTx(to, gas, gasPrice)

	// Prepare mock keeper: gasless allowed with a given sponsor address
	sponsor := sdk.AccAddress("sponsor-address-1234567890")
	mk := &mockGaslessKeeper{
		allowed:       true,
		returnSponsor: sponsor,
	}

	dec := NewGaslessDecorator(mk)

	// Minimal context; we don't need a real store because mock keeper ignores it
	ctx := sdk.Context{}.WithContext(context.Background())

	next := func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		return ctx, nil
	}

	newCtx, err := dec.AnteHandle(ctx, tx, false, next)
	if err != nil {
		t.Fatalf("AnteHandle returned error: %v", err)
	}

	if !mk.chargedFee.IsAllPositive() {
		t.Fatalf("expected sponsor to be charged a positive fee, got: %s", mk.chargedFee)
	}
	if !mk.chargedSponsor.Equals(sponsor) {
		t.Fatalf("expected sponsor %s to be charged, got %s", sponsor.String(), mk.chargedSponsor.String())
	}

	// Ensure GaslessInfo is set in context
	info, ok := GetGaslessInfo(newCtx)
	if !ok || !info.Enabled {
		t.Fatalf("expected GaslessInfo to be enabled in context")
	}
}

func TestGaslessDecorator_NoopWhenNotAllowed(t *testing.T) {
	// Initialize EVM coin info for testing
	evmtypes.SetDefaultEvmCoinInfo(evmtypes.EvmCoinInfo{
		Denom:         "aevmos",
		Decimals:      18,
		ExtendedDenom: "aevmos",
		DisplayDenom:  "evmos",
	})

	to := common.HexToAddress("0xBb00000000000000000000000000000000000000")
	gas := uint64(21_000)
	gasPrice := big.NewInt(1_000_000_000)

	tx := newEthMsgTx(to, gas, gasPrice)

	mk := &mockGaslessKeeper{allowed: false}
	dec := NewGaslessDecorator(mk)

	ctx := sdk.Context{}.WithContext(context.Background())

	var nextCalled bool
	next := func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		nextCalled = true
		return ctx, nil
	}

	newCtx, err := dec.AnteHandle(ctx, tx, false, next)
	if err != nil {
		t.Fatalf("AnteHandle returned error: %v", err)
	}
	if !nextCalled {
		t.Fatalf("expected next ante handler to be called")
	}

	if !mk.chargedFee.IsZero() {
		t.Fatalf("expected no fee to be charged, got: %s", mk.chargedFee)
	}

	if _, ok := GetGaslessInfo(newCtx); ok {
		t.Fatalf("expected no GaslessInfo in context when not allowed")
	}
}

func TestGaslessDecorator_ExceedsGasLimit(t *testing.T) {
	// Initialize EVM coin info for testing
	evmtypes.SetDefaultEvmCoinInfo(evmtypes.EvmCoinInfo{
		Denom:         "aevmos",
		Decimals:      18,
		ExtendedDenom: "aevmos",
		DisplayDenom:  "evmos",
	})

	to := common.HexToAddress("0xCc00000000000000000000000000000000000000")
	gas := uint64(2_000_000) // Exceeds mock limit of 1_000_000
	gasPrice := big.NewInt(1_000_000_000)

	tx := newEthMsgTx(to, gas, gasPrice)

	sponsor := sdk.AccAddress("sponsor-address-1234567890")
	mk := &mockGaslessKeeper{
		allowed:       true,
		returnSponsor: sponsor,
	}

	dec := NewGaslessDecorator(mk)
	ctx := sdk.Context{}.WithContext(context.Background())

	next := func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		t.Fatal("next handler should not be called when gas limit exceeded")
		return ctx, nil
	}

	_, err := dec.AnteHandle(ctx, tx, false, next)
	if err == nil {
		t.Fatalf("expected error for gas limit exceeded, got nil")
	}

	if mk.chargedFee.IsAllPositive() {
		t.Fatalf("expected no fee to be charged when gas limit exceeded, got: %s", mk.chargedFee)
	}
}

func TestGaslessDecorator_ExceedsBlockSubsidyLimit(t *testing.T) {
	// Initialize EVM coin info for testing
	evmtypes.SetDefaultEvmCoinInfo(evmtypes.EvmCoinInfo{
		Denom:         "aevmos",
		Decimals:      18,
		ExtendedDenom: "aevmos",
		DisplayDenom:  "evmos",
	})

	to := common.HexToAddress("0xDd00000000000000000000000000000000000000")
	gas := uint64(100_000)
	gasPrice := big.NewInt(1_000_000_000)

	tx := newEthMsgTx(to, gas, gasPrice)

	sponsor := sdk.AccAddress("sponsor-address-1234567890")
	mk := &mockGaslessKeeper{
		allowed:       true,
		returnSponsor: sponsor,
		checkBlockSubsidyFn: func(ctx sdk.Context, newFee sdk.Coins) error {
			return fmt.Errorf("block subsidy limit exceeded")
		},
	}

	dec := NewGaslessDecorator(mk)
	ctx := sdk.Context{}.WithContext(context.Background())

	next := func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		t.Fatal("next handler should not be called when block subsidy limit exceeded")
		return ctx, nil
	}

	_, err := dec.AnteHandle(ctx, tx, false, next)
	if err == nil {
		t.Fatalf("expected error for block subsidy limit exceeded, got nil")
	}

	if mk.chargedFee.IsAllPositive() {
		t.Fatalf("expected no fee to be charged when subsidy limit exceeded, got: %s", mk.chargedFee)
	}
}

func TestGaslessDecorator_ZeroGasPrice(t *testing.T) {
	// Initialize EVM coin info for testing
	evmtypes.SetDefaultEvmCoinInfo(evmtypes.EvmCoinInfo{
		Denom:         "aevmos",
		Decimals:      18,
		ExtendedDenom: "aevmos",
		DisplayDenom:  "evmos",
	})

	to := common.HexToAddress("0xAa00000000000000000000000000000000000000")
	gas := uint64(21_000)
	gasPrice := big.NewInt(0) // Zero gas price

	tx := newEthMsgTx(to, gas, gasPrice)

	sponsor := sdk.AccAddress("sponsor-address-1234567890")
	mk := &mockGaslessKeeper{
		allowed:       true,
		returnSponsor: sponsor,
	}

	dec := NewGaslessDecorator(mk)
	ctx := sdk.Context{}.WithContext(context.Background())

	nextCalled := false
	next := func(ctx sdk.Context, tx sdk.Tx, simulate bool) (sdk.Context, error) {
		nextCalled = true
		return ctx, nil
	}

	newCtx, err := dec.AnteHandle(ctx, tx, false, next)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if !nextCalled {
		t.Fatalf("expected next ante handler to be called for zero gas price")
	}

	// Should NOT charge sponsor when gas price is zero
	if !mk.chargedFee.IsZero() {
		t.Fatalf("expected no fee to be charged for zero gas price, got: %s", mk.chargedFee)
	}

	// Should NOT set GaslessInfo when gas price is zero
	if _, ok := GetGaslessInfo(newCtx); ok {
		t.Fatalf("expected no GaslessInfo in context for zero gas price")
	}
}
