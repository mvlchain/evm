package matchboard

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var (
	errIntentExists       = errors.New("intent already exists")
	errIntentNotFound     = errors.New("intent not found")
	errResponseExists     = errors.New("response already exists")
	errResponseNotFound   = errors.New("response not found")
	errFinalizeExists     = errors.New("finalize already exists")
	errHashMismatch       = errors.New("hash mismatch")
	errOperationNotFound  = errors.New("proposed operation not found")
	errOperationDuplicate = errors.New("duplicate operation id in request")
	errOperationEmpty     = errors.New("operation id must not be empty")
)

type intentKey struct {
	poolID   string
	intentID string
}

type responseKey struct {
	poolID     string
	intentID   string
	responseID string
}

type finalizeKey struct {
	poolID     string
	intentID   string
	responseID string
	finalizeID string
}

type intentRecord struct {
	poolID         string
	intentID       string
	sender         string
	recipient      string
	contextHash    string
	intentSignHash string
	createdUnix    int64
}

type responseRecord struct {
	poolID           string
	intentID         string
	responseID       string
	sender           string
	recipient        string
	contextHash      string
	intentSignHash   string
	responseSignHash string
	createdUnix      int64
}

type finalizeRecord struct {
	poolID           string
	intentID         string
	responseID       string
	finalizeID       string
	sender           string
	recipient        string
	contextHash      string
	intentSignHash   string
	responseSignHash string
	finalizeSignHash string
	matchCertificate []byte
	createdUnix      int64
}

type inMemoryStore struct {
	mu sync.RWMutex

	intents   map[intentKey]intentRecord
	responses map[responseKey]responseRecord
	finalizes map[finalizeKey]finalizeRecord

	inboxByRecipient map[string][]BoardRecord
	outboxBySender   map[string][]BoardRecord
	pendingOps       map[string]ProposedOperation
	enableABCI       bool
}

func newInMemoryStore(enableABCI bool) *inMemoryStore {
	return &inMemoryStore{
		intents:          make(map[intentKey]intentRecord),
		responses:        make(map[responseKey]responseRecord),
		finalizes:        make(map[finalizeKey]finalizeRecord),
		inboxByRecipient: make(map[string][]BoardRecord),
		outboxBySender:   make(map[string][]BoardRecord),
		pendingOps:       make(map[string]ProposedOperation),
		enableABCI:       enableABCI,
	}
}

func (s *inMemoryStore) createIntent(req PublishIntentRequest, nowUnix int64) (publishIntentResponse, error) {
	k := intentKey{poolID: req.PoolID, intentID: req.IntentID}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.intents[k]; exists {
		return publishIntentResponse{}, errIntentExists
	}

	rec := intentRecord{
		poolID:         req.PoolID,
		intentID:       req.IntentID,
		sender:         req.Sender,
		recipient:      req.Recipient,
		contextHash:    req.ContextHash,
		intentSignHash: req.IntentSignHash,
		createdUnix:    nowUnix,
	}
	s.intents[k] = rec

	boardRecord := BoardRecord{
		RecordType:     RecordTypeIntent,
		PoolID:         req.PoolID,
		IntentID:       req.IntentID,
		Sender:         req.Sender,
		Recipient:      req.Recipient,
		CreatedUnix:    nowUnix,
		ContextHash:    req.ContextHash,
		IntentSignHash: req.IntentSignHash,
	}
	appendBoardRecord(s.inboxByRecipient, req.Recipient, boardRecord)
	appendBoardRecord(s.outboxBySender, req.Sender, boardRecord)
	s.enqueuePendingOperation(boardRecord, nil)

	return publishIntentResponse{
		PoolID:         req.PoolID,
		IntentID:       req.IntentID,
		IntentSignHash: req.IntentSignHash,
		StoredUnix:     nowUnix,
	}, nil
}

