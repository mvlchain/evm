package ridehail

import (
	"cosmossdk.io/core/appmodule"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	"github.com/cosmos/evm/x/ridehail/keeper"
	ridehailtypes "github.com/cosmos/evm/x/ridehail/types"
)

var (
	_ module.AppModuleBasic = AppModuleBasic{}
	_ appmodule.AppModule   = AppModule{}
)

// AppModuleBasic defines the basic application module used by the ridehail module.
type AppModuleBasic struct{}

// Name returns the ridehail module's name.
func (AppModuleBasic) Name() string {
	return ridehailtypes.ModuleName
}

// RegisterLegacyAminoCodec registers the ridehail module's types on the LegacyAmino codec.
func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {}

// RegisterInterfaces registers the module's interface types
func (b AppModuleBasic) RegisterInterfaces(registry types.InterfaceRegistry) {}

// DefaultGenesis returns default genesis state as raw bytes for the ridehail module.
func (AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) []byte {
	return []byte("{}")
}

// ValidateGenesis performs genesis state validation for the ridehail module.
func (AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz []byte) error {
	return nil
}

// RegisterGRPCGatewayRoutes registers the gRPC Gateway routes for the ridehail module.
func (AppModuleBasic) RegisterGRPCGatewayRoutes(_ client.Context, _ *runtime.ServeMux) {}

// ----------------------------------------------------------------------------
// AppModule
// ----------------------------------------------------------------------------

// AppModule implements an application module for the ridehail module.
type AppModule struct {
	AppModuleBasic
	keeper keeper.Keeper
}

// NewAppModule creates a new AppModule object
func NewAppModule(
	keeper keeper.Keeper,
) AppModule {
	return AppModule{
		AppModuleBasic: AppModuleBasic{},
		keeper:         keeper,
	}
}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (am AppModule) IsOnePerModuleType() {}

// IsAppModule implements the appmodule.AppModule interface.
func (am AppModule) IsAppModule() {}

// Name returns the ridehail module's name.
func (AppModule) Name() string {
	return ridehailtypes.ModuleName
}

// RegisterInvariants registers the ridehail module invariants.
func (am AppModule) RegisterInvariants(ir sdk.InvariantRegistry) {}

// RegisterServices registers module services.
func (am AppModule) RegisterServices(cfg module.Configurator) {
	// MsgServer implementation is available but not registered with gRPC yet
	// The precompile will call it directly
}

// BeginBlock processes matching logic at the start of each block
func (am AppModule) BeginBlock(ctx sdk.Context) error {
	return am.keeper.ProcessMatching(ctx)
}

// InitGenesis performs genesis initialization for the ridehail module.
func (am AppModule) InitGenesis(ctx sdk.Context, cdc codec.JSONCodec, data []byte) {
	// Initialize default state
	am.keeper.SetNextRequestId(ctx, 1)
	am.keeper.SetNextSessionId(ctx, 1)
}

// ExportGenesis returns the exported genesis state as raw bytes for the ridehail module.
func (am AppModule) ExportGenesis(ctx sdk.Context, cdc codec.JSONCodec) []byte {
	return []byte("{}")
}

// ConsensusVersion implements AppModule/ConsensusVersion.
func (AppModule) ConsensusVersion() uint64 { return 1 }
