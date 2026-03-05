package match

import (
	"encoding/json"
	"fmt"

	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"
	"github.com/spf13/cobra"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/cosmos/evm/x/match/keeper"
	"github.com/cosmos/evm/x/match/types"

	"cosmossdk.io/core/appmodule"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

const consensusVersion = 1

var (
	_ module.AppModule      = AppModule{} //nolint:staticcheck // keep for legacy purposes
	_ module.AppModuleBasic = AppModuleBasic{}
	_ module.HasABCIGenesis = AppModule{}

	_ appmodule.AppModule = AppModule{}
)

// AppModuleBasic defines the basic application module used by x/match.
type AppModuleBasic struct{}

// Name returns the x/match module name.
func (AppModuleBasic) Name() string {
	return types.ModuleName
}

// RegisterLegacyAminoCodec registers x/match legacy amino types.
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	types.RegisterLegacyAminoCodec(cdc)
}

// RegisterInterfaces registers x/match interfaces and msg service descriptors.
func (AppModuleBasic) RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	types.RegisterInterfaces(registry)
}

// ConsensusVersion returns the x/match consensus version.
func (AppModuleBasic) ConsensusVersion() uint64 {
	return consensusVersion
}

// DefaultGenesis returns default genesis state as raw bytes for x/match.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	bz, err := json.Marshal(types.DefaultGenesisState())
	if err != nil {
		panic(fmt.Errorf("failed to marshal default %s genesis state: %w", types.ModuleName, err))
	}
	return bz
}

// ValidateGenesis validates x/match genesis state.
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var genesisState types.GenesisState
	if err := json.Unmarshal(bz, &genesisState); err != nil {
		return fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err)
	}

	return genesisState.Validate()
}

// RegisterRESTRoutes performs a no-op as x/match doesn't expose REST endpoints.
func (AppModuleBasic) RegisterRESTRoutes(_ client.Context, _ *mux.Router) {}

// RegisterGRPCGatewayRoutes performs a no-op as x/match doesn't expose query endpoints yet.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(_ client.Context, _ *runtime.ServeMux) {}

// GetTxCmd returns the root tx command for x/match.
func (AppModuleBasic) GetTxCmd() *cobra.Command { return nil }

// GetQueryCmd returns the root query command for x/match.
func (AppModuleBasic) GetQueryCmd() *cobra.Command { return nil }

// AppModule implements the application module for x/match.
type AppModule struct {
	AppModuleBasic
	keeper keeper.Keeper
}

// NewAppModule creates a new x/match AppModule.
func NewAppModule(k keeper.Keeper) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keeper:         k,
	}
}

// Name returns the x/match module name.
func (AppModule) Name() string {
	return types.ModuleName
}

// RegisterServices registers x/match gRPC Msg services.
func (am AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), keeper.NewMsgServerImpl(am.keeper))
}

// InitGenesis performs x/match genesis initialization.
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data json.RawMessage) []abci.ValidatorUpdate {
	var genesisState types.GenesisState
	if err := json.Unmarshal(data, &genesisState); err != nil {
		panic(fmt.Errorf("failed to unmarshal %s genesis state: %w", types.ModuleName, err))
	}
	InitGenesis(ctx, am.keeper, genesisState)
	return []abci.ValidatorUpdate{}
}

// ExportGenesis exports x/match genesis state as raw JSON.
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) json.RawMessage {
	gs := ExportGenesis(ctx, am.keeper)
	bz, err := json.Marshal(gs)
	if err != nil {
		panic(fmt.Errorf("failed to marshal %s genesis state: %w", types.ModuleName, err))
	}
	return bz
}

// IsAppModule implements the appmodule.AppModule interface.
func (am AppModule) IsAppModule() {}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (am AppModule) IsOnePerModuleType() {}
