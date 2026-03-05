package matchboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// NewHandler builds an authenticated and rate-limited off-chain board handler.
func NewHandler(cfg Config) (http.Handler, error) {
	normalized, err := normalizeConfig(cfg)
	if err != nil {
		return nil, err
	}

	h := &handler{
		cfg:         normalized,
		store:       newInMemoryStore(),
		rateLimiter: newFixedWindowRateLimiter(normalized.RateLimitRequests, normalized.RateLimitWindow),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/intents", h.withAuth(h.handlePublishIntent))
	mux.HandleFunc("/v1/responses", h.withAuth(h.handlePublishResponse))
	mux.HandleFunc("/v1/finalize", h.withAuth(h.handlePublishFinalize))
	mux.HandleFunc("/v1/inbox", h.withAuth(h.handleInbox))
	mux.HandleFunc("/v1/outbox", h.withAuth(h.handleOutbox))

	return mux, nil
}

type handler struct {
	cfg         Config
	store       *inMemoryStore
	rateLimiter *fixedWindowRateLimiter
}

type authedHandler func(http.ResponseWriter, *http.Request, string)

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
	if r.Body == nil {
		return &validationError{code: errorCodeInvalidRequest, field: "body", message: "request body is required"}
	}

	payload, err := io.ReadAll(io.LimitReader(r.Body, h.cfg.MaxBodyBytes+1))
	if err != nil {
		return &validationError{code: errorCodeInvalidRequest, field: "body", message: "failed to read request body"}
	}
	if int64(len(payload)) > h.cfg.MaxBodyBytes {
		return &validationError{code: errorCodeInvalidRequest, field: "body", message: "request body exceeds max size"}
	}

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
	case errors.Is(err, errIntentNotFound):
		h.writeError(w, http.StatusNotFound, errorCodeNotFound, "referenced intent not found", "intent_id", "", false)
	case errors.Is(err, errResponseNotFound):
		h.writeError(w, http.StatusNotFound, errorCodeNotFound, "referenced response not found", "response_id", "", false)
	case errors.Is(err, errHashMismatch):
		h.writeError(w, http.StatusConflict, errorCodeHashMismatch, "hash binding mismatch", "", "dependent sign hash does not match stored record", false)
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
