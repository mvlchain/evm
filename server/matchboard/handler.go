package matchboard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
)

// NewHandler builds an authenticated and rate-limited off-chain board handler.
func NewHandler(cfg Config) (http.Handler, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	h := &handler{
		cfg:         normalized,
		store:       newInMemoryStore(normalized.EnableABCIProposerOps, normalized.MatcherShardCount),
		rateLimiter: newFixedWindowRateLimiter(normalized.RateLimitRequests, normalized.RateLimitWindow),
		gossipClient: &http.Client{
			Timeout: normalized.GossipTimeout,
		},
		seenGossip: make(map[string]int64),
	}
	if normalized.EnableIntentStream {
		h.intentHub = newIntentStreamHub(normalized.IntentStreamBuffer)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/intents", h.withAuth(h.handlePublishIntent))
	mux.HandleFunc("/v1/responses", h.withAuth(h.handlePublishResponse))
	mux.HandleFunc("/v1/finalize", h.withAuth(h.handlePublishFinalize))
	mux.HandleFunc("/v1/inbox", h.withAuth(h.handleInbox))
	mux.HandleFunc("/v1/outbox", h.withAuth(h.handleOutbox))
	mux.HandleFunc("/v1/matcher/candidates", h.withAuth(h.handleListMatchCandidates))
	mux.HandleFunc("/v1/proposer/matches", h.withAuth(h.handleListProposerMatches))
	mux.HandleFunc("/v1/proposer/matches/build", h.withAuth(h.handleBuildProposerMatches))
	mux.HandleFunc("/v1/proposer/matches/commit", h.withAuth(h.handleCommitProposerMatches))
	mux.HandleFunc("/v1/proposer/operations", h.withAuth(h.handleListProposedOperations))
	mux.HandleFunc("/v1/proposer/operations/commit", h.withAuth(h.handleCommitProposedOperations))
	if normalized.EnableIntentStream {
		mux.HandleFunc("/v1/stream/intents", h.withAuth(h.handleStreamIntents))
	}
	if normalized.GossipSharedSecret != "" {
		mux.HandleFunc("/v1/internal/gossip/intents", h.withGossipAuth(h.handleGossipIntent))
		mux.HandleFunc("/v1/internal/gossip/responses", h.withGossipAuth(h.handleGossipResponse))
		mux.HandleFunc("/v1/internal/gossip/finalize", h.withGossipAuth(h.handleGossipFinalize))
	}

	return &matchboardHTTPHandler{
		core: h,
		mux:  mux,
	}, nil
}

type matchboardHTTPHandler struct {
	core *handler
	mux  *http.ServeMux
}

func (h *matchboardHTTPHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mux.ServeHTTP(w, r)
}

func (h *matchboardHTTPHandler) IngestGossipEnvelope(
	ctx context.Context,
	intentType string,
	envelope GossipEnvelope,
	origin string,
) error {
	return h.core.IngestGossipEnvelope(ctx, intentType, envelope, origin)
}

type handler struct {
	cfg          Config
	store        *inMemoryStore
	rateLimiter  *fixedWindowRateLimiter
	gossipClient *http.Client
	intentHub    *intentStreamHub

	seenMu     sync.Mutex
	seenGossip map[string]int64
}

var _ GossipIngestor = (*handler)(nil)

type authedHandler func(http.ResponseWriter, *http.Request, string)
type gossipHandler func(http.ResponseWriter, *http.Request)

func (h *handler) withAuth(next authedHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		principal, ok := h.authenticatePrincipal(r)
		if !ok {
			h.cfg.Logger.Warn("matchboard request rejected", "status", "unauthorized", "path", r.URL.Path)
			h.writeError(w, http.StatusUnauthorized, errorCodeUnauthorized, "missing or invalid bearer token", "authorization", "", false)
			return
		}

		now := h.cfg.NowFn()
		if !h.rateLimiter.allow(principal, now) {
			h.cfg.Logger.Warn("matchboard request rejected", "status", "rate_limited", "path", r.URL.Path, "principal", principal)
			h.writeError(w, http.StatusTooManyRequests, errorCodeRateLimited, "rate limit exceeded", "", "", true)
			return
		}

		next(w, r, principal)
	}
}

func (h *handler) withGossipAuth(next gossipHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secret := strings.TrimSpace(r.Header.Get(headerGossipSecret))
		if secret == "" || secret != h.cfg.GossipSharedSecret {
			h.cfg.Logger.Warn("matchboard gossip request rejected", "status", "unauthorized", "path", r.URL.Path)
			h.writeError(w, http.StatusUnauthorized, errorCodeUnauthorized, "missing or invalid gossip secret", headerGossipSecret, "", false)
			return
		}
		origin := strings.TrimSpace(r.Header.Get(headerGossipOrigin))
		if origin == "" {
			origin = strings.TrimSpace(r.RemoteAddr)
		}
		if !h.rateLimiter.allow("gossip:"+origin, h.cfg.NowFn()) {
			h.cfg.Logger.Warn("matchboard gossip request rejected", "status", "rate_limited", "path", r.URL.Path, "origin", origin)
			h.writeError(w, http.StatusTooManyRequests, errorCodeRateLimited, "gossip rate limit exceeded", "", "", true)
			return
		}
		next(w, r)
	}
}

