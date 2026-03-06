package matchboard

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

var (
	errIntentExists            = errors.New("intent already exists")
	errIntentNotFound          = errors.New("intent not found")
	errIntentExpired           = errors.New("intent is expired")
	errResponseExists          = errors.New("response already exists")
	errResponseNotFound        = errors.New("response not found")
	errResponseExpired         = errors.New("response is expired")
	errFinalizeExists          = errors.New("finalize already exists")
	errHashMismatch            = errors.New("hash mismatch")
	errOperationNotFound       = errors.New("proposed operation not found")
	errOperationDuplicate      = errors.New("duplicate operation id in request")
	errOperationEmpty          = errors.New("operation id must not be empty")
	errMatchNotFound           = errors.New("proposed match not found")
	errMatchDuplicate          = errors.New("duplicate match id in request")
	errMatchEmpty              = errors.New("match id must not be empty")
	errMatchFinalizeNotFound   = errors.New("match finalize not found")
	errMatchCertificateMissing = errors.New("match certificate missing")
	errMatchCertificateInvalid = errors.New("match certificate invalid")
	errOperationBuildFailed    = errors.New("proposed operation build failed")
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
	expiresUnix    int64
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
	expiresUnix      int64
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
	expiresUnix      int64
	createdUnix      int64
}

type inMemoryStore struct {
	mu sync.RWMutex

	intents   map[intentKey]intentRecord
	responses map[responseKey]responseRecord
	finalizes map[finalizeKey]finalizeRecord
	// responsesByIntent indexes accept intents by request intent.
	responsesByIntent map[intentKey][]responseRecord

	inboxByRecipient map[string][]BoardRecord
	outboxBySender   map[string][]BoardRecord
	pendingOps       map[string]ProposedOperation
	enableABCI       bool
	matcherShards    int
}

func newInMemoryStore(enableABCI bool, matcherShards int) *inMemoryStore {
	if matcherShards <= 0 {
		matcherShards = defaultMatcherShardCount
	}
	return &inMemoryStore{
		intents:           make(map[intentKey]intentRecord),
		responses:         make(map[responseKey]responseRecord),
		finalizes:         make(map[finalizeKey]finalizeRecord),
		responsesByIntent: make(map[intentKey][]responseRecord),
		inboxByRecipient:  make(map[string][]BoardRecord),
		outboxBySender:    make(map[string][]BoardRecord),
		pendingOps:        make(map[string]ProposedOperation),
		enableABCI:        enableABCI,
		matcherShards:     matcherShards,
	}
}

