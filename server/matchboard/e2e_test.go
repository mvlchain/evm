package matchboard

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const (
	testTokenAlice     = "token-alice"
	testTokenBob       = "token-bob"
	testPrincipalAlice = "alice"
	testPrincipalBob   = "bob"
)

type testClock struct {
	now time.Time
}

func (c *testClock) Now() time.Time {
	return c.now
}

func (c *testClock) Advance(d time.Duration) {
	c.now = c.now.Add(d)
}

func TestMatchboardE2EHappyPath(t *testing.T) {
	t.Parallel()

	fixedNow := time.Unix(1_710_000_000, 0).UTC()
	h := newMatchboardTestHandler(t, Config{
		TokenPrincipalMap: map[string]string{
			testTokenAlice: testPrincipalAlice,
			testTokenBob:   testPrincipalBob,
		},
		NowFn: func() time.Time { return fixedNow },
	})

	intentSignHash := testHash("1")
	responseSignHash := testHash("2")
	finalizeSignHash := testHash("3")
	intentContextHash := testHash("a")
	responseContextHash := testHash("b")
	finalizeContextHash := testHash("c")

	intentReq := PublishIntentRequest{
		PoolID:          "pool-1",
		IntentID:        "intent-1",
		Sender:          testPrincipalAlice,
		Recipient:       testPrincipalBob,
		ExpiresUnix:     fixedNow.Unix() + 600,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  intentSignHash,
		ContextHash:     intentContextHash,
		Signature: SignatureMetadata{
			Signer:    testPrincipalAlice,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: "sig-intent",
		},
	}
	intentResp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", testTokenAlice, intentReq)
	require.Equal(t, http.StatusCreated, intentResp.StatusCode)
	intentCreated := decodeJSONResponse[publishIntentResponse](t, intentResp)
	require.Equal(t, intentReq.PoolID, intentCreated.PoolID)
	require.Equal(t, intentReq.IntentID, intentCreated.IntentID)
	require.Equal(t, intentSignHash, intentCreated.IntentSignHash)
	require.Equal(t, fixedNow.Unix(), intentCreated.StoredUnix)

	responseReq := PublishResponseRequest{
		PoolID:           "pool-1",
		IntentID:         "intent-1",
		ResponseID:       "response-1",
		Sender:           testPrincipalBob,
		Recipient:        testPrincipalAlice,
		ExpiresUnix:      fixedNow.Unix() + 600,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		IntentSignHash:   intentSignHash,
		ResponseSignHash: responseSignHash,
		ContextHash:      responseContextHash,
		Signature: SignatureMetadata{
			Signer:    testPrincipalBob,
			Algorithm: SignatureAlgorithmEd25519,
			Signature: "sig-response",
		},
	}
	responseResp := doJSONRequest(t, h, http.MethodPost, "/v1/responses", testTokenBob, responseReq)
	require.Equal(t, http.StatusCreated, responseResp.StatusCode)
	responseCreated := decodeJSONResponse[publishResponseResponse](t, responseResp)
	require.Equal(t, responseReq.PoolID, responseCreated.PoolID)
	require.Equal(t, responseReq.IntentID, responseCreated.IntentID)
	require.Equal(t, responseReq.ResponseID, responseCreated.ResponseID)
	require.Equal(t, responseSignHash, responseCreated.ResponseSignHash)
	require.Equal(t, fixedNow.Unix(), responseCreated.StoredUnix)

	finalizeReq := PublishFinalizeRequest{
		PoolID:           "pool-1",
		IntentID:         "intent-1",
		ResponseID:       "response-1",
		FinalizeID:       "finalize-1",
		Sender:           testPrincipalAlice,
		Recipient:        testPrincipalBob,
		ExpiresUnix:      fixedNow.Unix() + 600,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		IntentSignHash:   intentSignHash,
		ResponseSignHash: responseSignHash,
		FinalizeSignHash: finalizeSignHash,
		ContextHash:      finalizeContextHash,
		InitiatorSignature: SignatureMetadata{
			Signer:    testPrincipalAlice,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: "sig-finalize-initiator",
		},
		ResponderSignature: SignatureMetadata{
			Signer:    testPrincipalBob,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: "sig-finalize-responder",
		},
	}
	finalizeResp := doJSONRequest(t, h, http.MethodPost, "/v1/finalize", testTokenAlice, finalizeReq)
	require.Equal(t, http.StatusCreated, finalizeResp.StatusCode)
	finalizeCreated := decodeJSONResponse[publishFinalizeResponse](t, finalizeResp)
	require.Equal(t, finalizeReq.PoolID, finalizeCreated.PoolID)
	require.Equal(t, finalizeReq.IntentID, finalizeCreated.IntentID)
	require.Equal(t, finalizeReq.ResponseID, finalizeCreated.ResponseID)
	require.Equal(t, finalizeReq.FinalizeID, finalizeCreated.FinalizeID)
	require.Equal(t, finalizeSignHash, finalizeCreated.FinalizeSignHash)
	require.Equal(t, fixedNow.Unix(), finalizeCreated.StoredUnix)

	aliceInboxResp := doJSONRequest(t, h, http.MethodGet, "/v1/inbox", testTokenAlice, nil)
	require.Equal(t, http.StatusOK, aliceInboxResp.StatusCode)
	aliceInbox := decodeJSONResponse[listRecordsResponse](t, aliceInboxResp)
	require.Equal(t, testPrincipalAlice, aliceInbox.Principal)
	require.Len(t, aliceInbox.Records, 1)
	require.EqualValues(t, 1, aliceInbox.Total)
	require.Equal(t, RecordTypeResponse, aliceInbox.Records[0].RecordType)
	require.Equal(t, responseContextHash, aliceInbox.Records[0].ContextHash)
	require.Equal(t, intentSignHash, aliceInbox.Records[0].IntentSignHash)
	require.Equal(t, responseSignHash, aliceInbox.Records[0].ResponseSignHash)

	aliceOutboxResp := doJSONRequest(t, h, http.MethodGet, "/v1/outbox", testTokenAlice, nil)
	require.Equal(t, http.StatusOK, aliceOutboxResp.StatusCode)
	aliceOutbox := decodeJSONResponse[listRecordsResponse](t, aliceOutboxResp)
	require.Equal(t, testPrincipalAlice, aliceOutbox.Principal)
	require.Len(t, aliceOutbox.Records, 2)
	require.EqualValues(t, 2, aliceOutbox.Total)
	require.Equal(t, RecordTypeIntent, aliceOutbox.Records[0].RecordType)
	require.Equal(t, intentContextHash, aliceOutbox.Records[0].ContextHash)
	require.Equal(t, intentSignHash, aliceOutbox.Records[0].IntentSignHash)
	require.Equal(t, RecordTypeFinalize, aliceOutbox.Records[1].RecordType)
	require.Equal(t, finalizeContextHash, aliceOutbox.Records[1].ContextHash)
	require.Equal(t, finalizeSignHash, aliceOutbox.Records[1].FinalizeSignHash)

	bobInboxResp := doJSONRequest(t, h, http.MethodGet, "/v1/inbox", testTokenBob, nil)
	require.Equal(t, http.StatusOK, bobInboxResp.StatusCode)
	bobInbox := decodeJSONResponse[listRecordsResponse](t, bobInboxResp)
	require.Equal(t, testPrincipalBob, bobInbox.Principal)
	require.Len(t, bobInbox.Records, 2)
	require.EqualValues(t, 2, bobInbox.Total)
	require.Equal(t, RecordTypeIntent, bobInbox.Records[0].RecordType)
	require.Equal(t, RecordTypeFinalize, bobInbox.Records[1].RecordType)

	bobOutboxResp := doJSONRequest(t, h, http.MethodGet, "/v1/outbox", testTokenBob, nil)
	require.Equal(t, http.StatusOK, bobOutboxResp.StatusCode)
	bobOutbox := decodeJSONResponse[listRecordsResponse](t, bobOutboxResp)
	require.Equal(t, testPrincipalBob, bobOutbox.Principal)
	require.Len(t, bobOutbox.Records, 1)
	require.EqualValues(t, 1, bobOutbox.Total)
	require.Equal(t, RecordTypeResponse, bobOutbox.Records[0].RecordType)
	require.Equal(t, responseContextHash, bobOutbox.Records[0].ContextHash)
}

