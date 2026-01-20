package keeper

import (
	"encoding/json"

	storetypes "cosmossdk.io/store/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/evm/x/ridehail/types"
)

// StorePendingRequest stores a pending request waiting for driver commits
func (k Keeper) StorePendingRequest(ctx sdk.Context, req *types.PendingRequest) {
	store := ctx.KVStore(k.storeKey)
	key := types.PendingRequestKey(req.RequestId)

	bz, err := json.Marshal(req)
	if err != nil {
		panic(err)
	}

	store.Set(key, bz)
}

// GetPendingRequest retrieves a pending request
func (k Keeper) GetPendingRequest(ctx sdk.Context, requestId uint64) (*types.PendingRequest, bool) {
	store := ctx.KVStore(k.storeKey)
	key := types.PendingRequestKey(requestId)

	bz := store.Get(key)
	if bz == nil {
		return nil, false
	}

	var req types.PendingRequest
	if err := json.Unmarshal(bz, &req); err != nil {
		return nil, false
	}

	return &req, true
}

// DeletePendingRequest removes a pending request
func (k Keeper) DeletePendingRequest(ctx sdk.Context, requestId uint64) {
	store := ctx.KVStore(k.storeKey)
	key := types.PendingRequestKey(requestId)
	store.Delete(key)
}

// GetAllPendingRequests returns all pending requests
func (k Keeper) GetAllPendingRequests(ctx sdk.Context) []*types.PendingRequest {
	store := ctx.KVStore(k.storeKey)
	iterator := storetypes.KVStorePrefixIterator(store, types.KeyPrefixPendingRequest)
	defer iterator.Close()

	var requests []*types.PendingRequest
	for ; iterator.Valid(); iterator.Next() {
		var req types.PendingRequest
		if err := json.Unmarshal(iterator.Value(), &req); err != nil {
			continue
		}
		requests = append(requests, &req)
	}

	return requests
}

// StoreDriverCommit stores a driver's commitment
func (k Keeper) StoreDriverCommit(ctx sdk.Context, commit *types.DriverCommit) {
	store := ctx.KVStore(k.storeKey)
	key := types.DriverCommitKey(commit.RequestId, commit.Driver)

	bz, err := json.Marshal(commit)
	if err != nil {
		panic(err)
	}

	store.Set(key, bz)
}

// GetDriverCommits retrieves all driver commits for a request
func (k Keeper) GetDriverCommits(ctx sdk.Context, requestId uint64) []*types.DriverCommit {
	store := ctx.KVStore(k.storeKey)

	// Construct prefix: KeyPrefixDriverCommit + requestId
	reqKey := make([]byte, 8)
	reqKey[0] = byte(requestId >> 56)
	reqKey[1] = byte(requestId >> 48)
	reqKey[2] = byte(requestId >> 40)
	reqKey[3] = byte(requestId >> 32)
	reqKey[4] = byte(requestId >> 24)
	reqKey[5] = byte(requestId >> 16)
	reqKey[6] = byte(requestId >> 8)
	reqKey[7] = byte(requestId)

	prefix := append(types.KeyPrefixDriverCommit, reqKey...)
	iterator := storetypes.KVStorePrefixIterator(store, prefix)
	defer iterator.Close()

	var commits []*types.DriverCommit
	for ; iterator.Valid(); iterator.Next() {
		var commit types.DriverCommit
		if err := json.Unmarshal(iterator.Value(), &commit); err != nil {
			continue
		}
		commits = append(commits, &commit)
	}

	return commits
}

// DeleteDriverCommits removes all driver commits for a request
func (k Keeper) DeleteDriverCommits(ctx sdk.Context, requestId uint64) {
	store := ctx.KVStore(k.storeKey)

	reqKey := make([]byte, 8)
	reqKey[0] = byte(requestId >> 56)
	reqKey[1] = byte(requestId >> 48)
	reqKey[2] = byte(requestId >> 40)
	reqKey[3] = byte(requestId >> 32)
	reqKey[4] = byte(requestId >> 24)
	reqKey[5] = byte(requestId >> 16)
	reqKey[6] = byte(requestId >> 8)
	reqKey[7] = byte(requestId)

	prefix := append(types.KeyPrefixDriverCommit, reqKey...)
	iterator := storetypes.KVStorePrefixIterator(store, prefix)
	defer iterator.Close()

	var keysToDelete [][]byte
	for ; iterator.Valid(); iterator.Next() {
		keysToDelete = append(keysToDelete, iterator.Key())
	}

	for _, key := range keysToDelete {
		store.Delete(key)
	}
}
