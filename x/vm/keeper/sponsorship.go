package keeper

import (
	"fmt"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/cosmos/evm/x/vm/types"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

const (
	// SponsorshipKeyPrefix is the prefix for sponsorship storage
	SponsorshipKeyPrefix = "sponsorship/"
	// SponsorshipIndexPrefix is the prefix for sponsorship index by beneficiary
	SponsorshipIndexPrefix = "sponsorship-index/"
	// SponsorshipUsagePrefix is the prefix for daily usage tracking
	SponsorshipUsagePrefix = "sponsorship-usage/"
)

// CreateSponsorship creates a new fee sponsorship
func (k *Keeper) CreateSponsorship(
	ctx sdk.Context,
	sponsor common.Address,
	beneficiary common.Address,
	maxGasPerTx uint64,
	totalGasBudget uint64,
	expirationHeight int64,
	conditions *types.SponsorshipConditions,
) (string, error) {
	ctx, span := ctx.StartSpan(tracer, "CreateSponsorship", trace.WithAttributes(
		attribute.String("sponsor", sponsor.Hex()),
		attribute.String("beneficiary", beneficiary.Hex()),
		attribute.Int64("max_gas_per_tx", int64(maxGasPerTx)),
	))
	defer span.End()

	// Validation
	if sponsor == (common.Address{}) {
		return "", errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "sponsor address cannot be empty")
	}
	if beneficiary == (common.Address{}) {
		return "", errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "beneficiary address cannot be empty")
	}
	if maxGasPerTx == 0 {
		return "", errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "max gas per tx must be greater than 0")
	}
	if totalGasBudget == 0 {
		return "", errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "total gas budget must be greater than 0")
	}
	if expirationHeight <= ctx.BlockHeight() {
		return "", errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "expiration height must be in the future")
	}

	// Generate unique sponsorship ID
	sponsorshipID := generateSponsorshipID(sponsor, beneficiary, ctx.BlockHeight())

	// Create sponsorship
	sponsorship := &types.FeeSponsor{
		Sponsor:           sponsor.Hex(),
		Beneficiary:       beneficiary.Hex(),
		MaxGasPerTx:       maxGasPerTx,
		TotalGasBudget:    totalGasBudget,
		ExpirationHeight:  expirationHeight,
		CreatedAt:         ctx.BlockHeight(),
		SponsorshipId:     sponsorshipID,
		IsActive:          true,
		GasUsed:           0,
		TransactionCount:  0,
	}

	if conditions != nil {
		sponsorship.Conditions = conditions
	}

	// Store sponsorship
	k.setSponsorshipInStore(ctx, sponsorship)

	// Create index for quick lookup by beneficiary
	k.addSponsorshipToBeneficiaryIndex(ctx, beneficiary, sponsorshipID)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"sponsorship_created",
			sdk.NewAttribute("sponsorship_id", sponsorshipID),
			sdk.NewAttribute("sponsor", sponsor.Hex()),
			sdk.NewAttribute("beneficiary", beneficiary.Hex()),
			sdk.NewAttribute("max_gas_per_tx", fmt.Sprintf("%d", maxGasPerTx)),
			sdk.NewAttribute("total_gas_budget", fmt.Sprintf("%d", totalGasBudget)),
			sdk.NewAttribute("expiration_height", fmt.Sprintf("%d", expirationHeight)),
		),
	)

	return sponsorshipID, nil
}

// GetActiveSponsorshipFor finds an active sponsorship for a beneficiary's transaction
func (k *Keeper) GetActiveSponsorshipFor(
	ctx sdk.Context,
	beneficiary common.Address,
	gasLimit uint64,
	targetContract *common.Address,
	txValue *math.Int,
) (*types.FeeSponsor, error) {
	ctx, span := ctx.StartSpan(tracer, "GetActiveSponsorshipFor", trace.WithAttributes(
		attribute.String("beneficiary", beneficiary.Hex()),
		attribute.Int64("gas_limit", int64(gasLimit)),
	))
	defer span.End()

	// Get all sponsorships for beneficiary
	sponsorshipIDs := k.getSponsorshipIDsForBeneficiary(ctx, beneficiary)
	if len(sponsorshipIDs) == 0 {
		return nil, nil
	}

	currentHeight := ctx.BlockHeight()

	// Find first matching active sponsorship
	for _, sponsorshipID := range sponsorshipIDs {
		sponsorship := k.getSponsorshipFromStore(ctx, sponsorshipID)
		if sponsorship == nil {
			continue
		}

		// Check if sponsorship is valid
		if !k.isSponsorshipValid(ctx, sponsorship, gasLimit, targetContract, txValue, currentHeight) {
			continue
		}

		return sponsorship, nil
	}

	return nil, nil
}