// IngestGossipEnvelope consumes a gossip envelope delivered by node-native transport (e.g. CometBFT reactor).
func (h *handler) IngestGossipEnvelope(ctx context.Context, intentType string, envelope GossipEnvelope, origin string) error {
	body, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal gossip envelope: %w", err)
	}

	var (
		path string
		fn   gossipHandler
	)
	switch strings.ToLower(strings.TrimSpace(intentType)) {
	case IntentTypeRequest:
		path = "/v1/internal/gossip/intents"
		fn = h.handleGossipIntent
	case IntentTypeAccept:
		path = "/v1/internal/gossip/responses"
		fn = h.handleGossipResponse
	case IntentTypeFinalize:
		path = "/v1/internal/gossip/finalize"
		fn = h.handleGossipFinalize
	default:
		return fmt.Errorf("unsupported gossip intent_type %q", intentType)
	}

	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	if ctx != nil {
		req = req.WithContext(ctx)
	}
	if origin != "" {
		req.Header.Set(headerGossipOrigin, origin)
	}
	rec := httptest.NewRecorder()
	fn(rec, req)
	if rec.Code >= http.StatusMultipleChoices {
		return fmt.Errorf("gossip ingest failed: status=%d body=%s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}
	return nil
}

func (h *handler) handlePublishIntent(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodPost) {
		return
	}

	var req PublishIntentRequest
	if err := h.decodeJSONBody(r, &req); err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	nowUnix := h.cfg.NowFn().Unix()
	if err := validateAndNormalizeIntent(&req, principal, nowUnix); err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	result, err := h.store.createIntent(req, nowUnix)
	if err != nil {
		h.handleStoreError(w, err, "intent", req.PoolID, req.IntentID, req.IntentSignHash)
		return
	}

	h.cfg.Logger.Info("matchboard intent stored",
		"status", "stored",
		"principal", principal,
		"pool_id", req.PoolID,
		"intent_id", req.IntentID,
		"intent_sign_hash", req.IntentSignHash,
	)
	h.publishIntentStreamEvent(intentStreamEvent{
		EventID:        result.IntentSignHash,
		IntentType:     IntentTypeRequest,
		PoolID:         req.PoolID,
		IntentID:       req.IntentID,
		Requester:      req.Sender,
		Responder:      req.Recipient,
		ExpiryUnix:     req.ExpiresUnix,
		CreatedUnix:    result.StoredUnix,
		IntentSignHash: req.IntentSignHash,
	})
	h.gossipPayload(r, "/v1/internal/gossip/intents", req, IntentTypeRequest, req.Sender, req.Recipient, req.ExpiresUnix)
	h.writeJSON(w, http.StatusCreated, result)
}

func (h *handler) handlePublishResponse(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodPost) {
		return
	}

	var req PublishResponseRequest
	if err := h.decodeJSONBody(r, &req); err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	nowUnix := h.cfg.NowFn().Unix()
	if err := validateAndNormalizeResponse(&req, principal, nowUnix); err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	result, err := h.store.createResponse(req, nowUnix)
	if err != nil {
		h.handleStoreError(w, err, "response", req.PoolID, req.IntentID, req.ResponseSignHash)
		return
	}

	h.cfg.Logger.Info("matchboard response stored",
		"status", "stored",
		"principal", principal,
		"pool_id", req.PoolID,
		"intent_id", req.IntentID,
		"response_id", req.ResponseID,
		"response_sign_hash", req.ResponseSignHash,
	)
	h.publishIntentStreamEvent(intentStreamEvent{
		EventID:          result.ResponseSignHash,
		IntentType:       IntentTypeAccept,
		PoolID:           req.PoolID,
		IntentID:         req.IntentID,
		ResponseID:       req.ResponseID,
		Requester:        req.Recipient,
		Responder:        req.Sender,
		ExpiryUnix:       req.ExpiresUnix,
		CreatedUnix:      result.StoredUnix,
		IntentSignHash:   req.IntentSignHash,
		ResponseSignHash: req.ResponseSignHash,
	})
	h.gossipPayload(r, "/v1/internal/gossip/responses", req, IntentTypeAccept, req.Recipient, req.Sender, req.ExpiresUnix)
	h.writeJSON(w, http.StatusCreated, result)
}

func (h *handler) handlePublishFinalize(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodPost) {
		return
	}

	var req PublishFinalizeRequest
	if err := h.decodeJSONBody(r, &req); err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	nowUnix := h.cfg.NowFn().Unix()
	if err := validateAndNormalizeFinalize(&req, principal, nowUnix); err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	result, err := h.store.createFinalize(req, nowUnix)
	if err != nil {
		h.handleStoreError(w, err, "finalize", req.PoolID, req.IntentID, req.FinalizeSignHash)
		return
	}

	h.cfg.Logger.Info("matchboard finalize stored",
		"status", "stored",
		"principal", principal,
		"pool_id", req.PoolID,
		"intent_id", req.IntentID,
		"response_id", req.ResponseID,
		"finalize_id", req.FinalizeID,
		"finalize_sign_hash", req.FinalizeSignHash,
	)
	h.publishIntentStreamEvent(intentStreamEvent{
		EventID:          result.FinalizeSignHash,
		IntentType:       IntentTypeFinalize,
		PoolID:           req.PoolID,
		IntentID:         req.IntentID,
		ResponseID:       req.ResponseID,
		FinalizeID:       req.FinalizeID,
		Requester:        req.Sender,
		Responder:        req.Recipient,
		ExpiryUnix:       req.ExpiresUnix,
		CreatedUnix:      result.StoredUnix,
		IntentSignHash:   req.IntentSignHash,
		ResponseSignHash: req.ResponseSignHash,
		FinalizeSignHash: req.FinalizeSignHash,
	})
	h.gossipPayload(r, "/v1/internal/gossip/finalize", req, IntentTypeFinalize, req.Sender, req.Recipient, req.ExpiresUnix)
	h.writeJSON(w, http.StatusCreated, result)
}

