package types

const (
	// ModuleName is the name of the ridehail module
	ModuleName = "ridehail"

	// StoreKey is the string store representation
	StoreKey = ModuleName
)

var (
	// KeyPrefixNextRequestId is the prefix for storing nextRequestId
	KeyPrefixNextRequestId = []byte{0x01}

	// KeyPrefixNextSessionId is the prefix for storing nextSessionId
	KeyPrefixNextSessionId = []byte{0x02}

	// KeyPrefixRequest is the prefix for storing requests
	KeyPrefixRequest = []byte{0x03}

	// KeyPrefixSession is the prefix for storing sessions
	KeyPrefixSession = []byte{0x04}

	// KeyPrefixPendingRequest is the prefix for pending requests (core matching)
	KeyPrefixPendingRequest = []byte{0x05}

	// KeyPrefixDriverCommit is the prefix for driver commits
	KeyPrefixDriverCommit = []byte{0x06}

	// KeyPrefixRequestIndex is the prefix for request by rider index
	KeyPrefixRequestIndex = []byte{0x07}
)

// RequestKey returns the key for a request
func RequestKey(requestId uint64) []byte {
	key := make([]byte, 9)
	key[0] = KeyPrefixRequest[0]
	// Store requestId as big-endian uint64
	key[1] = byte(requestId >> 56)
	key[2] = byte(requestId >> 48)
	key[3] = byte(requestId >> 40)
	key[4] = byte(requestId >> 32)
	key[5] = byte(requestId >> 24)
	key[6] = byte(requestId >> 16)
	key[7] = byte(requestId >> 8)
	key[8] = byte(requestId)
	return key
}

// SessionKey returns the key for a session
func SessionKey(sessionId uint64) []byte {
	key := make([]byte, 9)
	key[0] = KeyPrefixSession[0]
	key[1] = byte(sessionId >> 56)
	key[2] = byte(sessionId >> 48)
	key[3] = byte(sessionId >> 40)
	key[4] = byte(sessionId >> 32)
	key[5] = byte(sessionId >> 24)
	key[6] = byte(sessionId >> 16)
	key[7] = byte(sessionId >> 8)
	key[8] = byte(sessionId)
	return key
}

// PendingRequestKey returns the key for a pending request
func PendingRequestKey(requestId uint64) []byte {
	key := make([]byte, 9)
	key[0] = KeyPrefixPendingRequest[0]
	key[1] = byte(requestId >> 56)
	key[2] = byte(requestId >> 48)
	key[3] = byte(requestId >> 40)
	key[4] = byte(requestId >> 32)
	key[5] = byte(requestId >> 24)
	key[6] = byte(requestId >> 16)
	key[7] = byte(requestId >> 8)
	key[8] = byte(requestId)
	return key
}

// DriverCommitKey returns the key for a driver commit
func DriverCommitKey(requestId uint64, driverAddr string) []byte {
	reqKey := make([]byte, 8)
	reqKey[0] = byte(requestId >> 56)
	reqKey[1] = byte(requestId >> 48)
	reqKey[2] = byte(requestId >> 40)
	reqKey[3] = byte(requestId >> 32)
	reqKey[4] = byte(requestId >> 24)
	reqKey[5] = byte(requestId >> 16)
	reqKey[6] = byte(requestId >> 8)
	reqKey[7] = byte(requestId)

	key := append([]byte{KeyPrefixDriverCommit[0]}, reqKey...)
	key = append(key, []byte(driverAddr)...)
	return key
}
