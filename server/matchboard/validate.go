package matchboard

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/url"
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

	normalizedPeers := make([]string, 0, len(cfg.GossipPeers))
	seenPeers := make(map[string]struct{}, len(cfg.GossipPeers))
	for _, peer := range cfg.GossipPeers {
		peer = strings.TrimSpace(peer)
		if peer == "" {
			continue
		}
		parsed, err := url.Parse(peer)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return Config{}, fmt.Errorf("invalid gossip peer URL %q", peer)
		}
		normalized := strings.TrimRight(parsed.String(), "/")
		if _, exists := seenPeers[normalized]; exists {
			continue
		}
		seenPeers[normalized] = struct{}{}
		normalizedPeers = append(normalizedPeers, normalized)
	}
	cfg.GossipPeers = normalizedPeers
	cfg.GossipSharedSecret = strings.TrimSpace(cfg.GossipSharedSecret)
	cfg.GossipNodeID = strings.TrimSpace(cfg.GossipNodeID)

	if cfg.GossipTimeout <= 0 {
		cfg.GossipTimeout = defaultGossipTimeout
	}
	if cfg.GossipMessageTTL <= 0 {
		cfg.GossipMessageTTL = defaultGossipMessageTTL
	}
	if cfg.GossipSeenTTL <= 0 {
		cfg.GossipSeenTTL = defaultGossipSeenTTL
	}
	if cfg.GossipMaxHops <= 0 {
		cfg.GossipMaxHops = defaultGossipMaxHops
	}
	if len(cfg.GossipPeers) > 0 && cfg.GossipSharedSecret == "" {
		return Config{}, errors.New("gossip peers require gossip shared secret")
	}
	if cfg.GossipMaxHops < 1 {
		return Config{}, errors.New("gossip max hops must be positive")
	}

	if cfg.IntentStreamBuffer <= 0 {
		cfg.IntentStreamBuffer = defaultIntentStreamQueue
	}
	if cfg.MatcherShardCount <= 0 {
		cfg.MatcherShardCount = defaultMatcherShardCount
	}
	if cfg.MatcherShardCount > 1024 {
		return Config{}, errors.New("matcher shard count must be <= 1024")
	}

	return cfg, nil
}

func validateAndNormalizeIntent(req *PublishIntentRequest, nowUnix int64) error {
	if strings.TrimSpace(req.PoolID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "pool_id", message: "pool_id is required"}
	}
	if strings.TrimSpace(req.IntentID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "intent_id", message: "intent_id is required"}
	}
	if strings.TrimSpace(req.Sender) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "sender", message: "sender is required"}
	}
	if !common.IsHexAddress(req.Sender) {
		return &validationError{code: errorCodeInvalidRequest, field: "sender", message: "sender must be a valid Ethereum address"}
	}
	if strings.TrimSpace(req.Recipient) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "recipient", message: "recipient is required"}
	}
	if !common.IsHexAddress(req.Recipient) {
		return &validationError{code: errorCodeInvalidRequest, field: "recipient", message: "recipient must be a valid Ethereum address"}
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

	if err := validateOffer("offer", req.Offer); err != nil {
		return err
	}
	if err := validateSignatureMetadata("signature", req.Signature, req.Sender); err != nil {
		return err
	}
	if err := validateEthereumSignature("signature", req.IntentSignHash, req.Sender, req.Signature); err != nil {
		return err
	}

	return nil
}

func validateAndNormalizeResponse(req *PublishResponseRequest, nowUnix int64) error {
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
	if !common.IsHexAddress(req.Sender) {
		return &validationError{code: errorCodeInvalidRequest, field: "sender", message: "sender must be a valid Ethereum address"}
	}
	if strings.TrimSpace(req.Recipient) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "recipient", message: "recipient is required"}
	}
	if !common.IsHexAddress(req.Recipient) {
		return &validationError{code: errorCodeInvalidRequest, field: "recipient", message: "recipient must be a valid Ethereum address"}
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

	if err := validateOffer("offer", req.Offer); err != nil {
		return err
	}
	if err := validateResponseType(req.ResponseType); err != nil {
		return err
	}
	if err := validateSignatureMetadata("signature", req.Signature, req.Sender); err != nil {
		return err
	}
	if err := validateEthereumSignature("signature", req.ResponseSignHash, req.Sender, req.Signature); err != nil {
		return err
	}

	return nil
}