func (h *handler) handleGossipIntent(w http.ResponseWriter, r *http.Request) {
	if !h.requireMethod(w, r, http.MethodPost) {
		return
	}

	body, err := h.readBodyLimited(r)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	nowUnix := h.cfg.NowFn().Unix()
	req, env, duplicate, err := h.decodeGossipIntentBody(body, nowUnix)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}
	if duplicate {
		h.writeJSON(w, http.StatusOK, publishIntentResponse{
			PoolID:         req.PoolID,
			IntentID:       req.IntentID,
			IntentSignHash: req.IntentSignHash,
			StoredUnix:     nowUnix,
		})
		return
	}

	if err := validateAndNormalizeIntent(&req, req.Sender, nowUnix); err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	result, err := h.store.createIntent(req, nowUnix)
	if err != nil {
		if errors.Is(err, errIntentExists) {
			h.writeJSON(w, http.StatusOK, publishIntentResponse{
				PoolID:         req.PoolID,
				IntentID:       req.IntentID,
				IntentSignHash: req.IntentSignHash,
				StoredUnix:     nowUnix,
			})
			return
		}
		h.handleStoreError(w, err, "intent", req.PoolID, req.IntentID, req.IntentSignHash)
		return
	}

	h.cfg.Logger.Info("matchboard intent gossiped",
		"status", "stored",
		"pool_id", req.PoolID,
		"intent_id", req.IntentID,
		"intent_sign_hash", req.IntentSignHash,
		"origin", strings.TrimSpace(r.Header.Get(headerGossipOrigin)),
	)
	h.publishIntentStreamEvent(intentStreamEvent{
		EventID:        req.IntentSignHash,
		IntentType:     IntentTypeRequest,
		PoolID:         req.PoolID,
		IntentID:       req.IntentID,
		Requester:      req.Sender,
		Responder:      req.Recipient,
		ExpiryUnix:     req.ExpiresUnix,
		CreatedUnix:    result.StoredUnix,
		IntentSignHash: req.IntentSignHash,
	})
	h.forwardGossipEnvelope(r, IntentTypeRequest, "/v1/internal/gossip/intents", env)
	h.writeJSON(w, http.StatusCreated, result)
}

func (h *handler) handleGossipResponse(w http.ResponseWriter, r *http.Request) {
	if !h.requireMethod(w, r, http.MethodPost) {
		return
	}

	body, err := h.readBodyLimited(r)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	nowUnix := h.cfg.NowFn().Unix()
	req, env, duplicate, err := h.decodeGossipResponseBody(body, nowUnix)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}
	if duplicate {
		h.writeJSON(w, http.StatusOK, publishResponseResponse{
			PoolID:           req.PoolID,
			IntentID:         req.IntentID,
			ResponseID:       req.ResponseID,
			ResponseSignHash: req.ResponseSignHash,
			StoredUnix:       nowUnix,
		})
		return
	}

	if err := validateAndNormalizeResponse(&req, req.Sender, nowUnix); err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	result, err := h.store.createResponse(req, nowUnix)
	if err != nil {
		if errors.Is(err, errResponseExists) {
			h.writeJSON(w, http.StatusOK, publishResponseResponse{
				PoolID:           req.PoolID,
				IntentID:         req.IntentID,
				ResponseID:       req.ResponseID,
				ResponseSignHash: req.ResponseSignHash,
				StoredUnix:       nowUnix,
			})
			return
		}
		h.handleStoreError(w, err, "response", req.PoolID, req.IntentID, req.ResponseSignHash)
		return
	}

	h.cfg.Logger.Info("matchboard response gossiped",
		"status", "stored",
		"pool_id", req.PoolID,
		"intent_id", req.IntentID,
		"response_id", req.ResponseID,
		"response_sign_hash", req.ResponseSignHash,
		"origin", strings.TrimSpace(r.Header.Get(headerGossipOrigin)),
	)
	h.publishIntentStreamEvent(intentStreamEvent{
		EventID:          req.ResponseSignHash,
		IntentType:       IntentTypeAccept,
		PoolID:           req.PoolID,
		IntentID:         req.IntentID,
		ResponseID:       req.ResponseID,
		Requester:        req.Recipient,
		Responder:        req.Sender,
		ExpiryUnix:       req.ExpiresUnix,
		CreatedUnix:      result.StoredUnix,
		IntentSignHash:   req.IntentSignHash,
		ResponseSignHash: req.ResponseSignHash,
	})
	h.forwardGossipEnvelope(r, IntentTypeAccept, "/v1/internal/gossip/responses", env)
	h.writeJSON(w, http.StatusCreated, result)
}

