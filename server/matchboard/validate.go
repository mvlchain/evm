package matchboard

import (
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type validationError struct {
	code    string
	field   string
	message string
	detail  string
}

func (e *validationError) Error() string {
	if e.field == "" {
		return e.message
	}
	return e.field + ": " + e.message
}

func normalizeConfig(cfg Config) (Config, error) {
	if len(cfg.TokenPrincipalMap) == 0 {
		return Config{}, errors.New("token principal map must not be empty")
	}

	normalizedMap := make(map[string]string, len(cfg.TokenPrincipalMap))
	for token, principal := range cfg.TokenPrincipalMap {
		token = strings.TrimSpace(token)
		principal = strings.TrimSpace(principal)
		if token == "" || principal == "" {
			return Config{}, errors.New("token principal map contains empty token or principal")
		}
		normalizedMap[token] = principal
	}
	cfg.TokenPrincipalMap = normalizedMap

	if cfg.RateLimitRequests <= 0 {
		cfg.RateLimitRequests = defaultRateLimitRequests
	}
	if cfg.RateLimitWindow <= 0 {
		cfg.RateLimitWindow = defaultRateLimitWindow
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = defaultMaxBodyBytes
	}
	if cfg.DefaultPageLimit <= 0 {
		cfg.DefaultPageLimit = defaultPageLimit
	}
	if cfg.MaxPageLimit <= 0 {
		cfg.MaxPageLimit = defaultMaxPageLimit
	}
	if cfg.DefaultPageLimit > cfg.MaxPageLimit {
		cfg.DefaultPageLimit = cfg.MaxPageLimit
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.NowFn == nil {
		cfg.NowFn = time.Now
	}

	return cfg, nil
}

func validateAndNormalizeIntent(req *PublishIntentRequest, principal string, nowUnix int64) error {
	if strings.TrimSpace(req.PoolID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "pool_id", message: "pool_id is required"}
	}
	if strings.TrimSpace(req.IntentID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "intent_id", message: "intent_id is required"}
	}
	if strings.TrimSpace(req.Sender) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "sender", message: "sender is required"}
	}
	if !identitiesEqual(req.Sender, principal) {
		return &validationError{code: errorCodeSignerMismatch, field: "sender", message: "sender must match authenticated principal"}
	}
	if strings.TrimSpace(req.Recipient) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "recipient", message: "recipient is required"}
	}
	if req.ExpiresUnix < nowUnix {
		return &validationError{code: errorCodeExpired, field: "expires_unix", message: "artifact is expired"}
	}
	if err := validateDigestAlgorithm(req.DigestAlgorithm); err != nil {
		return err
	}

	var err error
	if req.IntentSignHash, err = normalizeRequiredHash("intent_sign_hash", req.IntentSignHash); err != nil {
		return err
	}
	if req.ContextHash, err = normalizeRequiredHash("context_hash", req.ContextHash); err != nil {
		return err
	}
	if req.TermsHash, err = normalizeOptionalHash("terms_hash", req.TermsHash); err != nil {
		return err
	}
	if req.PolicyHash, err = normalizeOptionalHash("policy_hash", req.PolicyHash); err != nil {
		return err
	}

	if err := validateSignatureMetadata("signature", req.Signature, req.Sender); err != nil {
		return err
	}
	if err := validateEthereumSignatureIfRequired("signature", req.IntentSignHash, req.Sender, req.Signature); err != nil {
		return err
	}

	return nil
}

func validateAndNormalizeResponse(req *PublishResponseRequest, principal string, nowUnix int64) error {
	if strings.TrimSpace(req.PoolID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "pool_id", message: "pool_id is required"}
	}
	if strings.TrimSpace(req.IntentID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "intent_id", message: "intent_id is required"}
	}
	if strings.TrimSpace(req.ResponseID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "response_id", message: "response_id is required"}
	}
	if strings.TrimSpace(req.Sender) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "sender", message: "sender is required"}
	}
	if !identitiesEqual(req.Sender, principal) {
		return &validationError{code: errorCodeSignerMismatch, field: "sender", message: "sender must match authenticated principal"}
	}
	if strings.TrimSpace(req.Recipient) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "recipient", message: "recipient is required"}
	}
	if req.ExpiresUnix < nowUnix {
		return &validationError{code: errorCodeExpired, field: "expires_unix", message: "artifact is expired"}
	}
	if err := validateDigestAlgorithm(req.DigestAlgorithm); err != nil {
		return err
	}

	var err error
	if req.IntentSignHash, err = normalizeRequiredHash("intent_sign_hash", req.IntentSignHash); err != nil {
		return err
	}
	if req.ResponseSignHash, err = normalizeRequiredHash("response_sign_hash", req.ResponseSignHash); err != nil {
		return err
	}
	if req.ContextHash, err = normalizeRequiredHash("context_hash", req.ContextHash); err != nil {
		return err
	}
	if req.TermsHash, err = normalizeOptionalHash("terms_hash", req.TermsHash); err != nil {
		return err
	}
	if req.PolicyHash, err = normalizeOptionalHash("policy_hash", req.PolicyHash); err != nil {
		return err
	}

	if err := validateSignatureMetadata("signature", req.Signature, req.Sender); err != nil {
		return err
	}
	if err := validateEthereumSignatureIfRequired("signature", req.ResponseSignHash, req.Sender, req.Signature); err != nil {
		return err
	}

	return nil
}

