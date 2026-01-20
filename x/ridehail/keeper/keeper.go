package keeper

import (
	"cosmossdk.io/log"
	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"
	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/cosmos/evm/x/ridehail/types"
)

type Keeper struct {
	cdc      codec.BinaryCodec
	storeKey storetypes.StoreKey
}

func NewKeeper(
	cdc codec.BinaryCodec,
	storeKey storetypes.StoreKey,
) Keeper {
	return Keeper{
		cdc:      cdc,
		storeKey: storeKey,
	}
}

func (k Keeper) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("module", types.ModuleName)
}

// GetNextRequestId returns the next request ID
func (k Keeper) GetNextRequestId(ctx sdk.Context) uint64 {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixNextRequestId)
	bz := store.Get([]byte{0x00})
	if bz == nil {
		return 1
	}
	return sdk.BigEndianToUint64(bz)
}

// SetNextRequestId sets the next request ID
func (k Keeper) SetNextRequestId(ctx sdk.Context, id uint64) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixNextRequestId)
	store.Set([]byte{0x00}, sdk.Uint64ToBigEndian(id))
}

// GetNextSessionId returns the next session ID
func (k Keeper) GetNextSessionId(ctx sdk.Context) uint64 {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixNextSessionId)
	bz := store.Get([]byte{0x00})
	if bz == nil {
		return 1
	}
	return sdk.BigEndianToUint64(bz)
}

// SetNextSessionId sets the next session ID
func (k Keeper) SetNextSessionId(ctx sdk.Context, id uint64) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.KeyPrefixNextSessionId)
	store.Set([]byte{0x00}, sdk.Uint64ToBigEndian(id))
}

// SetRequest stores a request
func (k Keeper) SetRequest(ctx sdk.Context, requestId uint64, data []byte) {
	store := ctx.KVStore(k.storeKey)
	key := types.RequestKey(requestId)
	store.Set(key, data)
}

// GetRequest retrieves a request
func (k Keeper) GetRequest(ctx sdk.Context, requestId uint64) []byte {
	store := ctx.KVStore(k.storeKey)
	key := types.RequestKey(requestId)
	return store.Get(key)
}

// SetSession stores a session
func (k Keeper) SetSession(ctx sdk.Context, sessionId uint64, data []byte) {
	store := ctx.KVStore(k.storeKey)
	key := types.SessionKey(sessionId)
	store.Set(key, data)
}

// GetSession retrieves a session
func (k Keeper) GetSession(ctx sdk.Context, sessionId uint64) []byte {
	store := ctx.KVStore(k.storeKey)
	key := types.SessionKey(sessionId)
	return store.Get(key)
}