func (h *handler) handleGossipFinalize(w http.ResponseWriter, r *http.Request) {
	if !h.requireMethod(w, r, http.MethodPost) {
		return
	}

	body, err := h.readBodyLimited(r)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	nowUnix := h.cfg.NowFn().Unix()
	req, env, duplicate, err := h.decodeGossipFinalizeBody(body, nowUnix)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}
	if duplicate {
		h.writeJSON(w, http.StatusOK, publishFinalizeResponse{
			PoolID:           req.PoolID,
			IntentID:         req.IntentID,
			ResponseID:       req.ResponseID,
			FinalizeID:       req.FinalizeID,
			FinalizeSignHash: req.FinalizeSignHash,
			StoredUnix:       nowUnix,
		})
		return
	}

	if err := validateAndNormalizeFinalize(&req, req.Sender, nowUnix); err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	result, err := h.store.createFinalize(req, nowUnix)
	if err != nil {
		if errors.Is(err, errFinalizeExists) {
			h.writeJSON(w, http.StatusOK, publishFinalizeResponse{
				PoolID:           req.PoolID,
				IntentID:         req.IntentID,
				ResponseID:       req.ResponseID,
				FinalizeID:       req.FinalizeID,
				FinalizeSignHash: req.FinalizeSignHash,
				StoredUnix:       nowUnix,
			})
			return
		}
		h.handleStoreError(w, err, "finalize", req.PoolID, req.IntentID, req.FinalizeSignHash)
		return
	}

	h.cfg.Logger.Info("matchboard finalize gossiped",
		"status", "stored",
		"pool_id", req.PoolID,
		"intent_id", req.IntentID,
		"response_id", req.ResponseID,
		"finalize_id", req.FinalizeID,
		"finalize_sign_hash", req.FinalizeSignHash,
		"origin", strings.TrimSpace(r.Header.Get(headerGossipOrigin)),
	)
	h.publishIntentStreamEvent(intentStreamEvent{
		EventID:          req.FinalizeSignHash,
		IntentType:       IntentTypeFinalize,
		PoolID:           req.PoolID,
		IntentID:         req.IntentID,
		ResponseID:       req.ResponseID,
		FinalizeID:       req.FinalizeID,
		Requester:        req.Sender,
		Responder:        req.Recipient,
		ExpiryUnix:       req.ExpiresUnix,
		CreatedUnix:      result.StoredUnix,
		IntentSignHash:   req.IntentSignHash,
		ResponseSignHash: req.ResponseSignHash,
		FinalizeSignHash: req.FinalizeSignHash,
	})
	h.forwardGossipEnvelope(r, IntentTypeFinalize, "/v1/internal/gossip/finalize", env)
	h.writeJSON(w, http.StatusCreated, result)
}

