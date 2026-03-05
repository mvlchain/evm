package types

import (
	"crypto/sha256"
	"fmt"

	proto "github.com/cosmos/gogoproto/proto"
)

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

// IntentSignDocHash computes SHA-256 over the deterministic IntentSignDoc bytes.
func IntentSignDocHash(payload *IntentPayload) ([]byte, error) {
	return DeterministicProtoSHA256(&IntentSignDoc{
		SignDocType: SignDocType_SIGN_DOC_TYPE_INTENT,
		Payload:     payload,
	})
}

// ResponseSignDocHash computes SHA-256 over the deterministic ResponseSignDoc bytes.
func ResponseSignDocHash(payload *ResponsePayload) ([]byte, error) {
	return DeterministicProtoSHA256(&ResponseSignDoc{
		SignDocType: SignDocType_SIGN_DOC_TYPE_RESPONSE,
		Payload:     payload,
	})
}

// FinalizeSignDocHash computes SHA-256 over the deterministic FinalizeSignDoc bytes.
func FinalizeSignDocHash(payload *FinalizePayload) ([]byte, error) {
	return DeterministicProtoSHA256(&FinalizeSignDoc{
		SignDocType: SignDocType_SIGN_DOC_TYPE_FINALIZE,
		Payload:     payload,
	})
}

// CertificateSignDocHash computes SHA-256 over the deterministic CertificateSignDoc bytes.
func CertificateSignDocHash(payload *CertificatePayload) ([]byte, error) {
	return DeterministicProtoSHA256(&CertificateSignDoc{
		SignDocType: SignDocType_SIGN_DOC_TYPE_CERTIFICATE,
		Payload:     payload,
	})
}
