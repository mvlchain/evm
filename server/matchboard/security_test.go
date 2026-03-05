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

func TestInboxOutboxAuthorizationEnforcement(t *testing.T) {
	t.Parallel()

	h := newSecurityTestHandler(t, nil)

	publishIntentPayload := mustJSON(t, map[string]any{
		"pool_id":          "pool-authz",
		"intent_id":        "intent-authz",
		"sender":           "alice",
		"recipient":        "bob",
		"expires_unix":     securityNow.Unix() + 300,
		"digest_algorithm": DigestAlgorithmSHA256,
		"intent_sign_hash": hashChar('a'),
		"context_hash":     hashChar('b'),
		"signature": map[string]any{
			"signer":    "alice",
			"algorithm": SignatureAlgorithmSecp256k1,
			"signature": "sig-alice",
		},
	})

	publishResp := performJSONRequest(t, h, http.MethodPost, "/v1/intents", tokenAlice, publishIntentPayload)
	require.Equal(t, http.StatusCreated, publishResp.Code)

	allowedInbox := performJSONRequest(t, h, http.MethodGet, "/v1/inbox", tokenBob, nil)
	require.Equal(t, http.StatusOK, allowedInbox.Code)
	var inbox listRecordsResponse
	mustDecodeJSON(t, allowedInbox.Body.Bytes(), &inbox)
	require.Equal(t, "bob", inbox.Principal)
	require.EqualValues(t, 1, inbox.Total)
	require.Len(t, inbox.Records, 1)
	require.Equal(t, "alice", inbox.Records[0].Sender)
	require.Equal(t, "bob", inbox.Records[0].Recipient)

	allowedOutbox := performJSONRequest(t, h, http.MethodGet, "/v1/outbox", tokenAlice, nil)
	require.Equal(t, http.StatusOK, allowedOutbox.Code)
	var outbox listRecordsResponse
	mustDecodeJSON(t, allowedOutbox.Body.Bytes(), &outbox)
	require.Equal(t, "alice", outbox.Principal)
	require.EqualValues(t, 1, outbox.Total)
	require.Len(t, outbox.Records, 1)
	require.Equal(t, "alice", outbox.Records[0].Sender)
	require.Equal(t, "bob", outbox.Records[0].Recipient)

	forbiddenInbox := performJSONRequest(t, h, http.MethodGet, "/v1/inbox?recipient=alice", tokenBob, nil)
	require.Equal(t, http.StatusForbidden, forbiddenInbox.Code)
	assertErrorEnvelope(t, forbiddenInbox.Body.Bytes(), errorCodeForbidden, "recipient")

	forbiddenOutbox := performJSONRequest(t, h, http.MethodGet, "/v1/outbox?sender=bob", tokenAlice, nil)
	require.Equal(t, http.StatusForbidden, forbiddenOutbox.Code)
	assertErrorEnvelope(t, forbiddenOutbox.Body.Bytes(), errorCodeForbidden, "sender")
}

func TestSensitivePlaintextNotLeakedInLogs(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	h := newSecurityTestHandler(t, logger)

	const secretPlaintext = "very-sensitive-plaintext-should-never-appear-in-logs"
	publishIntentPayload := mustJSON(t, map[string]any{
		"pool_id":          "pool-logs",
		"intent_id":        "intent-logs",
		"sender":           "alice",
		"recipient":        "bob",
		"expires_unix":     securityNow.Unix() + 300,
		"digest_algorithm": DigestAlgorithmSHA256,
		"intent_sign_hash": hashChar('c'),
		"context_hash":     hashChar('d'),
		"signature": map[string]any{
			"signer":     "alice",
			"algorithm":  SignatureAlgorithmSecp256k1,
			"public_key": secretPlaintext,
			"signature":  "sig-alice",
		},
	})

	resp := performJSONRequest(t, h, http.MethodPost, "/v1/intents", tokenAlice, publishIntentPayload)
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
		token   string
		payload map[string]any
	}{
		{
			name:  "intent unknown field rejected",
			path:  "/v1/intents",
			token: tokenAlice,
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
			name:  "response unknown field rejected",
			path:  "/v1/responses",
			token: tokenBob,
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
			name:  "finalize unknown field rejected",
			path:  "/v1/finalize",
			token: tokenAlice,
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
			resp := performJSONRequest(t, h, http.MethodPost, tc.path, tc.token, mustJSON(t, tc.payload))
			require.Equal(t, http.StatusBadRequest, resp.Code)
			assertErrorEnvelope(t, resp.Body.Bytes(), errorCodeInvalidRequest, "body")
			require.Contains(t, resp.Body.String(), "unknown field")
		})
	}

	outbox := performJSONRequest(t, h, http.MethodGet, "/v1/outbox", tokenAlice, nil)
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
		TokenPrincipalMap: map[string]string{
			tokenAlice: alice.address,
			tokenBob:   bob.address,
		},
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

	okResp := performJSONRequest(t, h, http.MethodPost, "/v1/intents", tokenAlice, publishIntentPayload)
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

	badResp := performJSONRequest(t, h, http.MethodPost, "/v1/intents", tokenAlice, invalidIntentPayload)
	require.Equal(t, http.StatusBadRequest, badResp.Code)
	assertErrorEnvelope(t, badResp.Body.Bytes(), errorCodeInvalidSignature, "signature.signature")
}

var securityNow = time.Unix(1_700_000_000, 0)

const (
	tokenAlice = "token-alice"
	tokenBob   = "token-bob"
)

func newSecurityTestHandler(t *testing.T, logger *slog.Logger) http.Handler {
	t.Helper()

	h, err := NewHandler(Config{
		TokenPrincipalMap: map[string]string{
			tokenAlice: "alice",
			tokenBob:   "bob",
		},
		NowFn:  func() time.Time { return securityNow },
		Logger: logger,
	})
	require.NoError(t, err)
	return h
}

func performJSONRequest(t *testing.T, h http.Handler, method, path, token string, payload []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body io.Reader
	if len(payload) > 0 {
		body = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, path, body)
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+token)
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