func (h *handler) handleInbox(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodGet) {
		return
	}

	recipient := strings.TrimSpace(r.URL.Query().Get("recipient"))
	if recipient == "" {
		recipient = principal
	}
	if recipient != principal {
		h.cfg.Logger.Warn("matchboard inbox forbidden", "status", "forbidden", "principal", principal, "recipient", recipient)
		h.writeError(w, http.StatusForbidden, errorCodeForbidden, "recipient must match authenticated principal", "recipient", "", false)
		return
	}

	cursor, limit, err := h.parsePagination(r)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	records, nextCursor, total := h.store.listInbox(recipient, cursor, limit)
	resp := listRecordsResponse{
		Principal:  recipient,
		Records:    records,
		NextCursor: nextCursor,
		Total:      total,
	}

	h.cfg.Logger.Info("matchboard inbox listed",
		"status", "ok",
		"principal", principal,
		"recipient", recipient,
		"record_count", len(records),
		"total", total,
	)
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *handler) handleOutbox(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodGet) {
		return
	}

	sender := strings.TrimSpace(r.URL.Query().Get("sender"))
	if sender == "" {
		sender = principal
	}
	if sender != principal {
		h.cfg.Logger.Warn("matchboard outbox forbidden", "status", "forbidden", "principal", principal, "sender", sender)
		h.writeError(w, http.StatusForbidden, errorCodeForbidden, "sender must match authenticated principal", "sender", "", false)
		return
	}

	cursor, limit, err := h.parsePagination(r)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	records, nextCursor, total := h.store.listOutbox(sender, cursor, limit)
	resp := listRecordsResponse{
		Principal:  sender,
		Records:    records,
		NextCursor: nextCursor,
		Total:      total,
	}

	h.cfg.Logger.Info("matchboard outbox listed",
		"status", "ok",
		"principal", principal,
		"sender", sender,
		"record_count", len(records),
		"total", total,
	)
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *handler) handleListProposedOperations(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodGet) {
		return
	}

	limit, err := h.parseLimit(r)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	operations, canonicalBatchHash, totalPending := h.store.listProposedOperations(limit, h.cfg.NowFn().Unix())
	resp := listProposedOperationsResponse{
		Proposer:           principal,
		Operations:         operations,
		CanonicalBatchHash: canonicalBatchHash,
		TotalPending:       totalPending,
	}

	h.cfg.Logger.Info("matchboard proposer operations listed",
		"status", "ok",
		"principal", principal,
		"operation_count", len(operations),
		"total_pending", totalPending,
		"canonical_batch_hash", canonicalBatchHash,
	)
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *handler) handleListMatchCandidates(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodGet) {
		return
	}

	limit, err := h.parseLimit(r)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	allCandidates, _ := h.store.listMatchCandidates(0, h.cfg.NowFn().Unix())
	filtered := make([]MatchCandidate, 0, len(allCandidates))
	for _, candidate := range allCandidates {
		if identitiesEqual(candidate.Requester, principal) || identitiesEqual(candidate.Responder, principal) {
			filtered = append(filtered, candidate)
		}
	}
	if limit <= 0 || limit > len(filtered) {
		limit = len(filtered)
	}
	page := filtered
	if len(filtered) > limit {
		page = filtered[:limit]
	}

	resp := listMatchCandidatesResponse{
		Matcher:    principal,
		Candidates: page,
		Total:      uint64(len(filtered)),
	}

	h.cfg.Logger.Info("matchboard matcher candidates listed",
		"status", "ok",
		"principal", principal,
		"candidate_count", len(page),
		"total", len(filtered),
	)
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *handler) handleListProposerMatches(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodGet) {
		return
	}

	limit, err := h.parseLimit(r)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	matches, canonicalHash, total := h.store.listProposerMatches(limit, h.cfg.NowFn().Unix())
	resp := listProposerMatchesResponse{
		Proposer:                principal,
		Matches:                 matches,
		CanonicalMatchBatchHash: canonicalHash,
		TotalPending:            total,
	}

	h.cfg.Logger.Info("matchboard proposer matches listed",
		"status", "ok",
		"principal", principal,
		"match_count", len(matches),
		"total_pending", total,
		"canonical_match_batch_hash", canonicalHash,
	)
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *handler) handleCommitProposerMatches(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodPost) {
		return
	}

	var req commitProposerMatchesRequest
	if err := h.decodeJSONBody(r, &req); err != nil {
		h.handleValidationFailure(w, err)
		return
	}
	if len(req.MatchIDs) == 0 {
		h.writeError(w, http.StatusBadRequest, errorCodeInvalidRequest, "match_ids must include at least one id", "match_ids", "", false)
		return
	}

	committed, remaining, canonicalHash, err := h.store.commitProposerMatches(req.MatchIDs, h.cfg.NowFn().Unix())
	if err != nil {
		h.cfg.Logger.Warn("matchboard proposer match commit rejected",
			"status", "rejected",
			"principal", principal,
			"reason", err.Error(),
		)
		switch {
		case errors.Is(err, errMatchEmpty):
			h.writeError(w, http.StatusBadRequest, errorCodeInvalidRequest, "match_ids must not contain empty values", "match_ids", "", false)
		case errors.Is(err, errMatchDuplicate):
			h.writeError(w, http.StatusConflict, errorCodeStateConflict, "duplicate match id in request", "match_ids", "", false)
		case errors.Is(err, errMatchNotFound):
			h.writeError(w, http.StatusConflict, errorCodeStateConflict, "match commit failed and rolled back", "match_ids", "match id not found in pending set", false)
		default:
			h.writeError(w, http.StatusInternalServerError, errorCodeInternal, "internal error", "", "", true)
		}
		return
	}

	resp := commitProposerMatchesResponse{
		Proposer:                principal,
		Committed:               committed,
		Remaining:               remaining,
		CanonicalMatchBatchHash: canonicalHash,
	}

	h.cfg.Logger.Info("matchboard proposer match commit applied",
		"status", "ok",
		"principal", principal,
		"committed", committed,
		"remaining", remaining,
		"canonical_match_batch_hash", canonicalHash,
	)
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *handler) handleBuildProposerMatches(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodPost) {
		return
	}

	var req buildProposerMatchesRequest
	if err := h.decodeJSONBody(r, &req); err != nil {
		h.handleValidationFailure(w, err)
		return
	}
	if len(req.MatchIDs) == 0 {
		h.writeError(w, http.StatusBadRequest, errorCodeInvalidRequest, "match_ids must include at least one id", "match_ids", "", false)
		return
	}

	submitter := strings.TrimSpace(req.Submitter)
	if submitter == "" {
		submitter = principal
	}
	if !identitiesEqual(submitter, principal) {
		h.writeError(w, http.StatusForbidden, errorCodeForbidden, "submitter must match authenticated principal", "submitter", "", false)
		return
	}

	requireCertificate := true
	if req.RequireCertificate != nil {
		requireCertificate = *req.RequireCertificate
	}

	items, canonicalHash, err := h.store.buildProposerMatches(req.MatchIDs, submitter, h.cfg.NowFn().Unix(), requireCertificate)
	if err != nil {
		h.cfg.Logger.Warn("matchboard proposer match build rejected",
			"status", "rejected",
			"principal", principal,
			"submitter", submitter,
			"reason", err.Error(),
		)
		switch {
		case errors.Is(err, errMatchEmpty):
			h.writeError(w, http.StatusBadRequest, errorCodeInvalidRequest, "match_ids must not contain empty values", "match_ids", "", false)
		case errors.Is(err, errMatchDuplicate):
			h.writeError(w, http.StatusConflict, errorCodeStateConflict, "duplicate match id in request", "match_ids", "", false)
		case errors.Is(err, errMatchNotFound):
			h.writeError(w, http.StatusConflict, errorCodeStateConflict, "match id not found in pending set", "match_ids", "", false)
		case errors.Is(err, errMatchFinalizeNotFound):
			h.writeError(w, http.StatusConflict, errorCodeStateConflict, "finalize artifact not found for selected match", "match_ids", "", false)
		case errors.Is(err, errMatchCertificateMissing):
			h.writeError(w, http.StatusConflict, errorCodeStateConflict, "match certificate missing for selected match", "match_ids", "", false)
		case errors.Is(err, errMatchCertificateInvalid):
			h.writeError(w, http.StatusConflict, errorCodeStateConflict, "match certificate invalid for selected match", "match_ids", "", false)
		default:
			h.writeError(w, http.StatusInternalServerError, errorCodeInternal, "internal error", "", "", true)
		}
		return
	}

	resp := buildProposerMatchesResponse{
		Proposer:           principal,
		Submitter:          submitter,
		Items:              items,
		CanonicalBuildHash: canonicalHash,
		RequireCertificate: requireCertificate,
	}

	h.cfg.Logger.Info("matchboard proposer match build generated",
		"status", "ok",
		"principal", principal,
		"submitter", submitter,
		"item_count", len(items),
		"canonical_build_hash", canonicalHash,
	)
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *handler) handleCommitProposedOperations(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodPost) {
		return
	}

	var req commitProposedOperationsRequest
	if err := h.decodeJSONBody(r, &req); err != nil {
		h.handleValidationFailure(w, err)
		return
	}
	if len(req.OperationIDs) == 0 {
		h.writeError(w, http.StatusBadRequest, errorCodeInvalidRequest, "operation_ids must include at least one id", "operation_ids", "", false)
		return
	}

	committed, remaining, canonicalBatchHash, err := h.store.commitProposedOperations(req.OperationIDs)
	if err != nil {
		h.cfg.Logger.Warn("matchboard proposer operation commit rejected",
			"status", "rejected",
			"principal", principal,
			"reason", err.Error(),
		)
		switch {
		case errors.Is(err, errOperationEmpty):
			h.writeError(w, http.StatusBadRequest, errorCodeInvalidRequest, "operation_ids must not contain empty values", "operation_ids", "", false)
		case errors.Is(err, errOperationDuplicate):
			h.writeError(w, http.StatusConflict, errorCodeStateConflict, "duplicate operation id in request", "operation_ids", "", false)
		case errors.Is(err, errOperationNotFound):
			h.writeError(w, http.StatusConflict, errorCodeStateConflict, "operation commit failed and rolled back", "operation_ids", "operation id not found in pending set", false)
		default:
			h.writeError(w, http.StatusInternalServerError, errorCodeInternal, "internal error", "", "", true)
		}
		return
	}

	resp := commitProposedOperationsResponse{
		Proposer:           principal,
		Committed:          committed,
		Remaining:          remaining,
		CanonicalBatchHash: canonicalBatchHash,
	}

	h.cfg.Logger.Info("matchboard proposer operation commit applied",
		"status", "ok",
		"principal", principal,
		"committed", committed,
		"remaining", remaining,
		"canonical_batch_hash", canonicalBatchHash,
	)
	h.writeJSON(w, http.StatusOK, resp)
}

