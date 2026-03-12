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
	prefixReplayParties
	prefixCancelledIntent
)

var (
	// KeyPrefixReplayIndex stores replay index entries keyed by (pool_id, intent_id).
	KeyPrefixReplayIndex = []byte{prefixReplayIndex}
	// KeyPrefixReplayParties stores replay participant metadata keyed by (pool_id, intent_id).
	KeyPrefixReplayParties = []byte{prefixReplayParties}
	// KeyPrefixCancelledIntent stores on-chain cancellation records keyed by (pool_id, intent_id).
	KeyPrefixCancelledIntent = []byte{prefixCancelledIntent}
)

// ReplayKeyString renders the canonical replay key for logs/events.
func ReplayKeyString(poolID, intentID string) string {
	return poolID + ":" + intentID
}

// ReplayIndexStoreKey encodes (pool_id, intent_id) for the replay index KV key.
func ReplayIndexStoreKey(poolID, intentID string) []byte {
	return replayCompositeStoreKey(KeyPrefixReplayIndex, poolID, intentID)
}

// ReplayPartiesStoreKey encodes (pool_id, intent_id) for replay participant metadata.
func ReplayPartiesStoreKey(poolID, intentID string) []byte {
	return replayCompositeStoreKey(KeyPrefixReplayParties, poolID, intentID)
}

func replayCompositeStoreKey(prefix []byte, poolID, intentID string) []byte {
	poolBz := []byte(poolID)
	intentBz := []byte(intentID)

	// key = prefix | pool_len(4) | pool | intent_len(4) | intent
	key := make([]byte, 1+4+len(poolBz)+4+len(intentBz))
	key[0] = prefix[0]
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
	return parseReplayCompositeStoreKey(key, KeyPrefixReplayIndex)
}

func parseReplayCompositeStoreKey(key, expectedPrefix []byte) (poolID, intentID string, err error) {
	const minSize = 1 + 4 + 4
	if len(key) < minSize {
		return "", "", fmt.Errorf("invalid replay index key length: %d", len(key))
	}
	if key[0] != expectedPrefix[0] {
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

// EncodeReplayPartiesValue encodes requester/responder metadata.
func EncodeReplayPartiesValue(requester, responder string) []byte {
	requesterBz := []byte(requester)
	responderBz := []byte(responder)

	// value = requester_len(4) | requester | responder_len(4) | responder
	value := make([]byte, 4+len(requesterBz)+4+len(responderBz))
	offset := 0
	binary.BigEndian.PutUint32(value[offset:offset+4], uint32(len(requesterBz)))
	offset += 4
	copy(value[offset:offset+len(requesterBz)], requesterBz)
	offset += len(requesterBz)
	binary.BigEndian.PutUint32(value[offset:offset+4], uint32(len(responderBz)))
	offset += 4
	copy(value[offset:offset+len(responderBz)], responderBz)
	return value
}

// ParseReplayPartiesValue decodes requester/responder metadata.
func ParseReplayPartiesValue(value []byte) (requester, responder string, err error) {
	const minSize = 4 + 4
	if len(value) < minSize {
		return "", "", fmt.Errorf("invalid replay parties value length: %d", len(value))
	}

	offset := 0
	requesterLen := int(binary.BigEndian.Uint32(value[offset : offset+4]))
	offset += 4
	if requesterLen < 0 || offset+requesterLen+4 > len(value) {
		return "", "", fmt.Errorf("invalid requester length in replay parties value: %d", requesterLen)
	}
	requester = string(value[offset : offset+requesterLen])
	offset += requesterLen

	responderLen := int(binary.BigEndian.Uint32(value[offset : offset+4]))
	offset += 4
	if responderLen < 0 || offset+responderLen != len(value) {
		return "", "", fmt.Errorf("invalid responder length in replay parties value: %d", responderLen)
	}
	responder = string(value[offset : offset+responderLen])
	return requester, responder, nil
}

// CancelledIntentStoreKey encodes (pool_id, intent_id) for the cancellation index.
func CancelledIntentStoreKey(poolID, intentID string) []byte {
	return replayCompositeStoreKey(KeyPrefixCancelledIntent, poolID, intentID)
}
