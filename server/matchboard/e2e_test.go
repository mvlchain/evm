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
		NowFn: func() time.Time { return fixedNow },
	})

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	intentSignHash := testHash("1")
	responseSignHash := testHash("2")
	finalizeSignHash := testHash("3")
	intentContextHash := testHash("a")
	responseContextHash := testHash("b")
	finalizeContextHash := testHash("c")

	intentReq := PublishIntentRequest{
		PoolID:          "pool-1",
		IntentID:        "intent-1",
		Sender:          alice.address,
		Recipient:       bob.address,
		ExpiresUnix:     fixedNow.Unix() + 600,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  intentSignHash,
		ContextHash:     intentContextHash,
		Signature: SignatureMetadata{
			Signer:    alice.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(alice.priv, intentSignHash),
		},
	}
	intentResp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", intentReq)
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
		Sender:           bob.address,
		Recipient:        alice.address,
		ExpiresUnix:      fixedNow.Unix() + 600,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		ResponseType:     "ACCEPT",
		IntentSignHash:   intentSignHash,
		ResponseSignHash: responseSignHash,
		ContextHash:      responseContextHash,
		Signature: SignatureMetadata{
			Signer:    bob.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(bob.priv, responseSignHash),
		},
	}
	responseResp := doJSONRequest(t, h, http.MethodPost, "/v1/responses", responseReq)
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
		Sender:           alice.address,
		Recipient:        bob.address,
		ExpiresUnix:      fixedNow.Unix() + 600,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		IntentSignHash:   intentSignHash,
		ResponseSignHash: responseSignHash,
		FinalizeSignHash: finalizeSignHash,
		ContextHash:      finalizeContextHash,
		InitiatorSignature: SignatureMetadata{
			Signer:    alice.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(alice.priv, finalizeSignHash),
		},
		ResponderSignature: SignatureMetadata{
			Signer:    bob.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(bob.priv, finalizeSignHash),
		},
	}
	finalizeResp := doJSONRequest(t, h, http.MethodPost, "/v1/finalize", finalizeReq)
	require.Equal(t, http.StatusCreated, finalizeResp.StatusCode)
	finalizeCreated := decodeJSONResponse[publishFinalizeResponse](t, finalizeResp)
	require.Equal(t, finalizeReq.PoolID, finalizeCreated.PoolID)
	require.Equal(t, finalizeReq.IntentID, finalizeCreated.IntentID)
	require.Equal(t, finalizeReq.ResponseID, finalizeCreated.ResponseID)
	require.Equal(t, finalizeReq.FinalizeID, finalizeCreated.FinalizeID)
	require.Equal(t, finalizeSignHash, finalizeCreated.FinalizeSignHash)
	require.Equal(t, fixedNow.Unix(), finalizeCreated.StoredUnix)

	aliceInboxResp := doJSONRequest(t, h, http.MethodGet, "/v1/inbox?recipient="+alice.address, nil)
	require.Equal(t, http.StatusOK, aliceInboxResp.StatusCode)
	aliceInbox := decodeJSONResponse[listRecordsResponse](t, aliceInboxResp)
	require.Equal(t, alice.address, aliceInbox.Principal)
	require.Len(t, aliceInbox.Records, 1)
	require.EqualValues(t, 1, aliceInbox.Total)
	require.Equal(t, RecordTypeResponse, aliceInbox.Records[0].RecordType)
	require.Equal(t, responseContextHash, aliceInbox.Records[0].ContextHash)
	require.Equal(t, intentSignHash, aliceInbox.Records[0].IntentSignHash)
	require.Equal(t, responseSignHash, aliceInbox.Records[0].ResponseSignHash)

	aliceOutboxResp := doJSONRequest(t, h, http.MethodGet, "/v1/outbox?sender="+alice.address, nil)
	require.Equal(t, http.StatusOK, aliceOutboxResp.StatusCode)
	aliceOutbox := decodeJSONResponse[listRecordsResponse](t, aliceOutboxResp)
	require.Equal(t, alice.address, aliceOutbox.Principal)
	require.Len(t, aliceOutbox.Records, 2)
	require.EqualValues(t, 2, aliceOutbox.Total)
	require.Equal(t, RecordTypeIntent, aliceOutbox.Records[0].RecordType)
	require.Equal(t, intentContextHash, aliceOutbox.Records[0].ContextHash)
	require.Equal(t, intentSignHash, aliceOutbox.Records[0].IntentSignHash)
	require.Equal(t, RecordTypeFinalize, aliceOutbox.Records[1].RecordType)
	require.Equal(t, finalizeContextHash, aliceOutbox.Records[1].ContextHash)
	require.Equal(t, finalizeSignHash, aliceOutbox.Records[1].FinalizeSignHash)

	bobInboxResp := doJSONRequest(t, h, http.MethodGet, "/v1/inbox?recipient="+bob.address, nil)
	require.Equal(t, http.StatusOK, bobInboxResp.StatusCode)
	bobInbox := decodeJSONResponse[listRecordsResponse](t, bobInboxResp)
	require.Equal(t, bob.address, bobInbox.Principal)
	require.Len(t, bobInbox.Records, 2)
	require.EqualValues(t, 2, bobInbox.Total)
	require.Equal(t, RecordTypeIntent, bobInbox.Records[0].RecordType)
	require.Equal(t, RecordTypeFinalize, bobInbox.Records[1].RecordType)

	bobOutboxResp := doJSONRequest(t, h, http.MethodGet, "/v1/outbox?sender="+bob.address, nil)
	require.Equal(t, http.StatusOK, bobOutboxResp.StatusCode)
	bobOutbox := decodeJSONResponse[listRecordsResponse](t, bobOutboxResp)
	require.Equal(t, bob.address, bobOutbox.Principal)
	require.Len(t, bobOutbox.Records, 1)
	require.EqualValues(t, 1, bobOutbox.Total)
	require.Equal(t, RecordTypeResponse, bobOutbox.Records[0].RecordType)
	require.Equal(t, responseContextHash, bobOutbox.Records[0].ContextHash)
}

