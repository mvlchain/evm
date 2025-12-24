package gasless

import (
	"context"
	"math/big"

	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"cosmossdk.io/math"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// gaslessCtxKey is used as a context key for passing gasless information down the ante chain.
type gaslessCtxKey struct{}

// GaslessInfo carries information about a gasless transaction through the ante chain.
type GaslessInfo struct {
	Enabled bool
	Sponsor sdk.AccAddress
	Fee     sdk.Coins
}

// GaslessKeeperI is the subset of the x/gasless keeper used by the decorator.
type GaslessKeeperI interface {
	IsGaslessAllowed(ctx sdk.Context, ethTo string) (bool, sdk.AccAddress, error)
	ChargeSponsor(ctx sdk.Context, sponsor sdk.AccAddress, fee sdk.Coins) error
	ValidateGasLimit(ctx sdk.Context, gas uint64) error
	CheckBlockSubsidyLimit(ctx sdk.Context, newFee sdk.Coins) error
}

// GaslessDecorator inspects EVM transactions and, when allowed by x/gasless
// policy, charges a sponsor account instead of relying on the EVM ante handler
// to collect fees from the sender.
type GaslessDecorator struct {
	gaslessKeeper GaslessKeeperI
}

func NewGaslessDecorator(k GaslessKeeperI) GaslessDecorator {
	return GaslessDecorator{gaslessKeeper: k}
}

// AnteHandle implements sdk.AnteDecorator.
func (d GaslessDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	ctx.Logger().Error("GASLESS ANTE HANDLER CALLED!!!")
	msgs := tx.GetMsgs()
	if len(msgs) != 1 {
		ctx.Logger().Error("Gasless: skipping - multiple messages", "count", len(msgs))
		return next(ctx, tx, simulate)
	}

	_, ethTx, err := evmtypes.UnpackEthMsg(msgs[0])
	if err != nil {
		// Not an EVM tx, just pass through.
		ctx.Logger().Info("Gasless: skipping - not EVM tx", "error", err)
		return next(ctx, tx, simulate)
	}

	to := ethTx.To()
	if to == nil {
		ctx.Logger().Info("Gasless: skipping - contract creation")
		return next(ctx, tx, simulate)
	}

	ctx.Logger().Info("Gasless: checking address", "to", to.Hex())
	allowed, sponsor, err := d.gaslessKeeper.IsGaslessAllowed(ctx, to.Hex())
	if err != nil || !allowed {
		ctx.Logger().Info("Gasless: not allowed", "to", to.Hex(), "allowed", allowed, "error", err)
		return next(ctx, tx, simulate)
	}

	ctx.Logger().Info("Gasless: APPROVED!", "to", to.Hex(), "sponsor", sponsor.String())

	// Compute fee similarly to existing EVM ante logic: fee = gas * gasPrice (or effective gas price).
	gas := ethTx.Gas()

	// Validate gas limit against max allowed for gasless txs
	if err := d.gaslessKeeper.ValidateGasLimit(ctx, gas); err != nil {
		return ctx, err
	}
	gasPrice := ethTx.GasPrice()
	if ethTx.Type() >= ethtypes.DynamicFeeTxType {
		// For 1559-style txs, effective gas price is min(maxFeePerGas, baseFee+tip).
		// Here we approximate with GasPrice() which go-ethereum already backfills.
		gasPrice = ethTx.GasPrice()
	}

	// Reject transactions with zero gas price to prevent spam attacks
	// Even for gasless transactions, the original tx must have a valid gasPrice
	// because the sponsor will pay this amount on behalf of the user
	if gasPrice == nil || gasPrice.Sign() <= 0 {
		return next(ctx, tx, simulate)
	}

	feeAmt := new(big.Int).Mul(new(big.Int).SetUint64(gas), gasPrice)
	if feeAmt.Sign() <= 0 {
		return next(ctx, tx, simulate)
	}

	evmDenom := evmtypes.GetEVMCoinDenom()
	feeCoins := sdk.NewCoins(sdk.NewCoin(evmDenom, math.NewIntFromBigInt(feeAmt)))

	// Check if this fee would exceed the per-block subsidy limit
	if err := d.gaslessKeeper.CheckBlockSubsidyLimit(ctx, feeCoins); err != nil {
		return ctx, err
	}

	if err := d.gaslessKeeper.ChargeSponsor(ctx, sponsor, feeCoins); err != nil {
		return ctx, err
	}

	info := GaslessInfo{
		Enabled: true,
		Sponsor: sponsor,
		Fee:     feeCoins,
	}
	ctx = ctx.WithContext(context.WithValue(ctx.Context(), gaslessCtxKey{}, info))

	// Emit event to mark this transaction as gasless
	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"gasless_tx",
			sdk.NewAttribute("enabled", "true"),
			sdk.NewAttribute("sponsor", sponsor.String()),
			sdk.NewAttribute("to", to.Hex()),
		),
	)

	return next(ctx, tx, simulate)
}

// GetGaslessInfo retrieves GaslessInfo from the context, if present.
func GetGaslessInfo(ctx sdk.Context) (GaslessInfo, bool) {
	v := ctx.Context().Value(gaslessCtxKey{})
	if v == nil {
		return GaslessInfo{}, false
	}
	info, ok := v.(GaslessInfo)
	return info, ok
}