func (s *inMemoryStore) createResponse(req PublishResponseRequest, nowUnix int64) (publishResponseResponse, error) {
	intentK := intentKey{poolID: req.PoolID, intentID: req.IntentID}
	responseK := responseKey{poolID: req.PoolID, intentID: req.IntentID, responseID: req.ResponseID}

	s.mu.Lock()
	defer s.mu.Unlock()

	intent, ok := s.intents[intentK]
	if !ok {
		return publishResponseResponse{}, errIntentNotFound
	}
	if intent.intentSignHash != req.IntentSignHash {
		return publishResponseResponse{}, errHashMismatch
	}
	if _, exists := s.responses[responseK]; exists {
		return publishResponseResponse{}, errResponseExists
	}

	rec := responseRecord{
		poolID:           req.PoolID,
		intentID:         req.IntentID,
		responseID:       req.ResponseID,
		sender:           req.Sender,
		recipient:        req.Recipient,
		contextHash:      req.ContextHash,
		intentSignHash:   req.IntentSignHash,
		responseSignHash: req.ResponseSignHash,
		createdUnix:      nowUnix,
	}
	s.responses[responseK] = rec

	boardRecord := BoardRecord{
		RecordType:       RecordTypeResponse,
		PoolID:           req.PoolID,
		IntentID:         req.IntentID,
		ResponseID:       req.ResponseID,
		Sender:           req.Sender,
		Recipient:        req.Recipient,
		CreatedUnix:      nowUnix,
		ContextHash:      req.ContextHash,
		IntentSignHash:   req.IntentSignHash,
		ResponseSignHash: req.ResponseSignHash,
	}
	appendBoardRecord(s.inboxByRecipient, req.Recipient, boardRecord)
	appendBoardRecord(s.outboxBySender, req.Sender, boardRecord)
	s.enqueuePendingOperation(boardRecord, nil)

	return publishResponseResponse{
		PoolID:           req.PoolID,
		IntentID:         req.IntentID,
		ResponseID:       req.ResponseID,
		ResponseSignHash: req.ResponseSignHash,
		StoredUnix:       nowUnix,
	}, nil
}

func (s *inMemoryStore) createFinalize(req PublishFinalizeRequest, nowUnix int64) (publishFinalizeResponse, error) {
	intentK := intentKey{poolID: req.PoolID, intentID: req.IntentID}
	responseK := responseKey{poolID: req.PoolID, intentID: req.IntentID, responseID: req.ResponseID}
	finalizeK := finalizeKey{poolID: req.PoolID, intentID: req.IntentID, responseID: req.ResponseID, finalizeID: req.FinalizeID}

	s.mu.Lock()
	defer s.mu.Unlock()

	intent, ok := s.intents[intentK]
	if !ok {
		return publishFinalizeResponse{}, errIntentNotFound
	}
	if intent.intentSignHash != req.IntentSignHash {
		return publishFinalizeResponse{}, errHashMismatch
	}

	response, ok := s.responses[responseK]
	if !ok {
		return publishFinalizeResponse{}, errResponseNotFound
	}
	if response.responseSignHash != req.ResponseSignHash || response.intentSignHash != req.IntentSignHash {
		return publishFinalizeResponse{}, errHashMismatch
	}

	if _, exists := s.finalizes[finalizeK]; exists {
		return publishFinalizeResponse{}, errFinalizeExists
	}

	rec := finalizeRecord{
		poolID:           req.PoolID,
		intentID:         req.IntentID,
		responseID:       req.ResponseID,
		finalizeID:       req.FinalizeID,
		sender:           req.Sender,
		recipient:        req.Recipient,
		contextHash:      req.ContextHash,
		intentSignHash:   req.IntentSignHash,
		responseSignHash: req.ResponseSignHash,
		finalizeSignHash: req.FinalizeSignHash,
		matchCertificate: cloneOperationBytes(req.MatchCertificate),
		createdUnix:      nowUnix,
	}
	s.finalizes[finalizeK] = rec

	boardRecord := BoardRecord{
		RecordType:       RecordTypeFinalize,
		PoolID:           req.PoolID,
		IntentID:         req.IntentID,
		ResponseID:       req.ResponseID,
		FinalizeID:       req.FinalizeID,
		Sender:           req.Sender,
		Recipient:        req.Recipient,
		CreatedUnix:      nowUnix,
		ContextHash:      req.ContextHash,
		IntentSignHash:   req.IntentSignHash,
		ResponseSignHash: req.ResponseSignHash,
		FinalizeSignHash: req.FinalizeSignHash,
	}
	appendBoardRecord(s.inboxByRecipient, req.Recipient, boardRecord)
	appendBoardRecord(s.outboxBySender, req.Sender, boardRecord)
	s.enqueuePendingOperation(boardRecord, req.MatchCertificate)

	return publishFinalizeResponse{
		PoolID:           req.PoolID,
		IntentID:         req.IntentID,
		ResponseID:       req.ResponseID,
		FinalizeID:       req.FinalizeID,
		FinalizeSignHash: req.FinalizeSignHash,
		StoredUnix:       nowUnix,
	}, nil
}

func (s *inMemoryStore) listInbox(recipient string, cursor, limit int) ([]BoardRecord, string, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records := s.inboxByRecipient[recipient]
	return paginateRecords(records, cursor, limit)
}

func (s *inMemoryStore) listOutbox(sender string, cursor, limit int) ([]BoardRecord, string, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	records := s.outboxBySender[sender]
	return paginateRecords(records, cursor, limit)
}

