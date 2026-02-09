package mvl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/types/query"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	auctiontypes "github.com/cosmos/evm/x/auction/types"
	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
)

type PublicAPI struct {
	cdc      codec.Codec
	auctionQ auctiontypes.QueryClient
	stakingQ stakingtypes.QueryClient
}

func NewPublicAPI(clientCtx client.Context) *PublicAPI {
	return &PublicAPI{
		cdc:      clientCtx.Codec,
		auctionQ: auctiontypes.NewQueryClient(clientCtx),
		stakingQ: stakingtypes.NewQueryClient(clientCtx),
	}
}

func (a *PublicAPI) DummyMethod() string {
	return "This is a dummy method"
}

type AskMessage struct {
	Type      string         `json:"type"`
	AuctionID string         `json:"auction_id"`
	Seller    string         `json:"seller"`
	Denom     string         `json:"denom"`
	MinPrice  string         `json:"min_price"`
	EndHeight int64          `json:"end_height"`
	ItemMeta  map[string]any `json:"item_meta,omitempty"`
	Signature string         `json:"sig"`
}

type BidMessage struct {
	Type      string `json:"type"`
	AuctionID string `json:"auction_id"`
	Bidder    string `json:"bidder"`
	Price     string `json:"price"`
	EndHeight int64  `json:"end_height"`
	Signature string `json:"sig"`
}

type CancelMessage struct {
	Type      string `json:"type"`
	AuctionID string `json:"auction_id"`
	Actor     string `json:"actor"`
	Signature string `json:"sig"`
}

type ListItemsResponse struct {
	Items  []ListItem `json:"items"`
	Cursor string     `json:"cursor"`
}

type ListBidsResponse struct {
	Bids   []json.RawMessage `json:"bids"`
	Cursor string            `json:"cursor"`
}

type ListItem struct {
	Ask    json.RawMessage  `json:"ask"`
	TopBid *json.RawMessage `json:"top_bid"`
}

type ListAuctionsResponse struct {
	Confirmed    json.RawMessage   `json:"confirmed"`
	Active       []json.RawMessage `json:"active"`
	ActiveCursor string            `json:"active_cursor"`
}

// Publish publishes a payload to Redis Pub/Sub.
// JSON-RPC: mvl_publish
func (a *PublicAPI) Publish(topic string, payload any) (bool, error) {
	ctx := context.Background()
	if err := PublishRedis(ctx, topic, payload); err != nil {
		return false, err
	}
	return true, nil
}

// PublishAsk stores an ask and publishes it to Redis Pub/Sub.
// JSON-RPC: mvl_publishAsk
func (a *PublicAPI) PublishAsk(msg AskMessage) (bool, error) {
	if msg.Type == "" {
		msg.Type = "ask"
	}
	if err := validateAskMessage(msg); err != nil {
		return false, err
	}
	ctx := context.Background()
	if err := storeAsk(ctx, msg); err != nil {
		return false, err
	}
	gossipBroadcast(msg)
	return true, nil
}

// PublishBid stores a bid and publishes it to Redis Pub/Sub.
// JSON-RPC: mvl_publishBid
func (a *PublicAPI) PublishBid(msg BidMessage) (bool, error) {
	if msg.Type == "" {
		msg.Type = "bid"
	}
	if err := validateBidMessage(msg); err != nil {
		return false, err
	}
	ctx := context.Background()
	if err := storeBid(ctx, msg); err != nil {
		return false, err
	}
	gossipBroadcast(msg)
	return true, nil
}

// PublishCancel publishes a cancel request to Redis Pub/Sub.
// JSON-RPC: mvl_publishCancel
func (a *PublicAPI) PublishCancel(msg CancelMessage) (bool, error) {
	if msg.Type == "" {
		msg.Type = "cancel"
	}
	if err := validateCancelMessage(msg); err != nil {
		return false, err
	}
	ctx := context.Background()
	if err := publishCancel(ctx, msg); err != nil {
		return false, err
	}
	gossipBroadcast(msg)
	return true, nil
}

// ListItems returns active auctions.
// JSON-RPC: mvl_listItems
func (a *PublicAPI) ListItems(limit int64, cursor string) (*ListItemsResponse, error) {
	if limit <= 0 {
		limit = 20
	}
	ctx := context.Background()
	ids, next, err := SScanRedis(ctx, "auction:active", cursor, limit)
	if err != nil {
		return nil, err
	}

	items := make([]ListItem, 0, len(ids))
	for _, id := range ids {
		bz, err := GetRedis(ctx, askKey(id))
		if err != nil {
			continue
		}
		var topBid *json.RawMessage
		members, err := ZRevRangeRedis(ctx, bidsKey(id), 0, 0)
		if err == nil && len(members) > 0 {
			bidID := bidIDFromMember(members[0])
			if bidID != "" {
				bzBid, err := GetRedis(ctx, bidKey(id, bidID))
				if err == nil {
					raw := json.RawMessage(bzBid)
					topBid = &raw
				}
			}
		}
		items = append(items, ListItem{Ask: json.RawMessage(bz), TopBid: topBid})
	}
	return &ListItemsResponse{Items: items, Cursor: next}, nil
}

