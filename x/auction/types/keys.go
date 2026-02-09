package types

const (
	ModuleName = "auction"
	StoreKey   = ModuleName
	RouterKey  = ModuleName
)

var AuctionKeyPrefix = []byte{0x01}

func AuctionKey(auctionID string) []byte {
	return append(AuctionKeyPrefix, []byte(auctionID)...)
}
