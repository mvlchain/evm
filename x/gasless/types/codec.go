package types

import (
	"github.com/cosmos/gogoproto/proto"

	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
)

var (
	amino = codec.NewLegacyAmino()
)

func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	// no concrete msgs yet; keep for future extension
}

func RegisterInterfaces(reg types.InterfaceRegistry) {
	// no concrete msgs yet; keep for future extension
	_ = proto.Marshal
}
