package matchboard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultAddr = ":8080"
)


// RuntimeConfig controls matchboard process-level startup behavior.
type RuntimeConfig struct {
	Address string
	Handler Config

	// OnGossipIngestorReady is invoked when handler is constructed and supports node-native gossip ingestion.
	OnGossipIngestorReady func(GossipIngestor)
}

// RuntimeConfigFromEnv builds runtime config from MATCHBOARD_* environment variables.
func RuntimeConfigFromEnv(logger *slog.Logger, lookupEnv func(string) (string, bool)) (RuntimeConfig, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if lookupEnv == nil {
		return RuntimeConfig{}, errors.New("lookupEnv is required")
	}

	addr := strings.TrimSpace(getEnvWithDefault(lookupEnv, "MATCHBOARD_ADDR", DefaultAddr))

	rateLimitRequests, err := intEnv(lookupEnv, "MATCHBOARD_RATE_LIMIT_REQUESTS", 60)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_RATE_LIMIT_REQUESTS: %w", err)
	}
	rateLimitWindow, err := durationEnv(lookupEnv, "MATCHBOARD_RATE_LIMIT_WINDOW", time.Minute)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_RATE_LIMIT_WINDOW: %w", err)
	}
	gossipSecret := strings.TrimSpace(getEnvWithDefault(lookupEnv, "MATCHBOARD_GOSSIP_SHARED_SECRET", ""))
	gossipNodeID := strings.TrimSpace(getEnvWithDefault(lookupEnv, "MATCHBOARD_GOSSIP_NODE_ID", ""))
	gossipTimeout, err := durationEnv(lookupEnv, "MATCHBOARD_GOSSIP_TIMEOUT", defaultGossipTimeout)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_GOSSIP_TIMEOUT: %w", err)
	}
	gossipMessageTTL, err := durationEnv(lookupEnv, "MATCHBOARD_GOSSIP_MESSAGE_TTL", defaultGossipMessageTTL)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_GOSSIP_MESSAGE_TTL: %w", err)
	}
	gossipSeenTTL, err := durationEnv(lookupEnv, "MATCHBOARD_GOSSIP_SEEN_TTL", defaultGossipSeenTTL)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_GOSSIP_SEEN_TTL: %w", err)
	}
	gossipMaxHops, err := intEnv(lookupEnv, "MATCHBOARD_GOSSIP_MAX_HOPS", defaultGossipMaxHops)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_GOSSIP_MAX_HOPS: %w", err)
	}
	proposerABCIEnable, err := boolEnv(lookupEnv, "MATCHBOARD_PROPOSER_ABCI_ENABLE", false)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_PROPOSER_ABCI_ENABLE: %w", err)
	}
	intentStreamEnable, err := boolEnv(lookupEnv, "MATCHBOARD_INTENT_STREAM_ENABLE", true)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_INTENT_STREAM_ENABLE: %w", err)
	}
	intentStreamBuffer, err := intEnv(lookupEnv, "MATCHBOARD_INTENT_STREAM_BUFFER", defaultIntentStreamQueue)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_INTENT_STREAM_BUFFER: %w", err)
	}
	matcherShards, err := intEnv(lookupEnv, "MATCHBOARD_MATCHER_SHARDS", defaultMatcherShardCount)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_MATCHER_SHARDS: %w", err)
	}

	return RuntimeConfig{
		Address: addr,
		Handler: Config{
			RateLimitRequests:     rateLimitRequests,
			RateLimitWindow:       rateLimitWindow,
			Logger:                logger,
			GossipSharedSecret:    gossipSecret,
			GossipNodeID:          gossipNodeID,
			GossipTimeout:         gossipTimeout,
			GossipMessageTTL:      gossipMessageTTL,
			GossipSeenTTL:         gossipSeenTTL,
			GossipMaxHops:         gossipMaxHops,
			EnableABCIProposerOps: proposerABCIEnable,
			EnableIntentStream:    intentStreamEnable,
			IntentStreamBuffer:    intentStreamBuffer,
			MatcherShardCount:     matcherShards,
		},
	}, nil
}

// StartServer starts matchboard and gracefully shuts down when ctx is cancelled.
func StartServer(ctx context.Context, cfg RuntimeConfig) error {
	h, err := NewHandler(cfg.Handler)
	if err != nil {
		return fmt.Errorf("failed to build matchboard handler: %w", err)
	}
	if cfg.OnGossipIngestorReady != nil {
		ingestor, ok := h.(GossipIngestor)
		if !ok {
			return errors.New("matchboard handler does not support gossip ingestion")
		}
		cfg.OnGossipIngestorReady(ingestor)
	}
	logger := cfg.Handler.Logger
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	mux.Handle("/", h)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	srv := &http.Server{
		Addr:    strings.TrimSpace(cfg.Address),
		Handler: mux,
	}
	if srv.Addr == "" {
		srv.Addr = DefaultAddr
	}

	errCh := make(chan error, 1)
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		errCh <- srv.Shutdown(shutdownCtx)
	}()

	logger.Info(
		"starting matchboard server",
		"addr", srv.Addr,
		"rate_limit_requests", cfg.Handler.RateLimitRequests,
		"rate_limit_window", cfg.Handler.RateLimitWindow.String(),
		"intent_stream_enable", cfg.Handler.EnableIntentStream,
		"gossip_max_hops", cfg.Handler.GossipMaxHops,
		"gossip_message_ttl", cfg.Handler.GossipMessageTTL.String(),
		"gossip_seen_ttl", cfg.Handler.GossipSeenTTL.String(),
		"matcher_shards", cfg.Handler.MatcherShardCount,
	)

	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	default:
	}

	return nil
}


func intEnv(lookupEnv func(string) (string, bool), key string, fallback int) (int, error) {
	raw := strings.TrimSpace(getEnvWithDefault(lookupEnv, key, ""))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func durationEnv(lookupEnv func(string) (string, bool), key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(getEnvWithDefault(lookupEnv, key, ""))
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func getEnvWithDefault(lookupEnv func(string) (string, bool), key, fallback string) string {
	raw, ok := lookupEnv(key)
	if !ok {
		return fallback
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback
	}
	return raw
}

func boolEnv(lookupEnv func(string) (string, bool), key string, fallback bool) (bool, error) {
	raw := strings.TrimSpace(getEnvWithDefault(lookupEnv, key, ""))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, err
	}
	return v, nil
}
