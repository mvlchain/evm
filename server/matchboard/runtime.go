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

const (
	defaultTokenAlice     = "token-alice"
	defaultTokenBob       = "token-bob"
	defaultPrincipalAlice = "0xC6Fe5D33615a1C52c08018c47E8Bc53646A0E101"
	defaultPrincipalBob   = "0x963EBDf2e1f8DB8707D05FC75bfeFFBa1B5BaC17"
)

// RuntimeConfig controls matchboard process-level startup behavior.
type RuntimeConfig struct {
	Address string
	Handler Config
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
	tokenMap, err := loadTokenPrincipalMap(lookupEnv)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid token configuration: %w", err)
	}

	rateLimitRequests, err := intEnv(lookupEnv, "MATCHBOARD_RATE_LIMIT_REQUESTS", 60)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_RATE_LIMIT_REQUESTS: %w", err)
	}
	rateLimitWindow, err := durationEnv(lookupEnv, "MATCHBOARD_RATE_LIMIT_WINDOW", time.Minute)
	if err != nil {
		return RuntimeConfig{}, fmt.Errorf("invalid MATCHBOARD_RATE_LIMIT_WINDOW: %w", err)
	}

	return RuntimeConfig{
		Address: addr,
		Handler: Config{
			TokenPrincipalMap: tokenMap,
			RateLimitRequests: rateLimitRequests,
			RateLimitWindow:   rateLimitWindow,
			Logger:            logger,
		},
	}, nil
}

// StartServer starts matchboard and gracefully shuts down when ctx is cancelled.
func StartServer(ctx context.Context, cfg RuntimeConfig) error {
	h, err := NewHandler(cfg.Handler)
	if err != nil {
		return fmt.Errorf("failed to build matchboard handler: %w", err)
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

func loadTokenPrincipalMap(lookupEnv func(string) (string, bool)) (map[string]string, error) {
	if raw := strings.TrimSpace(getEnvWithDefault(lookupEnv, "MATCHBOARD_TOKEN_MAP", "")); raw != "" {
		out := make(map[string]string)
		pairs := strings.Split(raw, ",")
		for _, pair := range pairs {
			pair = strings.TrimSpace(pair)
			if pair == "" {
				continue
			}

			parts := strings.SplitN(pair, "=", 2)
			if len(parts) != 2 {
				return nil, strconv.ErrSyntax
			}

			token := strings.TrimSpace(parts[0])
			principal := strings.TrimSpace(parts[1])
			if token == "" || principal == "" {
				return nil, strconv.ErrSyntax
			}
			out[token] = principal
		}
		if len(out) == 0 {
			return nil, strconv.ErrSyntax
		}
		return out, nil
	}

	aliceToken := strings.TrimSpace(getEnvWithDefault(lookupEnv, "MATCHBOARD_TOKEN_ALICE", defaultTokenAlice))
	bobToken := strings.TrimSpace(getEnvWithDefault(lookupEnv, "MATCHBOARD_TOKEN_BOB", defaultTokenBob))
	alicePrincipal := strings.TrimSpace(getEnvWithDefault(lookupEnv, "MATCHBOARD_PRINCIPAL_ALICE", defaultPrincipalAlice))
	bobPrincipal := strings.TrimSpace(getEnvWithDefault(lookupEnv, "MATCHBOARD_PRINCIPAL_BOB", defaultPrincipalBob))

	if aliceToken == "" || bobToken == "" || alicePrincipal == "" || bobPrincipal == "" {
		return nil, strconv.ErrSyntax
	}

	return map[string]string{
		aliceToken: alicePrincipal,
		bobToken:   bobPrincipal,
	}, nil
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
