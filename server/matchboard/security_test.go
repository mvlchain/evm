package matchboard

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestInboxOutboxOpenAccess(t *testing.T) {
	t.Parallel()

	h := newSecurityTestHandler(t, nil)

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	intentHash := hashChar('a')
	publishIntentPayload := mustJSON(t, map[string]any{
		"pool_id":          "pool-authz",
		"intent_id":        "intent-authz",
		"sender":           alice.address,
		"recipient":        bob.address,
		"expires_unix":     securityNow.Unix() + 300,
		"digest_algorithm": DigestAlgorithmSHA256,
		"intent_sign_hash": intentHash,
		"context_hash":     hashChar('b'),
		"signature": map[string]any{
			"signer":    alice.address,
			"algorithm": SignatureAlgorithmSecp256k1,
			"signature": mustSignHashHex(alice.priv, intentHash),
		},
	})

	publishResp := performJSONRequest(t, h, http.MethodPost, "/v1/intents", publishIntentPayload)
	require.Equal(t, http.StatusCreated, publishResp.Code)

	// anyone can query bob's inbox by address
	bobInbox := performJSONRequest(t, h, http.MethodGet, "/v1/inbox?recipient="+bob.address, nil)
	require.Equal(t, http.StatusOK, bobInbox.Code)
	var inbox listRecordsResponse
	mustDecodeJSON(t, bobInbox.Body.Bytes(), &inbox)
	require.Equal(t, bob.address, inbox.Principal)
	require.EqualValues(t, 1, inbox.Total)
	require.Len(t, inbox.Records, 1)
	require.Equal(t, alice.address, inbox.Records[0].Sender)
	require.Equal(t, bob.address, inbox.Records[0].Recipient)

	// anyone can query alice's outbox by address
	aliceOutbox := performJSONRequest(t, h, http.MethodGet, "/v1/outbox?sender="+alice.address, nil)
	require.Equal(t, http.StatusOK, aliceOutbox.Code)
	var outbox listRecordsResponse
	mustDecodeJSON(t, aliceOutbox.Body.Bytes(), &outbox)
	require.Equal(t, alice.address, outbox.Principal)
	require.EqualValues(t, 1, outbox.Total)
	require.Len(t, outbox.Records, 1)
	require.Equal(t, alice.address, outbox.Records[0].Sender)
	require.Equal(t, bob.address, outbox.Records[0].Recipient)
}

func TestSensitivePlaintextNotLeakedInLogs(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := newSecurityTestHandler(t, logger)

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	const secretPlaintext = "very-sensitive-plaintext-should-never-appear-in-logs"
	intentHash := hashChar('c')
	publishIntentPayload := mustJSON(t, map[string]any{
		"pool_id":          "pool-logs",
		"intent_id":        "intent-logs",
		"sender":           alice.address,
		"recipient":        bob.address,
		"expires_unix":     securityNow.Unix() + 300,
		"digest_algorithm": DigestAlgorithmSHA256,
		"intent_sign_hash": intentHash,
		"context_hash":     hashChar('d'),
		"signature": map[string]any{
			"signer":     alice.address,
			"algorithm":  SignatureAlgorithmSecp256k1,
			"public_key": secretPlaintext,
			"signature":  mustSignHashHex(alice.priv, intentHash),
		},
	})

	resp := performJSONRequest(t, h, http.MethodPost, "/v1/intents", publishIntentPayload)
	require.Equal(t, http.StatusCreated, resp.Code)

	logOutput := logs.String()
	require.Contains(t, logOutput, "matchboard intent stored")
	require.NotContains(t, logOutput, secretPlaintext)
}

