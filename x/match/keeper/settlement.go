package keeper

import (
	"bytes"
	"context"
	"crypto/sha256"
	"math/big"
	"strings"

	errorsmod "cosmossdk.io/errors"
	sdkmath "cosmossdk.io/math"

	"github.com/cosmos/evm/x/match/types"

	ethcommon "github.com/ethereum/go-ethereum/common"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// BankKeeper defines the bank operations required for settlement execution.
type BankKeeper interface {
	SendCoins(ctx context.Context, fromAddr sdk.AccAddress, toAddr sdk.AccAddress, amt sdk.Coins) error
}

// ERC20Registry defines the ERC-20 registration queries required for settlement asset resolution.
type ERC20Registry interface {
	// GetDenomForERC20 returns the cosmos denom for a registered ERC-20 token address.
	// Returns ("", false) if the address is not registered.
	GetDenomForERC20(ctx sdk.Context, erc20Addr ethcommon.Address) (string, bool)
}

// executeSettlement verifies the settlement hash commitment and performs the atomic token swap.
// It is called inside SubmitMatchCertificate after replay protection has been set.
func (k Keeper) executeSettlement(ctx sdk.Context, cert types.MatchCertificate) error {
	si := cert.Settlement
	if si == nil {
		// Settlement is optional — certificates without settlement data are accepted (record-only mode).
		return nil
	}

	// 1. Verify the settlement_hash commitment in FinalizePayload.
	if err := k.verifySettlementHash(cert, si); err != nil {
		return err
	}

	// 2. Validate addresses.
	if !ethcommon.IsHexAddress(si.Initiator) {
		return errorsmod.Wrapf(types.ErrSettlementFailed, "initiator is not a valid ethereum address: %s", si.Initiator)
	}
	if !ethcommon.IsHexAddress(si.Responder) {
		return errorsmod.Wrapf(types.ErrSettlementFailed, "responder is not a valid ethereum address: %s", si.Responder)
	}

	// 3. Verify settlement parties match the certificate parties exactly.
	if cert.Payload == nil {
		return errorsmod.Wrap(types.ErrSettlementFailed, "certificate payload missing")
	}
	if ethcommon.HexToAddress(si.Initiator) != ethcommon.HexToAddress(cert.Payload.Initiator) {
		return errorsmod.Wrap(types.ErrSettlementFailed, "settlement.initiator does not match certificate.initiator")
	}
	if ethcommon.HexToAddress(si.Responder) != ethcommon.HexToAddress(cert.Payload.Responder) {
		return errorsmod.Wrap(types.ErrSettlementFailed, "settlement.responder does not match certificate.responder")
	}

	initiatorAddr := sdk.AccAddress(ethcommon.HexToAddress(si.Initiator).Bytes())
	responderAddr := sdk.AccAddress(ethcommon.HexToAddress(si.Responder).Bytes())

	// 4. Transfer asset_in: initiator → responder.
	if err := k.executeAssetTransfer(ctx, initiatorAddr, responderAddr, si.AssetIn, si.AmountIn); err != nil {
		return errorsmod.Wrapf(types.ErrSettlementFailed, "asset_in transfer failed: %v", err)
	}

	// 5. Transfer asset_out: responder → initiator.
	if err := k.executeAssetTransfer(ctx, responderAddr, initiatorAddr, si.AssetOut, si.AmountOut); err != nil {
		return errorsmod.Wrapf(types.ErrSettlementFailed, "asset_out transfer failed: %v", err)
	}

	return nil
}

// verifySettlementHash checks that sha256(proto.Marshal(si)) matches FinalizePayload.settlement_hash.
// settlement_hash is mandatory when a SettlementInstruction is present.
func (k Keeper) verifySettlementHash(cert types.MatchCertificate, si *types.SettlementInstruction) error {
	if cert.Finalize == nil || cert.Finalize.Payload == nil {
		return errorsmod.Wrap(types.ErrSettlementFailed, "finalize payload missing")
	}
	settlementHash := cert.Finalize.Payload.SettlementHash
	if len(settlementHash) == 0 {
		return errorsmod.Wrap(types.ErrSettlementFailed, "settlement_hash is required when settlement instruction is present")
	}
	bz, err := si.Marshal()
	if err != nil {
		return errorsmod.Wrapf(types.ErrSettlementFailed, "failed to marshal settlement instruction: %v", err)
	}
	computed := sha256.Sum256(bz)
	if !bytes.Equal(computed[:], settlementHash) {
		return errorsmod.Wrap(types.ErrHashMismatch, "settlement_instruction hash does not match finalize.settlement_hash")
	}
	return nil
}

// executeAssetTransfer moves `amount` of `asset` from `from` to `to`.
// asset is either "native" (chain coin) or an ERC-20 hex address registered in the ERC-20 module.
func (k Keeper) executeAssetTransfer(ctx sdk.Context, from, to sdk.AccAddress, asset, amountStr string) error {
	if k.bankKeeper == nil {
		return errorsmod.Wrap(types.ErrSettlementFailed, "bank keeper not configured")
	}

	amount, ok := new(big.Int).SetString(amountStr, 10)
	if !ok || amount.Sign() <= 0 {
		return errorsmod.Wrapf(types.ErrSettlementFailed, "invalid settlement amount: %q", amountStr)
	}

	var denom string
	if strings.EqualFold(asset, types.NativeAsset) {
		if k.nativeDenom == "" {
			return errorsmod.Wrap(types.ErrSettlementFailed, "native denom not configured")
		}
		denom = k.nativeDenom
	} else {
		if k.erc20Registry == nil {
			return errorsmod.Wrap(types.ErrSettlementFailed, "erc20 registry not configured")
		}
		if !ethcommon.IsHexAddress(asset) {
			return errorsmod.Wrapf(types.ErrSettlementFailed, "asset is not a valid ethereum address or 'native': %s", asset)
		}
		var found bool
		denom, found = k.erc20Registry.GetDenomForERC20(ctx, ethcommon.HexToAddress(asset))
		if !found {
			return errorsmod.Wrapf(types.ErrSettlementFailed, "ERC-20 %s is not registered in the token pair registry", asset)
		}
	}

	coin := sdk.NewCoin(denom, sdkmath.NewIntFromBigInt(amount))
	return k.bankKeeper.SendCoins(ctx, from, to, sdk.NewCoins(coin))
}