func (h *handler) parsePagination(r *http.Request) (int, int, error) {
	query := r.URL.Query()

	cursor := 0
	if raw := strings.TrimSpace(query.Get("cursor")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v < 0 {
			return 0, 0, &validationError{code: errorCodeInvalidRequest, field: "cursor", message: "cursor must be a non-negative integer"}
		}
		cursor = v
	}

	limit := h.cfg.DefaultPageLimit
	if raw := strings.TrimSpace(query.Get("limit")); raw != "" {
		v, err := strconv.Atoi(raw)
		if err != nil || v <= 0 {
			return 0, 0, &validationError{code: errorCodeInvalidRequest, field: "limit", message: "limit must be a positive integer"}
		}
		limit = v
	}
	if limit > h.cfg.MaxPageLimit {
		limit = h.cfg.MaxPageLimit
	}

	return cursor, limit, nil
}

func (h *handler) parseLimit(r *http.Request) (int, error) {
	limit := h.cfg.DefaultPageLimit
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return limit, nil
	}

	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return 0, &validationError{code: errorCodeInvalidRequest, field: "limit", message: "limit must be a positive integer"}
	}
	if v > h.cfg.MaxPageLimit {
		return h.cfg.MaxPageLimit, nil
	}
	return v, nil
}

func (h *handler) authenticatePrincipal(r *http.Request) (string, bool) {
	header := strings.TrimSpace(r.Header.Get("Authorization"))
	if header == "" {
		return "", false
	}

	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}

	principal, ok := h.cfg.TokenPrincipalMap[parts[1]]
	if !ok {
		return "", false
	}

	return principal, true
}

func (h *handler) decodeJSONBody(r *http.Request, dst any) error {
	payload, err := h.readBodyLimited(r)
	if err != nil {
		return err
	}
	return decodeStrictJSON(payload, dst)
}

func (h *handler) readBodyLimited(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, &validationError{code: errorCodeInvalidRequest, field: "body", message: "request body is required"}
	}

	payload, err := io.ReadAll(io.LimitReader(r.Body, h.cfg.MaxBodyBytes+1))
	if err != nil {
		return nil, &validationError{code: errorCodeInvalidRequest, field: "body", message: "failed to read request body"}
	}
	if int64(len(payload)) > h.cfg.MaxBodyBytes {
		return nil, &validationError{code: errorCodeInvalidRequest, field: "body", message: "request body exceeds max size"}
	}
	return payload, nil
}

