package types

// PendingRequest represents a ride request waiting for driver commits
type PendingRequest struct {
	RequestId     uint64
	Rider         string
	CellTopic     []byte
	RegionTopic   []byte
	ParamsHash    []byte
	PickupCommit  []byte
	DropoffCommit []byte
	MaxDriverEta  uint32
	Ttl           uint32
	CreatedAt     int64
	ExpiresAt     int64
	Deposit       string
}

// DriverCommit represents a driver's commitment to a request
type DriverCommit struct {
	RequestId    uint64
	Driver       string
	DriverCommit []byte
	Eta          uint32
	SubmittedAt  int64
}

// Session represents a matched ride session
type Session struct {
	SessionId       uint64
	RequestId       uint64
	Rider           string
	Driver          string
	PickupRevealed  bool
	DropoffRevealed bool
	PickupCoord     []byte
	DropoffCoord    []byte
	Status          SessionStatus
	CreatedAt       int64
}

type SessionStatus uint8

const (
	SessionStatusPending SessionStatus = iota
	SessionStatusActive
	SessionStatusCompleted
	SessionStatusCancelled
)
