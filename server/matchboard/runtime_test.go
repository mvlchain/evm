package matchboard

import (
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestRuntimeConfigFromEnvDefaults(t *testing.T) {
	t.Parallel()

	cfg, err := RuntimeConfigFromEnv(slog.Default(), mapLookupEnv(nil))
	require.NoError(t, err)
	require.Equal(t, DefaultAddr, cfg.Address)
	require.Equal(t, 60, cfg.Handler.RateLimitRequests)
	require.Equal(t, time.Minute, cfg.Handler.RateLimitWindow)
	require.Equal(t, defaultGossipTimeout, cfg.Handler.GossipTimeout)
	require.False(t, cfg.Handler.EnableABCIProposerOps)
	require.Equal(t, map[string]string{
		defaultTokenAlice: defaultPrincipalAlice,
		defaultTokenBob:   defaultPrincipalBob,
	}, cfg.Handler.TokenPrincipalMap)
}

func TestRuntimeConfigFromEnvTokenMap(t *testing.T) {
	t.Parallel()

	cfg, err := RuntimeConfigFromEnv(slog.Default(), mapLookupEnv(map[string]string{
		"MATCHBOARD_ADDR":                 "127.0.0.1:18080",
		"MATCHBOARD_TOKEN_MAP":            "token-a=alice,token-b=bob",
		"MATCHBOARD_RATE_LIMIT_REQUESTS":  "7",
		"MATCHBOARD_RATE_LIMIT_WINDOW":    "3s",
		"MATCHBOARD_GOSSIP_PEERS":         "http://127.0.0.1:18081,https://peer-2.example.com",
		"MATCHBOARD_GOSSIP_SHARED_SECRET": "secret-1",
		"MATCHBOARD_GOSSIP_NODE_ID":       "node-a",
		"MATCHBOARD_GOSSIP_TIMEOUT":       "5s",
		"MATCHBOARD_PROPOSER_ABCI_ENABLE": "true",
	}))
	require.NoError(t, err)
	require.Equal(t, "127.0.0.1:18080", cfg.Address)
	require.Equal(t, 7, cfg.Handler.RateLimitRequests)
	require.Equal(t, 3*time.Second, cfg.Handler.RateLimitWindow)
	require.Equal(t, []string{"http://127.0.0.1:18081", "https://peer-2.example.com"}, cfg.Handler.GossipPeers)
	require.Equal(t, "secret-1", cfg.Handler.GossipSharedSecret)
	require.Equal(t, "node-a", cfg.Handler.GossipNodeID)
	require.Equal(t, 5*time.Second, cfg.Handler.GossipTimeout)
	require.True(t, cfg.Handler.EnableABCIProposerOps)
	require.Equal(t, map[string]string{
		"token-a": "alice",
		"token-b": "bob",
	}, cfg.Handler.TokenPrincipalMap)
}

func TestRuntimeConfigFromEnvInvalidTokenMap(t *testing.T) {
	t.Parallel()

	_, err := RuntimeConfigFromEnv(slog.Default(), mapLookupEnv(map[string]string{
		"MATCHBOARD_TOKEN_MAP": "invalid",
	}))
	require.Error(t, err)
}

func TestRuntimeConfigFromEnvInvalidGossipTimeout(t *testing.T) {
	t.Parallel()

	_, err := RuntimeConfigFromEnv(slog.Default(), mapLookupEnv(map[string]string{
		"MATCHBOARD_GOSSIP_TIMEOUT": "bad",
	}))
	require.Error(t, err)
}

func TestRuntimeConfigFromEnvInvalidProposerABCIEnable(t *testing.T) {
	t.Parallel()

	_, err := RuntimeConfigFromEnv(slog.Default(), mapLookupEnv(map[string]string{
		"MATCHBOARD_PROPOSER_ABCI_ENABLE": "nope",
	}))
	require.Error(t, err)
}

func mapLookupEnv(values map[string]string) func(string) (string, bool) {
	return func(key string) (string, bool) {
		v, ok := values[key]
		return v, ok
	}
}
