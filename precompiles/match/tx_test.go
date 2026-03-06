package match

import (
	"bytes"
	"crypto/ecdsa"
	"testing"
	"time"

	precompiletest "github.com/cosmos/evm/precompiles/testutil"
	matchtypes "github.com/cosmos/evm/x/match/types"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type submitMockKeeper struct {
	lastSubmitReq *matchtypes.MsgSubmitMatchCertificate
	submitResp    *matchtypes.MsgSubmitMatchCertificateResponse
	submitErr     error
}

func (m *submitMockKeeper) HasReplay(_ sdk.Context, _, _ string) bool { return false }

func (m *submitMockKeeper) GetReplayMatchID(_ sdk.Context, _, _ string) (string, bool) {
	return "", false
}

func (m *submitMockKeeper) GetReplayParties(_ sdk.Context, _, _ string) (string, string, bool) {
	return "", "", false
}

func (m *submitMockKeeper) SubmitMatchCertificate(_ sdk.Context, req *matchtypes.MsgSubmitMatchCertificate) (*matchtypes.MsgSubmitMatchCertificateResponse, error) {
	m.lastSubmitReq = req
	if m.submitErr != nil {
		return nil, m.submitErr
	}
	if m.submitResp != nil {
		return m.submitResp, nil
	}
	return &matchtypes.MsgSubmitMatchCertificateResponse{}, nil
}

func TestSubmitMatchCertificate(t *testing.T) {
	initiator := mustIdentity(t)
	responder := mustIdentity(t)
	nowUnix := time.Unix(1_700_000_000, 0).Unix()
	certificate := buildValidCertificate(t, initiator, responder, nowUnix, nowUnix+600)
	certificateBz, err := certificate.Marshal()
	require.NoError(t, err)

	mockKeeper := &submitMockKeeper{
		submitResp: &matchtypes.MsgSubmitMatchCertificateResponse{
			MatchId:         "match-1",
			ReplayKey:       "pool-1::intent-1",
			CertificateHash: bytes.Repeat([]byte{0x01}, 32),
		},
	}
	p := NewPrecompile(mockKeeper)
	method := p.Methods[SubmitMatchCertificateMethod]

	contract, _ := precompiletest.NewPrecompileContract(
		t,
		sdk.Context{},
		common.HexToAddress(initiator.address),
		p.Address(),
		1_000_000,
	)

	bz, err := p.SubmitMatchCertificate(sdk.Context{}, contract, &method, []interface{}{certificateBz})
	require.NoError(t, err)

	out, err := method.Outputs.Unpack(bz)
	require.NoError(t, err)
	require.Len(t, out, 3)
	require.Equal(t, "match-1", out[0].(string))
	require.Equal(t, "pool-1::intent-1", out[1].(string))
	require.Equal(t, bytes.Repeat([]byte{0x01}, 32), out[2].([]byte))

	require.NotNil(t, mockKeeper.lastSubmitReq)
	require.Equal(t, common.HexToAddress(initiator.address).Hex(), mockKeeper.lastSubmitReq.Submitter)
	require.Equal(t, "pool-1", mockKeeper.lastSubmitReq.Certificate.Payload.PoolId)
}

func TestSubmitMatchCertificateRejectsNonInitiator(t *testing.T) {
	initiator := mustIdentity(t)
	responder := mustIdentity(t)
	other := mustIdentity(t)
	nowUnix := time.Unix(1_700_000_000, 0).Unix()
	certificate := buildValidCertificate(t, initiator, responder, nowUnix, nowUnix+600)
	certificateBz, err := certificate.Marshal()
	require.NoError(t, err)

	p := NewPrecompile(&submitMockKeeper{})
	method := p.Methods[SubmitMatchCertificateMethod]
	contract, _ := precompiletest.NewPrecompileContract(
		t,
		sdk.Context{},
		common.HexToAddress(other.address),
		p.Address(),
		1_000_000,
	)

	_, err = p.SubmitMatchCertificate(sdk.Context{}, contract, &method, []interface{}{certificateBz})
	require.Error(t, err)
	require.Contains(t, err.Error(), "msg.sender address")
}

func TestSubmitMatchCertificateRejectsInvalidBytes(t *testing.T) {
	initiator := mustIdentity(t)
	p := NewPrecompile(&submitMockKeeper{})
	method := p.Methods[SubmitMatchCertificateMethod]
	contract, _ := precompiletest.NewPrecompileContract(
		t,
		sdk.Context{},
		common.HexToAddress(initiator.address),
		p.Address(),
		1_000_000,
	)

	_, err := p.SubmitMatchCertificate(sdk.Context{}, contract, &method, []interface{}{[]byte{0x01, 0x02}})
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid certificate bytes")
}

type identity struct {
	priv    *ecdsa.PrivateKey
	address string
}

func mustIdentity(t *testing.T) identity {
	t.Helper()
	priv, err := crypto.GenerateKey()
	require.NoError(t, err)
	return identity{
		priv:    priv,
		address: crypto.PubkeyToAddress(priv.PublicKey).Hex(),
	}
}

func buildValidCertificate(
	t *testing.T,
	initiator identity,
	responder identity,
	issuedUnix int64,
	expiresUnix int64,
) matchtypes.MatchCertificate {
	t.Helper()

	contextHash := bytes.Repeat([]byte{0xaa}, 32)
	intentPayload := &matchtypes.IntentPayload{
		PoolId:          "pool-1",
		IntentId:        "intent-1",
		Initiator:       initiator.address,
		IssuedUnix:      issuedUnix,
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
		IssuedUnix:      issuedUnix,
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
		IssuedUnix:       issuedUnix,
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
		IssuedUnix:       issuedUnix,
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
				Signature: mustSignHash(t, initiator.priv, intentHash),
			},
			SignBytesHash: intentHash,
		},
		Response: &matchtypes.SignedResponse{
			Payload: responsePayload,
			Signature: &matchtypes.Signature{
				Signer:    responder.address,
				Algorithm: matchtypes.SignatureAlgorithmSecp256k1,
				Signature: mustSignHash(t, responder.priv, responseHash),
			},
			SignBytesHash: responseHash,
		},
		Finalize: &matchtypes.SignedFinalize{
			Payload: finalizePayload,
			InitiatorSignature: &matchtypes.Signature{
				Signer:    initiator.address,
				Algorithm: matchtypes.SignatureAlgorithmSecp256k1,
				Signature: mustSignHash(t, initiator.priv, finalizeHash),
			},
			ResponderSignature: &matchtypes.Signature{
				Signer:    responder.address,
				Algorithm: matchtypes.SignatureAlgorithmSecp256k1,
				Signature: mustSignHash(t, responder.priv, finalizeHash),
			},
			SignBytesHash: finalizeHash,
		},
		BoardSignature: &matchtypes.Signature{
			Signer:    initiator.address,
			Algorithm: matchtypes.SignatureAlgorithmSecp256k1,
			Signature: mustSignHash(t, initiator.priv, certificateHash),
		},
		SignBytesHash: certificateHash,
	}
}

func mustSignHash(t *testing.T, priv *ecdsa.PrivateKey, hash []byte) []byte {
	t.Helper()
	sig, err := crypto.Sign(hash, priv)
	require.NoError(t, err)
	return sig
}