func TestStrictJSONUnknownFieldsRejected(t *testing.T) {
	t.Parallel()

	h := newSecurityTestHandler(t, nil)

	cases := []struct {
		name    string
		path    string
		payload map[string]any
	}{
		{
			name: "intent unknown field rejected",
			path: "/v1/intents",
			payload: map[string]any{
				"pool_id":           "pool-strict-intent",
				"intent_id":         "intent-strict-intent",
				"sender":            "alice",
				"recipient":         "bob",
				"expires_unix":      securityNow.Unix() + 300,
				"digest_algorithm":  DigestAlgorithmSHA256,
				"intent_sign_hash":  hashChar('e'),
				"context_hash":      hashChar('f'),
				"plaintext_profile": "secret-profile-plaintext",
				"signature": map[string]any{
					"signer":    "alice",
					"algorithm": SignatureAlgorithmSecp256k1,
					"signature": "sig-alice",
				},
			},
		},
		{
			name: "response unknown field rejected",
			path: "/v1/responses",
			payload: map[string]any{
				"pool_id":            "pool-strict-response",
				"intent_id":          "intent-strict-response",
				"response_id":        "response-strict-response",
				"sender":             "bob",
				"recipient":          "alice",
				"expires_unix":       securityNow.Unix() + 300,
				"digest_algorithm":   DigestAlgorithmSHA256,
				"intent_sign_hash":   hashChar('1'),
				"response_sign_hash": hashChar('2'),
				"context_hash":       hashChar('3'),
				"plaintext_message":  "secret-message-plaintext",
				"signature": map[string]any{
					"signer":    "bob",
					"algorithm": SignatureAlgorithmSecp256k1,
					"signature": "sig-bob",
				},
			},
		},
		{
			name: "finalize unknown field rejected",
			path: "/v1/finalize",
			payload: map[string]any{
				"pool_id":            "pool-strict-finalize",
				"intent_id":          "intent-strict-finalize",
				"response_id":        "response-strict-finalize",
				"finalize_id":        "finalize-strict-finalize",
				"sender":             "alice",
				"recipient":          "bob",
				"expires_unix":       securityNow.Unix() + 300,
				"digest_algorithm":   DigestAlgorithmSHA256,
				"intent_sign_hash":   hashChar('4'),
				"response_sign_hash": hashChar('5'),
				"finalize_sign_hash": hashChar('6'),
				"context_hash":       hashChar('7'),
				"plaintext_terms":    "secret-terms-plaintext",
				"initiator_signature": map[string]any{
					"signer":    "alice",
					"algorithm": SignatureAlgorithmSecp256k1,
					"signature": "sig-alice",
				},
				"responder_signature": map[string]any{
					"signer":    "bob",
					"algorithm": SignatureAlgorithmSecp256k1,
					"signature": "sig-bob",
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := performJSONRequest(t, h, http.MethodPost, tc.path, mustJSON(t, tc.payload))
			require.Equal(t, http.StatusBadRequest, resp.Code)
			assertErrorEnvelope(t, resp.Body.Bytes(), errorCodeInvalidRequest, "body")
			require.Contains(t, resp.Body.String(), "unknown field")
		})
	}

	outbox := performJSONRequest(t, h, http.MethodGet, "/v1/outbox?sender=alice", nil)
	require.Equal(t, http.StatusOK, outbox.Code)
	var outboxBody listRecordsResponse
	mustDecodeJSON(t, outbox.Body.Bytes(), &outboxBody)
	require.EqualValues(t, 0, outboxBody.Total)
	require.Len(t, outboxBody.Records, 0)
}

func TestEthereumAddressSignatureVerification(t *testing.T) {
	t.Parallel()

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	h, err := NewHandler(Config{
		NowFn: func() time.Time { return securityNow },
	})
	require.NoError(t, err)

	intentHash := hashChar('9')
	validIntentSignature := mustSignHashHex(alice.priv, intentHash)

	publishIntentPayload := mustJSON(t, map[string]any{
		"pool_id":          "pool-eth-sig",
		"intent_id":        "intent-eth-sig",
		"sender":           alice.address,
		"recipient":        bob.address,
		"expires_unix":     securityNow.Unix() + 300,
		"digest_algorithm": DigestAlgorithmSHA256,
		"intent_sign_hash": intentHash,
		"context_hash":     hashChar('8'),
		"signature": map[string]any{
			"signer":    alice.address,
			"algorithm": SignatureAlgorithmSecp256k1,
			"signature": validIntentSignature,
		},
	})

	okResp := performJSONRequest(t, h, http.MethodPost, "/v1/intents", publishIntentPayload)
	require.Equal(t, http.StatusCreated, okResp.Code)

	invalidIntentSignature := mustSetRecoveryIDHex(validIntentSignature, 0xff)
	invalidIntentPayload := mustJSON(t, map[string]any{
		"pool_id":          "pool-eth-sig-invalid",
		"intent_id":        "intent-eth-sig-invalid",
		"sender":           alice.address,
		"recipient":        bob.address,
		"expires_unix":     securityNow.Unix() + 300,
		"digest_algorithm": DigestAlgorithmSHA256,
		"intent_sign_hash": hashChar('7'),
		"context_hash":     hashChar('6'),
		"signature": map[string]any{
			"signer":    alice.address,
			"algorithm": SignatureAlgorithmSecp256k1,
			"signature": invalidIntentSignature,
		},
	})

	badResp := performJSONRequest(t, h, http.MethodPost, "/v1/intents", invalidIntentPayload)
	require.Equal(t, http.StatusBadRequest, badResp.Code)
	assertErrorEnvelope(t, badResp.Body.Bytes(), errorCodeInvalidSignature, "signature.signature")
}

func TestInternalGossipAuthEnforcement(t *testing.T) {
	t.Parallel()

	h, err := NewHandler(Config{
		NowFn:              func() time.Time { return securityNow },
		GossipSharedSecret: "gossip-secret",
	})
	require.NoError(t, err)

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	intentHash := hashChar('c')
	payload := mustJSON(t, map[string]any{
		"pool_id":          "pool-gossip-auth",
		"intent_id":        "intent-gossip-auth",
		"sender":           alice.address,
		"recipient":        bob.address,
		"expires_unix":     securityNow.Unix() + 300,
		"digest_algorithm": DigestAlgorithmSHA256,
		"intent_sign_hash": intentHash,
		"context_hash":     hashChar('d'),
		"signature": map[string]any{
			"signer":    alice.address,
			"algorithm": SignatureAlgorithmSecp256k1,
			"signature": mustSignHashHex(alice.priv, intentHash),
		},
	})

	reqNoSecret := httptest.NewRequest(http.MethodPost, "/v1/internal/gossip/intents", bytes.NewReader(payload))
	reqNoSecret.Header.Set("Content-Type", "application/json")
	noSecretResp := httptest.NewRecorder()
	h.ServeHTTP(noSecretResp, reqNoSecret)
	require.Equal(t, http.StatusUnauthorized, noSecretResp.Code)
	assertErrorEnvelope(t, noSecretResp.Body.Bytes(), errorCodeUnauthorized, headerGossipSecret)

	reqWrongSecret := httptest.NewRequest(http.MethodPost, "/v1/internal/gossip/intents", bytes.NewReader(payload))
	reqWrongSecret.Header.Set("Content-Type", "application/json")
	reqWrongSecret.Header.Set(headerGossipSecret, "wrong-secret")
	wrongSecretResp := httptest.NewRecorder()
	h.ServeHTTP(wrongSecretResp, reqWrongSecret)
	require.Equal(t, http.StatusUnauthorized, wrongSecretResp.Code)
	assertErrorEnvelope(t, wrongSecretResp.Body.Bytes(), errorCodeUnauthorized, headerGossipSecret)

	reqOK := httptest.NewRequest(http.MethodPost, "/v1/internal/gossip/intents", bytes.NewReader(payload))
	reqOK.Header.Set("Content-Type", "application/json")
	reqOK.Header.Set(headerGossipSecret, "gossip-secret")
	okResp := httptest.NewRecorder()
	h.ServeHTTP(okResp, reqOK)
	require.Equal(t, http.StatusCreated, okResp.Code)
}

var securityNow = time.Unix(1_700_000_000, 0)

func newSecurityTestHandler(t *testing.T, logger *slog.Logger) http.Handler {
	t.Helper()

	h, err := NewHandler(Config{
		NowFn:  func() time.Time { return securityNow },
		Logger: logger,
	})
	require.NoError(t, err)
	return h
}

func performJSONRequest(t *testing.T, h http.Handler, method, path string, payload []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body io.Reader
	if len(payload) > 0 {
		body = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, path, body)
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()

	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func mustDecodeJSON(t *testing.T, data []byte, v any) {
	t.Helper()
	require.NoError(t, json.Unmarshal(data, v))
}

func assertErrorEnvelope(t *testing.T, raw []byte, code, field string) {
	t.Helper()

	var env errorEnvelope
	mustDecodeJSON(t, raw, &env)
	require.Equal(t, code, env.Error.Code)
	require.Equal(t, field, env.Error.Field)
}

func hashChar(ch byte) string {
	return strings.Repeat(string(ch), 64)
}

type ethereumIdentity struct {
	priv    *ecdsa.PrivateKey
	address string
}

func mustNewEthereumIdentity() ethereumIdentity {
	key, err := crypto.GenerateKey()
	if err != nil {
		panic(err)
	}
	return ethereumIdentity{
		priv:    key,
		address: crypto.PubkeyToAddress(key.PublicKey).Hex(),
	}
}

func mustSignHashHex(priv *ecdsa.PrivateKey, hashHex string) string {
	hash, err := decodeHexBytes(hashHex)
	if err != nil {
		panic(err)
	}
	sig, err := crypto.Sign(hash, priv)
	if err != nil {
		panic(err)
	}
	return "0x" + strings.ToLower(hex.EncodeToString(sig))
}

func mustSetRecoveryIDHex(signatureHex string, recoveryID byte) string {
	raw, err := decodeHexBytes(signatureHex)
	if err != nil {
		panic(err)
	}
	if len(raw) != crypto.SignatureLength {
		panic("unexpected signature length")
	}
	raw[crypto.RecoveryIDOffset] = recoveryID
	return "0x" + strings.ToLower(hex.EncodeToString(raw))
}