func (s *inMemoryStore) listProposedOperations(limit int) ([]ProposedOperation, string, uint64) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ops := s.sortedPendingOperationsLocked()
	total := uint64(len(ops))
	if limit <= 0 || limit > len(ops) {
		limit = len(ops)
	}

	page := make([]ProposedOperation, limit)
	copy(page, ops[:limit])

	return page, canonicalBatchHash(page), total
}

func (s *inMemoryStore) commitProposedOperations(operationIDs []string) (int, uint64, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

		if _, exists := s.pendingOps[opID]; !exists {
			return 0, 0, "", errOperationNotFound
		}
	}

	for opID := range seen {
		delete(s.pendingOps, opID)
	}
	if s.enableABCI {
		_, _, _, _ = CommitABCIProposedOperations(operationIDs)
	}

	remaining := s.sortedPendingOperationsLocked()
	return len(operationIDs), uint64(len(remaining)), canonicalBatchHash(remaining), nil
}

func paginateRecords(records []BoardRecord, cursor, limit int) ([]BoardRecord, string, uint64) {
	total := uint64(len(records))
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(records) {
		return []BoardRecord{}, "", total
	}

	end := cursor + limit
	if end > len(records) {
		end = len(records)
	}

	page := make([]BoardRecord, end-cursor)
	copy(page, records[cursor:end])

	nextCursor := ""
	if end < len(records) {
		nextCursor = strconv.Itoa(end)
	}

	return page, nextCursor, total
}

func appendBoardRecord(idx map[string][]BoardRecord, principal string, record BoardRecord) {
	idx[principal] = append(idx[principal], record)
}

func (s *inMemoryStore) enqueuePendingOperation(record BoardRecord, matchCertificate []byte) {
	op := toProposedOperation(record, matchCertificate)
	s.pendingOps[op.OperationID] = op
	if s.enableABCI {
		PublishABCIProposedOperation(op)
	}
}

func (s *inMemoryStore) sortedPendingOperationsLocked() []ProposedOperation {
	ops := make([]ProposedOperation, 0, len(s.pendingOps))
	for _, op := range s.pendingOps {
		ops = append(ops, op)
	}
	sort.Slice(ops, func(i, j int) bool {
		return ops[i].OperationID < ops[j].OperationID
	})
	return ops
}

func toProposedOperation(record BoardRecord, matchCertificate []byte) ProposedOperation {
	op := ProposedOperation{
		RecordType:  record.RecordType,
		PoolID:      record.PoolID,
		IntentID:    record.IntentID,
		ResponseID:  record.ResponseID,
		FinalizeID:  record.FinalizeID,
		Sender:      record.Sender,
		Recipient:   record.Recipient,
		CreatedUnix: record.CreatedUnix,

		IntentSignHash:   record.IntentSignHash,
		ResponseSignHash: record.ResponseSignHash,
		FinalizeSignHash: record.FinalizeSignHash,
		MatchCertificate: cloneOperationBytes(matchCertificate),
	}
	op.OperationID = BuildOperationIDFromProposedOperation(op)
	return op
}

// BuildOperationIDFromProposedOperation deterministically computes operation_id.
func BuildOperationIDFromProposedOperation(op ProposedOperation) string {
	record := BoardRecord{
		RecordType:       op.RecordType,
		PoolID:           op.PoolID,
		IntentID:         op.IntentID,
		ResponseID:       op.ResponseID,
		FinalizeID:       op.FinalizeID,
		IntentSignHash:   op.IntentSignHash,
		ResponseSignHash: op.ResponseSignHash,
		FinalizeSignHash: op.FinalizeSignHash,
	}
	return buildOperationID(record)
}

func buildOperationID(record BoardRecord) string {
	var b strings.Builder
	b.WriteString(record.RecordType)
	b.WriteString("|")
	b.WriteString(record.PoolID)
	b.WriteString("|")
	b.WriteString(record.IntentID)
	b.WriteString("|")
	b.WriteString(record.ResponseID)
	b.WriteString("|")
	b.WriteString(record.FinalizeID)
	b.WriteString("|")
	b.WriteString(record.IntentSignHash)
	b.WriteString("|")
	b.WriteString(record.ResponseSignHash)
	b.WriteString("|")
	b.WriteString(record.FinalizeSignHash)

	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func canonicalBatchHash(ops []ProposedOperation) string {
	if len(ops) == 0 {
		return ""
	}
	var b strings.Builder
	for i := range ops {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(ops[i].OperationID)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func cloneOperationBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	out := make([]byte, len(value))
	copy(out, value)
	return out
}