func decodeStrictJSON(payload []byte, dst any) error {
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return &validationError{code: errorCodeInvalidRequest, field: "body", message: "request body is required"}
		}
		return &validationError{code: errorCodeInvalidRequest, field: "body", message: "invalid JSON payload", detail: err.Error()}
	}

	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return &validationError{code: errorCodeInvalidRequest, field: "body", message: "request body must contain a single JSON object"}
	}

	return nil
}

func (h *handler) handleStoreError(w http.ResponseWriter, err error, artifactType, poolID, intentID, hash string) {
	h.cfg.Logger.Warn("matchboard store rejected artifact",
		"status", "rejected",
		"artifact_type", artifactType,
		"pool_id", poolID,
		"intent_id", intentID,
		"hash", hash,
		"reason", err.Error(),
	)

	switch {
	case errors.Is(err, errIntentExists), errors.Is(err, errResponseExists), errors.Is(err, errFinalizeExists):
		h.writeError(w, http.StatusConflict, errorCodeReplayDetected, "duplicate artifact submission", "", "composite key already exists", false)
	case errors.Is(err, errIntentExpired), errors.Is(err, errResponseExpired):
		h.writeError(w, http.StatusBadRequest, errorCodeExpired, "referenced artifact is expired", "", "", false)
	case errors.Is(err, errIntentNotFound):
		h.writeError(w, http.StatusNotFound, errorCodeNotFound, "referenced intent not found", "intent_id", "", false)
	case errors.Is(err, errResponseNotFound):
		h.writeError(w, http.StatusNotFound, errorCodeNotFound, "referenced response not found", "response_id", "", false)
	case errors.Is(err, errHashMismatch):
		h.writeError(w, http.StatusConflict, errorCodeHashMismatch, "hash binding mismatch", "", "dependent sign hash does not match stored record", false)
	case errors.Is(err, errOperationBuildFailed):
		h.writeError(w, http.StatusInternalServerError, errorCodeInternal, "failed to build proposer operation", "", "", true)
	default:
		h.writeError(w, http.StatusInternalServerError, errorCodeInternal, "internal error", "", "", true)
	}
}

func (h *handler) handleValidationFailure(w http.ResponseWriter, err error) {
	var vErr *validationError
	if errors.As(err, &vErr) {
		status := http.StatusBadRequest
		switch vErr.code {
		case errorCodeUnauthorized:
			status = http.StatusUnauthorized
		case errorCodeForbidden:
			status = http.StatusForbidden
		case errorCodeExpired:
			status = http.StatusBadRequest
		case errorCodeSignerMismatch:
			status = http.StatusBadRequest
		}
		h.writeError(w, status, vErr.code, vErr.message, vErr.field, vErr.detail, false)
		return
	}

	h.writeError(w, http.StatusBadRequest, errorCodeInvalidRequest, err.Error(), "", "", false)
}

func (h *handler) requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}

	w.Header().Set("Allow", method)
	h.writeError(w, http.StatusMethodNotAllowed, errorCodeInvalidRequest, fmt.Sprintf("method %s not allowed", r.Method), "method", "", false)
	return false
}

func (h *handler) writeError(w http.ResponseWriter, status int, code, message, field, detail string, retryable bool) {
	env := errorEnvelope{
		Error: errorStatus{
			Code:          code,
			SuggestedCode: suggestedCodeByError[code],
			Message:       message,
			Field:         field,
			Detail:        detail,
			Retryable:     retryable,
		},
	}
	h.writeJSON(w, status, env)
}

func (h *handler) writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		h.cfg.Logger.Error("matchboard failed to write JSON response", "status", "write_failed", "code", status)
	}
}

func (h *handler) gossipPayload(
	r *http.Request,
	gossipPath string,
	payload any,
	intentType string,
	requester string,
	responder string,
	expiryUnix int64,
) {
	if h.cfg.GossipPublisher == nil && (len(h.cfg.GossipPeers) == 0 || h.cfg.GossipSharedSecret == "") {
		return
	}

	body, err := json.Marshal(payload)
	if err != nil {
		h.cfg.Logger.Warn("matchboard gossip marshal failed", "path", gossipPath, "error", err.Error())
		return
	}

	nowUnix := h.cfg.NowFn().Unix()
	gossipExpiry := nowUnix + int64(h.cfg.GossipMessageTTL.Seconds())
	if expiryUnix > 0 && expiryUnix < gossipExpiry {
		gossipExpiry = expiryUnix
	}

	envelope := GossipEnvelope{
		MessageID:   buildGossipMessageID(intentType, requester, responder, body),
		IntentType:  intentType,
		Requester:   requester,
		Responder:   responder,
		ExpiryUnix:  gossipExpiry,
		CreatedUnix: nowUnix,
		Hops:        0,
		Payload:     body,
	}
	h.markGossipMessageSeen(envelope.MessageID, nowUnix)
	h.sendGossipEnvelope(r.Context(), intentType, gossipPath, envelope)
}