func TestMatchboardE2EAuthFailures(t *testing.T) {
	t.Parallel()

	fixedNow := time.Unix(1_710_000_300, 0).UTC()
	h := newMatchboardTestHandler(t, Config{
		TokenPrincipalMap: map[string]string{
			testTokenAlice: testPrincipalAlice,
			testTokenBob:   testPrincipalBob,
		},
		NowFn: func() time.Time { return fixedNow },
	})

	intentReq := PublishIntentRequest{
		PoolID:          "pool-auth",
		IntentID:        "intent-auth",
		Sender:          testPrincipalAlice,
		Recipient:       testPrincipalBob,
		ExpiresUnix:     fixedNow.Unix() + 600,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  testHash("4"),
		ContextHash:     testHash("d"),
		Signature: SignatureMetadata{
			Signer:    testPrincipalAlice,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: "sig-auth",
		},
	}

	missingAuthResp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", "", intentReq)
	require.Equal(t, http.StatusUnauthorized, missingAuthResp.StatusCode)
	missingAuthErr := decodeJSONResponse[errorEnvelope](t, missingAuthResp)
	require.Equal(t, errorCodeUnauthorized, missingAuthErr.Error.Code)

	unknownTokenResp := doJSONRequest(t, h, http.MethodGet, "/v1/inbox", "unknown-token", nil)
	require.Equal(t, http.StatusUnauthorized, unknownTokenResp.StatusCode)
	unknownTokenErr := decodeJSONResponse[errorEnvelope](t, unknownTokenResp)
	require.Equal(t, errorCodeUnauthorized, unknownTokenErr.Error.Code)

	forbiddenInboxResp := doJSONRequest(t, h, http.MethodGet, "/v1/inbox?recipient="+testPrincipalBob, testTokenAlice, nil)
	require.Equal(t, http.StatusForbidden, forbiddenInboxResp.StatusCode)
	forbiddenInboxErr := decodeJSONResponse[errorEnvelope](t, forbiddenInboxResp)
	require.Equal(t, errorCodeForbidden, forbiddenInboxErr.Error.Code)
	require.Equal(t, "recipient", forbiddenInboxErr.Error.Field)
}

