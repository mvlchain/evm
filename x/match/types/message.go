package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	errorsmod "cosmossdk.io/errors"
)

const (
	DigestAlgorithmUnspecified = DigestAlgorithm_DIGEST_ALGORITHM_UNSPECIFIED
	DigestAlgorithmSHA256      = DigestAlgorithm_DIGEST_ALGORITHM_SHA256
)

const (
	SignatureAlgorithmUnspecified = SignatureAlgorithm_SIGNATURE_ALGORITHM_UNSPECIFIED
	SignatureAlgorithmSecp256k1   = SignatureAlgorithm_SIGNATURE_ALGORITHM_SECP256K1
	SignatureAlgorithmEd25519     = SignatureAlgorithm_SIGNATURE_ALGORITHM_ED25519
)

// ValidateBasic performs stateless checks.
func (m *MsgSubmitMatchCertificate) ValidateBasic() error {
	if m == nil {
		return errorsmod.Wrap(ErrInvalidRequest, "request is required")
	}
	if strings.TrimSpace(m.Submitter) == "" {
		return errorsmod.Wrap(ErrInvalidRequest, "submitter is required")
	}

	return m.Certificate.ValidateBasic(m.Submitter)
}

// ValidateForSubmission performs full checks requiring current time.
func (m *MsgSubmitMatchCertificate) ValidateForSubmission(nowUnix int64) error {
	if err := m.ValidateBasic(); err != nil {
		return err
	}

	return m.Certificate.ValidateForSubmission(nowUnix)
}

