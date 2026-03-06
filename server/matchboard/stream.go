package matchboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type intentStreamFilter struct {
	intentType string
	requester  string
	responder  string
	poolID     string
}

type intentStreamSubscriber struct {
	filter intentStreamFilter
	ch     chan intentStreamEvent
}

type intentStreamHub struct {
	mu     sync.RWMutex
	nextID uint64
	subs   map[uint64]*intentStreamSubscriber
}

func newIntentStreamHub(queueSize int) *intentStreamHub {
	if queueSize <= 0 {
		queueSize = defaultIntentStreamQueue
	}
	return &intentStreamHub{
		subs: make(map[uint64]*intentStreamSubscriber),
	}
}

func (h *intentStreamHub) subscribe(filter intentStreamFilter, queueSize int) (uint64, <-chan intentStreamEvent, func()) {
	if queueSize <= 0 {
		queueSize = defaultIntentStreamQueue
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.nextID++
	id := h.nextID
	sub := &intentStreamSubscriber{
		filter: filter,
		ch:     make(chan intentStreamEvent, queueSize),
	}
	h.subs[id] = sub
	cancel := func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		existing, ok := h.subs[id]
		if !ok {
			return
		}
		delete(h.subs, id)
		close(existing.ch)
	}
	return id, sub.ch, cancel
}

func (h *intentStreamHub) publish(event intentStreamEvent) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, sub := range h.subs {
		if !matchesIntentStreamFilter(sub.filter, event) {
			continue
		}
		select {
		case sub.ch <- event:
		default:
			// Drop oldest event under burst load and keep newest event.
			select {
			case <-sub.ch:
			default:
			}
			select {
			case sub.ch <- event:
			default:
			}
		}
	}
}

func matchesIntentStreamFilter(filter intentStreamFilter, event intentStreamEvent) bool {
	if filter.intentType != "" && !strings.EqualFold(filter.intentType, event.IntentType) {
		return false
	}
	if filter.requester != "" && !identitiesEqual(filter.requester, event.Requester) {
		return false
	}
	if filter.responder != "" && !identitiesEqual(filter.responder, event.Responder) {
		return false
	}
	if filter.poolID != "" && filter.poolID != event.PoolID {
		return false
	}
	return true
}

func (h *handler) handleStreamIntents(w http.ResponseWriter, r *http.Request, principal string) {
	if !h.requireMethod(w, r, http.MethodGet) {
		return
	}
	if h.intentHub == nil {
		h.writeError(w, http.StatusServiceUnavailable, errorCodeBackendUnavailable, "intent stream disabled", "", "", true)
		return
	}

	filter, err := parseIntentStreamFilter(r, principal)
	if err != nil {
		h.handleValidationFailure(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, errorCodeInternal, "stream flush not supported", "", "", true)
		return
	}

	subID, ch, cancel := h.intentHub.subscribe(filter, h.cfg.IntentStreamBuffer)
	defer cancel()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	_, _ = fmt.Fprintf(w, ": subscribed id=%d\n\n", subID)
	flusher.Flush()

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case event, ok := <-ch:
			if !ok {
				return
			}
			encoded, marshalErr := json.Marshal(event)
			if marshalErr != nil {
				continue
			}
			eventID := strings.TrimSpace(event.EventID)
			if eventID == "" {
				eventID = fmt.Sprintf("%d", time.Now().UnixNano())
			}
			_, _ = fmt.Fprintf(w, "id: %s\n", eventID)
			_, _ = fmt.Fprintf(w, "event: intent\n")
			_, _ = fmt.Fprintf(w, "data: %s\n\n", encoded)
			flusher.Flush()
		case <-heartbeat.C:
			_, _ = fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func parseIntentStreamFilter(r *http.Request, principal string) (intentStreamFilter, error) {
	query := r.URL.Query()
	filter := intentStreamFilter{
		intentType: strings.ToLower(strings.TrimSpace(query.Get("intent_type"))),
		requester:  strings.TrimSpace(query.Get("requester")),
		responder:  strings.TrimSpace(query.Get("responder")),
		poolID:     strings.TrimSpace(query.Get("pool_id")),
	}

	switch filter.intentType {
	case "", IntentTypeRequest, IntentTypeAccept, IntentTypeFinalize:
	default:
		return intentStreamFilter{}, &validationError{code: errorCodeInvalidRequest, field: "intent_type", message: "intent_type must be request, accept, or finalize"}
	}

	if filter.requester == "" && filter.responder == "" {
		filter.responder = principal
	}
	if filter.requester != "" && !identitiesEqual(filter.requester, principal) {
		return intentStreamFilter{}, &validationError{code: errorCodeForbidden, field: "requester", message: "requester must match authenticated principal"}
	}
	if filter.responder != "" && !identitiesEqual(filter.responder, principal) {
		return intentStreamFilter{}, &validationError{code: errorCodeForbidden, field: "responder", message: "responder must match authenticated principal"}
	}

	return filter, nil
}
