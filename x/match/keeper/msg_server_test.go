package keeper_test

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"strings"
	"testing"
	"time"

	matchkeeper "github.com/cosmos/evm/x/match/keeper"
	"github.com/cosmos/evm/x/match/types"
	"github.com/ethereum/go-ethereum/crypto"

	storetypes "cosmossdk.io/store/types"

	"github.com/cosmos/cosmos-sdk/testutil"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	"github.com/stretchr/testify/require"
)

func TestSubmitMatchCertificateSuccessAndReplayProtection(t *testing.T) {
	k, ctx := setupKeeper(t)
	msg := validSubmitMessage(ctx.BlockTime().Unix())

	resp, err := k.SubmitMatchCertificate(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Equal(t, "pool-1:intent-1", resp.ReplayKey)
	require.Equal(t, msg.Certificate.MatchID(), resp.MatchId)
	require.Equal(t, msg.Certificate.CertificateHash(), resp.CertificateHash)

	replayedMatchID, ok := k.GetReplayMatchID(ctx, "pool-1", "intent-1")
	require.True(t, ok)
	require.Equal(t, resp.MatchId, replayedMatchID)

	requester, responder, partiesFound := k.GetReplayParties(ctx, "pool-1", "intent-1")
	require.True(t, partiesFound)
	require.Equal(t, msg.Certificate.Payload.Initiator, requester)
	require.Equal(t, msg.Certificate.Payload.Responder, responder)

	events := ctx.EventManager().Events()
	require.NotEmpty(t, events)
	require.Equal(t, types.EventTypeSubmitMatchCertificate, events[len(events)-1].Type)

	_, err = k.SubmitMatchCertificate(ctx, msg)
	require.Error(t, err)
	require.ErrorContains(t, err, types.ErrReplayDetected.Error())
}

func TestSubmitMatchCertificateRejectsHashChainMismatch(t *testing.T) {
	k, ctx := setupKeeper(t)
	msg := validSubmitMessage(ctx.BlockTime().Unix())
	msg.Certificate.Payload.ResponseSignHash = bytes.Repeat([]byte{0x99}, 32)

	_, err := k.SubmitMatchCertificate(ctx, msg)
	require.Error(t, err)
	require.ErrorContains(t, err, types.ErrHashMismatch.Error())
}

func TestSubmitMatchCertificateRejectsExpiredCertificate(t *testing.T) {
	k, ctx := setupKeeper(t)
	msg := validSubmitMessageWithExpiry(ctx.BlockTime().Unix(), ctx.BlockTime().Unix()-1)

	_, err := k.SubmitMatchCertificate(ctx, msg)
	require.Error(t, err)
	require.ErrorContains(t, err, types.ErrExpired.Error())
}

func TestSubmitMatchCertificateRejectsSignerRoleMismatch(t *testing.T) {
	k, ctx := setupKeeper(t)
	msg := validSubmitMessage(ctx.BlockTime().Unix())
	msg.Certificate.Response.Signature.Signer = msg.Certificate.Intent.Payload.Initiator

	_, err := k.SubmitMatchCertificate(ctx, msg)
	require.Error(t, err)
	require.ErrorContains(t, err, types.ErrSignerMismatch.Error())
}

func TestSubmitMatchCertificateRejectsInvalidHashLength(t *testing.T) {
	k, ctx := setupKeeper(t)
	msg := validSubmitMessage(ctx.BlockTime().Unix())
	msg.Certificate.Finalize.SignBytesHash = bytes.Repeat([]byte{0x55}, 31)

	_, err := k.SubmitMatchCertificate(ctx, msg)
	require.Error(t, err)
	require.ErrorContains(t, err, types.ErrInvalidRequest.Error())
	require.ErrorContains(t, err, "must be 32 bytes")
}

func TestSubmitMatchCertificateRejectsInvalidCryptographicSignature(t *testing.T) {
	k, ctx := setupKeeper(t)
	msg := validSubmitMessage(ctx.BlockTime().Unix())
	msg.Certificate.Intent.Signature.Signature[0] ^= 0x01

	_, err := k.SubmitMatchCertificate(ctx, msg)
	require.Error(t, err)
	require.True(
		t,
		strings.Contains(err.Error(), types.ErrInvalidSignature.Error()) ||
			strings.Contains(err.Error(), types.ErrSignerMismatch.Error()),
		"expected invalid signature or signer mismatch error, got: %v",
		err,
	)
}

func TestSubmitMatchCertificateEmitsAllEventAttributes(t *testing.T) {
	k, ctx := setupKeeper(t)
	msg := validSubmitMessage(ctx.BlockTime().Unix())

	resp, err := k.SubmitMatchCertificate(ctx, msg)
	require.NoError(t, err)
	require.NotNil(t, resp)

	events := ctx.EventManager().Events()
	require.NotEmpty(t, events)
	event := events[len(events)-1]
	require.Equal(t, types.EventTypeSubmitMatchCertificate, event.Type)

	attrs := make(map[string]string, len(event.Attributes))
	for _, attr := range event.Attributes {
		attrs[string(attr.Key)] = string(attr.Value)
	}

	expected := map[string]string{
		types.AttributeKeySubmitter:       msg.Submitter,
		types.AttributeKeyPoolID:          msg.Certificate.Payload.PoolId,
		types.AttributeKeyIntentID:        msg.Certificate.Payload.IntentId,
		types.AttributeKeyResponseID:      msg.Certificate.Payload.ResponseId,
		types.AttributeKeyFinalizeID:      msg.Certificate.Payload.FinalizeId,
		types.AttributeKeyCertificateHash: hex.EncodeToString(msg.Certificate.CertificateHash()),
		types.AttributeKeyReplayKey:       types.ReplayKeyString(msg.Certificate.Payload.PoolId, msg.Certificate.Payload.IntentId),
		types.AttributeKeyMatchID:         msg.Certificate.MatchID(),
	}

	require.Len(t, attrs, len(expected))
	require.Equal(t, expected, attrs)
}

func setupKeeper(t *testing.T) (matchkeeper.Keeper, sdk.Context) {
	t.Helper()

	storeKey := storetypes.NewKVStoreKey(types.StoreKey)
	transientStoreKey := storetypes.NewTransientStoreKey("match-test")
	ctx := testutil.DefaultContext(storeKey, transientStoreKey).
		WithBlockTime(time.Unix(1_700_000_000, 0)).
		WithEventManager(sdk.NewEventManager())

	encCfg := moduletestutil.MakeTestEncodingConfig()
	k := matchkeeper.NewKeeper(encCfg.Codec, storeKey)
	return k, ctx
}

func validSubmitMessage(nowUnix int64) *types.MsgSubmitMatchCertificate {
	return validSubmitMessageWithExpiry(nowUnix, nowUnix+3600)
}

func validSubmitMessageWithExpiry(nowUnix, expiresUnix int64) *types.MsgSubmitMatchCertificate {
	contextHash := bytes.Repeat([]byte{0xaa}, 32)
	maker := mustNewIdentity()
	responder := mustNewIdentity()
	board := mustNewIdentity()

	intentPayload := &types.IntentPayload{
		PoolId:          "pool-1",
		IntentId:        "intent-1",
		Initiator:       maker.address,
		IssuedUnix:      nowUnix,
		ExpiresUnix:     expiresUnix,
		ContextHash:     contextHash,
		DigestAlgorithm: types.DigestAlgorithmSHA256,
	}
	intentHash := mustHash(types.IntentSignDocHash(intentPayload))

	responsePayload := &types.ResponsePayload{
		PoolId:          "pool-1",
		IntentId:        "intent-1",
		IntentSignHash:  intentHash,
		ResponseId:      "response-1",
		Responder:       responder.address,
		IssuedUnix:      nowUnix,
		ExpiresUnix:     expiresUnix,
		ContextHash:     contextHash,
		DigestAlgorithm: types.DigestAlgorithmSHA256,
	}
	responseHash := mustHash(types.ResponseSignDocHash(responsePayload))

	finalizePayload := &types.FinalizePayload{
		PoolId:           "pool-1",
		IntentId:         "intent-1",
		ResponseId:       "response-1",
		IntentSignHash:   intentHash,
		ResponseSignHash: responseHash,
		FinalizeId:       "finalize-1",
		Initiator:        maker.address,
		Responder:        responder.address,
		IssuedUnix:       nowUnix,
		ExpiresUnix:      expiresUnix,
		ContextHash:      contextHash,
		DigestAlgorithm:  types.DigestAlgorithmSHA256,
	}
	finalizeHash := mustHash(types.FinalizeSignDocHash(finalizePayload))

	certificatePayload := &types.CertificatePayload{
		PoolId:           "pool-1",
		IntentId:         "intent-1",
		ResponseId:       "response-1",
		FinalizeId:       "finalize-1",
		CertificateId:    "certificate-1",
		IntentSignHash:   intentHash,
		ResponseSignHash: responseHash,
		FinalizeSignHash: finalizeHash,
		Initiator:        maker.address,
		Responder:        responder.address,
		IssuedUnix:       nowUnix,
		ExpiresUnix:      expiresUnix,
		ContextHash:      contextHash,
		DigestAlgorithm:  types.DigestAlgorithmSHA256,
	}
	certificateHash := mustHash(types.CertificateSignDocHash(certificatePayload))

	return &types.MsgSubmitMatchCertificate{
		Submitter: board.address,
		Certificate: types.MatchCertificate{
			Payload: certificatePayload,
			Intent: &types.SignedIntent{
				Payload: intentPayload,
				Signature: &types.Signature{
					Signer:    maker.address,
					Algorithm: types.SignatureAlgorithmSecp256k1,
					Signature: mustSignHash(maker.priv, intentHash),
				},
				SignBytesHash: intentHash,
			},
			Response: &types.SignedResponse{
				Payload: responsePayload,
				Signature: &types.Signature{
					Signer:    responder.address,
					Algorithm: types.SignatureAlgorithmSecp256k1,
					Signature: mustSignHash(responder.priv, responseHash),
				},
				SignBytesHash: responseHash,
			},
			Finalize: &types.SignedFinalize{
				Payload: finalizePayload,
				InitiatorSignature: &types.Signature{
					Signer:    maker.address,
					Algorithm: types.SignatureAlgorithmSecp256k1,
					Signature: mustSignHash(maker.priv, finalizeHash),
				},
				ResponderSignature: &types.Signature{
					Signer:    responder.address,
					Algorithm: types.SignatureAlgorithmSecp256k1,
					Signature: mustSignHash(responder.priv, finalizeHash),
				},
				SignBytesHash: finalizeHash,
			},
			BoardSignature: &types.Signature{
				Signer:    board.address,
				Algorithm: types.SignatureAlgorithmSecp256k1,
				Signature: mustSignHash(board.priv, certificateHash),
			},
			SignBytesHash: certificateHash,
		},
	}
}

type testIdentity struct {
	priv    *ecdsa.PrivateKey
	address string
}

func mustNewIdentity() testIdentity {
	key, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	return testIdentity{
		priv:    key,
		address: crypto.PubkeyToAddress(key.PublicKey).Hex(),
	}
}

func mustHash(hash []byte, err error) []byte {
	if err != nil {
		panic(err)
	}
	return hash
}

func mustSignHash(priv *ecdsa.PrivateKey, hash []byte) []byte {
	sig, err := crypto.Sign(hash, priv)
	if err != nil {
		panic(err)
	}
	return sig
}
