package matchboard

import (
	"errors"
	"strconv"
	"sync"
)

var (
	errIntentExists     = errors.New("intent already exists")
	errIntentNotFound   = errors.New("intent not found")
	errResponseExists   = errors.New("response already exists")
	errResponseNotFound = errors.New("response not found")
	errFinalizeExists   = errors.New("finalize already exists")
	errHashMismatch     = errors.New("hash mismatch")
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
	createdUnix      int64
}

type inMemoryStore struct {
	mu sync.RWMutex

	intents   map[intentKey]intentRecord
	responses map[responseKey]responseRecord
	finalizes map[finalizeKey]finalizeRecord

	inboxByRecipient map[string][]BoardRecord
	outboxBySender   map[string][]BoardRecord
}

func newInMemoryStore() *inMemoryStore {
	return &inMemoryStore{
		intents:          make(map[intentKey]intentRecord),
		responses:        make(map[responseKey]responseRecord),
		finalizes:        make(map[finalizeKey]finalizeRecord),
		inboxByRecipient: make(map[string][]BoardRecord),
		outboxBySender:   make(map[string][]BoardRecord),
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
