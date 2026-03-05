package main

import (
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cosmos/evm/server/matchboard"
)

const (
	defaultAddr = ":8080"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	addr := strings.TrimSpace(getEnvWithDefault("MATCHBOARD_ADDR", defaultAddr))
	tokenMap, err := loadTokenPrincipalMap()
	if err != nil {
		logger.Error("invalid token configuration", "err", err)
		os.Exit(1)
	}

	rateLimitRequests, err := intEnv("MATCHBOARD_RATE_LIMIT_REQUESTS", 60)
	if err != nil {
		logger.Error("invalid MATCHBOARD_RATE_LIMIT_REQUESTS", "err", err)
		os.Exit(1)
	}
	rateLimitWindow, err := durationEnv("MATCHBOARD_RATE_LIMIT_WINDOW", time.Minute)
	if err != nil {
		logger.Error("invalid MATCHBOARD_RATE_LIMIT_WINDOW", "err", err)
		os.Exit(1)
	}

	h, err := matchboard.NewHandler(matchboard.Config{
		TokenPrincipalMap: tokenMap,
		RateLimitRequests: rateLimitRequests,
		RateLimitWindow:   rateLimitWindow,
		Logger:            logger,
	})
	if err != nil {
		logger.Error("failed to build matchboard handler", "err", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("/", h)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	logger.Info("starting matchboard server", "addr", addr, "rate_limit_requests", rateLimitRequests, "rate_limit_window", rateLimitWindow.String())
	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("matchboard server exited", "err", err)
		os.Exit(1)
	}
}

func loadTokenPrincipalMap() (map[string]string, error) {
	if raw := strings.TrimSpace(os.Getenv("MATCHBOARD_TOKEN_MAP")); raw != "" {
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

	aliceToken := strings.TrimSpace(getEnvWithDefault("MATCHBOARD_TOKEN_ALICE", "token-alice"))
	bobToken := strings.TrimSpace(getEnvWithDefault("MATCHBOARD_TOKEN_BOB", "token-bob"))
	alicePrincipal := strings.TrimSpace(getEnvWithDefault("MATCHBOARD_PRINCIPAL_ALICE", "alice"))
	bobPrincipal := strings.TrimSpace(getEnvWithDefault("MATCHBOARD_PRINCIPAL_BOB", "bob"))

	if aliceToken == "" || bobToken == "" || alicePrincipal == "" || bobPrincipal == "" {
		return nil, strconv.ErrSyntax
	}

	return map[string]string{
		aliceToken: alicePrincipal,
		bobToken:   bobPrincipal,
	}, nil
}

func intEnv(key string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func durationEnv(key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback, nil
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	return v, nil
}

func getEnvWithDefault(key, fallback string) string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	return raw
}
