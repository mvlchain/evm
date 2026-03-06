package matchboard

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/cosmos/evm/x/match/types"
	"github.com/cosmos/gogoproto/proto"
)

// NormalizeOperationCertificate validates and canonicalizes optional certificate bytes
// attached to an injected proposed operation.
func NormalizeOperationCertificate(op ProposedOperation) ([]byte, *types.MatchCertificate, error) {
	if len(op.MatchCertificate) == 0 {
		return nil, nil, nil
	}
	if op.RecordType != RecordTypeFinalize {
		return nil, nil, fmt.Errorf("match_certificate is only allowed for finalize operations")
	}

	var cert types.MatchCertificate
	if err := proto.Unmarshal(op.MatchCertificate, &cert); err != nil {
		return nil, nil, fmt.Errorf("decode match_certificate: %w", err)
	}
	if err := cert.ValidateBasic(op.Sender); err != nil {
		return nil, nil, fmt.Errorf("invalid match_certificate: %w", err)
	}
	if err := validateOperationCertificateBinding(op, &cert); err != nil {
		return nil, nil, err
	}

	canonical, err := types.DeterministicProtoMarshal(&cert)
	if err != nil {
		return nil, nil, fmt.Errorf("canonical marshal match_certificate: %w", err)
	}

	return canonical, &cert, nil
}

// DecodeOperationCertificate decodes and validates optional certificate bytes attached to an operation.
// It additionally requires that the encoded bytes are canonical deterministic protobuf bytes.
func DecodeOperationCertificate(op ProposedOperation) (*types.MatchCertificate, error) {
	if len(op.MatchCertificate) == 0 {
		return nil, nil
	}

	canonical, cert, err := NormalizeOperationCertificate(op)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(canonical, op.MatchCertificate) {
		return nil, fmt.Errorf("match_certificate must use deterministic protobuf canonical bytes")
	}

	return cert, nil
}

// BuildSubmitMatchCertificateMsgPayload builds deterministic protobuf tx payload bytes
// for match.v1.MsgSubmitMatchCertificate from canonical certificate bytes.
func BuildSubmitMatchCertificateMsgPayload(canonicalCertificate []byte, submitter string) ([]byte, error) {
	if len(canonicalCertificate) == 0 {
		return nil, nil
	}
	submitter = strings.TrimSpace(submitter)
	if submitter == "" {
		return nil, fmt.Errorf("submitter is required")
	}

	var cert types.MatchCertificate
	if err := proto.Unmarshal(canonicalCertificate, &cert); err != nil {
		return nil, fmt.Errorf("decode match_certificate: %w", err)
	}
	normalizedCert, err := types.DeterministicProtoMarshal(&cert)
	if err != nil {
		return nil, fmt.Errorf("canonical marshal match_certificate: %w", err)
	}
	if !bytes.Equal(normalizedCert, canonicalCertificate) {
		return nil, fmt.Errorf("match_certificate must use deterministic protobuf canonical bytes")
	}

	msg := &types.MsgSubmitMatchCertificate{
		Submitter:   submitter,
		Certificate: cert,
	}
	if err := msg.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("invalid msg_submit_match_certificate: %w", err)
	}

	msgBz, err := types.DeterministicProtoMarshal(msg)
	if err != nil {
		return nil, fmt.Errorf("canonical marshal msg_submit_match_certificate: %w", err)
	}
	return msgBz, nil
}

// DecodeSubmitMatchCertificateMsgPayload decodes and validates deterministic protobuf tx payload bytes.
func DecodeSubmitMatchCertificateMsgPayload(payload []byte) (*types.MsgSubmitMatchCertificate, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	var msg types.MsgSubmitMatchCertificate
	if err := proto.Unmarshal(payload, &msg); err != nil {
		return nil, fmt.Errorf("decode msg_submit_match_certificate payload: %w", err)
	}
	if err := msg.ValidateBasic(); err != nil {
		return nil, fmt.Errorf("invalid msg_submit_match_certificate payload: %w", err)
	}

	normalized, err := types.DeterministicProtoMarshal(&msg)
	if err != nil {
		return nil, fmt.Errorf("canonical marshal msg_submit_match_certificate payload: %w", err)
	}
	if !bytes.Equal(normalized, payload) {
		return nil, fmt.Errorf("msg_submit_match_certificate payload must use deterministic protobuf canonical bytes")
	}
	return &msg, nil
}

func validateOperationCertificateBinding(op ProposedOperation, cert *types.MatchCertificate) error {
	if cert == nil || cert.Payload == nil {
		return fmt.Errorf("match_certificate.payload is required")
	}
	payload := cert.Payload

	if strings.TrimSpace(payload.PoolId) != strings.TrimSpace(op.PoolID) {
		return fmt.Errorf("match_certificate.payload.pool_id mismatch")
	}
	if strings.TrimSpace(payload.IntentId) != strings.TrimSpace(op.IntentID) {
		return fmt.Errorf("match_certificate.payload.intent_id mismatch")
	}
	if strings.TrimSpace(payload.ResponseId) != strings.TrimSpace(op.ResponseID) {
		return fmt.Errorf("match_certificate.payload.response_id mismatch")
	}
	if strings.TrimSpace(payload.FinalizeId) != strings.TrimSpace(op.FinalizeID) {
		return fmt.Errorf("match_certificate.payload.finalize_id mismatch")
	}

	if !identitiesEqual(payload.Initiator, op.Sender) {
		return fmt.Errorf("match_certificate.payload.initiator mismatch")
	}
	if !identitiesEqual(payload.Responder, op.Recipient) {
		return fmt.Errorf("match_certificate.payload.responder mismatch")
	}

	if err := requireHashBinding("intent_sign_hash", op.IntentSignHash, payload.IntentSignHash); err != nil {
		return err
	}
	if err := requireHashBinding("response_sign_hash", op.ResponseSignHash, payload.ResponseSignHash); err != nil {
		return err
	}
	if err := requireHashBinding("finalize_sign_hash", op.FinalizeSignHash, payload.FinalizeSignHash); err != nil {
		return err
	}

	return nil
}

func requireHashBinding(field string, expectedHex string, actual []byte) error {
	expectedHex = strings.TrimSpace(strings.ToLower(expectedHex))
	if expectedHex == "" {
		return fmt.Errorf("%s is required for certificate binding", field)
	}
	if len(actual) == 0 {
		return fmt.Errorf("match_certificate.payload.%s is required", field)
	}
	if len(actual) != 32 {
		return fmt.Errorf("match_certificate.payload.%s must be 32 bytes", field)
	}
	if expectedHex != hex.EncodeToString(actual) {
		return fmt.Errorf("match_certificate.payload.%s mismatch", field)
	}
	return nil
}
