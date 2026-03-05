package types

import (
	"encoding/binary"
	"fmt"
)

const (
	// ModuleName defines the module name.
	ModuleName = "match"
	// StoreKey defines the primary module store key.
	StoreKey = ModuleName
	// RouterKey defines the module message router key.
	RouterKey = ModuleName
)

const (
	prefixReplayIndex = iota + 1
)

var (
	// KeyPrefixReplayIndex stores replay index entries keyed by (pool_id, intent_id).
	KeyPrefixReplayIndex = []byte{prefixReplayIndex}
)

// ReplayKeyString renders the canonical replay key for logs/events.
func ReplayKeyString(poolID, intentID string) string {
	return poolID + ":" + intentID
}

// ReplayIndexStoreKey encodes (pool_id, intent_id) for the replay index KV key.
func ReplayIndexStoreKey(poolID, intentID string) []byte {
	poolBz := []byte(poolID)
	intentBz := []byte(intentID)

	// key = prefix | pool_len(4) | pool | intent_len(4) | intent
	key := make([]byte, 1+4+len(poolBz)+4+len(intentBz))
	key[0] = KeyPrefixReplayIndex[0]
	offset := 1
	binary.BigEndian.PutUint32(key[offset:offset+4], uint32(len(poolBz)))
	offset += 4
	copy(key[offset:offset+len(poolBz)], poolBz)
	offset += len(poolBz)
	binary.BigEndian.PutUint32(key[offset:offset+4], uint32(len(intentBz)))
	offset += 4
	copy(key[offset:offset+len(intentBz)], intentBz)
	return key
}

// ParseReplayIndexStoreKey decodes a replay index key into its (pool_id, intent_id) components.
func ParseReplayIndexStoreKey(key []byte) (poolID, intentID string, err error) {
	const minSize = 1 + 4 + 4
	if len(key) < minSize {
		return "", "", fmt.Errorf("invalid replay index key length: %d", len(key))
	}
	if key[0] != KeyPrefixReplayIndex[0] {
		return "", "", fmt.Errorf("invalid replay index key prefix: %d", key[0])
	}

	offset := 1
	poolLen := int(binary.BigEndian.Uint32(key[offset : offset+4]))
	offset += 4
	if poolLen < 0 || offset+poolLen+4 > len(key) {
		return "", "", fmt.Errorf("invalid pool_id length in replay index key: %d", poolLen)
	}
	poolID = string(key[offset : offset+poolLen])
	offset += poolLen

	intentLen := int(binary.BigEndian.Uint32(key[offset : offset+4]))
	offset += 4
	if intentLen < 0 || offset+intentLen != len(key) {
		return "", "", fmt.Errorf("invalid intent_id length in replay index key: %d", intentLen)
	}
	intentID = string(key[offset : offset+intentLen])

	return poolID, intentID, nil
}
