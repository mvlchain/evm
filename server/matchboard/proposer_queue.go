package matchboard

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
)

var globalProposerQueue = newInMemoryProposerQueue()

type inMemoryProposerQueue struct {
	mu      sync.RWMutex
	pending map[string]ProposedOperation
}

// InjectedBatchMeta anchors the canonical operation set appended by proposer injection.
type InjectedBatchMeta struct {
	CanonicalBatchHash string
	OperationCount     uint32
}

func newInMemoryProposerQueue() *inMemoryProposerQueue {
	return &inMemoryProposerQueue{
		pending: make(map[string]ProposedOperation),
	}
}

// PublishABCIProposedOperation adds an operation to the global proposer queue.
func PublishABCIProposedOperation(op ProposedOperation) {
	if strings.TrimSpace(op.OperationID) == "" {
		return
	}
	op.MatchCertificate = cloneOperationBytes(op.MatchCertificate)
	op.MatchSubmitMsgPayload = cloneOperationBytes(op.MatchSubmitMsgPayload)

	globalProposerQueue.mu.Lock()
	defer globalProposerQueue.mu.Unlock()
	globalProposerQueue.pending[op.OperationID] = op
}

// SnapshotABCIProposedOperations returns canonical pending operations from the global queue.
func SnapshotABCIProposedOperations(limit int) ([]ProposedOperation, string, uint64) {
	globalProposerQueue.mu.RLock()
	defer globalProposerQueue.mu.RUnlock()

	ops := make([]ProposedOperation, 0, len(globalProposerQueue.pending))
	for _, op := range globalProposerQueue.pending {
		op.MatchCertificate = cloneOperationBytes(op.MatchCertificate)
		op.MatchSubmitMsgPayload = cloneOperationBytes(op.MatchSubmitMsgPayload)
		ops = append(ops, op)
	}
	sortProposedOperations(ops)
	total := uint64(len(ops))

	if limit <= 0 || limit > len(ops) {
		limit = len(ops)
	}
	page := make([]ProposedOperation, limit)
	copy(page, ops[:limit])
	return page, BuildCanonicalBatchHash(page), total
}

// CommitABCIProposedOperations removes operations from the global proposer queue atomically.
func CommitABCIProposedOperations(operationIDs []string) (int, uint64, string, error) {
	globalProposerQueue.mu.Lock()
	defer globalProposerQueue.mu.Unlock()

	if len(operationIDs) == 0 {
		return 0, 0, "", errOperationEmpty
	}

	seen := make(map[string]struct{}, len(operationIDs))
	for _, rawID := range operationIDs {
		opID := strings.TrimSpace(rawID)
		if opID == "" {
			return 0, 0, "", errOperationEmpty
		}
		if _, exists := seen[opID]; exists {
			return 0, 0, "", errOperationDuplicate
		}
		seen[opID] = struct{}{}
		if _, exists := globalProposerQueue.pending[opID]; !exists {
			return 0, 0, "", errOperationNotFound
		}
	}

	for opID := range seen {
		delete(globalProposerQueue.pending, opID)
	}

	remaining := make([]ProposedOperation, 0, len(globalProposerQueue.pending))
	for _, op := range globalProposerQueue.pending {
		op.MatchCertificate = cloneOperationBytes(op.MatchCertificate)
		op.MatchSubmitMsgPayload = cloneOperationBytes(op.MatchSubmitMsgPayload)
		remaining = append(remaining, op)
	}
	sortProposedOperations(remaining)

	return len(operationIDs), uint64(len(remaining)), BuildCanonicalBatchHash(remaining), nil
}

func sortProposedOperations(ops []ProposedOperation) {
	// operation_id is already canonical hash, lexical order is deterministic.
	sort.Slice(ops, func(i, j int) bool {
		return ops[i].OperationID < ops[j].OperationID
	})
}

// ClearABCIProposedOperationsForTest clears the shared queue state for tests.
func ClearABCIProposedOperationsForTest() {
	globalProposerQueue.mu.Lock()
	defer globalProposerQueue.mu.Unlock()
	globalProposerQueue.pending = make(map[string]ProposedOperation)
}

// BuildCanonicalBatchHash deterministically hashes canonical operation IDs.
func BuildCanonicalBatchHash(ops []ProposedOperation) string {
	return canonicalBatchHash(ops)
}