// ValidateBasic checks certificate structure, chain binding and signer-role binding.
func (c MatchCertificate) ValidateBasic(submitter string) error {
	if err := c.Payload.validateBasic(); err != nil {
		return err
	}
	if err := c.Intent.validateBasic(); err != nil {
		return err
	}
	if err := c.Response.validateBasic(); err != nil {
		return err
	}
	if err := c.Finalize.validateBasic(); err != nil {
		return err
	}
	if err := validateSignature("board_signature", c.BoardSignature); err != nil {
		return err
	}
	if err := requireHash32("certificate.sign_bytes_hash", c.SignBytesHash); err != nil {
		return err
	}
	if strings.TrimSpace(submitter) == "" {
		return errorsmod.Wrap(ErrInvalidRequest, "submitter is required")
	}

	payload := c.Payload
	intentPayload := c.Intent.Payload
	responsePayload := c.Response.Payload
	finalizePayload := c.Finalize.Payload

	if payload.PoolId != intentPayload.PoolId ||
		payload.PoolId != responsePayload.PoolId ||
		payload.PoolId != finalizePayload.PoolId {
		return errorsmod.Wrap(ErrHashMismatch, "pool_id mismatch across certificate stages")
	}
	if payload.IntentId != intentPayload.IntentId ||
		payload.IntentId != responsePayload.IntentId ||
		payload.IntentId != finalizePayload.IntentId {
		return errorsmod.Wrap(ErrHashMismatch, "intent_id mismatch across certificate stages")
	}
	if payload.ResponseId != responsePayload.ResponseId ||
		payload.ResponseId != finalizePayload.ResponseId {
		return errorsmod.Wrap(ErrHashMismatch, "response_id mismatch across certificate stages")
	}
	if payload.FinalizeId != finalizePayload.FinalizeId {
		return errorsmod.Wrap(ErrHashMismatch, "finalize_id mismatch across certificate stages")
	}

	if !bytes.Equal(payload.ContextHash, intentPayload.ContextHash) ||
		!bytes.Equal(payload.ContextHash, responsePayload.ContextHash) ||
		!bytes.Equal(payload.ContextHash, finalizePayload.ContextHash) {
		return errorsmod.Wrap(ErrHashMismatch, "context_hash mismatch across certificate stages")
	}

	if !identitiesEqual(c.Intent.Signature.Signer, intentPayload.Initiator) {
		return errorsmod.Wrapf(ErrSignerMismatch, "intent signature signer must match initiator: expected %q got %q", intentPayload.Initiator, c.Intent.Signature.Signer)
	}
	if !identitiesEqual(c.Response.Signature.Signer, responsePayload.Responder) {
		return errorsmod.Wrapf(ErrSignerMismatch, "response signature signer must match responder: expected %q got %q", responsePayload.Responder, c.Response.Signature.Signer)
	}
	if !identitiesEqual(c.Finalize.InitiatorSignature.Signer, finalizePayload.Initiator) {
		return errorsmod.Wrapf(ErrSignerMismatch, "finalize initiator signature signer must match initiator: expected %q got %q", finalizePayload.Initiator, c.Finalize.InitiatorSignature.Signer)
	}
	if !identitiesEqual(c.Finalize.ResponderSignature.Signer, finalizePayload.Responder) {
		return errorsmod.Wrapf(ErrSignerMismatch, "finalize responder signature signer must match responder: expected %q got %q", finalizePayload.Responder, c.Finalize.ResponderSignature.Signer)
	}
	if !identitiesEqual(payload.Initiator, intentPayload.Initiator) || !identitiesEqual(payload.Initiator, finalizePayload.Initiator) {
		return errorsmod.Wrap(ErrSignerMismatch, "initiator mismatch across certificate stages")
	}
	if !identitiesEqual(payload.Responder, responsePayload.Responder) || !identitiesEqual(payload.Responder, finalizePayload.Responder) {
		return errorsmod.Wrap(ErrSignerMismatch, "responder mismatch across certificate stages")
	}

	if !bytes.Equal(responsePayload.IntentSignHash, c.Intent.SignBytesHash) {
		return errorsmod.Wrap(ErrHashMismatch, "response.intent_sign_hash does not match intent.sign_bytes_hash")
	}
	if !bytes.Equal(finalizePayload.IntentSignHash, c.Intent.SignBytesHash) {
		return errorsmod.Wrap(ErrHashMismatch, "finalize.intent_sign_hash does not match intent.sign_bytes_hash")
	}
	if !bytes.Equal(finalizePayload.ResponseSignHash, c.Response.SignBytesHash) {
		return errorsmod.Wrap(ErrHashMismatch, "finalize.response_sign_hash does not match response.sign_bytes_hash")
	}
	if !bytes.Equal(payload.IntentSignHash, c.Intent.SignBytesHash) {
		return errorsmod.Wrap(ErrHashMismatch, "certificate.intent_sign_hash does not match intent.sign_bytes_hash")
	}
	if !bytes.Equal(payload.ResponseSignHash, c.Response.SignBytesHash) {
		return errorsmod.Wrap(ErrHashMismatch, "certificate.response_sign_hash does not match response.sign_bytes_hash")
	}
	if !bytes.Equal(payload.FinalizeSignHash, c.Finalize.SignBytesHash) {
		return errorsmod.Wrap(ErrHashMismatch, "certificate.finalize_sign_hash does not match finalize.sign_bytes_hash")
	}

	if payload.DigestAlgorithm != DigestAlgorithmSHA256 ||
		intentPayload.DigestAlgorithm != DigestAlgorithmSHA256 ||
		responsePayload.DigestAlgorithm != DigestAlgorithmSHA256 ||
		finalizePayload.DigestAlgorithm != DigestAlgorithmSHA256 {
		return errorsmod.Wrap(ErrInvalidRequest, "digest_algorithm must be SHA256 for all stages")
	}

	if err := c.validateDeterministicSignatures(); err != nil {
		return err
	}

	return nil
}

// ValidateForSubmission checks expiry at submission time.
func (c MatchCertificate) ValidateForSubmission(nowUnix int64) error {
	if nowUnix > c.Intent.Payload.ExpiresUnix {
		return errorsmod.Wrapf(ErrExpired, "intent expired at %d", c.Intent.Payload.ExpiresUnix)
	}
	if nowUnix > c.Response.Payload.ExpiresUnix {
		return errorsmod.Wrapf(ErrExpired, "response expired at %d", c.Response.Payload.ExpiresUnix)
	}
	if nowUnix > c.Finalize.Payload.ExpiresUnix {
		return errorsmod.Wrapf(ErrExpired, "finalize expired at %d", c.Finalize.Payload.ExpiresUnix)
	}
	if nowUnix > c.Payload.ExpiresUnix {
		return errorsmod.Wrapf(ErrExpired, "certificate expired at %d", c.Payload.ExpiresUnix)
	}

	return nil
}

// CertificateHash returns the canonical certificate hash used in response/events.
func (c MatchCertificate) CertificateHash() []byte {
	if len(c.SignBytesHash) != 0 {
		return cloneBytes(c.SignBytesHash)
	}

	payload := c.Payload
	if payload == nil {
		return nil
	}

	// Defensive fallback: should not happen when ValidateBasic succeeded.
	joined := payload.PoolId + "|" + payload.IntentId + "|" + payload.ResponseId + "|" + payload.FinalizeId
	sum := sha256.Sum256([]byte(joined))
	return sum[:]
}

