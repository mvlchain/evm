package ante

import (
	evmante "github.com/cosmos/evm/ante/evm"
	"github.com/cosmos/evm/ante/gasless"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// newMonoEVMAnteHandler creates the sdk.AnteHandler implementation for the EVM transactions.
func newMonoEVMAnteHandler(ctx sdk.Context, options HandlerOptions) sdk.AnteHandler {
	evmParams := options.EvmKeeper.GetParams(ctx)
	feemarketParams := options.FeeMarketKeeper.GetParams(ctx)

	decorators := []sdk.AnteDecorator{}

	// Add gasless decorator first if gasless keeper is available
	if options.GaslessKeeper != nil {
		decorators = append(decorators, gasless.NewGaslessDecorator(options.GaslessKeeper))
	}

	// Add main EVM decorator
	decorators = append(decorators,
		evmante.NewEVMMonoDecorator(
			options.AccountKeeper,
			options.FeeMarketKeeper,
			options.EvmKeeper,
			options.MaxTxGasWanted,
			&evmParams,
			&feemarketParams,
		),
		NewTxListenerDecorator(options.PendingTxListener),
	)

	return sdk.ChainAnteDecorators(decorators...)
}
