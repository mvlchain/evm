package types

import (
	"crypto/sha256"
	"fmt"
	"strings"

	proto "github.com/cosmos/gogoproto/proto"
)

// Typed domain constants — hardcoded, never user-configurable.
// Format: "{module}/{version}/{action}"
// These act as the domain separator analogous to EIP-712 typeHash.
const (
	TypeURLIntent      = "cosmos-evm/match/v1/INTENT"
	TypeURLResponse    = "cosmos-evm/match/v1/RESPONSE"
	TypeURLFinalize    = "cosmos-evm/match/v1/FINALIZE"
	TypeURLCertificate = "cosmos-evm/match/v1/CERTIFICATE"
)

// TypedSignDocHash computes the domain-separated sign hash for a match artifact.
//
// hash = sha256( sha256(typeURL) || sha256(canonical_proto(payload)) )
//
// This binds the signature to:
//   - The specific protocol module and version (typeURL — hardcoded constant)
//   - The specific action type (INTENT / RESPONSE / FINALIZE / CERTIFICATE)
//   - The payload content including chain_id, intent_id, signer, nonce, expiry
//
// Prevents:
//   - Cross-chain replay      (chain_id inside payload)
//   - Cross-module reuse      (typeURL hardcoded, not user-supplied)
//   - Cross-action confusion  (INTENT hash ≠ RESPONSE hash for same payload)
//   - Settlement substitution (settlement_hash inside FinalizePayload)
func TypedSignDocHash(typeURL string, msg proto.Message) ([]byte, error) {
	if strings.TrimSpace(typeURL) == "" {
		return nil, fmt.Errorf("typeURL must not be empty")
	}
	if msg == nil {
		return nil, fmt.Errorf("payload must not be nil")
	}

	typeURLHash := sha256.Sum256([]byte(typeURL))

	payloadHash, err := DeterministicProtoSHA256(msg)
	if err != nil {
		return nil, fmt.Errorf("failed to hash payload: %w", err)
	}

	h := sha256.New()
	h.Write(typeURLHash[:])
	h.Write(payloadHash)
	return h.Sum(nil), nil
}

type deterministicMarshaler interface {
	XXX_Marshal([]byte, bool) ([]byte, error)
}

// DeterministicProtoMarshal marshals a proto message using deterministic encoding.
func DeterministicProtoMarshal(msg proto.Message) ([]byte, error) {
	if msg == nil {
		return nil, fmt.Errorf("message is required")
	}

	if marshaler, ok := msg.(deterministicMarshaler); ok {
		return marshaler.XXX_Marshal(nil, true)
	}

	buf := proto.NewBuffer(nil)
	buf.SetDeterministic(true)
	if err := buf.Marshal(msg); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// DeterministicProtoSHA256 returns SHA-256(deterministic_proto_bytes(msg)).
func DeterministicProtoSHA256(msg proto.Message) ([]byte, error) {
	signBytes, err := DeterministicProtoMarshal(msg)
	if err != nil {
		return nil, err
	}

	sum := sha256.Sum256(signBytes)
	return sum[:], nil
}

// IntentSignDocHash computes the typed domain hash for an intent payload.
// Binds to: module=cosmos-evm/match, version=v1, action=INTENT, + all payload fields.
func IntentSignDocHash(payload *IntentPayload) ([]byte, error) {
	return TypedSignDocHash(TypeURLIntent, payload)
}

// ResponseSignDocHash computes the typed domain hash for a response payload.
// Binds to: module=cosmos-evm/match, version=v1, action=RESPONSE, + all payload fields.
func ResponseSignDocHash(payload *ResponsePayload) ([]byte, error) {
	return TypedSignDocHash(TypeURLResponse, payload)
}

// FinalizeSignDocHash computes the typed domain hash for a finalize payload.
// Binds to: module=cosmos-evm/match, version=v1, action=FINALIZE, + all payload fields
// including settlement_hash which commits to the exact swap terms.
func FinalizeSignDocHash(payload *FinalizePayload) ([]byte, error) {
	return TypedSignDocHash(TypeURLFinalize, payload)
}

// CertificateSignDocHash computes the typed domain hash for a certificate payload.
// Binds to: module=cosmos-evm/match, version=v1, action=CERTIFICATE, + all payload fields.
func CertificateSignDocHash(payload *CertificatePayload) ([]byte, error) {
	return TypedSignDocHash(TypeURLCertificate, payload)
}
