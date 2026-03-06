package server

import (
	"testing"

	cmtcfg "github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/p2p"
	p2pconn "github.com/cometbft/cometbft/p2p/conn"
	"github.com/stretchr/testify/require"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestDeriveMatchboardGossipPeersFromCometConfig(t *testing.T) {
	t.Parallel()

	cfg := cmtcfg.DefaultConfig()
	cfg.P2P.PersistentPeers = "id1@10.0.0.2:26656,id2@peer-b.local:26656"
	cfg.P2P.Seeds = "id3@seed-a.local:26656,id1@10.0.0.2:26656"

	peers, err := deriveMatchboardGossipPeersFromCometConfig(cfg, ":8080", nil)
	require.NoError(t, err)
	require.Equal(t, []string{
		"http://10.0.0.2:8080",
		"http://peer-b.local:8080",
		"http://seed-a.local:8080",
	}, peers)
}

func TestDeriveMatchboardGossipPeersFromCometConfig_WithEnvOverrides(t *testing.T) {
	t.Parallel()

	cfg := cmtcfg.DefaultConfig()
	cfg.P2P.PersistentPeers = "id1@10.0.0.3:26656"

	peers, err := deriveMatchboardGossipPeersFromCometConfig(cfg, ":8080", mapLookupEnv(map[string]string{
		matchboardGossipPeerPortEnv:   "18443",
		matchboardGossipPeerSchemeEnv: "https",
	}))
	require.NoError(t, err)
	require.Equal(t, []string{"https://10.0.0.3:18443"}, peers)
}

func TestDeriveMatchboardGossipPeersFromCometConfig_InvalidScheme(t *testing.T) {
	t.Parallel()

	cfg := cmtcfg.DefaultConfig()
	cfg.P2P.PersistentPeers = "id1@10.0.0.3:26656"

	_, err := deriveMatchboardGossipPeersFromCometConfig(cfg, ":8080", mapLookupEnv(map[string]string{
		matchboardGossipPeerSchemeEnv: "tcp",
	}))
	require.Error(t, err)
}

func TestResolveMatchboardPeerPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		addr    string
		env     map[string]string
		want    int
		wantErr bool
	}{
		{
			name: "default",
			addr: "",
			want: 8080,
		},
		{
			name: "colon port",
			addr: ":18080",
			want: 18080,
		},
		{
			name: "host port",
			addr: "0.0.0.0:19090",
			want: 19090,
		},
		{
			name: "url port",
			addr: "http://0.0.0.0:28080",
			want: 28080,
		},
		{
			name: "env override",
			addr: ":8080",
			env:  map[string]string{matchboardGossipPeerPortEnv: "9443"},
			want: 9443,
		},
		{
			name:    "invalid env override",
			addr:    ":8080",
			env:     map[string]string{matchboardGossipPeerPortEnv: "abc"},
			wantErr: true,
		},
		{
			name:    "invalid addr",
			addr:    "bad-addr",
			wantErr: true,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := resolveMatchboardPeerPort(tc.addr, mapLookupEnv(tc.env))
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestExtractPeerHost(t *testing.T) {
	t.Parallel()

	tests := []struct {
		raw    string
		want   string
		hasVal bool
	}{
		{raw: "id1@10.0.0.2:26656", want: "10.0.0.2", hasVal: true},
		{raw: "id2@peer-a.example.com:26656", want: "peer-a.example.com", hasVal: true},
		{raw: "http://peer-b.example.com:26656", want: "peer-b.example.com", hasVal: true},
		{raw: "[2001:db8::10]:26656", want: "2001:db8::10", hasVal: true},
		{raw: "", hasVal: false},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			got, ok := extractPeerHost(tc.raw)
			require.Equal(t, tc.hasVal, ok)
			require.Equal(t, tc.want, got)
		})
	}
}

type testReactor struct {
	*p2p.BaseReactor
	channels []*p2pconn.ChannelDescriptor
}

func newTestReactor(channels ...*p2pconn.ChannelDescriptor) *testReactor {
	r := &testReactor{channels: channels}
	r.BaseReactor = p2p.NewBaseReactor("test", r)
	return r
}

func (r *testReactor) GetChannels() []*p2pconn.ChannelDescriptor {
	return r.channels
}

func TestPickMatchboardGossipChannelID(t *testing.T) {
	t.Parallel()

	cfg := cmtcfg.TestConfig().P2P
	sw := p2p.MakeSwitch(cfg, 1, func(_ int, sw *p2p.Switch) *p2p.Switch { return sw })
	sw.AddReactor("existing", newTestReactor(&p2pconn.ChannelDescriptor{
		ID:          matchboardGossipReactorChannelID,
		Priority:    1,
		MessageType: &wrapperspb.BytesValue{},
	}))

	picked := pickMatchboardGossipChannelID(sw, matchboardGossipReactorChannelID)
	require.NotEqual(t, matchboardGossipReactorChannelID, picked)
	require.GreaterOrEqual(t, int(picked), 0x70)
}

func mapLookupEnv(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}
