package mvl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/p2p"
	"github.com/cometbft/cometbft/p2p/conn"
	gogotypes "github.com/cosmos/gogoproto/types"
)

const (
	gossipChannel   = byte(0x60)
	gossipMaxBytes  = 1 << 20
	gossipDedupTTL  = 5 * time.Minute
	gossipPruneTick = 1 * time.Minute
)

type GossipReactor struct {
	*p2p.BaseReactor
	logger    log.Logger
	mu        sync.Mutex
	seen      map[string]time.Time
	lastPrune time.Time
}

type gossipBase struct {
	Type string `json:"type"`
}

var (
	gossipMu  sync.RWMutex
	gossipRef *GossipReactor
)

func RegisterGossipReactor(r *GossipReactor) {
	gossipMu.Lock()
	gossipRef = r
	gossipMu.Unlock()
}

func gossipBroadcast(payload any) {
	gossipMu.RLock()
	r := gossipRef
	gossipMu.RUnlock()
	if r == nil {
		return
	}
	bz, err := json.Marshal(payload)
	if err != nil {
		return
	}
	r.BroadcastLocal(bz)
}

func NewGossipReactor() *GossipReactor {
	r := &GossipReactor{
		logger: log.NewNopLogger(),
		seen:   make(map[string]time.Time),
	}
	r.BaseReactor = p2p.NewBaseReactor("MVL_GOSSIP", r)
	return r
}

func (r *GossipReactor) SetLogger(l log.Logger) {
	r.logger = l
}

func (r *GossipReactor) GetChannels() []*conn.ChannelDescriptor {
	return []*conn.ChannelDescriptor{
		{
			ID:                  gossipChannel,
			Priority:            1,
			RecvMessageCapacity: gossipMaxBytes,
			MessageType:         &gogotypes.BytesValue{},
		},
	}
}

func (r *GossipReactor) Receive(e p2p.Envelope) {
	msg, ok := e.Message.(*gogotypes.BytesValue)
	if !ok || len(msg.Value) == 0 || len(msg.Value) > gossipMaxBytes {
		return
	}
	if !r.markSeen(msg.Value) {
		return
	}
	if err := r.handlePayload(msg.Value); err != nil {
		r.logger.Debug("gossip payload rejected", "err", err.Error())
		return
	}
	r.broadcast(msg.Value, e.Src)
}

func (r *GossipReactor) BroadcastLocal(payload []byte) {
	if len(payload) == 0 || len(payload) > gossipMaxBytes {
		return
	}
	if !r.markSeen(payload) {
		return
	}
	r.broadcast(payload, nil)
}

func (r *GossipReactor) broadcast(payload []byte, exclude p2p.Peer) {
	if r.Switch == nil {
		return
	}
	for _, peer := range r.Switch.Peers().List() {
		if exclude != nil && peer.ID() == exclude.ID() {
			continue
		}
		_ = peer.TrySend(p2p.Envelope{
			ChannelID: gossipChannel,
			Message:   &gogotypes.BytesValue{Value: payload},
		})
	}
}

func (r *GossipReactor) markSeen(payload []byte) bool {
	sum := sha256.Sum256(payload)
	key := hex.EncodeToString(sum[:])
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if ts, ok := r.seen[key]; ok && now.Sub(ts) < gossipDedupTTL {
		return false
	}
	r.seen[key] = now
	if r.lastPrune.IsZero() || now.Sub(r.lastPrune) >= gossipPruneTick {
		for k, ts := range r.seen {
			if now.Sub(ts) >= gossipDedupTTL {
				delete(r.seen, k)
			}
		}
		r.lastPrune = now
	}
	return true
}

func (r *GossipReactor) handlePayload(payload []byte) error {
	var base gossipBase
	if err := json.Unmarshal(payload, &base); err != nil {
		return err
	}
	ctx := context.Background()
	switch base.Type {
	case "ask":
		var msg AskMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			return err
		}
		if err := validateAskMessage(msg); err != nil {
			return err
		}
		return storeAsk(ctx, msg)
	case "bid":
		var msg BidMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			return err
		}
		if err := validateBidMessage(msg); err != nil {
			return err
		}
		return storeBid(ctx, msg)
	case "cancel":
		var msg CancelMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			return err
		}
		if err := validateCancelMessage(msg); err != nil {
			return err
		}
		return publishCancel(ctx, msg)
	default:
		return nil
	}
}
