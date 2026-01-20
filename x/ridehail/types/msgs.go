package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

// MsgCreateRequest - Rider creates a ride request at core level
type MsgCreateRequest struct {
	Rider         string `json:"rider"`
	CellTopic     []byte `json:"cell_topic"`
	RegionTopic   []byte `json:"region_topic"`
	ParamsHash    []byte `json:"params_hash"`
	PickupCommit  []byte `json:"pickup_commit"`
	DropoffCommit []byte `json:"dropoff_commit"`
	MaxDriverEta  uint32 `json:"max_driver_eta"`
	Ttl           uint32 `json:"ttl"`
	Deposit       string `json:"deposit"` // Cosmos SDK coin format
}

func (msg MsgCreateRequest) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Rider); err != nil {
		return err
	}
	if len(msg.CellTopic) != 32 {
		return ErrInvalidCellTopic
	}
	if len(msg.PickupCommit) != 32 {
		return ErrInvalidPickupCommit
	}
	if len(msg.DropoffCommit) != 32 {
		return ErrInvalidDropoffCommit
	}
	return nil
}

func (msg MsgCreateRequest) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Rider)
	return []sdk.AccAddress{addr}
}

// MsgSubmitDriverCommit - Driver submits commitment at core level
type MsgSubmitDriverCommit struct {
	Driver       string `json:"driver"`
	RequestId    uint64 `json:"request_id"`
	DriverCommit []byte `json:"driver_commit"`
	Eta          uint32 `json:"eta"`
}

func (msg MsgSubmitDriverCommit) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Driver); err != nil {
		return err
	}
	if len(msg.DriverCommit) != 32 {
		return ErrInvalidDriverCommit
	}
	return nil
}

func (msg MsgSubmitDriverCommit) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Driver)
	return []sdk.AccAddress{addr}
}

// MsgRevealPickup - Rider reveals pickup location
type MsgRevealPickup struct {
	Rider       string `json:"rider"`
	SessionId   uint64 `json:"session_id"`
	PickupCoord []byte `json:"pickup_coord"`
	PickupSalt  []byte `json:"pickup_salt"`
}

func (msg MsgRevealPickup) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Rider); err != nil {
		return err
	}
	return nil
}

func (msg MsgRevealPickup) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Rider)
	return []sdk.AccAddress{addr}
}

// MsgRevealDropoff - Rider reveals dropoff location
type MsgRevealDropoff struct {
	Rider        string `json:"rider"`
	SessionId    uint64 `json:"session_id"`
	DropoffCoord []byte `json:"dropoff_coord"`
	DropoffSalt  []byte `json:"dropoff_salt"`
}

func (msg MsgRevealDropoff) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Rider); err != nil {
		return err
	}
	return nil
}

func (msg MsgRevealDropoff) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Rider)
	return []sdk.AccAddress{addr}
}

// Response types
type MsgCreateRequestResponse struct {
	RequestId uint64 `json:"request_id"`
}

type MsgSubmitDriverCommitResponse struct {
	Success bool `json:"success"`
}

type MsgRevealPickupResponse struct {
	Success bool `json:"success"`
}

type MsgRevealDropoffResponse struct {
	Success bool `json:"success"`
}