// UseSponsorshipForTransaction deducts gas from a sponsorship
func (k *Keeper) UseSponsorshipForTransaction(
	ctx sdk.Context,
	sponsorshipID string,
	gasUsed uint64,
) error {
	ctx, span := ctx.StartSpan(tracer, "UseSponsorshipForTransaction", trace.WithAttributes(
		attribute.String("sponsorship_id", sponsorshipID),
		attribute.Int64("gas_used", int64(gasUsed)),
	))
	defer span.End()

	sponsorship := k.getSponsorshipFromStore(ctx, sponsorshipID)
	if sponsorship == nil {
		return errorsmod.Wrap(sdkerrors.ErrNotFound, "sponsorship not found")
	}

	// Deduct gas from budget
	if sponsorship.TotalGasBudget < gasUsed {
		sponsorship.TotalGasBudget = 0
		sponsorship.IsActive = false
	} else {
		sponsorship.TotalGasBudget -= gasUsed
	}

	sponsorship.GasUsed += gasUsed
	sponsorship.TransactionCount++

	// Check if budget exhausted
	if sponsorship.TotalGasBudget == 0 {
		sponsorship.IsActive = false
	}

	// Update storage
	k.setSponsorshipInStore(ctx, sponsorship)

	// Track daily usage if daily limit is set
	if sponsorship.Conditions != nil && sponsorship.Conditions.DailyGasLimit > 0 {
		k.trackDailyUsage(ctx, sponsorshipID, gasUsed)
	}

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"sponsorship_used",
			sdk.NewAttribute("sponsorship_id", sponsorshipID),
			sdk.NewAttribute("gas_used", fmt.Sprintf("%d", gasUsed)),
			sdk.NewAttribute("remaining_budget", fmt.Sprintf("%d", sponsorship.TotalGasBudget)),
			sdk.NewAttribute("transaction_count", fmt.Sprintf("%d", sponsorship.TransactionCount)),
		),
	)

	return nil
}

// CancelSponsorship cancels a sponsorship and returns the sponsor address for refund
func (k *Keeper) CancelSponsorship(
	ctx sdk.Context,
	sponsorshipID string,
	caller common.Address,
) (common.Address, uint64, error) {
	ctx, span := ctx.StartSpan(tracer, "CancelSponsorship", trace.WithAttributes(
		attribute.String("sponsorship_id", sponsorshipID),
	))
	defer span.End()

	sponsorship := k.getSponsorshipFromStore(ctx, sponsorshipID)
	if sponsorship == nil {
		return common.Address{}, 0, errorsmod.Wrap(sdkerrors.ErrNotFound, "sponsorship not found")
	}

	sponsor := common.HexToAddress(sponsorship.Sponsor)

	// Only sponsor can cancel
	if caller != sponsor {
		return common.Address{}, 0, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "only sponsor can cancel")
	}

	// Deactivate sponsorship
	sponsorship.IsActive = false
	remainingBudget := sponsorship.TotalGasBudget
	sponsorship.TotalGasBudget = 0

	k.setSponsorshipInStore(ctx, sponsorship)

	// Remove from beneficiary index
	beneficiary := common.HexToAddress(sponsorship.Beneficiary)
	k.removeSponsorshipFromBeneficiaryIndex(ctx, beneficiary, sponsorshipID)

	ctx.EventManager().EmitEvent(
		sdk.NewEvent(
			"sponsorship_cancelled",
			sdk.NewAttribute("sponsorship_id", sponsorshipID),
			sdk.NewAttribute("sponsor", sponsor.Hex()),
			sdk.NewAttribute("refunded_budget", fmt.Sprintf("%d", remainingBudget)),
		),
	)

	return sponsor, remainingBudget, nil
}

// GetSponsorship retrieves a sponsorship by ID
func (k *Keeper) GetSponsorship(ctx sdk.Context, sponsorshipID string) (*types.FeeSponsor, error) {
	sponsorship := k.getSponsorshipFromStore(ctx, sponsorshipID)
	if sponsorship == nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrNotFound, "sponsorship not found")
	}
	return sponsorship, nil
}

