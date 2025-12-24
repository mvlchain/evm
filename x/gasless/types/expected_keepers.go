package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type BankKeeper interface {
	SendCoinsFromAccountToModule(ctx context.Context, fromAddr sdk.AccAddress, moduleName string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, moduleName string, toAddr sdk.AccAddress, amt sdk.Coins) error
}

type AccountKeeper interface {
	GetModuleAddress(moduleName string) sdk.AccAddress
}

type EVMKeeper interface {
	// placeholder for future EVM-related queries
}