// ListBids returns bids for a specific auction ordered by price then time (desc).
// JSON-RPC: mvl_listBids
func (a *PublicAPI) ListBids(auctionID string, offset, limit int64) (*ListBidsResponse, error) {
	if auctionID == "" {
		return nil, fmt.Errorf("invalid auction_id")
	}
	if limit <= 0 {
		limit = 50
	}
	start := offset
	stop := offset + limit - 1

	ctx := context.Background()
	members, err := ZRevRangeRedis(ctx, bidsKey(auctionID), start, stop)
	if err != nil {
		return nil, err
	}

	bids := make([]json.RawMessage, 0, len(members))
	for _, m := range members {
		bidID := bidIDFromMember(m)
		if bidID == "" {
			continue
		}
		bz, err := GetRedis(ctx, bidKey(auctionID, bidID))
		if err != nil {
			continue
		}
		bids = append(bids, json.RawMessage(bz))
	}
	next := offset + int64(len(members))
	return &ListBidsResponse{Bids: bids, Cursor: fmt.Sprintf("%d", next)}, nil
}

// GetAuction returns a confirmed on-chain auction result.
// JSON-RPC: mvl_getAuction
func (a *PublicAPI) GetAuction(auctionID string) (json.RawMessage, error) {
	ctx := context.Background()
	res, err := a.auctionQ.Auction(ctx, &auctiontypes.QueryAuctionRequest{AuctionId: auctionID})
	if err != nil {
		return nil, err
	}
	bz, err := a.cdc.MarshalJSON(res)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(bz), nil
}

// ListAuctions returns confirmed auctions plus currently active asks.
// JSON-RPC: mvl_listAuctions
func (a *PublicAPI) ListAuctions(page *query.PageRequest) (json.RawMessage, error) {
	ctx := context.Background()
	res, err := a.auctionQ.Auctions(ctx, &auctiontypes.QueryAuctionsRequest{Pagination: page})
	if err != nil {
		return nil, err
	}
	confirmedBz, err := a.cdc.MarshalJSON(res)
	if err != nil {
		return nil, err
	}

	ids, next, err := SScanRedis(ctx, "auction:active", "0", 50)
	if err != nil {
		return nil, err
	}
	active := make([]json.RawMessage, 0, len(ids))
	for _, id := range ids {
		bz, err := GetRedis(ctx, askKey(id))
		if err != nil {
			continue
		}
		active = append(active, json.RawMessage(bz))
	}

	resp := ListAuctionsResponse{
		Confirmed:    json.RawMessage(confirmedBz),
		Active:       active,
		ActiveCursor: next,
	}
	bz, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(bz), nil
}

// Validators returns staking validators with optional pagination.
// JSON-RPC: mvl_validators
func (a *PublicAPI) Validators(status string, page *query.PageRequest) (json.RawMessage, error) {
	ctx := context.Background()
	req := &stakingtypes.QueryValidatorsRequest{
		Status:     status,
		Pagination: page,
	}
	res, err := a.stakingQ.Validators(ctx, req)
	if err != nil {
		return nil, err
	}

	bz, err := a.cdc.MarshalJSON(res)
	if err != nil {
		return nil, err
	}

	return json.RawMessage(bz), nil
}

func askKey(auctionID string) string {
	return "auction:ask:" + auctionID
}

func bidsKey(auctionID string) string {
	return "auction:bids:" + auctionID
}

func sellerKey(seller string) string {
	return "auction:by_seller:" + strings.ToLower(seller)
}

func bidKey(auctionID, bidID string) string {
	return "auction:bid:" + auctionID + ":" + bidID
}

func bidMember(price string, ts int64, bidID string) string {
	return fmt.Sprintf("%040s:%020d:%s", price, ts, bidID)
}

func bidIDFromMember(member string) string {
	idx := strings.LastIndex(member, ":")
	if idx < 0 || idx == len(member)-1 {
		return ""
	}
	return member[idx+1:]
}

func validateHex(addr string) error {
	if !common.IsHexAddress(addr) {
		return fmt.Errorf("invalid hex address")
	}
	return nil
}