func (h *handler) forwardGossipEnvelope(r *http.Request, intentType string, gossipPath string, envelope *GossipEnvelope) {
	if envelope == nil {
		return
	}
	if envelope.Hops >= h.cfg.GossipMaxHops {
		return
	}
	next := *envelope
	next.Hops++
	h.sendGossipEnvelope(r.Context(), intentType, gossipPath, next)
}

func (h *handler) sendGossipEnvelope(ctx context.Context, intentType string, gossipPath string, envelope GossipEnvelope) {
	if h.cfg.GossipPublisher != nil {
		h.cfg.GossipPublisher.PublishGossip(ctx, intentType, envelope)
		return
	}
	if len(h.cfg.GossipPeers) == 0 || h.cfg.GossipSharedSecret == "" {
		return
	}

	encoded, err := json.Marshal(envelope)
	if err != nil {
		h.cfg.Logger.Warn("matchboard gossip envelope marshal failed", "path", gossipPath, "error", err.Error())
		return
	}

	origin := h.cfg.GossipNodeID
	if origin == "" {
		origin = "matchboard"
	}

	for _, peer := range h.cfg.GossipPeers {
		url := peer + gossipPath
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
		if err != nil {
			h.cfg.Logger.Warn("matchboard gossip request build failed", "peer", peer, "path", gossipPath, "error", err.Error())
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(headerGossipSecret, h.cfg.GossipSharedSecret)
		req.Header.Set(headerGossipOrigin, origin)

		resp, err := h.gossipClient.Do(req)
		if err != nil {
			h.cfg.Logger.Warn("matchboard gossip relay failed", "peer", peer, "path", gossipPath, "error", err.Error())
			continue
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 32<<10))
		_ = resp.Body.Close()
		if resp.StatusCode >= http.StatusMultipleChoices {
			h.cfg.Logger.Warn("matchboard gossip relay rejected", "peer", peer, "path", gossipPath, "status_code", resp.StatusCode)
		}
	}
}

func (h *handler) decodeGossipIntentBody(body []byte, nowUnix int64) (PublishIntentRequest, *GossipEnvelope, bool, error) {
	return decodeGossipBody[PublishIntentRequest](h, body, nowUnix, IntentTypeRequest)
}

func (h *handler) decodeGossipResponseBody(body []byte, nowUnix int64) (PublishResponseRequest, *GossipEnvelope, bool, error) {
	return decodeGossipBody[PublishResponseRequest](h, body, nowUnix, IntentTypeAccept)
}

func (h *handler) decodeGossipFinalizeBody(body []byte, nowUnix int64) (PublishFinalizeRequest, *GossipEnvelope, bool, error) {
	return decodeGossipBody[PublishFinalizeRequest](h, body, nowUnix, IntentTypeFinalize)
}

func decodeGossipBody[T any](h *handler, body []byte, nowUnix int64, expectedIntentType string) (T, *GossipEnvelope, bool, error) {
	var envelope GossipEnvelope
	if err := decodeStrictJSON(body, &envelope); err == nil && len(envelope.Payload) > 0 {
		if strings.TrimSpace(envelope.MessageID) == "" {
			return *new(T), nil, false, &validationError{code: errorCodeInvalidRequest, field: "message_id", message: "message_id is required"}
		}
		if !strings.EqualFold(strings.TrimSpace(envelope.IntentType), expectedIntentType) {
			return *new(T), nil, false, &validationError{code: errorCodeInvalidRequest, field: "intent_type", message: "unexpected intent_type for gossip endpoint"}
		}
		if envelope.ExpiryUnix < nowUnix {
			var out T
			if err := decodeStrictJSON(envelope.Payload, &out); err != nil {
				return out, nil, false, err
			}
			return out, &envelope, true, nil
		}
		if envelope.Hops < 0 {
			return *new(T), nil, false, &validationError{code: errorCodeInvalidRequest, field: "hops", message: "hops must be non-negative"}
		}
		duplicate := h.markGossipMessageSeen(envelope.MessageID, nowUnix)
		var out T
		if err := decodeStrictJSON(envelope.Payload, &out); err != nil {
			return out, nil, false, err
		}
		return out, &envelope, duplicate, nil
	}

	var req T
	if err := decodeStrictJSON(body, &req); err != nil {
		return req, nil, false, err
	}
	return req, nil, false, nil
}

func (h *handler) markGossipMessageSeen(messageID string, nowUnix int64) bool {
	h.seenMu.Lock()
	defer h.seenMu.Unlock()

	for id, expiry := range h.seenGossip {
		if expiry < nowUnix {
			delete(h.seenGossip, id)
		}
	}

	if expiry, exists := h.seenGossip[messageID]; exists && expiry >= nowUnix {
		return true
	}

	h.seenGossip[messageID] = nowUnix + int64(h.cfg.GossipSeenTTL.Seconds())
	return false
}

func buildGossipMessageID(intentType, requester, responder string, payload []byte) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(strings.TrimSpace(intentType)))
	b.WriteString("|")
	b.WriteString(strings.ToLower(strings.TrimSpace(requester)))
	b.WriteString("|")
	b.WriteString(strings.ToLower(strings.TrimSpace(responder)))
	b.WriteString("|")
	b.Write(payload)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}

func (h *handler) publishIntentStreamEvent(event intentStreamEvent) {
	if h.intentHub == nil {
		return
	}
	h.intentHub.publish(event)
}