func (s *inMemoryStore) createIntent(req PublishIntentRequest, nowUnix int64) (publishIntentResponse, error) {
	k := intentKey{poolID: req.PoolID, intentID: req.IntentID}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupExpiredLocked(nowUnix)

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
		expiresUnix:    req.ExpiresUnix,
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
	if err := s.enqueuePendingOperation(boardRecord, nil); err != nil {
		return publishIntentResponse{}, err
	}

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
	s.cleanupExpiredLocked(nowUnix)

	intent, ok := s.intents[intentK]
	if !ok {
		return publishResponseResponse{}, errIntentNotFound
	}
	if intent.expiresUnix < nowUnix {
		return publishResponseResponse{}, errIntentExpired
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
		expiresUnix:      req.ExpiresUnix,
		createdUnix:      nowUnix,
	}
	s.responses[responseK] = rec
	s.responsesByIntent[intentK] = append(s.responsesByIntent[intentK], rec)

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
	if err := s.enqueuePendingOperation(boardRecord, nil); err != nil {
		return publishResponseResponse{}, err
	}

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
	s.cleanupExpiredLocked(nowUnix)

	intent, ok := s.intents[intentK]
	if !ok {
		return publishFinalizeResponse{}, errIntentNotFound
	}
	if intent.expiresUnix < nowUnix {
		return publishFinalizeResponse{}, errIntentExpired
	}
	if intent.intentSignHash != req.IntentSignHash {
		return publishFinalizeResponse{}, errHashMismatch
	}

	response, ok := s.responses[responseK]
	if !ok {
		return publishFinalizeResponse{}, errResponseNotFound
	}
	if response.expiresUnix < nowUnix {
		return publishFinalizeResponse{}, errResponseExpired
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
		expiresUnix:      req.ExpiresUnix,
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
	if err := s.enqueuePendingOperation(boardRecord, req.MatchCertificate); err != nil {
		return publishFinalizeResponse{}, err
	}

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

func (s *inMemoryStore) listProposedOperations(limit int, nowUnix int64) ([]ProposedOperation, string, uint64) {
	s.mu.RLock()
	if !s.hasExpiredLocked(nowUnix) {
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
	s.mu.RUnlock()

	s.mu.Lock()
	s.cleanupExpiredLocked(nowUnix)
	ops := s.sortedPendingOperationsLocked()
	total := uint64(len(ops))
	if limit <= 0 || limit > len(ops) {
		limit = len(ops)
	}
	page := make([]ProposedOperation, limit)
	copy(page, ops[:limit])
	s.mu.Unlock()

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

func (s *inMemoryStore) listMatchCandidates(limit int, nowUnix int64) ([]MatchCandidate, uint64) {
	intents, responsesByIntent := s.snapshotMatcherState(nowUnix)
	candidates := buildMatchCandidatesParallel(intents, responsesByIntent, s.matcherShards, nowUnix)
	total := uint64(len(candidates))
	if limit <= 0 || limit > len(candidates) {
		limit = len(candidates)
	}
	page := make([]MatchCandidate, limit)
	copy(page, candidates[:limit])
	return page, total
}

func (s *inMemoryStore) listProposerMatches(limit int, nowUnix int64) ([]MatchCandidate, string, uint64) {
	matches, total := s.listMatchCandidates(limit, nowUnix)
	return matches, canonicalMatchCandidateHash(matches), total
}

func (s *inMemoryStore) commitProposerMatches(matchIDs []string, nowUnix int64) (int, uint64, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupExpiredLocked(nowUnix)
	if len(matchIDs) == 0 {
		return 0, 0, "", errMatchEmpty
	}

	intents := make([]indexedIntent, 0, len(s.intents))
	for key, rec := range s.intents {
		intents = append(intents, indexedIntent{key: key, record: rec})
	}
	responsesByIntent := make(map[intentKey][]responseRecord, len(s.responsesByIntent))
	for key, responses := range s.responsesByIntent {
		copied := make([]responseRecord, len(responses))
		copy(copied, responses)
		responsesByIntent[key] = copied
	}
	candidates := buildMatchCandidatesParallel(intents, responsesByIntent, s.matcherShards, nowUnix)
	byID := make(map[string]MatchCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.MatchID] = candidate
	}

	seen := make(map[string]struct{}, len(matchIDs))
	consumedIntents := make(map[intentKey]struct{}, len(matchIDs))
	for _, rawID := range matchIDs {
		matchID := strings.TrimSpace(rawID)
		if matchID == "" {
			return 0, 0, "", errMatchEmpty
		}
		if _, exists := seen[matchID]; exists {
			return 0, 0, "", errMatchDuplicate
		}
		seen[matchID] = struct{}{}

		candidate, exists := byID[matchID]
		if !exists {
			return 0, 0, "", errMatchNotFound
		}
		intentK := intentKey{poolID: candidate.PoolID, intentID: candidate.IntentID}
		if _, already := consumedIntents[intentK]; already {
			return 0, 0, "", errMatchDuplicate
		}
		consumedIntents[intentK] = struct{}{}
	}

	for intentK := range consumedIntents {
		s.consumeIntentMatchLocked(intentK)
	}

	remainingIntents := make([]indexedIntent, 0, len(s.intents))
	for key, rec := range s.intents {
		remainingIntents = append(remainingIntents, indexedIntent{key: key, record: rec})
	}
	remainingResponsesByIntent := make(map[intentKey][]responseRecord, len(s.responsesByIntent))
	for key, responses := range s.responsesByIntent {
		copied := make([]responseRecord, len(responses))
		copy(copied, responses)
		remainingResponsesByIntent[key] = copied
	}
	remainingMatches := buildMatchCandidatesParallel(remainingIntents, remainingResponsesByIntent, s.matcherShards, nowUnix)
	return len(matchIDs), uint64(len(remainingMatches)), canonicalMatchCandidateHash(remainingMatches), nil
}

func (s *inMemoryStore) buildProposerMatches(
	matchIDs []string,
	submitter string,
	nowUnix int64,
	requireCertificate bool,
) ([]proposerMatchBuildItem, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupExpiredLocked(nowUnix)
	submitter = strings.TrimSpace(submitter)
	if submitter == "" {
		return nil, "", errMatchEmpty
	}
	if len(matchIDs) == 0 {
		return nil, "", errMatchEmpty
	}

	intents := make([]indexedIntent, 0, len(s.intents))
	for key, rec := range s.intents {
		intents = append(intents, indexedIntent{key: key, record: rec})
	}
	responsesByIntent := make(map[intentKey][]responseRecord, len(s.responsesByIntent))
	for key, responses := range s.responsesByIntent {
		copied := make([]responseRecord, len(responses))
		copy(copied, responses)
		responsesByIntent[key] = copied
	}
	candidates := buildMatchCandidatesParallel(intents, responsesByIntent, s.matcherShards, nowUnix)
	byID := make(map[string]MatchCandidate, len(candidates))
	for _, candidate := range candidates {
		byID[candidate.MatchID] = candidate
	}

	seen := make(map[string]struct{}, len(matchIDs))
	selected := make([]MatchCandidate, 0, len(matchIDs))
	for _, rawID := range matchIDs {
		matchID := strings.TrimSpace(rawID)
		if matchID == "" {
			return nil, "", errMatchEmpty
		}
		if _, exists := seen[matchID]; exists {
			return nil, "", errMatchDuplicate
		}
		seen[matchID] = struct{}{}
		candidate, exists := byID[matchID]
		if !exists {
			return nil, "", errMatchNotFound
		}
		selected = append(selected, candidate)
	}
	sortMatchCandidates(selected)

	items := make([]proposerMatchBuildItem, 0, len(selected))
	for _, candidate := range selected {
		item := proposerMatchBuildItem{
			MatchID:    candidate.MatchID,
			PoolID:     candidate.PoolID,
			IntentID:   candidate.IntentID,
			ResponseID: candidate.ResponseID,
			Requester:  candidate.Requester,
			Responder:  candidate.Responder,
		}

		finalizeRec, found := s.pickFinalizeForMatchLocked(candidate.PoolID, candidate.IntentID, candidate.ResponseID, nowUnix)
		if !found {
			if requireCertificate {
				return nil, "", fmt.Errorf("%w: %s", errMatchFinalizeNotFound, candidate.MatchID)
			}
			items = append(items, item)
			continue
		}
		item.FinalizeID = finalizeRec.finalizeID

		if len(finalizeRec.matchCertificate) == 0 {
			if requireCertificate {
				return nil, "", fmt.Errorf("%w: %s", errMatchCertificateMissing, candidate.MatchID)
			}
			items = append(items, item)
			continue
		}

		op := ProposedOperation{
			RecordType:       RecordTypeFinalize,
			PoolID:           finalizeRec.poolID,
			IntentID:         finalizeRec.intentID,
			ResponseID:       finalizeRec.responseID,
			FinalizeID:       finalizeRec.finalizeID,
			Sender:           finalizeRec.sender,
			Recipient:        finalizeRec.recipient,
			IntentSignHash:   finalizeRec.intentSignHash,
			ResponseSignHash: finalizeRec.responseSignHash,
			FinalizeSignHash: finalizeRec.finalizeSignHash,
			MatchCertificate: cloneOperationBytes(finalizeRec.matchCertificate),
		}
		if _, err := DecodeOperationCertificate(op); err != nil {
			return nil, "", fmt.Errorf("%w: %s", errMatchCertificateInvalid, err.Error())
		}

		msgBz, err := BuildSubmitMatchCertificateMsgPayload(op.MatchCertificate, submitter)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %v", errMatchCertificateInvalid, err)
		}

		msgHash := sha256.Sum256(msgBz)
		item.HasMatchCertificate = true
		item.MatchCertificate = cloneOperationBytes(op.MatchCertificate)
		item.MsgSubmitMatchTxPayload = msgBz
		item.MsgPayloadHash = hex.EncodeToString(msgHash[:])
		items = append(items, item)
	}

	return items, canonicalBuildHash(items), nil
}

type indexedIntent struct {
	key    intentKey
	record intentRecord
}

func (s *inMemoryStore) snapshotMatcherState(nowUnix int64) ([]indexedIntent, map[intentKey][]responseRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupExpiredLocked(nowUnix)

	intents := make([]indexedIntent, 0, len(s.intents))
	for key, rec := range s.intents {
		intents = append(intents, indexedIntent{
			key:    key,
			record: rec,
		})
	}

	responsesByIntent := make(map[intentKey][]responseRecord, len(s.responsesByIntent))
	for key, responses := range s.responsesByIntent {
		copied := make([]responseRecord, len(responses))
		copy(copied, responses)
		responsesByIntent[key] = copied
	}

	return intents, responsesByIntent
}

func buildMatchCandidatesParallel(
	intents []indexedIntent,
	responsesByIntent map[intentKey][]responseRecord,
	shardCount int,
	nowUnix int64,
) []MatchCandidate {
	if len(intents) == 0 {
		return nil
	}
	if shardCount <= 1 {
		return buildMatchCandidatesShard(intents, responsesByIntent, nowUnix)
	}

	shards := make([][]indexedIntent, shardCount)
	for _, intent := range intents {
		idx := matcherShardIndex(intent, shardCount)
		shards[idx] = append(shards[idx], intent)
	}

	results := make([][]MatchCandidate, shardCount)
	var wg sync.WaitGroup
	for shardIdx := range shards {
		if len(shards[shardIdx]) == 0 {
			continue
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = buildMatchCandidatesShard(shards[i], responsesByIntent, nowUnix)
		}(shardIdx)
	}
	wg.Wait()

	candidates := make([]MatchCandidate, 0, len(intents))
	for _, shardCandidates := range results {
		candidates = append(candidates, shardCandidates...)
	}
	sortMatchCandidates(candidates)
	return candidates
}

func buildMatchCandidatesShard(
	intents []indexedIntent,
	responsesByIntent map[intentKey][]responseRecord,
	nowUnix int64,
) []MatchCandidate {
	candidates := make([]MatchCandidate, 0, len(intents))
	for _, req := range intents {
		reqIntent := req.record
		if reqIntent.expiresUnix < nowUnix {
			continue
		}
		accepts := responsesByIntent[req.key]
		if len(accepts) == 0 {
			continue
		}

		var (
			best      *responseRecord
			bestScore string
		)
		for i := range accepts {
			resp := accepts[i]
			if resp.expiresUnix < nowUnix {
				continue
			}
			score := buildMatchScoreHash(reqIntent, resp)
			if best == nil || score < bestScore || (score == bestScore && responseLexLess(resp, *best)) {
				candidate := resp
				best = &candidate
				bestScore = score
			}
		}
		if best == nil {
			continue
		}

		expiryUnix := reqIntent.expiresUnix
		if best.expiresUnix < expiryUnix {
			expiryUnix = best.expiresUnix
		}

		candidates = append(candidates, MatchCandidate{
			MatchID:             buildMatchID(reqIntent, *best),
			PoolID:              reqIntent.poolID,
			IntentID:            reqIntent.intentID,
			ResponseID:          best.responseID,
			Requester:           reqIntent.sender,
			Responder:           best.sender,
			IntentSignHash:      reqIntent.intentSignHash,
			ResponseSignHash:    best.responseSignHash,
			ScoreHash:           bestScore,
			RequestCreatedUnix:  reqIntent.createdUnix,
			ResponseCreatedUnix: best.createdUnix,
			ExpiryUnix:          expiryUnix,
		})
	}
	return candidates
}

func matcherShardIndex(intent indexedIntent, shardCount int) int {
	if shardCount <= 1 {
		return 0
	}
	var b strings.Builder
	b.WriteString(strings.ToLower(strings.TrimSpace(intent.record.sender)))
	b.WriteString("|")
	b.WriteString(intent.record.poolID)
	b.WriteString("|")
	b.WriteString(intent.record.intentID)
	sum := sha256.Sum256([]byte(b.String()))
	return int(binary.BigEndian.Uint64(sum[:8]) % uint64(shardCount))
}

func sortMatchCandidates(candidates []MatchCandidate) {
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ScoreHash != candidates[j].ScoreHash {
			return candidates[i].ScoreHash < candidates[j].ScoreHash
		}
		if candidates[i].RequestCreatedUnix != candidates[j].RequestCreatedUnix {
			return candidates[i].RequestCreatedUnix < candidates[j].RequestCreatedUnix
		}
		if candidates[i].ResponseCreatedUnix != candidates[j].ResponseCreatedUnix {
			return candidates[i].ResponseCreatedUnix < candidates[j].ResponseCreatedUnix
		}
		if candidates[i].IntentID != candidates[j].IntentID {
			return candidates[i].IntentID < candidates[j].IntentID
		}
		return candidates[i].ResponseID < candidates[j].ResponseID
	})
}

