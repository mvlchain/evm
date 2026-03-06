package matchboard

import (
	"log/slog"
	"time"
)

const (
	defaultRateLimitRequests = 60
	defaultRateLimitWindow   = time.Minute
	defaultMaxBodyBytes      = 1 << 20 // 1 MiB
	defaultPageLimit         = 50
	defaultMaxPageLimit      = 200
	defaultGossipTimeout     = 2 * time.Second
)

const (
	DigestAlgorithmSHA256 = "sha256"
)

const (
	SignatureAlgorithmSecp256k1 = "secp256k1"
	SignatureAlgorithmEd25519   = "ed25519"
)

const (
	RecordTypeIntent   = "intent"
	RecordTypeResponse = "response"
	RecordTypeFinalize = "finalize"
)

const (
	errorCodeInvalidRequest     = "ERROR_CODE_INVALID_REQUEST"
	errorCodeUnauthorized       = "ERROR_CODE_UNAUTHORIZED"
	errorCodeForbidden          = "ERROR_CODE_FORBIDDEN"
	errorCodeNotFound           = "ERROR_CODE_NOT_FOUND"
	errorCodeReplayDetected     = "ERROR_CODE_REPLAY_DETECTED"
	errorCodeExpired            = "ERROR_CODE_EXPIRED"
	errorCodeSignerMismatch     = "ERROR_CODE_SIGNER_MISMATCH"
	errorCodeInvalidSignature   = "ERROR_CODE_INVALID_SIGNATURE"
	errorCodeHashMismatch       = "ERROR_CODE_HASH_MISMATCH"
	errorCodeStateConflict      = "ERROR_CODE_STATE_CONFLICT"
	errorCodeRateLimited        = "ERROR_CODE_RATE_LIMITED"
	errorCodeInternal           = "ERROR_CODE_INTERNAL"
	errorCodeBackendUnavailable = "ERROR_CODE_BACKEND_UNAVAILABLE"
)

const (
	headerGossipSecret = "X-Matchboard-Gossip-Secret"
	headerGossipOrigin = "X-Matchboard-Gossip-Origin"
)

const (
	injectedOperationMagic = "MOP1"
)

var suggestedCodeByError = map[string]string{
	errorCodeInvalidRequest:     "MCH-1000",
	errorCodeForbidden:          "MCH-1300",
	errorCodeReplayDetected:     "MCH-1202",
	errorCodeExpired:            "MCH-1201",
	errorCodeSignerMismatch:     "MCH-1102",
	errorCodeInvalidSignature:   "MCH-1101",
	errorCodeHashMismatch:       "MCH-1200",
	errorCodeStateConflict:      "MCH-1200",
	errorCodeRateLimited:        "MCH-1301",
	errorCodeUnauthorized:       "MCH-1300",
	errorCodeNotFound:           "MCH-1000",
	errorCodeInternal:           "MCH-1000",
	errorCodeBackendUnavailable: "MCH-1400",
}

// Config configures a matchboard HTTP handler.
type Config struct {
	// TokenPrincipalMap maps bearer tokens to principals.
	TokenPrincipalMap map[string]string

	// RateLimitRequests is the maximum requests allowed per principal inside one RateLimitWindow.
	RateLimitRequests int

	// RateLimitWindow is the window size for per-principal rate limiting.
	RateLimitWindow time.Duration

	// MaxBodyBytes is the maximum request body size accepted for POST endpoints.
	MaxBodyBytes int64

	// DefaultPageLimit is used when inbox/outbox limit is omitted or invalid.
	DefaultPageLimit int

	// MaxPageLimit caps the user-provided limit for inbox/outbox queries.
	MaxPageLimit int

	// Logger receives structured logs. If nil, slog.Default() is used.
	Logger *slog.Logger

	// NowFn injects time for tests; if nil, time.Now is used.
	NowFn func() time.Time

	// GossipPeers are peer base URLs that receive best-effort short-term payload gossip.
	GossipPeers []string

	// GossipSharedSecret authenticates /v1/internal/gossip/* relay requests.
	GossipSharedSecret string

	// GossipNodeID identifies this node in gossip forwarding logs/headers.
	GossipNodeID string

	// GossipTimeout bounds one outbound gossip relay request.
	GossipTimeout time.Duration

	// EnableABCIProposerOps enables app-side proposer operation injection queue bridging.
	EnableABCIProposerOps bool
}