func TestMatchboardE2ERateLimit(t *testing.T) {
	t.Parallel()

	clock := &testClock{now: time.Unix(1_710_000_600, 0).UTC()}
	h := newMatchboardTestHandler(t, Config{
		NowFn:             clock.Now,
		RateLimitRequests: 2,
		RateLimitWindow:   time.Minute,
	})

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	makeIntent := func(id, hash string) PublishIntentRequest {
		h := testHash(hash)
		return PublishIntentRequest{
			PoolID:          "pool-rl",
			IntentID:        id,
			Sender:          alice.address,
			Recipient:       bob.address,
			ExpiresUnix:     clock.now.Unix() + 600,
			DigestAlgorithm: DigestAlgorithmSHA256,
			IntentSignHash:  h,
			ContextHash:     h,
			Signature: SignatureMetadata{
				Signer:    alice.address,
				Algorithm: SignatureAlgorithmSecp256k1,
				Signature: mustSignHashHex(alice.priv, h),
			},
		}
	}

	first := doJSONRequest(t, h, http.MethodPost, "/v1/intents", makeIntent("rl-1", "1"))
	require.Equal(t, http.StatusCreated, first.StatusCode)
	require.NoError(t, first.Body.Close())

	second := doJSONRequest(t, h, http.MethodPost, "/v1/intents", makeIntent("rl-2", "2"))
	require.Equal(t, http.StatusCreated, second.StatusCode)
	require.NoError(t, second.Body.Close())

	third := doJSONRequest(t, h, http.MethodPost, "/v1/intents", makeIntent("rl-3", "3"))
	require.Equal(t, http.StatusTooManyRequests, third.StatusCode)
	thirdErr := decodeJSONResponse[errorEnvelope](t, third)
	require.Equal(t, errorCodeRateLimited, thirdErr.Error.Code)
	require.True(t, thirdErr.Error.Retryable)

	// bob has a separate rate limit bucket
	bh := testHash("b")
	bobIntent := PublishIntentRequest{
		PoolID:          "pool-rl",
		IntentID:        "rl-bob-1",
		Sender:          bob.address,
		Recipient:       alice.address,
		ExpiresUnix:     clock.now.Unix() + 600,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  bh,
		ContextHash:     bh,
		Signature: SignatureMetadata{
			Signer:    bob.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(bob.priv, bh),
		},
	}
	bobReq := doJSONRequest(t, h, http.MethodPost, "/v1/intents", bobIntent)
	require.Equal(t, http.StatusCreated, bobReq.StatusCode)
	require.NoError(t, bobReq.Body.Close())

	clock.Advance(time.Minute + time.Second)
	afterWindow := doJSONRequest(t, h, http.MethodPost, "/v1/intents", makeIntent("rl-4", "4"))
	require.Equal(t, http.StatusCreated, afterWindow.StatusCode)
	require.NoError(t, afterWindow.Body.Close())
}

