package matchboard

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"testing"

	matchtypes "github.com/cosmos/evm/x/match/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestDecodeOperationCertificateSuccess(t *testing.T) {
	nowUnix := int64(1_700_000_000)
	cert := validMatchCertificateForTest(t, nowUnix, nowUnix+3600)
	certBytes, err := matchtypes.DeterministicProtoMarshal(&cert)
	require.NoError(t, err)

	op := ProposedOperation{
		RecordType:       RecordTypeFinalize,
		PoolID:           cert.Payload.PoolId,
		IntentID:         cert.Payload.IntentId,
		ResponseID:       cert.Payload.ResponseId,
		FinalizeID:       cert.Payload.FinalizeId,
		Sender:           cert.Payload.Initiator,
		Recipient:        cert.Payload.Responder,
		IntentSignHash:   hex.EncodeToString(cert.Payload.IntentSignHash),
		ResponseSignHash: hex.EncodeToString(cert.Payload.ResponseSignHash),
		FinalizeSignHash: hex.EncodeToString(cert.Payload.FinalizeSignHash),
		MatchCertificate: certBytes,
	}
	op.OperationID = BuildOperationIDFromProposedOperation(op)

	decoded, err := DecodeOperationCertificate(op)
	require.NoError(t, err)
	require.NotNil(t, decoded)
	require.Equal(t, cert.Payload.PoolId, decoded.Payload.PoolId)
}

func TestDecodeOperationCertificateRejectsBindingMismatch(t *testing.T) {
	nowUnix := int64(1_700_000_000)
	cert := validMatchCertificateForTest(t, nowUnix, nowUnix+3600)
	certBytes, err := matchtypes.DeterministicProtoMarshal(&cert)
	require.NoError(t, err)

	op := ProposedOperation{
		RecordType:       RecordTypeFinalize,
		PoolID:           cert.Payload.PoolId,
		IntentID:         cert.Payload.IntentId,
		ResponseID:       cert.Payload.ResponseId,
		FinalizeID:       cert.Payload.FinalizeId,
		Sender:           cert.Payload.Initiator,
		Recipient:        cert.Payload.Responder,
		IntentSignHash:   testHash("1"),
		ResponseSignHash: hex.EncodeToString(cert.Payload.ResponseSignHash),
		FinalizeSignHash: hex.EncodeToString(cert.Payload.FinalizeSignHash),
		MatchCertificate: certBytes,
	}
	op.OperationID = BuildOperationIDFromProposedOperation(op)

	_, decodeErr := DecodeOperationCertificate(op)
	require.Error(t, decodeErr)
	require.Contains(t, decodeErr.Error(), "intent_sign_hash")
}

func TestValidateAndNormalizeFinalizeWithMatchCertificate(t *testing.T) {
	nowUnix := int64(1_700_000_000)
	cert := validMatchCertificateForTest(t, nowUnix, nowUnix+3600)
	certBytes, err := proto.Marshal(&cert)
	require.NoError(t, err)

	req := PublishFinalizeRequest{
		PoolID:           cert.Payload.PoolId,
		IntentID:         cert.Payload.IntentId,
		ResponseID:       cert.Payload.ResponseId,
		FinalizeID:       cert.Payload.FinalizeId,
		Sender:           cert.Payload.Initiator,
		Recipient:        cert.Payload.Responder,
		ExpiresUnix:      cert.Finalize.Payload.ExpiresUnix,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		IntentSignHash:   hex.EncodeToString(cert.Payload.IntentSignHash),
		ResponseSignHash: hex.EncodeToString(cert.Payload.ResponseSignHash),
		FinalizeSignHash: hex.EncodeToString(cert.Payload.FinalizeSignHash),
		ContextHash:      hex.EncodeToString(cert.Payload.ContextHash),
		InitiatorSignature: SignatureMetadata{
			Signer:    cert.Payload.Initiator,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: hex.EncodeToString(cert.Finalize.InitiatorSignature.Signature),
		},
		ResponderSignature: SignatureMetadata{
			Signer:    cert.Payload.Responder,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: hex.EncodeToString(cert.Finalize.ResponderSignature.Signature),
		},
		MatchCertificate: certBytes,
	}

	err = validateAndNormalizeFinalize(&req, req.Sender, nowUnix)
	require.NoError(t, err)

	normalizedCert, marshalErr := matchtypes.DeterministicProtoMarshal(&cert)
	require.NoError(t, marshalErr)
	require.Equal(t, normalizedCert, req.MatchCertificate)
}