// SignatureMetadata captures signature metadata attached to a signed artifact.
type SignatureMetadata struct {
	Signer    string `json:"signer"`
	Algorithm string `json:"algorithm"`
	PublicKey string `json:"public_key,omitempty"`
	Signature string `json:"signature"`
}

// PublishIntentRequest posts an intent artifact with sign-hash metadata.
type PublishIntentRequest struct {
	ProtocolVersion string `json:"protocol_version,omitempty"`
	BoardID         string `json:"board_id,omitempty"`
	ChainID         string `json:"chain_id,omitempty"`
	PoolID          string `json:"pool_id"`
	IntentID        string `json:"intent_id"`
	Sender          string `json:"sender"`
	Recipient       string `json:"recipient"`
	ExpiresUnix     int64  `json:"expires_unix"`

	DigestAlgorithm string `json:"digest_algorithm"`
	IntentSignHash  string `json:"intent_sign_hash"`
	ContextHash     string `json:"context_hash"`
	TermsHash       string `json:"terms_hash,omitempty"`
	PolicyHash      string `json:"policy_hash,omitempty"`

	Signature SignatureMetadata `json:"signature"`
}

// PublishResponseRequest posts a response artifact with sign-hash metadata.
type PublishResponseRequest struct {
	ProtocolVersion string `json:"protocol_version,omitempty"`
	BoardID         string `json:"board_id,omitempty"`
	ChainID         string `json:"chain_id,omitempty"`
	PoolID          string `json:"pool_id"`
	IntentID        string `json:"intent_id"`
	ResponseID      string `json:"response_id"`
	Sender          string `json:"sender"`
	Recipient       string `json:"recipient"`
	ExpiresUnix     int64  `json:"expires_unix"`

	DigestAlgorithm  string `json:"digest_algorithm"`
	IntentSignHash   string `json:"intent_sign_hash"`
	ResponseSignHash string `json:"response_sign_hash"`
	ContextHash      string `json:"context_hash"`
	TermsHash        string `json:"terms_hash,omitempty"`
	PolicyHash       string `json:"policy_hash,omitempty"`

	Signature SignatureMetadata `json:"signature"`
}

// PublishFinalizeRequest posts a finalize artifact with bilateral signatures.
type PublishFinalizeRequest struct {
	ProtocolVersion string `json:"protocol_version,omitempty"`
	BoardID         string `json:"board_id,omitempty"`
	ChainID         string `json:"chain_id,omitempty"`
	PoolID          string `json:"pool_id"`
	IntentID        string `json:"intent_id"`
	ResponseID      string `json:"response_id"`
	FinalizeID      string `json:"finalize_id"`
	Sender          string `json:"sender"`
	Recipient       string `json:"recipient"`
	ExpiresUnix     int64  `json:"expires_unix"`

	DigestAlgorithm  string `json:"digest_algorithm"`
	IntentSignHash   string `json:"intent_sign_hash"`
	ResponseSignHash string `json:"response_sign_hash"`
	FinalizeSignHash string `json:"finalize_sign_hash"`
	ContextHash      string `json:"context_hash"`

	InitiatorSignature SignatureMetadata `json:"initiator_signature"`
	ResponderSignature SignatureMetadata `json:"responder_signature"`
	// MatchCertificate is an optional deterministic protobuf-encoded match.v1.MatchCertificate blob.
	// When provided, proposer ABCI injection can directly submit certificates via x/match batch path.
	MatchCertificate []byte `json:"match_certificate,omitempty"`
}

