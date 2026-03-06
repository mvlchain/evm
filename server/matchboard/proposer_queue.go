package matchboard

import (
	"bytes"
	"encoding/binary"
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
		ops = append(ops, op)
	}
	sortProposedOperations(ops)
	total := uint64(len(ops))

	if limit <= 0 || limit > len(ops) {
		limit = len(ops)
	}
	page := make([]ProposedOperation, limit)
	copy(page, ops[:limit])
	return page, canonicalBatchHash(page), total
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
		remaining = append(remaining, op)
	}
	sortProposedOperations(remaining)

	return len(operationIDs), uint64(len(remaining)), canonicalBatchHash(remaining), nil
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
		if rd.Len() != 0 {
			return ProposedOperation{}, true, fmt.Errorf("unexpected trailing bytes in injected operation")
		}
	}

	op = ProposedOperation{
		OperationID:      fields[0],
		RecordType:       fields[1],
		PoolID:           fields[2],
		IntentID:         fields[3],
		ResponseID:       fields[4],
		FinalizeID:       fields[5],
		Sender:           fields[6],
		Recipient:        fields[7],
		IntentSignHash:   fields[8],
		ResponseSignHash: fields[9],
		FinalizeSignHash: fields[10],
		MatchCertificate: matchCertificate,
		CreatedUnix:      createdUnix,
	}

	return op, true, nil
}