func validMatchCertificateForTest(t *testing.T, nowUnix, expiresUnix int64) matchtypes.MatchCertificate {
	t.Helper()

	contextHash := bytes.Repeat([]byte{0xaa}, 32)
	initiator := mustNewIdentityForMatchCert(t)
	responder := mustNewIdentityForMatchCert(t)
	board := mustNewIdentityForMatchCert(t)

	intentPayload := &matchtypes.IntentPayload{
		PoolId:          "pool-1",
		IntentId:        "intent-1",
		Initiator:       initiator.address,
		IssuedUnix:      nowUnix,
		ExpiresUnix:     expiresUnix,
		ContextHash:     contextHash,
		DigestAlgorithm: matchtypes.DigestAlgorithmSHA256,
	}
	intentHash, err := matchtypes.IntentSignDocHash(intentPayload)
	require.NoError(t, err)

	responsePayload := &matchtypes.ResponsePayload{
		PoolId:          "pool-1",
		IntentId:        "intent-1",
		IntentSignHash:  intentHash,
		ResponseId:      "response-1",
		Responder:       responder.address,
		IssuedUnix:      nowUnix,
		ExpiresUnix:     expiresUnix,
		ContextHash:     contextHash,
		DigestAlgorithm: matchtypes.DigestAlgorithmSHA256,
	}
	responseHash, err := matchtypes.ResponseSignDocHash(responsePayload)
	require.NoError(t, err)

	finalizePayload := &matchtypes.FinalizePayload{
		PoolId:           "pool-1",
		IntentId:         "intent-1",
		ResponseId:       "response-1",
		IntentSignHash:   intentHash,
		ResponseSignHash: responseHash,
		FinalizeId:       "finalize-1",
		Initiator:        initiator.address,
		Responder:        responder.address,
		IssuedUnix:       nowUnix,
		ExpiresUnix:      expiresUnix,
		ContextHash:      contextHash,
		DigestAlgorithm:  matchtypes.DigestAlgorithmSHA256,
	}
	finalizeHash, err := matchtypes.FinalizeSignDocHash(finalizePayload)
	require.NoError(t, err)

	certificatePayload := &matchtypes.CertificatePayload{
		PoolId:           "pool-1",
		IntentId:         "intent-1",
		ResponseId:       "response-1",
		FinalizeId:       "finalize-1",
		CertificateId:    "certificate-1",
		IntentSignHash:   intentHash,
		ResponseSignHash: responseHash,
		FinalizeSignHash: finalizeHash,
		Initiator:        initiator.address,
		Responder:        responder.address,
		IssuedUnix:       nowUnix,
		ExpiresUnix:      expiresUnix,
		ContextHash:      contextHash,
		DigestAlgorithm:  matchtypes.DigestAlgorithmSHA256,
	}
	certificateHash, err := matchtypes.CertificateSignDocHash(certificatePayload)
	require.NoError(t, err)

	return matchtypes.MatchCertificate{
		Payload: certificatePayload,
		Intent: &matchtypes.SignedIntent{
			Payload: intentPayload,
			Signature: &matchtypes.Signature{
				Signer:    initiator.address,
				Algorithm: matchtypes.SignatureAlgorithmSecp256k1,
				Signature: mustSignHashForMatchCert(t, initiator.priv, intentHash),
			},
			SignBytesHash: intentHash,
		},
		Response: &matchtypes.SignedResponse{
			Payload: responsePayload,
			Signature: &matchtypes.Signature{
				Signer:    responder.address,
				Algorithm: matchtypes.SignatureAlgorithmSecp256k1,
				Signature: mustSignHashForMatchCert(t, responder.priv, responseHash),
			},
			SignBytesHash: responseHash,
		},
		Finalize: &matchtypes.SignedFinalize{
			Payload: finalizePayload,
			InitiatorSignature: &matchtypes.Signature{
				Signer:    initiator.address,
				Algorithm: matchtypes.SignatureAlgorithmSecp256k1,
				Signature: mustSignHashForMatchCert(t, initiator.priv, finalizeHash),
			},
			ResponderSignature: &matchtypes.Signature{
				Signer:    responder.address,
				Algorithm: matchtypes.SignatureAlgorithmSecp256k1,
				Signature: mustSignHashForMatchCert(t, responder.priv, finalizeHash),
			},
			SignBytesHash: finalizeHash,
		},
		BoardSignature: &matchtypes.Signature{
			Signer:    board.address,
			Algorithm: matchtypes.SignatureAlgorithmSecp256k1,
			Signature: mustSignHashForMatchCert(t, board.priv, certificateHash),
		},
		SignBytesHash: certificateHash,
	}
}

type matchCertIdentity struct {
	priv    *ecdsa.PrivateKey
	address string
}

func mustNewIdentityForMatchCert(t *testing.T) matchCertIdentity {
	t.Helper()
	key, err := crypto.GenerateKey()
	require.NoError(t, err)
	return matchCertIdentity{
		priv:    key,
		address: crypto.PubkeyToAddress(key.PublicKey).Hex(),
	}
}

func mustSignHashForMatchCert(t *testing.T, priv *ecdsa.PrivateKey, hash []byte) []byte {
	t.Helper()
	sig, err := crypto.Sign(hash, priv)
	require.NoError(t, err)
	return sig
}
