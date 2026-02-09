package types

import (
	errorsmod "cosmossdk.io/errors"
)

var (
	ErrAuctionExists    = errorsmod.Register(ModuleName, 1, "auction already confirmed")
	ErrAuctionNotFound  = errorsmod.Register(ModuleName, 2, "auction not found")
	ErrInvalidAddress   = errorsmod.Register(ModuleName, 3, "invalid hex address")
	ErrInvalidPrice     = errorsmod.Register(ModuleName, 4, "invalid price")
	ErrInvalidEndHeight = errorsmod.Register(ModuleName, 5, "invalid end_height")
	ErrInvalidSignature = errorsmod.Register(ModuleName, 6, "invalid signature")
)
