package server

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cometbft/cometbft/p2p"
	"github.com/cosmos/evm/server/matchboard"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

type mockGossipIngestor struct {
	called     bool
	intentType string
	envelope   matchboard.GossipEnvelope
	origin     string
}

func (m *mockGossipIngestor) IngestGossipEnvelope(_ context.Context, intentType string, envelope matchboard.GossipEnvelope, origin string) error {
	m.called = true
	m.intentType = intentType
	m.envelope = envelope
	m.origin = origin
	return nil
}

func TestMatchboardGossipReactorGetChannels(t *testing.T) {
	t.Parallel()

	r := newMatchboardGossipReactor(log.NewNopLogger(), matchboardGossipReactorChannelID)
	channels := r.GetChannels()
	require.Len(t, channels, 1)
	require.Equal(t, matchboardGossipReactorChannelID, channels[0].ID)
	_, ok := channels[0].MessageType.(*wrapperspb.BytesValue)
	require.True(t, ok)
}

func TestMatchboardGossipReactorReceive(t *testing.T) {
	t.Parallel()

	r := newMatchboardGossipReactor(log.NewNopLogger(), matchboardGossipReactorChannelID)
	mock := &mockGossipIngestor{}
	r.SetIngestor(mock)

	msg := matchboardP2PGossipMessage{
		IntentType: matchboard.IntentTypeRequest,
		Envelope: matchboard.GossipEnvelope{
			MessageID:  "msg-1",
			IntentType: matchboard.IntentTypeRequest,
			Payload:    []byte(`{"pool_id":"pool-1","intent_id":"intent-1"}`),
		},
	}
	bz, err := json.Marshal(msg)
	require.NoError(t, err)

	r.Receive(p2p.Envelope{
		ChannelID: matchboardGossipReactorChannelID,
		Message:   &wrapperspb.BytesValue{Value: bz},
	})

	require.True(t, mock.called)
	require.Equal(t, matchboard.IntentTypeRequest, mock.intentType)
	require.Equal(t, "msg-1", mock.envelope.MessageID)
	require.Equal(t, "p2p", mock.origin)
}