func TestMatchboardE2ERateLimit(t *testing.T) {
	t.Parallel()

	clock := &testClock{now: time.Unix(1_710_000_600, 0).UTC()}
	h := newMatchboardTestHandler(t, Config{
		TokenPrincipalMap: map[string]string{
			testTokenAlice: testPrincipalAlice,
			testTokenBob:   testPrincipalBob,
		},
		NowFn:             clock.Now,
		RateLimitRequests: 2,
		RateLimitWindow:   time.Minute,
	})

	first := doJSONRequest(t, h, http.MethodGet, "/v1/inbox", testTokenAlice, nil)
	require.Equal(t, http.StatusOK, first.StatusCode)
	require.NoError(t, first.Body.Close())

	second := doJSONRequest(t, h, http.MethodGet, "/v1/inbox", testTokenAlice, nil)
	require.Equal(t, http.StatusOK, second.StatusCode)
	require.NoError(t, second.Body.Close())

	third := doJSONRequest(t, h, http.MethodGet, "/v1/inbox", testTokenAlice, nil)
	require.Equal(t, http.StatusTooManyRequests, third.StatusCode)
	thirdErr := decodeJSONResponse[errorEnvelope](t, third)
	require.Equal(t, errorCodeRateLimited, thirdErr.Error.Code)
	require.True(t, thirdErr.Error.Retryable)

	bobRequest := doJSONRequest(t, h, http.MethodGet, "/v1/inbox", testTokenBob, nil)
	require.Equal(t, http.StatusOK, bobRequest.StatusCode)
	require.NoError(t, bobRequest.Body.Close())

	clock.Advance(time.Minute + time.Second)
	afterWindow := doJSONRequest(t, h, http.MethodGet, "/v1/inbox", testTokenAlice, nil)
	require.Equal(t, http.StatusOK, afterWindow.StatusCode)
	require.NoError(t, afterWindow.Body.Close())
}

func newMatchboardTestHandler(t *testing.T, cfg Config) http.Handler {
	t.Helper()

	h, err := NewHandler(cfg)
	require.NoError(t, err)
	return h
}

func doJSONRequest(t *testing.T, h http.Handler, method, path, token string, body any) *http.Response {
	t.Helper()

	var bodyReader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		require.NoError(t, err)
		bodyReader = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	recorder := httptest.NewRecorder()
	h.ServeHTTP(recorder, req)
	return recorder.Result()
}

func decodeJSONResponse[T any](t *testing.T, resp *http.Response) T {
	t.Helper()

	defer func() {
		require.NoError(t, resp.Body.Close())
	}()

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var out T
	require.NoError(t, json.Unmarshal(raw, &out), "raw body: %s", string(raw))
	return out
}

func testHash(c string) string {
	return strings.Repeat(c, 64)
}