func validateResponseType(rt string) error {
	switch rt {
	case "ACCEPT", "COUNTER_OFFER":
		return nil
	default:
		return &validationError{code: errorCodeInvalidRequest, field: "response_type", message: "response_type must be ACCEPT or COUNTER_OFFER"}
	}
}

func validateAndNormalizeFinalize(req *PublishFinalizeRequest, nowUnix int64) error {
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
	if !common.IsHexAddress(req.Sender) {
		return &validationError{code: errorCodeInvalidRequest, field: "sender", message: "sender must be a valid Ethereum address"}
	}
	if strings.TrimSpace(req.Recipient) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "recipient", message: "recipient is required"}
	}
	if !common.IsHexAddress(req.Recipient) {
		return &validationError{code: errorCodeInvalidRequest, field: "recipient", message: "recipient must be a valid Ethereum address"}
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
	if err := validateEthereumSignature("initiator_signature", req.FinalizeSignHash, req.Sender, req.InitiatorSignature); err != nil {
		return err
	}
	if err := validateEthereumSignature("responder_signature", req.FinalizeSignHash, req.Recipient, req.ResponderSignature); err != nil {
		return err
	}

	if len(req.MatchCertificate) > 0 {
		op := ProposedOperation{
			RecordType:       RecordTypeFinalize,
			PoolID:           req.PoolID,
			IntentID:         req.IntentID,
			ResponseID:       req.ResponseID,
			FinalizeID:       req.FinalizeID,
			Sender:           req.Sender,
			Recipient:        req.Recipient,
			IntentSignHash:   req.IntentSignHash,
			ResponseSignHash: req.ResponseSignHash,
			FinalizeSignHash: req.FinalizeSignHash,
			MatchCertificate: req.MatchCertificate,
		}
		canonical, cert, certErr := NormalizeOperationCertificate(op)
		if certErr != nil {
			return &validationError{
				code:    errorCodeInvalidRequest,
				field:   "match_certificate",
				message: "invalid match_certificate",
				detail:  certErr.Error(),
			}
		}
		if cert != nil && cert.Payload != nil {
			if err := cert.ValidateForSubmission(nowUnix); err != nil {
				return &validationError{
					code:    errorCodeExpired,
					field:   "match_certificate",
					message: "match certificate is expired",
					detail:  err.Error(),
				}
			}
			contextHex := strings.ToLower(hex.EncodeToString(cert.Payload.ContextHash))
			if contextHex != req.ContextHash {
				return &validationError{
					code:    errorCodeHashMismatch,
					field:   "match_certificate",
					message: "context hash mismatch",
					detail:  "match_certificate.payload.context_hash must match context_hash",
				}
			}
		}
		req.MatchCertificate = canonical
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
	if sigAlgorithm != SignatureAlgorithmSecp256k1 {
		return &validationError{
			code:    errorCodeInvalidRequest,
			field:   field + ".algorithm",
			message: fmt.Sprintf("unsupported signature algorithm: %q, only secp256k1 is accepted", sig.Algorithm),
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

// validateOffer validates the optional Offer fields. All four fields must be
// provided together or all omitted. Asset fields must be "native" or a valid
// Ethereum hex address. Amount fields must be positive uint256 decimal strings.
func validateOffer(fieldPrefix string, offer Offer) error {
	hasAny := offer.AssetIn != "" || offer.AmountIn != "" || offer.AssetOut != "" || offer.AmountOut != ""
	if !hasAny {
		return nil
	}
	if offer.AssetIn == "" {
		return &validationError{code: errorCodeInvalidRequest, field: fieldPrefix + ".asset_in", message: "asset_in is required when offer is provided"}
	}
	if offer.AmountIn == "" {
		return &validationError{code: errorCodeInvalidRequest, field: fieldPrefix + ".amount_in", message: "amount_in is required when offer is provided"}
	}
	if offer.AssetOut == "" {
		return &validationError{code: errorCodeInvalidRequest, field: fieldPrefix + ".asset_out", message: "asset_out is required when offer is provided"}
	}
	if offer.AmountOut == "" {
		return &validationError{code: errorCodeInvalidRequest, field: fieldPrefix + ".amount_out", message: "amount_out is required when offer is provided"}
	}
	if err := validateOfferAsset(fieldPrefix+".asset_in", offer.AssetIn); err != nil {
		return err
	}
	if err := validateOfferAsset(fieldPrefix+".asset_out", offer.AssetOut); err != nil {
		return err
	}
	if err := validateOfferAmount(fieldPrefix+".amount_in", offer.AmountIn); err != nil {
		return err
	}
	if err := validateOfferAmount(fieldPrefix+".amount_out", offer.AmountOut); err != nil {
		return err
	}
	return nil
}

func validateOfferAsset(field, value string) error {
	if strings.EqualFold(value, NativeAsset) {
		return nil
	}
	if !common.IsHexAddress(value) {
		return &validationError{code: errorCodeInvalidRequest, field: field, message: `asset must be "native" or a valid Ethereum address`}
	}
	return nil
}

func validateOfferAmount(field, value string) error {
	n := new(big.Int)
	if _, ok := n.SetString(strings.TrimSpace(value), 10); !ok {
		return &validationError{code: errorCodeInvalidRequest, field: field, message: "amount must be a decimal integer string"}
	}
	if n.Sign() <= 0 {
		return &validationError{code: errorCodeInvalidRequest, field: field, message: "amount must be greater than zero"}
	}
	return nil
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

// validateAndNormalizeCancelIntent validates a CancelIntentRequest.
// The signature must cover sha256("CANCEL_INTENT:" + pool_id + ":" + intent_id).
func validateAndNormalizeCancelIntent(req *CancelIntentRequest) error {
	if strings.TrimSpace(req.PoolID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "pool_id", message: "pool_id is required"}
	}
	if strings.TrimSpace(req.IntentID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "intent_id", message: "intent_id is required"}
	}
	if strings.TrimSpace(req.Canceller) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "canceller", message: "canceller is required"}
	}
	if !common.IsHexAddress(req.Canceller) {
		return &validationError{code: errorCodeInvalidRequest, field: "canceller", message: "canceller must be a valid Ethereum address"}
	}
	if err := validateSignatureMetadata("signature", req.Signature, req.Canceller); err != nil {
		return err
	}
	msg := fmt.Sprintf("CANCEL_INTENT:%d:%s:%d:%s", len(req.PoolID), req.PoolID, len(req.IntentID), req.IntentID)
	sum := sha256.Sum256([]byte(msg))
	signHashHex := hex.EncodeToString(sum[:])
	return validateEthereumSignature("signature", signHashHex, req.Canceller, req.Signature)
}

// validateAndNormalizeCancelResponse validates a CancelResponseRequest.
// The signature must cover sha256("CANCEL_RESPONSE:" + pool_id + ":" + intent_id + ":" + response_id).
func validateAndNormalizeCancelResponse(req *CancelResponseRequest) error {
	if strings.TrimSpace(req.PoolID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "pool_id", message: "pool_id is required"}
	}
	if strings.TrimSpace(req.IntentID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "intent_id", message: "intent_id is required"}
	}
	if strings.TrimSpace(req.ResponseID) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "response_id", message: "response_id is required"}
	}
	if strings.TrimSpace(req.Canceller) == "" {
		return &validationError{code: errorCodeInvalidRequest, field: "canceller", message: "canceller is required"}
	}
	if !common.IsHexAddress(req.Canceller) {
		return &validationError{code: errorCodeInvalidRequest, field: "canceller", message: "canceller must be a valid Ethereum address"}
	}
	if err := validateSignatureMetadata("signature", req.Signature, req.Canceller); err != nil {
		return err
	}
	msg := fmt.Sprintf("CANCEL_RESPONSE:%d:%s:%d:%s:%d:%s", len(req.PoolID), req.PoolID, len(req.IntentID), req.IntentID, len(req.ResponseID), req.ResponseID)
	sum := sha256.Sum256([]byte(msg))
	signHashHex := hex.EncodeToString(sum[:])
	return validateEthereumSignature("signature", signHashHex, req.Canceller, req.Signature)
}