func validateAskMessage(msg AskMessage) error {
	if err := validateHex(msg.Seller); err != nil {
		return err
	}
	if msg.AuctionID == "" || msg.EndHeight <= 0 {
		return fmt.Errorf("invalid auction_id or end_height")
	}
	if msg.Denom == "" {
		return fmt.Errorf("invalid denom")
	}
	if err := validateSigFormat(msg.Signature); err != nil {
		return err
	}
	if err := verifySig(msg.Seller, askPayload(msg), msg.Signature); err != nil {
		return err
	}
	return nil
}

func validateBidMessage(msg BidMessage) error {
	if err := validateHex(msg.Bidder); err != nil {
		return err
	}
	if msg.AuctionID == "" || msg.EndHeight <= 0 || msg.Price == "" {
		return fmt.Errorf("invalid auction_id, end_height, or price")
	}
	if _, err := normalizePrice(msg.Price); err != nil {
		return err
	}
	if err := validateSigFormat(msg.Signature); err != nil {
		return err
	}
	if err := verifySig(msg.Bidder, bidPayload(msg), msg.Signature); err != nil {
		return err
	}
	return nil
}

func validateCancelMessage(msg CancelMessage) error {
	if err := validateHex(msg.Actor); err != nil {
		return err
	}
	if msg.AuctionID == "" {
		return fmt.Errorf("invalid auction_id")
	}
	if err := validateSigFormat(msg.Signature); err != nil {
		return err
	}
	if err := verifySig(msg.Actor, cancelPayload(msg), msg.Signature); err != nil {
		return err
	}
	return nil
}

func storeAsk(ctx context.Context, msg AskMessage) error {
	askKey := askKey(msg.AuctionID)
	if err := SetRedis(ctx, askKey, msg); err != nil {
		return err
	}
	if err := SAddRedis(ctx, "auction:active", msg.AuctionID); err != nil {
		return err
	}
	if err := SAddRedis(ctx, sellerKey(msg.Seller), msg.AuctionID); err != nil {
		return err
	}
	if err := PublishRedis(ctx, "auction:ask", msg); err != nil {
		return err
	}
	return nil
}

func storeBid(ctx context.Context, msg BidMessage) error {
	price, err := normalizePrice(msg.Price)
	if err != nil {
		return err
	}
	bidID := fmt.Sprintf("%s-%d", msg.Bidder, NowUnixMilli())
	bidKey := bidKey(msg.AuctionID, bidID)
	if err := SetRedis(ctx, bidKey, msg); err != nil {
		return err
	}
	member := bidMember(price, NowUnixMilli(), bidID)
	if err := ZAddRedis(ctx, bidsKey(msg.AuctionID), 0, member); err != nil {
		return err
	}
	if err := PublishRedis(ctx, "auction:bid", msg); err != nil {
		return err
	}
	return nil
}

func publishCancel(ctx context.Context, msg CancelMessage) error {
	return PublishRedis(ctx, "auction:cancel", msg)
}

func validateSigFormat(sig string) error {
	bz, err := hexutil.Decode(sig)
	if err != nil || len(bz) != 65 {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func verifySig(addr, payload, sigHex string) error {
	hash := accounts.TextHash([]byte(payload))
	sig, err := hexutil.Decode(sigHex)
	if err != nil || len(sig) != 65 {
		return fmt.Errorf("invalid signature")
	}
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	pub, err := crypto.SigToPub(hash, sig)
	if err != nil {
		return fmt.Errorf("invalid signature")
	}
	recovered := crypto.PubkeyToAddress(*pub).Hex()
	if strings.ToLower(recovered) != strings.ToLower(addr) {
		return fmt.Errorf("invalid signature")
	}
	return nil
}

func askPayload(msg AskMessage) string {
	return strings.Join([]string{
		msg.AuctionID,
		strings.ToLower(msg.Seller),
		msg.MinPrice,
		msg.Denom,
		fmt.Sprintf("%d", msg.EndHeight),
	}, "|")
}

func bidPayload(msg BidMessage) string {
	return strings.Join([]string{
		msg.AuctionID,
		strings.ToLower(msg.Bidder),
		msg.Price,
		fmt.Sprintf("%d", msg.EndHeight),
	}, "|")
}

func cancelPayload(msg CancelMessage) string {
	return strings.Join([]string{
		msg.AuctionID,
		strings.ToLower(msg.Actor),
	}, "|")
}

func normalizePrice(price string) (string, error) {
	if price == "" {
		return "", fmt.Errorf("empty price")
	}
	for _, r := range price {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("price must be base-10 digits")
		}
	}
	if len(price) > 40 {
		return "", fmt.Errorf("price too large")
	}
	return price, nil
}