// MatchID returns a deterministic chain-side match identifier.
func (c MatchCertificate) MatchID() string {
	payload := c.Payload
	if payload == nil {
		return ""
	}
	return BuildMatchID(payload.PoolId, payload.IntentId, payload.CertificateId)
}

// ShortCertificateHashHex returns a short readable hash fragment.
func (c MatchCertificate) ShortCertificateHashHex() string {
	hash := c.CertificateHash()
	if len(hash) < 8 {
		return hex.EncodeToString(hash)
	}
	return hex.EncodeToString(hash[:8])
}

// BuildMatchID builds a deterministic match identifier from IDs.
func BuildMatchID(poolID, intentID, certificateID string) string {
	return poolID + "/" + intentID + "/" + certificateID
}

func (p *IntentPayload) validateBasic() error {
	if p == nil {
		return errorsmod.Wrap(ErrInvalidRequest, "intent.payload is required")
	}
	if err := requireID("intent.chain_id", p.ChainId); err != nil {
		return err
	}
	if err := requireID("intent.pool_id", p.PoolId); err != nil {
		return err
	}
	if err := requireID("intent.intent_id", p.IntentId); err != nil {
		return err
	}
	if err := requireID("intent.initiator", p.Initiator); err != nil {
		return err
	}
	if p.ExpiresUnix <= 0 {
		return errorsmod.Wrap(ErrInvalidRequest, "intent.expires_unix must be positive")
	}
	if err := requireHash32("intent.context_hash", p.ContextHash); err != nil {
		return err
	}
	return validateDigestAlgorithm("intent.digest_algorithm", p.DigestAlgorithm)
}

func (s *SignedIntent) validateBasic() error {
	if s == nil {
		return errorsmod.Wrap(ErrInvalidRequest, "intent is required")
	}
	if err := s.Payload.validateBasic(); err != nil {
		return err
	}
	if err := validateSignature("intent.signature", s.Signature); err != nil {
		return err
	}
	return requireHash32("intent.sign_bytes_hash", s.SignBytesHash)
}

func (p *ResponsePayload) validateBasic() error {
	if p == nil {
		return errorsmod.Wrap(ErrInvalidRequest, "response.payload is required")
	}
	if err := requireID("response.chain_id", p.ChainId); err != nil {
		return err
	}
	if err := requireID("response.pool_id", p.PoolId); err != nil {
		return err
	}
	if err := requireID("response.intent_id", p.IntentId); err != nil {
		return err
	}
	if err := requireID("response.response_id", p.ResponseId); err != nil {
		return err
	}
	if err := requireID("response.responder", p.Responder); err != nil {
		return err
	}
	if p.ExpiresUnix <= 0 {
		return errorsmod.Wrap(ErrInvalidRequest, "response.expires_unix must be positive")
	}
	if err := requireHash32("response.intent_sign_hash", p.IntentSignHash); err != nil {
		return err
	}
	if err := requireHash32("response.context_hash", p.ContextHash); err != nil {
		return err
	}
	if err := validateDigestAlgorithm("response.digest_algorithm", p.DigestAlgorithm); err != nil {
		return err
	}
	if p.ResponseType == ResponseType_RESPONSE_TYPE_UNSPECIFIED {
		return errorsmod.Wrap(ErrInvalidRequest, "response.response_type must be ACCEPT or COUNTER_OFFER")
	}
	return nil
}

func (s *SignedResponse) validateBasic() error {
	if s == nil {
		return errorsmod.Wrap(ErrInvalidRequest, "response is required")
	}
	if err := s.Payload.validateBasic(); err != nil {
		return err
	}
	if err := validateSignature("response.signature", s.Signature); err != nil {
		return err
	}
	return requireHash32("response.sign_bytes_hash", s.SignBytesHash)
}

