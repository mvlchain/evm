package types

import "fmt"

func DefaultGenesisState() *GenesisState {
	return &GenesisState{Auctions: []Auction{}}
}

func (gs GenesisState) Validate() error {
	seen := make(map[string]struct{})
	for _, a := range gs.Auctions {
		if a.AuctionId == "" {
			return fmt.Errorf("auction_id is required")
		}
		if _, ok := seen[a.AuctionId]; ok {
			return fmt.Errorf("duplicate auction_id: %s", a.AuctionId)
		}
		seen[a.AuctionId] = struct{}{}
	}
	return nil
}
