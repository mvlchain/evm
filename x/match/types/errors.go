package types

import (
	errorsmod "cosmossdk.io/errors"
)

const (
	// Suggested protocol-aligned error identifiers from docs/spec.md.
	SuggestedCodeInvalidRequest   = "MCH-1000"
	SuggestedCodeInvalidSignature = "MCH-1101"
	SuggestedCodeSignerMismatch   = "MCH-1102"
	SuggestedCodeHashMismatch     = "MCH-1200"
	SuggestedCodeExpired          = "MCH-1201"
	SuggestedCodeReplayDetected   = "MCH-1202"
	SuggestedCodeChainRejected    = "MCH-1400"
)

const (
	CodeInvalidRequest uint32 = 1000
	CodeInvalidSig     uint32 = 1101
	CodeSignerMismatch uint32 = 1102
	CodeHashMismatch   uint32 = 1200
	CodeExpired        uint32 = 1201
	CodeReplayDetected uint32 = 1202
	CodeChainRejected  uint32 = 1400
)

var (
	ErrInvalidRequest   = errorsmod.Register(ModuleName, CodeInvalidRequest, "invalid request")
	ErrInvalidSignature = errorsmod.Register(ModuleName, CodeInvalidSig, "invalid signature")
	ErrSignerMismatch   = errorsmod.Register(ModuleName, CodeSignerMismatch, "signer mismatch")
	ErrHashMismatch     = errorsmod.Register(ModuleName, CodeHashMismatch, "hash mismatch")
	ErrExpired          = errorsmod.Register(ModuleName, CodeExpired, "artifact expired")
	ErrReplayDetected   = errorsmod.Register(ModuleName, CodeReplayDetected, "replay detected")
	ErrChainRejected    = errorsmod.Register(ModuleName, CodeChainRejected, "chain rejected certificate")
)