func (p *FinalizePayload) validateBasic() error {
	if p == nil {
		return errorsmod.Wrap(ErrInvalidRequest, "finalize.payload is required")
	}
	if err := requireID("finalize.chain_id", p.ChainId); err != nil {
		return err
	}
	if err := requireID("finalize.pool_id", p.PoolId); err != nil {
		return err
	}
	if err := requireID("finalize.intent_id", p.IntentId); err != nil {
		return err
	}
	if err := requireID("finalize.response_id", p.ResponseId); err != nil {
		return err
	}
	if err := requireID("finalize.finalize_id", p.FinalizeId); err != nil {
		return err
	}
	if err := requireID("finalize.initiator", p.Initiator); err != nil {
		return err
	}
	if err := requireID("finalize.responder", p.Responder); err != nil {
		return err
	}
	if p.ExpiresUnix <= 0 {
		return errorsmod.Wrap(ErrInvalidRequest, "finalize.expires_unix must be positive")
	}
	if err := requireHash32("finalize.intent_sign_hash", p.IntentSignHash); err != nil {
		return err
	}
	if err := requireHash32("finalize.response_sign_hash", p.ResponseSignHash); err != nil {
		return err
	}
	if err := requireHash32("finalize.context_hash", p.ContextHash); err != nil {
		return err
	}
	return validateDigestAlgorithm("finalize.digest_algorithm", p.DigestAlgorithm)
}

func (s *SignedFinalize) validateBasic() error {
	if s == nil {
		return errorsmod.Wrap(ErrInvalidRequest, "finalize is required")
	}
	if err := s.Payload.validateBasic(); err != nil {
		return err
	}
	if err := validateSignature("finalize.initiator_signature", s.InitiatorSignature); err != nil {
		return err
	}
	if err := validateSignature("finalize.responder_signature", s.ResponderSignature); err != nil {
		return err
	}
	return requireHash32("finalize.sign_bytes_hash", s.SignBytesHash)
}

func (p *CertificatePayload) validateBasic() error {
	if p == nil {
		return errorsmod.Wrap(ErrInvalidRequest, "certificate.payload is required")
	}
	if err := requireID("certificate.chain_id", p.ChainId); err != nil {
		return err
	}
	if err := requireID("certificate.pool_id", p.PoolId); err != nil {
		return err
	}
	if err := requireID("certificate.intent_id", p.IntentId); err != nil {
		return err
	}
	if err := requireID("certificate.response_id", p.ResponseId); err != nil {
		return err
	}
	if err := requireID("certificate.finalize_id", p.FinalizeId); err != nil {
		return err
	}
	if err := requireID("certificate.certificate_id", p.CertificateId); err != nil {
		return err
	}
	if err := requireID("certificate.initiator", p.Initiator); err != nil {
		return err
	}
	if err := requireID("certificate.responder", p.Responder); err != nil {
		return err
	}
	if p.ExpiresUnix <= 0 {
		return errorsmod.Wrap(ErrInvalidRequest, "certificate.expires_unix must be positive")
	}
	if err := requireHash32("certificate.intent_sign_hash", p.IntentSignHash); err != nil {
		return err
	}
	if err := requireHash32("certificate.response_sign_hash", p.ResponseSignHash); err != nil {
		return err
	}
	if err := requireHash32("certificate.finalize_sign_hash", p.FinalizeSignHash); err != nil {
		return err
	}
	if err := requireHash32("certificate.context_hash", p.ContextHash); err != nil {
		return err
	}
	return validateDigestAlgorithm("certificate.digest_algorithm", p.DigestAlgorithm)
}

func validateSignature(field string, sig *Signature) error {
	// Signature envelope shape is validated here; cryptographic verification is performed by upper layers.
	if sig == nil {
		return errorsmod.Wrapf(ErrInvalidRequest, "%s is required", field)
	}
	if strings.TrimSpace(sig.Signer) == "" {
		return errorsmod.Wrapf(ErrInvalidRequest, "%s.signer is required", field)
	}
	switch sig.Algorithm {
	case SignatureAlgorithmSecp256k1, SignatureAlgorithmEd25519:
	default:
		return errorsmod.Wrapf(ErrInvalidRequest, "%s.algorithm is unsupported: %d", field, sig.Algorithm)
	}
	if len(sig.Signature) == 0 {
		return errorsmod.Wrapf(ErrInvalidRequest, "%s.signature is required", field)
	}

	return nil
}

func validateDigestAlgorithm(field string, algorithm DigestAlgorithm) error {
	if algorithm != DigestAlgorithmSHA256 {
		return errorsmod.Wrapf(ErrInvalidRequest, "%s must be SHA256", field)
	}
	return nil
}

func requireID(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return errorsmod.Wrapf(ErrInvalidRequest, "%s is required", field)
	}
	return nil
}

func requireHash32(field string, value []byte) error {
	if len(value) != sha256.Size {
		return errorsmod.Wrapf(ErrInvalidRequest, "%s must be %d bytes, got %d", field, sha256.Size, len(value))
	}
	return nil
}

func cloneBytes(value []byte) []byte {
	out := make([]byte, len(value))
	copy(out, value)
	return out
}
