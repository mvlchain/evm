package evmd

import (
	"fmt"

	evmmempool "github.com/cosmos/evm/mempool"
	"github.com/cosmos/evm/server"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	"cosmossdk.io/log/v2"

	"github.com/cosmos/cosmos-sdk/baseapp"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	sdkmempool "github.com/cosmos/cosmos-sdk/types/mempool"
)

// configureEVMMempool sets up the EVM mempool and related handlers using viper configuration.
func (app *EVMD) configureEVMMempool(appOpts servertypes.AppOptions, logger log.Logger) error {
	matchboardABCIEnabled := server.GetMatchboardProposerABCIEnable(appOpts, logger)

	if evmtypes.GetChainConfig() == nil {
		logger.Debug("evm chain config is not set, skipping mempool configuration")
		if matchboardABCIEnabled {
			app.configureMatchboardABCIHandlers(nil, logger)
		}
		return nil
	}

	cosmosPoolMaxTx := server.GetCosmosPoolMaxTx(appOpts, logger)
	if cosmosPoolMaxTx < 0 {
		logger.Debug("app-side mempool is disabled, skipping evm mempool configuration")
		if matchboardABCIEnabled {
			app.configureMatchboardABCIHandlers(nil, logger)
		}
		return nil
	}

	mempoolConfig, err := app.createMempoolConfig(appOpts, logger)
	if err != nil {
		return fmt.Errorf("failed to get mempool config: %w", err)
	}

	evmMempool := evmmempool.NewExperimentalEVMMempool(
		app.CreateQueryContext,
		logger,
		app.EVMKeeper,
		app.FeeMarketKeeper,
		app.txConfig,
		mempoolConfig,
		cosmosPoolMaxTx,
	)
	app.EVMMempool = evmMempool
	app.SetMempool(evmMempool)
	checkTxHandler := evmmempool.NewCheckTxHandler(evmMempool)
	app.SetCheckTxHandler(checkTxHandler)

	abciProposalHandler := baseapp.NewDefaultProposalHandler(evmMempool, app)
	abciProposalHandler.SetSignerExtractionAdapter(
		evmmempool.NewEthSignerExtractionAdapter(
			sdkmempool.NewDefaultSignerExtractionAdapter(),
		),
	)
	prepareHandler := abciProposalHandler.PrepareProposalHandler()
	processHandler := abciProposalHandler.ProcessProposalHandler()

	if matchboardABCIEnabled {
		matchHandler := newMatchProposalHandler(prepareHandler, processHandler, logger, defaultInjectedMatchOpsLimit)
		app.SetPrepareProposal(matchHandler.PrepareProposalHandler())
		app.SetProcessProposal(matchHandler.ProcessProposalHandler())
		app.matchProposerABCIEnabled = true
		logger.Info("enabled matchboard proposer abci injection handlers")
	} else {
		app.SetPrepareProposal(prepareHandler)
		app.SetProcessProposal(processHandler)
	}

	return nil
}

// configureMatchboardABCIHandlers sets up matchboard injection handlers wrapping the default proposal handlers.
// Used when the EVM mempool is disabled but matchboard ABCI injection is still required.
func (app *EVMD) configureMatchboardABCIHandlers(mempl sdkmempool.Mempool, logger log.Logger) {
	if mempl == nil {
		mempl = sdkmempool.NoOpMempool{}
	}
	defaultHandler := baseapp.NewDefaultProposalHandler(mempl, app)
	prepareHandler := defaultHandler.PrepareProposalHandler()
	processHandler := defaultHandler.ProcessProposalHandler()
	matchHandler := newMatchProposalHandler(prepareHandler, processHandler, logger, defaultInjectedMatchOpsLimit)
	app.SetPrepareProposal(matchHandler.PrepareProposalHandler())
	app.SetProcessProposal(matchHandler.ProcessProposalHandler())
	app.matchProposerABCIEnabled = true
	logger.Info("enabled matchboard proposer abci injection handlers (no-op mempool)")
}

// createMempoolConfig creates a new EVMMempoolConfig with the default configuration
// and overrides it with values from appOpts if they exist and are non-zero.
func (app *EVMD) createMempoolConfig(appOpts servertypes.AppOptions, logger log.Logger) (*evmmempool.EVMMempoolConfig, error) {
	return &evmmempool.EVMMempoolConfig{
		AnteHandler:      app.GetAnteHandler(),
		LegacyPoolConfig: server.GetLegacyPoolConfig(appOpts, logger),
		BlockGasLimit:    server.GetBlockGasLimit(appOpts, logger),
		MinTip:           server.GetMinTip(appOpts, logger),
	}, nil
}