func validateAndNormalizeFinalize(req *PublishFinalizeRequest, principal string, nowUnix int64) error {
	if strings.TrimSpace(req.PoolID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "pool_id", message: "pool_id is required"}
	}
	if strings.TrimSpace(req.IntentID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "intent_id", message: "intent_id is required"}
	}
	if strings.TrimSpace(req.ResponseID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "response_id", message: "response_id is required"}
	}
	if strings.TrimSpace(req.FinalizeID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "finalize_id", message: "finalize_id is required"}
	}
	if strings.TrimSpace(req.Sender) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "sender", message: "sender is required"}
	}
	if !identitiesEqual(req.Sender, principal) {
		return &validationError{code: errorCodeSignerMismatch, field: "sender", message: "sender must match authenticated principal"}
	}
	if strings.TrimSpace(req.Recipient) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "recipient", message: "recipient is required"}
	}
	if req.ExpiresUnix < nowUnix {
		return &validationError{code: errorCodeExpired, field: "expires_unix", message: "artifact is expired"}
	}
	if err := validateDigestAlgorithm(req.DigestAlgorithm); err != nil {
		return err
	}

	var err error
	if req.IntentSignHash, err = normalizeRequiredHash("intent_sign_hash", req.IntentSignHash); err != nil {
		return err
	}
	if req.ResponseSignHash, err = normalizeRequiredHash("response_sign_hash", req.ResponseSignHash); err != nil {
		return err
	}
	if req.FinalizeSignHash, err = normalizeRequiredHash("finalize_sign_hash", req.FinalizeSignHash); err != nil {
		return err
	}
	if req.ContextHash, err = normalizeRequiredHash("context_hash", req.ContextHash); err != nil {
		return err
	}

	if err := validateSignatureMetadata("initiator_signature", req.InitiatorSignature, req.Sender); err != nil {
		return err
	}
	if err := validateSignatureMetadata("responder_signature", req.ResponderSignature, req.Recipient); err != nil {
		return err
	}
	if err := validateEthereumSignatureIfRequired("initiator_signature", req.FinalizeSignHash, req.Sender, req.InitiatorSignature); err != nil {
		return err
	}
	if err := validateEthereumSignatureIfRequired("responder_signature", req.FinalizeSignHash, req.Recipient, req.ResponderSignature); err != nil {
		return err
	}

	return nil
}

func validateDigestAlgorithm(algorithm string) error {
	if strings.EqualFold(strings.TrimSpace(algorithm), DigestAlgorithmSHA256) {
		return nil
	}

	return &validationError{
		code:    errorCodeInvalidRequest,
		field:   "digest_algorithm",
		message: fmt.Sprintf("unsupported digest_algorithm: %q", algorithm),
		detail:  "only sha256 is accepted",
	}
}

func validateSignatureMetadata(field string, sig SignatureMetadata, expectedSigner string) error {
	if strings.TrimSpace(sig.Signer) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: field + ".signer", message: "signer is required"}
	}
	if !identitiesEqual(sig.Signer, expectedSigner) {
		return &validationError{code: errorCodeSignerMismatch, field: field + ".signer", message: "signature signer must match sender/recipient role"}
	}

	sigAlgorithm := strings.ToLower(strings.TrimSpace(sig.Algorithm))
	switch sigAlgorithm {
	case SignatureAlgorithmSecp256k1, SignatureAlgorithmEd25519:
	default:
		return &validationError{
			code:    errorCodeInvalidRequest,
			field:   field + ".algorithm",
			message: fmt.Sprintf("unsupported signature algorithm: %q", sig.Algorithm),
		}
	}

	if strings.TrimSpace(sig.Signature) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: field + ".signature", message: "signature is required"}
	}

	return nil
}

func identitiesEqual(a, b string) bool {
	if common.IsHexAddress(a) && common.IsHexAddress(b) {
		return common.HexToAddress(a) == common.HexToAddress(b)
	}
	return a == b
}

func normalizeRequiredHash(field, value string) (string, error) {
	normalized, err := normalizeHash(value)
	if err != nil {
		return "", &validationError{
			code:    errorCodeInvalidRequest,
			field:   field,
			message: "invalid hash",
			detail:  err.Error(),
		}
	}
	if normalized == "" {
		return "", &validationError{code: errorCodeInvalidRequest, field: field, message: "hash is required"}
	}
	return normalized, nil
}

func normalizeOptionalHash(field, value string) (string, error) {
	normalized, err := normalizeHash(value)
	if err != nil {
		return "", &validationError{
			code:    errorCodeInvalidRequest,
			field:   field,
			message: "invalid hash",
			detail:  err.Error(),
		}
	}
	return normalized, nil
}

func normalizeHash(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	trimmed = strings.TrimPrefix(trimmed, "0x")
	trimmed = strings.TrimPrefix(trimmed, "0X")

	decoded, err := hex.DecodeString(trimmed)
	if err != nil {
		return "", errors.New("hash must be hex encoded")
	}
	if len(decoded) != 32 {
		return "", fmt.Errorf("hash must be 32 bytes, got %d", len(decoded))
	}

	return strings.ToLower(trimmed), nil
}
