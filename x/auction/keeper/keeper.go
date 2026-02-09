package keeper

import (
	"context"

	"github.com/cosmos/evm/x/auction/types"

	"cosmossdk.io/store/prefix"
	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/cosmos-sdk/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
)

type Keeper struct {
	cdc      codec.BinaryCodec
	storeKey storetypes.StoreKey
}

func NewKeeper(cdc codec.BinaryCodec, storeKey storetypes.StoreKey) Keeper {
	return Keeper{
		cdc:      cdc,
		storeKey: storeKey,
	}
}

func (k Keeper) SetAuction(ctx sdk.Context, auction types.Auction) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AuctionKeyPrefix)
	bz := k.cdc.MustMarshal(&auction)
	store.Set([]byte(auction.AuctionId), bz)
}

func (k Keeper) HasAuction(ctx sdk.Context, auctionID string) bool {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AuctionKeyPrefix)
	return store.Has([]byte(auctionID))
}

func (k Keeper) GetAuction(ctx sdk.Context, auctionID string) (types.Auction, bool) {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AuctionKeyPrefix)
	bz := store.Get([]byte(auctionID))
	if bz == nil {
		return types.Auction{}, false
	}
	var auction types.Auction
	k.cdc.MustUnmarshal(bz, &auction)
	return auction, true
}

func (k Keeper) GetAllAuctions(ctx sdk.Context) []types.Auction {
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AuctionKeyPrefix)
	iterator := store.Iterator(nil, nil)
	defer iterator.Close()

	var auctions []types.Auction
	for ; iterator.Valid(); iterator.Next() {
		var a types.Auction
		k.cdc.MustUnmarshal(iterator.Value(), &a)
		auctions = append(auctions, a)
	}
	return auctions
}

func (k Keeper) GetCodec() codec.BinaryCodec {
	return k.cdc
}

func (k Keeper) GetStoreKey() storetypes.StoreKey {
	return k.storeKey
}

var _ types.QueryServer = Keeper{}
var _ types.MsgServer = Keeper{}

func (k Keeper) Auction(goCtx context.Context, req *types.QueryAuctionRequest) (*types.QueryAuctionResponse, error) {
	if req == nil || req.AuctionId == "" {
		return nil, types.ErrAuctionNotFound
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	auction, found := k.GetAuction(ctx, req.AuctionId)
	if !found {
		return nil, types.ErrAuctionNotFound
	}
	return &types.QueryAuctionResponse{Auction: auction}, nil
}

func (k Keeper) Auctions(goCtx context.Context, req *types.QueryAuctionsRequest) (*types.QueryAuctionsResponse, error) {
	if req == nil {
		req = &types.QueryAuctionsRequest{}
	}
	ctx := sdk.UnwrapSDKContext(goCtx)
	store := prefix.NewStore(ctx.KVStore(k.storeKey), types.AuctionKeyPrefix)
	pageRes, auctions, err := queryAuctionsWithPagination(store, req.Pagination, k.cdc)
	if err != nil {
		return nil, err
	}
	return &types.QueryAuctionsResponse{Auctions: auctions, Pagination: pageRes}, nil
}

func queryAuctionsWithPagination(store prefix.Store, pageReq *query.PageRequest, cdc codec.BinaryCodec) (*query.PageResponse, []types.Auction, error) {
	var auctions []types.Auction
	pageRes, err := query.Paginate(store, pageReq, func(_, value []byte) error {
		var a types.Auction
		cdc.MustUnmarshal(value, &a)
		auctions = append(auctions, a)
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return pageRes, auctions, nil
}