// GetSponsorshipsForBeneficiary retrieves all sponsorships for a beneficiary
// HasActiveSponsorshipFor checks if the given beneficiary has any active
// fee sponsorship. This is a lightweight check used by the mempool to
// determine whether to skip balance verification for a transaction sender.
func (k *Keeper) HasActiveSponsorshipFor(ctx sdk.Context, beneficiary common.Address) bool {
	sponsorshipIDs := k.getSponsorshipIDsForBeneficiary(ctx, beneficiary)
	if len(sponsorshipIDs) == 0 {
		return false
	}
	currentHeight := ctx.BlockHeight()
	for _, id := range sponsorshipIDs {
		sponsorship := k.getSponsorshipFromStore(ctx, id)
		if sponsorship == nil {
			continue
		}
		// Quick check: is the sponsorship active and not expired?
		if sponsorship.IsActive && (sponsorship.ExpirationHeight == 0 || sponsorship.ExpirationHeight > currentHeight) {
			return true
		}
	}
	return false
}

func (k *Keeper) GetSponsorshipsForBeneficiary(ctx sdk.Context, beneficiary common.Address) []*types.FeeSponsor {
	sponsorshipIDs := k.getSponsorshipIDsForBeneficiary(ctx, beneficiary)
	sponsorships := make([]*types.FeeSponsor, 0, len(sponsorshipIDs))

	for _, id := range sponsorshipIDs {
		if sponsorship := k.getSponsorshipFromStore(ctx, id); sponsorship != nil {
			sponsorships = append(sponsorships, sponsorship)
		}
	}

	return sponsorships
}

// Helper functions

func generateSponsorshipID(sponsor, beneficiary common.Address, blockHeight int64) string {
	data := append(sponsor.Bytes(), beneficiary.Bytes()...)
	data = append(data, []byte(fmt.Sprintf("%d", blockHeight))...)
	hash := crypto.Keccak256Hash(data)
	return hash.Hex()
}

