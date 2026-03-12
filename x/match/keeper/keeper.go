package keeper

import (
	"fmt"

	"cosmossdk.io/log/v2"
	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/evm/x/match/types"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// Keeper grants access to the Match module state.
type Keeper struct {
	cdc      codec.BinaryCodec
	storeKey storetypes.StoreKey

	// bankKeeper is used to execute native coin and registered-ERC20 transfers during settlement.
	bankKeeper BankKeeper
	// erc20Registry resolves ERC-20 contract addresses to cosmos denominations.
	erc20Registry ERC20Registry
	// nativeDenom is the chain's native coin denomination (e.g. "aevmos").
	nativeDenom string
}

// NewKeeper creates a new x/match keeper.
func NewKeeper(cdc codec.BinaryCodec, storeKey storetypes.StoreKey) Keeper {
	return Keeper{
		cdc:      cdc,
		storeKey: storeKey,
	}
}

// SetSettlementKeepers wires the optional bank and ERC-20 keepers for on-chain settlement execution.
// Must be called after all keepers are initialised (e.g. after NewKeeper in app.go).
func (k *Keeper) SetSettlementKeepers(bank BankKeeper, erc20 ERC20Registry, nativeDenom string) {
	k.bankKeeper = bank
	k.erc20Registry = erc20
	k.nativeDenom = nativeDenom
}

// Logger returns a module-specific logger.
func (k Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", fmt.Sprintf("x/%s", types.ModuleName))
}

// HasReplay reports whether (pool_id, intent_id) has already been submitted.
func (k Keeper) HasReplay(ctx sdk.Context, poolID, intentID string) bool {
	store := ctx.KVStore(k.storeKey)
	return store.Has(types.ReplayIndexStoreKey(poolID, intentID))
}

// SetReplay stores replay index data for (pool_id, intent_id).
func (k Keeper) SetReplay(ctx sdk.Context, poolID, intentID, matchID string) {
	store := ctx.KVStore(k.storeKey)
	store.Set(types.ReplayIndexStoreKey(poolID, intentID), []byte(matchID))
}

// SetReplayParties stores requester/responder metadata for (pool_id, intent_id).
func (k Keeper) SetReplayParties(ctx sdk.Context, poolID, intentID, requester, responder string) {
	store := ctx.KVStore(k.storeKey)
	store.Set(types.ReplayPartiesStoreKey(poolID, intentID), types.EncodeReplayPartiesValue(requester, responder))
}

// GetReplayMatchID returns the stored match_id for (pool_id, intent_id), if present.
func (k Keeper) GetReplayMatchID(ctx sdk.Context, poolID, intentID string) (string, bool) {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get(types.ReplayIndexStoreKey(poolID, intentID))
	if len(bz) == 0 {
		return "", false
	}
	return string(bz), true
}

// GetReplayParties returns requester/responder metadata for (pool_id, intent_id), if present.
func (k Keeper) GetReplayParties(ctx sdk.Context, poolID, intentID string) (requester, responder string, found bool) {
	store := ctx.KVStore(k.storeKey)
	bz := store.Get(types.ReplayPartiesStoreKey(poolID, intentID))
	if len(bz) == 0 {
		return "", "", false
	}

	requester, responder, err := types.ParseReplayPartiesValue(bz)
	if err != nil {
		return "", "", false
	}
	return requester, responder, true
}

// SetIntentCancelled records an on-chain cancellation for (pool_id, intent_id).
// Once set, SubmitMatchCertificate will reject any certificate referencing this intent.
func (k Keeper) SetIntentCancelled(ctx sdk.Context, poolID, intentID string) {
	store := ctx.KVStore(k.storeKey)
	store.Set(types.CancelledIntentStoreKey(poolID, intentID), []byte{1})
}

// IsIntentCancelled reports whether (pool_id, intent_id) has been cancelled on-chain.
func (k Keeper) IsIntentCancelled(ctx sdk.Context, poolID, intentID string) bool {
	store := ctx.KVStore(k.storeKey)
	return store.Has(types.CancelledIntentStoreKey(poolID, intentID))
}

// GetAllReplayEntries returns all replay index entries in deterministic key order.
func (k Keeper) GetAllReplayEntries(ctx sdk.Context) ([]types.ReplayIndexEntry, error) {
	store := ctx.KVStore(k.storeKey)
	iterator := storetypes.KVStorePrefixIterator(store, types.KeyPrefixReplayIndex)
	defer iterator.Close()

	entries := make([]types.ReplayIndexEntry, 0)
	for ; iterator.Valid(); iterator.Next() {
		poolID, intentID, err := types.ParseReplayIndexStoreKey(iterator.Key())
		if err != nil {
			return nil, err
		}

		entries = append(entries, types.ReplayIndexEntry{
			PoolId:   poolID,
			IntentId: intentID,
			MatchId:  string(iterator.Value()),
		})

		last := &entries[len(entries)-1]
		if requester, responder, ok := k.GetReplayParties(ctx, poolID, intentID); ok {
			last.Requester = requester
			last.Responder = responder
		}
	}

	return entries, nil
}
