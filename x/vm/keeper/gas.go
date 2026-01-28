package keeper

import (
	"math/big"

	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/params"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	evmtrace "github.com/cosmos/evm/trace"
	"github.com/cosmos/evm/x/vm/types"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

// GetEthIntrinsicGas returns the intrinsic gas cost for the transaction
func (k *Keeper) GetEthIntrinsicGas(ctx sdk.Context, msg core.Message, cfg *params.ChainConfig,
	isContractCreation bool,
) (_ uint64, err error) {
	ctx, span := ctx.StartSpan(tracer, "GetEthIntrinsicGas", trace.WithAttributes(
		attribute.Bool("is_contract_creation", isContractCreation),
		attribute.Int("data_size", len(msg.Data)),
	))
	defer func() { evmtrace.EndSpanErr(span, err) }()
	height := big.NewInt(ctx.BlockHeight())
	homestead := cfg.IsHomestead(height)
	istanbul := cfg.IsIstanbul(height)
	shanghai := cfg.IsShanghai(height, uint64(ctx.BlockTime().Unix())) //#nosec G115 -- int overflow is not a concern here
	return core.IntrinsicGas(msg.Data, msg.AccessList, msg.SetCodeAuthorizations, isContractCreation,
		homestead, istanbul, shanghai)
}

// RefundGas transfers the leftover gas to the sender of the message, capped to half of the total gas
// consumed in the transaction. Additionally, the function sets the total gas consumed to the value
// returned by the EVM execution, thus ignoring the previous intrinsic gas consumed during in the
// AnteHandler.
//
// For sponsored transactions, the refund is directed to the sponsor (who paid
// the fees upfront) instead of msg.From (the beneficiary).
func (k *Keeper) RefundGas(ctx sdk.Context, msg core.Message, leftoverGas uint64, denom string) (err error) {
	ctx, span := ctx.StartSpan(tracer, "RefundGas", trace.WithAttributes(attribute.Int64("leftover_gas", int64(leftoverGas)))) //nolint:gosec // G115
	defer func() { evmtrace.EndSpanErr(span, err) }()

	// Return EVM tokens for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(leftoverGas), msg.GasPrice)

	switch remaining.Sign() {
	case -1:
		// negative refund errors
		return errorsmod.Wrapf(types.ErrInvalidRefund, "refunded amount value cannot be negative %d", remaining.Int64())
	case 1:
		// positive amount refund
		refundedCoins := sdk.Coins{sdk.NewCoin(denom, sdkmath.NewIntFromBigInt(remaining))}

		// Determine refund recipient: sponsor if this is a sponsored tx,
		// otherwise the original sender.
		refundAddr := msg.From.Bytes()
		if sponsor, ok := k.GetTransientSponsor(ctx); ok {
			refundAddr = sponsor.Bytes()
		}

		// refund from the fee collector module account, which is the escrow account in charge of collecting tx fees
		var err error
		if k.virtualFeeCollection {
			err = k.bankWrapper.SendCoinsFromModuleToAccountVirtual(ctx, authtypes.FeeCollectorName, refundAddr, refundedCoins)
		} else {
			err = k.bankWrapper.SendCoinsFromModuleToAccount(ctx, authtypes.FeeCollectorName, refundAddr, refundedCoins)
		}
		if err != nil {
			err = errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "fee collector account failed to refund fees: %s", err.Error())
			return errorsmod.Wrapf(err, "failed to refund %d leftover gas (%s)", leftoverGas, refundedCoins.String())
		}
	default:
		// no refund, consume gas and update the tx gas meter
	}

	return nil
}

// ResetGasMeterAndConsumeGas reset first the gas meter consumed value to zero and set it back to the new value
// 'gasUsed'
func (k *Keeper) ResetGasMeterAndConsumeGas(ctx sdk.Context, gasUsed uint64) {
	// reset the gas count
	ctx, span := ctx.StartSpan(tracer, "ResetGasMeterAndConsumeGas", trace.WithAttributes(
		attribute.Int64("gas_used", int64(gasUsed)), //nolint:gosec // G115
	))
	defer span.End()
	ctx.GasMeter().RefundGas(ctx.GasMeter().GasConsumed(), "reset the gas count")
	ctx.GasMeter().ConsumeGas(gasUsed, "apply evm transaction")
}

// GasToRefund calculates the amount of gas the state machine should refund to the sender. It is
// capped by the refund quotient value.
// Note: do not pass 0 to refundQuotient
func GasToRefund(availableRefund, gasConsumed, refundQuotient uint64) uint64 {
	// Apply refund counter
	refund := gasConsumed / refundQuotient
	if refund > availableRefund {
		return availableRefund
	}
	return refund
}
