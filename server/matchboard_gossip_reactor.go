package server

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/cometbft/cometbft/p2p"
	p2pconn "github.com/cometbft/cometbft/p2p/conn"
	"github.com/cosmos/evm/server/matchboard"

	"cosmossdk.io/log/v2"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	matchboardGossipReactorName      = "MATCHBOARD_GOSSIP"
	matchboardGossipReactorChannelID = byte(0x72)
)

type matchboardP2PGossipMessage struct {
	IntentType string                    `json:"intent_type"`
	Envelope   matchboard.GossipEnvelope `json:"envelope"`
}

type matchboardGossipReactor struct {
	*p2p.BaseReactor

	logger    log.Logger
	channelID byte

	mu       sync.RWMutex
	ingestor matchboard.GossipIngestor
}

func newMatchboardGossipReactor(logger log.Logger, channelID byte) *matchboardGossipReactor {
	r := &matchboardGossipReactor{
		logger:    logger,
		channelID: channelID,
	}
	r.BaseReactor = p2p.NewBaseReactor("MatchboardGossipReactor", r)
	return r
}

func (r *matchboardGossipReactor) GetChannels() []*p2pconn.ChannelDescriptor {
	return []*p2pconn.ChannelDescriptor{
		{
			ID:                  r.channelID,
			Priority:            6,
			SendQueueCapacity:   1024,
			RecvBufferCapacity:  1024,
			RecvMessageCapacity: 1 << 20,
			MessageType:         &wrapperspb.BytesValue{},
		},
	}
}

func (r *matchboardGossipReactor) SetIngestor(ingestor matchboard.GossipIngestor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.ingestor = ingestor
}

func (r *matchboardGossipReactor) PublishGossip(ctx context.Context, intentType string, envelope matchboard.GossipEnvelope) {
	sw := r.Switch
	if sw == nil {
		return
	}

	wire := matchboardP2PGossipMessage{
		IntentType: strings.ToLower(strings.TrimSpace(intentType)),
		Envelope:   envelope,
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		r.logger.Warn("matchboard reactor gossip marshal failed", "error", err.Error())
		return
	}

	sw.BroadcastAsync(p2p.Envelope{
		ChannelID: r.channelID,
		Message:   &wrapperspb.BytesValue{Value: encoded},
	})
}

func (r *matchboardGossipReactor) Receive(envelope p2p.Envelope) {
	msg, ok := envelope.Message.(*wrapperspb.BytesValue)
	if !ok || msg == nil {
		r.logger.Warn("matchboard reactor received unexpected message type")
		return
	}

	var wire matchboardP2PGossipMessage
	if err := json.Unmarshal(msg.Value, &wire); err != nil {
		r.logger.Warn("matchboard reactor gossip unmarshal failed", "error", err.Error())
		return
	}

	intentType := strings.ToLower(strings.TrimSpace(wire.IntentType))
	if intentType == "" {
		intentType = strings.ToLower(strings.TrimSpace(wire.Envelope.IntentType))
	}
	if intentType == "" {
		r.logger.Warn("matchboard reactor gossip missing intent_type")
		return
	}

	r.mu.RLock()
	ingestor := r.ingestor
	r.mu.RUnlock()
	if ingestor == nil {
		return
	}

	origin := "p2p"
	if envelope.Src != nil {
		origin = string(envelope.Src.ID())
	}

	if err := ingestor.IngestGossipEnvelope(context.Background(), intentType, wire.Envelope, origin); err != nil {
		r.logger.Warn("matchboard reactor gossip ingest failed", "intent_type", intentType, "origin", origin, "error", err.Error())
	}
}