func TestMatchboardProposerOperationsCanonicalAndAtomicCommit(t *testing.T) {
	t.Parallel()

	fixedNow := time.Unix(1_710_000_900, 0).UTC()
	h := newMatchboardTestHandler(t, Config{
		NowFn: func() time.Time { return fixedNow },
	})

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	intentHash := testHash("9")
	responseHash := testHash("7")
	finalizeHash := testHash("5")

	intentReq := PublishIntentRequest{
		PoolID:          "pool-proposer",
		IntentID:        "intent-proposer",
		Sender:          alice.address,
		Recipient:       bob.address,
		ExpiresUnix:     fixedNow.Unix() + 600,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  intentHash,
		ContextHash:     testHash("8"),
		Signature: SignatureMetadata{
			Signer:    alice.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(alice.priv, intentHash),
		},
	}
	intentResp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", intentReq)
	require.Equal(t, http.StatusCreated, intentResp.StatusCode)
	require.NoError(t, intentResp.Body.Close())

	responseReq := PublishResponseRequest{
		PoolID:           intentReq.PoolID,
		IntentID:         intentReq.IntentID,
		ResponseID:       "response-proposer",
		Sender:           bob.address,
		Recipient:        alice.address,
		ExpiresUnix:      fixedNow.Unix() + 600,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		ResponseType:     "ACCEPT",
		IntentSignHash:   intentReq.IntentSignHash,
		ResponseSignHash: responseHash,
		ContextHash:      testHash("6"),
		Signature: SignatureMetadata{
			Signer:    bob.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(bob.priv, responseHash),
		},
	}
	responseResp := doJSONRequest(t, h, http.MethodPost, "/v1/responses", responseReq)
	require.Equal(t, http.StatusCreated, responseResp.StatusCode)
	require.NoError(t, responseResp.Body.Close())

	finalizeReq := PublishFinalizeRequest{
		PoolID:           intentReq.PoolID,
		IntentID:         intentReq.IntentID,
		ResponseID:       responseReq.ResponseID,
		FinalizeID:       "finalize-proposer",
		Sender:           alice.address,
		Recipient:        bob.address,
		ExpiresUnix:      fixedNow.Unix() + 600,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		IntentSignHash:   intentReq.IntentSignHash,
		ResponseSignHash: responseReq.ResponseSignHash,
		FinalizeSignHash: finalizeHash,
		ContextHash:      testHash("4"),
		InitiatorSignature: SignatureMetadata{
			Signer:    alice.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(alice.priv, finalizeHash),
		},
		ResponderSignature: SignatureMetadata{
			Signer:    bob.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(bob.priv, finalizeHash),
		},
	}
	finalizeResp := doJSONRequest(t, h, http.MethodPost, "/v1/finalize", finalizeReq)
	require.Equal(t, http.StatusCreated, finalizeResp.StatusCode)
	require.NoError(t, finalizeResp.Body.Close())

	listResp := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/operations?limit=10", nil)
	require.Equal(t, http.StatusOK, listResp.StatusCode)
	listed := decodeJSONResponse[listProposedOperationsResponse](t, listResp)
	require.Len(t, listed.Operations, 3)
	require.EqualValues(t, 3, listed.TotalPending)
	require.NotEmpty(t, listed.CanonicalBatchHash)

	for i := 1; i < len(listed.Operations); i++ {
		require.Less(t, listed.Operations[i-1].OperationID, listed.Operations[i].OperationID)
	}

	opIDs := make([]string, len(listed.Operations))
	for i := range listed.Operations {
		opIDs[i] = listed.Operations[i].OperationID
	}

	rollbackResp := doJSONRequest(
		t,
		h,
		http.MethodPost,
		"/v1/proposer/operations/commit",
		commitProposedOperationsRequest{
			OperationIDs: []string{opIDs[0], strings.Repeat("f", 64)},
		},
	)
	require.Equal(t, http.StatusConflict, rollbackResp.StatusCode)
	rollbackErr := decodeJSONResponse[errorEnvelope](t, rollbackResp)
	require.Equal(t, errorCodeStateConflict, rollbackErr.Error.Code)

	listAfterRollbackResp := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/operations?limit=10", nil)
	require.Equal(t, http.StatusOK, listAfterRollbackResp.StatusCode)
	listAfterRollback := decodeJSONResponse[listProposedOperationsResponse](t, listAfterRollbackResp)
	require.Len(t, listAfterRollback.Operations, 3)
	require.EqualValues(t, 3, listAfterRollback.TotalPending)
	require.Equal(t, listed.CanonicalBatchHash, listAfterRollback.CanonicalBatchHash)

	commitResp := doJSONRequest(
		t,
		h,
		http.MethodPost,
		"/v1/proposer/operations/commit",
		commitProposedOperationsRequest{OperationIDs: opIDs},
	)
	require.Equal(t, http.StatusOK, commitResp.StatusCode)
	commitResult := decodeJSONResponse[commitProposedOperationsResponse](t, commitResp)
	require.Equal(t, len(opIDs), commitResult.Committed)
	require.EqualValues(t, 0, commitResult.Remaining)
	require.Empty(t, commitResult.CanonicalBatchHash)

	emptyResp := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/operations?limit=10", nil)
	require.Equal(t, http.StatusOK, emptyResp.StatusCode)
	emptyListed := decodeJSONResponse[listProposedOperationsResponse](t, emptyResp)
	require.EqualValues(t, 0, emptyListed.TotalPending)
	require.Len(t, emptyListed.Operations, 0)
	require.Empty(t, emptyListed.CanonicalBatchHash)
}