// BoardRecord is a redacted list entry for inbox/outbox endpoints.
type BoardRecord struct {
	RecordType       string `json:"record_type"`
	PoolID           string `json:"pool_id"`
	IntentID         string `json:"intent_id"`
	ResponseID       string `json:"response_id,omitempty"`
	FinalizeID       string `json:"finalize_id,omitempty"`
	Sender           string `json:"sender"`
	Recipient        string `json:"recipient"`
	CreatedUnix      int64  `json:"created_unix"`
	ContextHash      string `json:"context_hash,omitempty"`
	IntentSignHash   string `json:"intent_sign_hash,omitempty"`
	ResponseSignHash string `json:"response_sign_hash,omitempty"`
	FinalizeSignHash string `json:"finalize_sign_hash,omitempty"`
}

type publishIntentResponse struct {
	PoolID         string `json:"pool_id"`
	IntentID       string `json:"intent_id"`
	IntentSignHash string `json:"intent_sign_hash"`
	StoredUnix     int64  `json:"stored_unix"`
}

type publishResponseResponse struct {
	PoolID           string `json:"pool_id"`
	IntentID         string `json:"intent_id"`
	ResponseID       string `json:"response_id"`
	ResponseSignHash string `json:"response_sign_hash"`
	StoredUnix       int64  `json:"stored_unix"`
}

type publishFinalizeResponse struct {
	PoolID           string `json:"pool_id"`
	IntentID         string `json:"intent_id"`
	ResponseID       string `json:"response_id"`
	FinalizeID       string `json:"finalize_id"`
	FinalizeSignHash string `json:"finalize_sign_hash"`
	StoredUnix       int64  `json:"stored_unix"`
}

type listRecordsResponse struct {
	Principal  string        `json:"principal"`
	Records    []BoardRecord `json:"records"`
	NextCursor string        `json:"next_cursor,omitempty"`
	Total      uint64        `json:"total"`
}

// ProposedOperation is a canonical short-term operation queued for proposer inclusion.
type ProposedOperation struct {
	OperationID string `json:"operation_id"`
	RecordType  string `json:"record_type"`

	PoolID     string `json:"pool_id"`
	IntentID   string `json:"intent_id"`
	ResponseID string `json:"response_id,omitempty"`
	FinalizeID string `json:"finalize_id,omitempty"`

	Sender      string `json:"sender"`
	Recipient   string `json:"recipient"`
	CreatedUnix int64  `json:"created_unix"`

	IntentSignHash   string `json:"intent_sign_hash,omitempty"`
	ResponseSignHash string `json:"response_sign_hash,omitempty"`
	FinalizeSignHash string `json:"finalize_sign_hash,omitempty"`
	// MatchCertificate carries optional deterministic protobuf certificate bytes for finalize operations.
	MatchCertificate []byte `json:"-"`
}

type listProposedOperationsResponse struct {
	Proposer           string              `json:"proposer"`
	Operations         []ProposedOperation `json:"operations"`
	CanonicalBatchHash string              `json:"canonical_batch_hash"`
	TotalPending       uint64              `json:"total_pending"`
}

type commitProposedOperationsRequest struct {
	OperationIDs []string `json:"operation_ids"`
}

type commitProposedOperationsResponse struct {
	Proposer           string `json:"proposer"`
	Committed          int    `json:"committed"`
	Remaining          uint64 `json:"remaining"`
	CanonicalBatchHash string `json:"canonical_batch_hash"`
}

type errorEnvelope struct {
	Error errorStatus `json:"error"`
}

type errorStatus struct {
	Code          string `json:"code"`
	SuggestedCode string `json:"suggested_code,omitempty"`
	Message       string `json:"message"`
	Field         string `json:"field,omitempty"`
	Detail        string `json:"detail,omitempty"`
	Retryable     bool   `json:"retryable"`
}