// BuildCanonicalMatchBuildHash deterministically hashes canonical injected operations
// together with their optional submit-msg payload hashes.
func BuildCanonicalMatchBuildHash(ops []ProposedOperation) string {
	if len(ops) == 0 {
		return ""
	}
	var b strings.Builder
	for i := range ops {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(ops[i].OperationID)
		b.WriteByte('|')
		switch {
		case len(ops[i].MatchSubmitMsgPayload) > 0:
			sum := sha256.Sum256(ops[i].MatchSubmitMsgPayload)
			b.WriteString(hex.EncodeToString(sum[:]))
		case len(ops[i].MatchCertificate) > 0:
			sum := sha256.Sum256(ops[i].MatchCertificate)
			b.WriteString(hex.EncodeToString(sum[:]))
		default:
			b.WriteString("none")
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

// EncodeABCIInjectedBatchMeta encodes canonical batch metadata for proposal validation.
func EncodeABCIInjectedBatchMeta(meta InjectedBatchMeta) ([]byte, error) {
	if strings.TrimSpace(meta.CanonicalBatchHash) == "" {
		return nil, fmt.Errorf("canonical_batch_hash is required")
	}

	buf := bytes.NewBuffer(nil)
	if _, err := buf.WriteString(injectedBatchMetaMagic); err != nil {
		return nil, err
	}

	hash := strings.TrimSpace(meta.CanonicalBatchHash)
	if len(hash) > 0xffff {
		return nil, fmt.Errorf("canonical_batch_hash too long")
	}
	if err := binary.Write(buf, binary.BigEndian, uint16(len(hash))); err != nil {
		return nil, err
	}
	if _, err := buf.WriteString(hash); err != nil {
		return nil, err
	}
	if err := binary.Write(buf, binary.BigEndian, meta.OperationCount); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DecodeABCIInjectedBatchMeta decodes batch metadata envelope.
// If payload is not batch metadata envelope, matched=false and err=nil are returned.
func DecodeABCIInjectedBatchMeta(raw []byte) (meta InjectedBatchMeta, matched bool, err error) {
	if len(raw) < len(injectedBatchMetaMagic) || string(raw[:len(injectedBatchMetaMagic)]) != injectedBatchMetaMagic {
		return InjectedBatchMeta{}, false, nil
	}

	rd := bytes.NewReader(raw[len(injectedBatchMetaMagic):])
	var hashLen uint16
	if readErr := binary.Read(rd, binary.BigEndian, &hashLen); readErr != nil {
		return InjectedBatchMeta{}, true, fmt.Errorf("decode injected batch meta hash length: %w", readErr)
	}

	hashBz := make([]byte, int(hashLen))
	if hashLen > 0 {
		if _, readErr := rd.Read(hashBz); readErr != nil {
			return InjectedBatchMeta{}, true, fmt.Errorf("decode injected batch meta hash: %w", readErr)
		}
	}

	var opCount uint32
	if readErr := binary.Read(rd, binary.BigEndian, &opCount); readErr != nil {
		return InjectedBatchMeta{}, true, fmt.Errorf("decode injected batch meta operation_count: %w", readErr)
	}
	if rd.Len() != 0 {
		return InjectedBatchMeta{}, true, fmt.Errorf("unexpected trailing bytes in injected batch meta")
	}

	meta = InjectedBatchMeta{
		CanonicalBatchHash: string(hashBz),
		OperationCount:     opCount,
	}
	if strings.TrimSpace(meta.CanonicalBatchHash) == "" {
		return InjectedBatchMeta{}, true, fmt.Errorf("canonical_batch_hash is required")
	}

	return meta, true, nil
}

// EncodeABCIInjectedOperation encodes a canonical operation envelope for proposal injection.
func EncodeABCIInjectedOperation(op ProposedOperation) ([]byte, error) {
	if strings.TrimSpace(op.OperationID) == "" {
		return nil, fmt.Errorf("operation_id is required")
	}

	buf := bytes.NewBuffer(nil)
	if _, err := buf.WriteString(injectedOperationMagic); err != nil {
		return nil, err
	}

	writeString := func(v string) error {
		if len(v) > 0xffff {
			return fmt.Errorf("field too long")
		}
		if err := binary.Write(buf, binary.BigEndian, uint16(len(v))); err != nil {
			return err
		}
		_, err := buf.WriteString(v)
		return err
	}

	fields := []string{
		op.OperationID,
		op.RecordType,
		op.PoolID,
		op.IntentID,
		op.ResponseID,
		op.FinalizeID,
		op.Sender,
		op.Recipient,
		op.IntentSignHash,
		op.ResponseSignHash,
		op.FinalizeSignHash,
	}
	for _, field := range fields {
		if err := writeString(field); err != nil {
			return nil, err
		}
	}
	if err := binary.Write(buf, binary.BigEndian, op.CreatedUnix); err != nil {
		return nil, err
	}
	if len(op.MatchCertificate) > 0xffffffff {
		return nil, fmt.Errorf("match_certificate too large")
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(len(op.MatchCertificate))); err != nil {
		return nil, err
	}
	if len(op.MatchCertificate) > 0 {
		if _, err := buf.Write(op.MatchCertificate); err != nil {
			return nil, err
		}
	}
	if len(op.MatchSubmitMsgPayload) > 0xffffffff {
		return nil, fmt.Errorf("match_submit_msg_payload too large")
	}
	if err := binary.Write(buf, binary.BigEndian, uint32(len(op.MatchSubmitMsgPayload))); err != nil {
		return nil, err
	}
	if len(op.MatchSubmitMsgPayload) > 0 {
		if _, err := buf.Write(op.MatchSubmitMsgPayload); err != nil {
			return nil, err
		}
	}

	return buf.Bytes(), nil
}

// DecodeABCIInjectedOperation decodes a proposal injected operation envelope.
// If the payload is not an injected operation, matched=false and err=nil are returned.
func DecodeABCIInjectedOperation(raw []byte) (op ProposedOperation, matched bool, err error) {
	if len(raw) < len(injectedOperationMagic) || string(raw[:len(injectedOperationMagic)]) != injectedOperationMagic {
		return ProposedOperation{}, false, nil
	}

	rd := bytes.NewReader(raw[len(injectedOperationMagic):])
	readString := func() (string, error) {
		var l uint16
		if err := binary.Read(rd, binary.BigEndian, &l); err != nil {
			return "", err
		}
		if l == 0 {
			return "", nil
		}
		bz := make([]byte, int(l))
		if _, err := rd.Read(bz); err != nil {
			return "", err
		}
		return string(bz), nil
	}

	fields := make([]string, 11)
	for i := range fields {
		v, readErr := readString()
		if readErr != nil {
			return ProposedOperation{}, true, fmt.Errorf("decode injected operation field %d: %w", i, readErr)
		}
		fields[i] = v
	}

	var createdUnix int64
	if readErr := binary.Read(rd, binary.BigEndian, &createdUnix); readErr != nil {
		return ProposedOperation{}, true, fmt.Errorf("decode injected operation timestamp: %w", readErr)
	}
	matchCertificate := []byte(nil)
	if rd.Len() > 0 {
		if rd.Len() < 4 {
			return ProposedOperation{}, true, fmt.Errorf("decode injected operation certificate length: insufficient bytes")
		}
		var certLen uint32
		if readErr := binary.Read(rd, binary.BigEndian, &certLen); readErr != nil {
			return ProposedOperation{}, true, fmt.Errorf("decode injected operation certificate length: %w", readErr)
		}
		if certLen > uint32(rd.Len()) {
			return ProposedOperation{}, true, fmt.Errorf("decode injected operation certificate length exceeds payload")
		}
		if certLen > 0 {
			matchCertificate = make([]byte, int(certLen))
			if _, readErr := rd.Read(matchCertificate); readErr != nil {
				return ProposedOperation{}, true, fmt.Errorf("decode injected operation certificate bytes: %w", readErr)
			}
		}
	}
	matchSubmitMsgPayload := []byte(nil)
	if rd.Len() > 0 {
		if rd.Len() < 4 {
			return ProposedOperation{}, true, fmt.Errorf("decode injected operation submit msg payload length: insufficient bytes")
		}
		var payloadLen uint32
		if readErr := binary.Read(rd, binary.BigEndian, &payloadLen); readErr != nil {
			return ProposedOperation{}, true, fmt.Errorf("decode injected operation submit msg payload length: %w", readErr)
		}
		if payloadLen > uint32(rd.Len()) {
			return ProposedOperation{}, true, fmt.Errorf("decode injected operation submit msg payload length exceeds payload")
		}
		if payloadLen > 0 {
			matchSubmitMsgPayload = make([]byte, int(payloadLen))
			if _, readErr := rd.Read(matchSubmitMsgPayload); readErr != nil {
				return ProposedOperation{}, true, fmt.Errorf("decode injected operation submit msg payload bytes: %w", readErr)
			}
		}
	}
	if rd.Len() != 0 {
		return ProposedOperation{}, true, fmt.Errorf("unexpected trailing bytes in injected operation")
	}

	op = ProposedOperation{
		OperationID:           fields[0],
		RecordType:            fields[1],
		PoolID:                fields[2],
		IntentID:              fields[3],
		ResponseID:            fields[4],
		FinalizeID:            fields[5],
		Sender:                fields[6],
		Recipient:             fields[7],
		IntentSignHash:        fields[8],
		ResponseSignHash:      fields[9],
		FinalizeSignHash:      fields[10],
		MatchCertificate:      matchCertificate,
		MatchSubmitMsgPayload: matchSubmitMsgPayload,
		CreatedUnix:           createdUnix,
	}

	return op, true, nil
}