func canonicalMatchCandidateHash(candidates []MatchCandidate) string {
	if len(candidates) == 0 {
		return ""
	}
	var b strings.Builder
	for i := range candidates {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(candidates[i].MatchID)
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func canonicalBuildHash(items []proposerMatchBuildItem) string {
	if len(items) == 0 {
		return ""
	}
	sorted := make([]proposerMatchBuildItem, len(items))
	copy(sorted, items)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].MatchID < sorted[j].MatchID
	})

	var b strings.Builder
	for i := range sorted {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(sorted[i].MatchID)
		b.WriteString("|")
		b.WriteString(sorted[i].FinalizeID)
		b.WriteString("|")
		if sorted[i].HasMatchCertificate {
			b.WriteString(sorted[i].MsgPayloadHash)
		} else {
			b.WriteString("none")
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
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

func (s *inMemoryStore) hasExpiredLocked(nowUnix int64) bool {
	for _, rec := range s.intents {
		if rec.expiresUnix < nowUnix {
			return true
		}
	}
	for _, rec := range s.responses {
		if rec.expiresUnix < nowUnix {
			return true
		}
	}
	for _, rec := range s.finalizes {
		if rec.expiresUnix < nowUnix {
			return true
		}
	}
	return false
}

func (s *inMemoryStore) cleanupExpiredLocked(nowUnix int64) {
	if nowUnix <= 0 {
		return
	}

	// Remove expired finalizes and their pending operations.
	for key, rec := range s.finalizes {
		if rec.expiresUnix >= nowUnix {
			continue
		}
		board := BoardRecord{
			RecordType:       RecordTypeFinalize,
			PoolID:           rec.poolID,
			IntentID:         rec.intentID,
			ResponseID:       rec.responseID,
			FinalizeID:       rec.finalizeID,
			IntentSignHash:   rec.intentSignHash,
			ResponseSignHash: rec.responseSignHash,
			FinalizeSignHash: rec.finalizeSignHash,
		}
		delete(s.pendingOps, buildOperationID(board))
		delete(s.finalizes, key)
	}

	// Remove expired responses and dependent finalizes/pending operations.
	for key, rec := range s.responses {
		if rec.expiresUnix >= nowUnix {
			continue
		}
		board := BoardRecord{
			RecordType:       RecordTypeResponse,
			PoolID:           rec.poolID,
			IntentID:         rec.intentID,
			ResponseID:       rec.responseID,
			IntentSignHash:   rec.intentSignHash,
			ResponseSignHash: rec.responseSignHash,
		}
		delete(s.pendingOps, buildOperationID(board))
		delete(s.responses, key)

		intentK := intentKey{poolID: key.poolID, intentID: key.intentID}
		s.removeResponseIndexLocked(intentK, key.responseID)

		for fKey, finalize := range s.finalizes {
			if fKey.poolID == key.poolID && fKey.intentID == key.intentID && fKey.responseID == key.responseID {
				finalizeBoard := BoardRecord{
					RecordType:       RecordTypeFinalize,
					PoolID:           finalize.poolID,
					IntentID:         finalize.intentID,
					ResponseID:       finalize.responseID,
					FinalizeID:       finalize.finalizeID,
					IntentSignHash:   finalize.intentSignHash,
					ResponseSignHash: finalize.responseSignHash,
					FinalizeSignHash: finalize.finalizeSignHash,
				}
				delete(s.pendingOps, buildOperationID(finalizeBoard))
				delete(s.finalizes, fKey)
			}
		}
	}

	// Remove expired intents and dependent responses/finalizes/pending operations.
	for key, rec := range s.intents {
		if rec.expiresUnix >= nowUnix {
			continue
		}
		board := BoardRecord{
			RecordType:     RecordTypeIntent,
			PoolID:         rec.poolID,
			IntentID:       rec.intentID,
			IntentSignHash: rec.intentSignHash,
		}
		delete(s.pendingOps, buildOperationID(board))
		delete(s.intents, key)

		for rKey, response := range s.responses {
			if rKey.poolID == key.poolID && rKey.intentID == key.intentID {
				responseBoard := BoardRecord{
					RecordType:       RecordTypeResponse,
					PoolID:           response.poolID,
					IntentID:         response.intentID,
					ResponseID:       response.responseID,
					IntentSignHash:   response.intentSignHash,
					ResponseSignHash: response.responseSignHash,
				}
				delete(s.pendingOps, buildOperationID(responseBoard))
				delete(s.responses, rKey)
			}
		}

		for fKey, finalize := range s.finalizes {
			if fKey.poolID == key.poolID && fKey.intentID == key.intentID {
				finalizeBoard := BoardRecord{
					RecordType:       RecordTypeFinalize,
					PoolID:           finalize.poolID,
					IntentID:         finalize.intentID,
					ResponseID:       finalize.responseID,
					FinalizeID:       finalize.finalizeID,
					IntentSignHash:   finalize.intentSignHash,
					ResponseSignHash: finalize.responseSignHash,
					FinalizeSignHash: finalize.finalizeSignHash,
				}
				delete(s.pendingOps, buildOperationID(finalizeBoard))
				delete(s.finalizes, fKey)
			}
		}

		delete(s.responsesByIntent, key)
	}
}

func (s *inMemoryStore) removeResponseIndexLocked(key intentKey, responseID string) {
	entries := s.responsesByIntent[key]
	if len(entries) == 0 {
		return
	}
	out := entries[:0]
	for i := range entries {
		if entries[i].responseID == responseID {
			continue
		}
		out = append(out, entries[i])
	}
	if len(out) == 0 {
		delete(s.responsesByIntent, key)
		return
	}
	trimmed := make([]responseRecord, len(out))
	copy(trimmed, out)
	s.responsesByIntent[key] = trimmed
}

func (s *inMemoryStore) pickFinalizeForMatchLocked(poolID, intentID, responseID string, nowUnix int64) (finalizeRecord, bool) {
	var (
		best finalizeRecord
		has  bool
	)
	for key, rec := range s.finalizes {
		if key.poolID != poolID || key.intentID != intentID || key.responseID != responseID {
			continue
		}
		if rec.expiresUnix < nowUnix {
			continue
		}
		if !has || finalizeRecordLess(rec, best) {
			best = rec
			has = true
		}
	}
	return best, has
}

func finalizeRecordLess(a, b finalizeRecord) bool {
	if a.createdUnix != b.createdUnix {
		return a.createdUnix < b.createdUnix
	}
	if a.finalizeID != b.finalizeID {
		return a.finalizeID < b.finalizeID
	}
	if a.finalizeSignHash != b.finalizeSignHash {
		return a.finalizeSignHash < b.finalizeSignHash
	}
	if a.sender != b.sender {
		return a.sender < b.sender
	}
	return a.recipient < b.recipient
}

func (s *inMemoryStore) consumeIntentMatchLocked(intentK intentKey) {
	intentRec, exists := s.intents[intentK]
	if exists {
		delete(s.intents, intentK)
		intentBoard := BoardRecord{
			RecordType:     RecordTypeIntent,
			PoolID:         intentRec.poolID,
			IntentID:       intentRec.intentID,
			IntentSignHash: intentRec.intentSignHash,
		}
		delete(s.pendingOps, buildOperationID(intentBoard))
	}

	for responseK, responseRec := range s.responses {
		if responseK.poolID != intentK.poolID || responseK.intentID != intentK.intentID {
			continue
		}
		responseBoard := BoardRecord{
			RecordType:       RecordTypeResponse,
			PoolID:           responseRec.poolID,
			IntentID:         responseRec.intentID,
			ResponseID:       responseRec.responseID,
			IntentSignHash:   responseRec.intentSignHash,
			ResponseSignHash: responseRec.responseSignHash,
		}
		delete(s.pendingOps, buildOperationID(responseBoard))
		delete(s.responses, responseK)
	}
	delete(s.responsesByIntent, intentK)

	for finalizeK, finalizeRec := range s.finalizes {
		if finalizeK.poolID != intentK.poolID || finalizeK.intentID != intentK.intentID {
			continue
		}
		finalizeBoard := BoardRecord{
			RecordType:       RecordTypeFinalize,
			PoolID:           finalizeRec.poolID,
			IntentID:         finalizeRec.intentID,
			ResponseID:       finalizeRec.responseID,
			FinalizeID:       finalizeRec.finalizeID,
			IntentSignHash:   finalizeRec.intentSignHash,
			ResponseSignHash: finalizeRec.responseSignHash,
			FinalizeSignHash: finalizeRec.finalizeSignHash,
		}
		delete(s.pendingOps, buildOperationID(finalizeBoard))
		delete(s.finalizes, finalizeK)
	}
}

func (s *inMemoryStore) enqueuePendingOperation(record BoardRecord, matchCertificate []byte) error {
	op, err := toProposedOperation(record, matchCertificate, DefaultInjectedMatchSubmitter)
	if err != nil {
		return err
	}
	s.pendingOps[op.OperationID] = op
	if s.enableABCI {
		PublishABCIProposedOperation(op)
	}
	return nil
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

func toProposedOperation(record BoardRecord, matchCertificate []byte, submitter string) (ProposedOperation, error) {
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
	if len(op.MatchCertificate) > 0 {
		payload, err := BuildSubmitMatchCertificateMsgPayload(op.MatchCertificate, submitter)
		if err != nil {
			return ProposedOperation{}, fmt.Errorf("%w: %v", errOperationBuildFailed, err)
		}
		op.MatchSubmitMsgPayload = payload
	}
	op.OperationID = BuildOperationIDFromProposedOperation(op)
	return op, nil
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

func buildMatchID(req intentRecord, resp responseRecord) string {
	var b strings.Builder
	b.WriteString(req.poolID)
	b.WriteString("|")
	b.WriteString(req.intentID)
	b.WriteString("|")
	b.WriteString(resp.responseID)
	b.WriteString("|")
	b.WriteString(req.intentSignHash)
	b.WriteString("|")
	b.WriteString(resp.responseSignHash)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func buildMatchScoreHash(req intentRecord, resp responseRecord) string {
	var b strings.Builder
	b.WriteString(req.poolID)
	b.WriteString("|")
	b.WriteString(req.intentID)
	b.WriteString("|")
	b.WriteString(req.sender)
	b.WriteString("|")
	b.WriteString(resp.sender)
	b.WriteString("|")
	b.WriteString(resp.responseID)
	b.WriteString("|")
	b.WriteString(req.intentSignHash)
	b.WriteString("|")
	b.WriteString(resp.responseSignHash)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func responseLexLess(a, b responseRecord) bool {
	if a.createdUnix != b.createdUnix {
		return a.createdUnix < b.createdUnix
	}
	if a.responseID != b.responseID {
		return a.responseID < b.responseID
	}
	if a.sender != b.sender {
		return a.sender < b.sender
	}
	return a.responseSignHash < b.responseSignHash
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
