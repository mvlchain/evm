package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

var (
	amino = codec.NewLegacyAmino()
	// ModuleCdc references the global match module codec.
	ModuleCdc = codec.NewProtoCodec(codectypes.NewInterfaceRegistry())

	// AminoCdc is an amino codec created to support amino JSON compatible msgs.
	AminoCdc = codec.NewAminoCodec(amino) //nolint:staticcheck
)

const (
	submitMatchCertificateName = "cosmos/evm/x/match/MsgSubmitMatchCertificate"
)

func init() {
	RegisterLegacyAminoCodec(amino)
	amino.Seal()
}

// RegisterInterfaces registers match interfaces and msg service descriptors.
func RegisterInterfaces(registry codectypes.InterfaceRegistry) {
	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgSubmitMatchCertificate{},
	)
	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}

// RegisterLegacyAminoCodec registers x/match concrete types on amino.
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgSubmitMatchCertificate{}, submitMatchCertificateName, nil)
}
