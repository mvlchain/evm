package types

import "errors"

var (
	ErrInvalidCellTopic     = errors.New("invalid cell topic: must be 32 bytes")
	ErrInvalidPickupCommit  = errors.New("invalid pickup commit: must be 32 bytes")
	ErrInvalidDropoffCommit = errors.New("invalid dropoff commit: must be 32 bytes")
	ErrInvalidDriverCommit  = errors.New("invalid driver commit: must be 32 bytes")
	ErrRequestNotFound      = errors.New("request not found")
	ErrRequestExpired       = errors.New("request has expired")
	ErrSessionNotFound      = errors.New("session not found")
	ErrInvalidReveal        = errors.New("invalid reveal: commitment mismatch")
	ErrNoMatchingDriver     = errors.New("no matching driver found")
	ErrInsufficientDeposit  = errors.New("insufficient deposit")
)
