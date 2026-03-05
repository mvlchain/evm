package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/cosmos/evm/server/matchboard"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	cfg, err := matchboard.RuntimeConfigFromEnv(logger, os.LookupEnv)
	if err != nil {
		logger.Error("invalid matchboard runtime configuration", "err", err)
		os.Exit(1)
	}

	if err := matchboard.StartServer(context.Background(), cfg); err != nil {
		logger.Error("matchboard server exited", "err", err)
		os.Exit(1)
	}
}