func (k *Keeper) isSponsorshipValid(
	ctx sdk.Context,
	sponsorship *types.FeeSponsor,
	gasLimit uint64,
	targetContract *common.Address,
	txValue *math.Int,
	currentHeight int64,
) bool {
	// Check if active
	if !sponsorship.IsActive {
		return false
	}

	// Check expiration
	if currentHeight >= sponsorship.ExpirationHeight {
		return false
	}

	// Check gas limits
	if gasLimit > sponsorship.MaxGasPerTx {
		return false
	}

	if sponsorship.TotalGasBudget < gasLimit {
		return false
	}

	// Check if sponsor has sufficient balance to pay for the gas
	sponsor := common.HexToAddress(sponsorship.Sponsor)
	sponsorAccAddr := sdk.AccAddress(sponsor.Bytes())
	params := k.GetParams(ctx)
	evmDenom := params.EvmDenom

	// Get sponsor's balance
	sponsorBalance := k.bankWrapper.GetBalance(ctx, sponsorAccAddr, evmDenom)

	// Get current base fee (gas price)
	baseFee := k.GetBaseFee(ctx)
	if baseFee == nil {
		// Fallback to minimum gas price if base fee is not set
		minGasPrice := k.GetMinGasPrice(ctx)
		baseFee = minGasPrice.TruncateInt().BigInt()
	}

	// Calculate estimated cost: gasLimit * baseFee (using big.Int)
	gasLimitBig := new(big.Int).SetUint64(gasLimit)
	estimatedCostBig := new(big.Int).Mul(gasLimitBig, baseFee)

	// Convert to math.Int for comparison with balance
	estimatedCost := math.NewIntFromBigInt(estimatedCostBig)

	// Check if sponsor has sufficient balance
	if sponsorBalance.Amount.LT(estimatedCost) {
		return false
	}

	// Check conditions if present
	if sponsorship.Conditions != nil {
		// Check whitelisted contracts
		if len(sponsorship.Conditions.WhitelistedContracts) > 0 && targetContract != nil {
			// Only validate whitelist if targetContract is provided
			// If targetContract is nil (e.g., from isSponsored query), skip this check
			found := false
			for _, addr := range sponsorship.Conditions.WhitelistedContracts {
				if common.HexToAddress(addr) == *targetContract {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}

		// Check max tx value
		if !sponsorship.Conditions.MaxTxValue.IsZero() && txValue != nil {
			if txValue.GT(sponsorship.Conditions.MaxTxValue) {
				return false
			}
		}

		// Check daily limit — compare accumulated usage so far against the cap.
		// We intentionally do NOT add gasLimit here because the gas limit is
		// a worst-case estimate set by the client and can be much larger than
		// the actual gas consumed. The per-tx cap is already enforced by
		// maxGasPerTx above. Actual usage is tracked post-execution via
		// UseSponsorshipForTransaction.
		if sponsorship.Conditions.DailyGasLimit > 0 {
			dailyUsage := k.getDailyUsage(ctx, sponsorship.SponsorshipId)
			if dailyUsage >= sponsorship.Conditions.DailyGasLimit {
				return false
			}
		}
	}

	return true
}

// Storage functions

func (k *Keeper) setSponsorshipInStore(ctx sdk.Context, sponsorship *types.FeeSponsor) {
	store := ctx.KVStore(k.storeKey)
	bz := k.cdc.MustMarshal(sponsorship)
	key := []byte(SponsorshipKeyPrefix + sponsorship.SponsorshipId)
	store.Set(key, bz)
}

func (k *Keeper) getSponsorshipFromStore(ctx sdk.Context, sponsorshipID string) *types.FeeSponsor {
	store := ctx.KVStore(k.storeKey)
	key := []byte(SponsorshipKeyPrefix + sponsorshipID)
	bz := store.Get(key)
	if bz == nil {
		return nil
	}

	var sponsorship types.FeeSponsor
	k.cdc.MustUnmarshal(bz, &sponsorship)
	return &sponsorship
}

func (k *Keeper) addSponsorshipToBeneficiaryIndex(ctx sdk.Context, beneficiary common.Address, sponsorshipID string) {
	store := ctx.KVStore(k.storeKey)
	key := []byte(SponsorshipIndexPrefix + beneficiary.Hex())

	// Get existing index
	index := &types.BeneficiarySponsorshipIndex{}
	bz := store.Get(key)
	if bz != nil {
		k.cdc.MustUnmarshal(bz, index)
	}

	// Add new ID
	index.SponsorshipIds = append(index.SponsorshipIds, sponsorshipID)

	// Save updated index
	bz = k.cdc.MustMarshal(index)
	store.Set(key, bz)
}

func (k *Keeper) removeSponsorshipFromBeneficiaryIndex(ctx sdk.Context, beneficiary common.Address, sponsorshipID string) {
	store := ctx.KVStore(k.storeKey)
	key := []byte(SponsorshipIndexPrefix + beneficiary.Hex())

	// Get existing index
	index := &types.BeneficiarySponsorshipIndex{}
	bz := store.Get(key)
	if bz != nil {
		k.cdc.MustUnmarshal(bz, index)
	}

	// Remove ID
	filtered := make([]string, 0, len(index.SponsorshipIds))
	for _, id := range index.SponsorshipIds {
		if id != sponsorshipID {
			filtered = append(filtered, id)
		}
	}

	// Save updated index
	if len(filtered) > 0 {
		index.SponsorshipIds = filtered
		bz = k.cdc.MustMarshal(index)
		store.Set(key, bz)
	} else {
		store.Delete(key)
	}
}

func (k *Keeper) getSponsorshipIDsForBeneficiary(ctx sdk.Context, beneficiary common.Address) []string {
	store := ctx.KVStore(k.storeKey)
	key := []byte(SponsorshipIndexPrefix + beneficiary.Hex())
	bz := store.Get(key)
	if bz == nil {
		return nil
	}

	index := &types.BeneficiarySponsorshipIndex{}
	k.cdc.MustUnmarshal(bz, index)
	return index.SponsorshipIds
}

func (k *Keeper) trackDailyUsage(ctx sdk.Context, sponsorshipID string, gasUsed uint64) {
	store := ctx.KVStore(k.storeKey)
	today := time.Unix(ctx.BlockTime().Unix(), 0).Truncate(24 * time.Hour).Unix()
	key := []byte(fmt.Sprintf("%s%s-%d", SponsorshipUsagePrefix, sponsorshipID, today))

	// Get existing usage
	dailyUsage := &types.DailyUsage{}
	bz := store.Get(key)
	if bz != nil {
		k.cdc.MustUnmarshal(bz, dailyUsage)
	}

	// Add new usage
	dailyUsage.GasUsed += gasUsed

	// Save
	bz = k.cdc.MustMarshal(dailyUsage)
	store.Set(key, bz)
}

func (k *Keeper) getDailyUsage(ctx sdk.Context, sponsorshipID string) uint64 {
	store := ctx.KVStore(k.storeKey)
	today := time.Unix(ctx.BlockTime().Unix(), 0).Truncate(24 * time.Hour).Unix()
	key := []byte(fmt.Sprintf("%s%s-%d", SponsorshipUsagePrefix, sponsorshipID, today))

	bz := store.Get(key)
	if bz == nil {
		return 0
	}

	dailyUsage := &types.DailyUsage{}
	k.cdc.MustUnmarshal(bz, dailyUsage)
	return dailyUsage.GasUsed
}