func TestMatchboardSignedPayloadGossipReplication(t *testing.T) {
	t.Parallel()

	fixedNow := time.Unix(1_710_001_200, 0).UTC()
	const gossipSecret = "gossip-secret"

	nodeB := newMatchboardTestHandler(t, Config{
		NowFn:              func() time.Time { return fixedNow },
		GossipSharedSecret: gossipSecret,
		GossipNodeID:       "node-b",
	})
	nodeBServer := httptest.NewServer(nodeB)
	defer nodeBServer.Close()

	nodeA := newMatchboardTestHandler(t, Config{
		NowFn:              func() time.Time { return fixedNow },
		GossipPeers:        []string{nodeBServer.URL},
		GossipSharedSecret: gossipSecret,
		GossipNodeID:       "node-a",
	})

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	intentHash := testHash("a")
	intentReq := PublishIntentRequest{
		PoolID:          "pool-gossip",
		IntentID:        "intent-gossip",
		Sender:          alice.address,
		Recipient:       bob.address,
		ExpiresUnix:     fixedNow.Unix() + 600,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  intentHash,
		ContextHash:     testHash("b"),
		Signature: SignatureMetadata{
			Signer:    alice.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(alice.priv, intentHash),
		},
	}
	publishResp := doJSONRequest(t, nodeA, http.MethodPost, "/v1/intents", intentReq)
	require.Equal(t, http.StatusCreated, publishResp.StatusCode)
	require.NoError(t, publishResp.Body.Close())

	bobInboxResp := doJSONRequest(t, nodeB, http.MethodGet, "/v1/inbox?recipient="+bob.address, nil)
	require.Equal(t, http.StatusOK, bobInboxResp.StatusCode)
	bobInbox := decodeJSONResponse[listRecordsResponse](t, bobInboxResp)
	require.Equal(t, bob.address, bobInbox.Principal)
	require.Len(t, bobInbox.Records, 1)
	require.Equal(t, RecordTypeIntent, bobInbox.Records[0].RecordType)
	require.Equal(t, intentReq.PoolID, bobInbox.Records[0].PoolID)
	require.Equal(t, intentReq.IntentID, bobInbox.Records[0].IntentID)
	require.Equal(t, intentReq.IntentSignHash, bobInbox.Records[0].IntentSignHash)
}

func newMatchboardTestHandler(t *testing.T, cfg Config) http.Handler {
	t.Helper()

	h, err := NewHandler(cfg)
	require.NoError(t, err)
	return h
}

func doJSONRequest(t *testing.T, h http.Handler, method, path string, body any) *http.Response {
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

func nowFn(t time.Time) func() time.Time {
	return func() time.Time { return t }
}
