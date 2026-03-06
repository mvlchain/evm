package matchboard

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	matchtypes "github.com/cosmos/evm/x/match/types"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"
)

func TestMatchboardMatcherCandidatesDeterministic(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_710_001_200, 0).UTC()
	h := newMatchboardTestHandler(t, Config{
		TokenPrincipalMap: map[string]string{
			testTokenAlice: testPrincipalAlice,
			testTokenBob:   testPrincipalBob,
		},
		NowFn: nowFn(now),
	})

	intentReq := PublishIntentRequest{
		PoolID:          "pool-matcher",
		IntentID:        "intent-1",
		Sender:          testPrincipalAlice,
		Recipient:       testPrincipalBob,
		ExpiresUnix:     now.Unix() + 300,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  testHash("1"),
		ContextHash:     testHash("a"),
		Signature: SignatureMetadata{
			Signer:    testPrincipalAlice,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: "sig-intent",
		},
	}
	resp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", testTokenAlice, intentReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	response1 := PublishResponseRequest{
		PoolID:           "pool-matcher",
		IntentID:         "intent-1",
		ResponseID:       "response-1",
		Sender:           testPrincipalBob,
		Recipient:        testPrincipalAlice,
		ExpiresUnix:      now.Unix() + 300,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		IntentSignHash:   testHash("1"),
		ResponseSignHash: testHash("2"),
		ContextHash:      testHash("b"),
		Signature: SignatureMetadata{
			Signer:    testPrincipalBob,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: "sig-r1",
		},
	}
	response2 := response1
	response2.ResponseID = "response-2"
	response2.ResponseSignHash = testHash("3")
	response2.Signature.Signature = "sig-r2"

	resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", testTokenBob, response1)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", testTokenBob, response2)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	list := doJSONRequest(t, h, http.MethodGet, "/v1/matcher/candidates", testTokenAlice, nil)
	require.Equal(t, http.StatusOK, list.StatusCode)
	payload := decodeJSONResponse[listMatchCandidatesResponse](t, list)
	require.EqualValues(t, 1, payload.Total)
	require.Len(t, payload.Candidates, 1)

	reqRec := intentRecord{
		poolID:         intentReq.PoolID,
		intentID:       intentReq.IntentID,
		sender:         intentReq.Sender,
		recipient:      intentReq.Recipient,
		intentSignHash: intentReq.IntentSignHash,
	}
	r1Rec := responseRecord{responseID: response1.ResponseID, sender: response1.Sender, responseSignHash: response1.ResponseSignHash}
	r2Rec := responseRecord{responseID: response2.ResponseID, sender: response2.Sender, responseSignHash: response2.ResponseSignHash}
	score1 := buildMatchScoreHash(reqRec, r1Rec)
	score2 := buildMatchScoreHash(reqRec, r2Rec)

	expectedResponse := response1.ResponseID
	if score2 < score1 {
		expectedResponse = response2.ResponseID
	}
	require.Equal(t, expectedResponse, payload.Candidates[0].ResponseID)
}

func TestMatchboardMatcherCandidatesDeterministicAcrossShards(t *testing.T) {
	t.Parallel()

	const (
		tokenCharlie = "token-charlie"
		charlie      = "charlie"
	)

	now := time.Unix(1_710_001_260, 0).UTC()
	h := newMatchboardTestHandler(t, Config{
		TokenPrincipalMap: map[string]string{
			testTokenAlice: testPrincipalAlice,
			testTokenBob:   testPrincipalBob,
			tokenCharlie:   charlie,
		},
		NowFn:             nowFn(now),
		MatcherShardCount: 8,
	})

	for i := 0; i < 20; i++ {
		intentID := "intent-shard-" + strings.Repeat("x", i%3) + strconv.Itoa(i)
		intentReq := PublishIntentRequest{
			PoolID:          "pool-sharded",
			IntentID:        intentID,
			Sender:          testPrincipalAlice,
			Recipient:       testPrincipalBob,
			ExpiresUnix:     now.Unix() + 600,
			DigestAlgorithm: DigestAlgorithmSHA256,
			IntentSignHash:  testHash("1"),
			ContextHash:     testHash("a"),
			Signature: SignatureMetadata{
				Signer:    testPrincipalAlice,
				Algorithm: SignatureAlgorithmSecp256k1,
				Signature: "sig-intent",
			},
		}
		resp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", testTokenAlice, intentReq)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()

		responseBob := PublishResponseRequest{
			PoolID:           "pool-sharded",
			IntentID:         intentID,
			ResponseID:       "r-bob-" + strconv.Itoa(i),
			Sender:           testPrincipalBob,
			Recipient:        testPrincipalAlice,
			ExpiresUnix:      now.Unix() + 600,
			DigestAlgorithm:  DigestAlgorithmSHA256,
			IntentSignHash:   testHash("1"),
			ResponseSignHash: testHash("2"),
			ContextHash:      testHash("b"),
			Signature: SignatureMetadata{
				Signer:    testPrincipalBob,
				Algorithm: SignatureAlgorithmSecp256k1,
				Signature: "sig-bob",
			},
		}
		responseCharlie := responseBob
		responseCharlie.ResponseID = "r-charlie-" + strconv.Itoa(i)
		responseCharlie.Sender = charlie
		responseCharlie.ResponseSignHash = testHash("3")
		responseCharlie.Signature.Signer = charlie
		responseCharlie.Signature.Signature = "sig-charlie"

		resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", testTokenBob, responseBob)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()
		resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", tokenCharlie, responseCharlie)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()
	}

	var baseline listMatchCandidatesResponse
	for i := 0; i < 5; i++ {
		list := doJSONRequest(t, h, http.MethodGet, "/v1/matcher/candidates?limit=100", testTokenAlice, nil)
		require.Equal(t, http.StatusOK, list.StatusCode)
		got := decodeJSONResponse[listMatchCandidatesResponse](t, list)
		require.EqualValues(t, 20, got.Total)
		require.Len(t, got.Candidates, 20)
		if i == 0 {
			baseline = got
			continue
		}
		require.Equal(t, baseline, got)
	}
}

func TestMatchboardProposerMatchesCanonicalAndAtomicCommit(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_710_001_360, 0).UTC()
	h := newMatchboardTestHandler(t, Config{
		TokenPrincipalMap: map[string]string{
			testTokenAlice: testPrincipalAlice,
			testTokenBob:   testPrincipalBob,
		},
		NowFn: nowFn(now),
	})

	publishPair := func(intentID, responseID, intentHash, responseHash string) {
		intentReq := PublishIntentRequest{
			PoolID:          "pool-proposer-matches",
			IntentID:        intentID,
			Sender:          testPrincipalAlice,
			Recipient:       testPrincipalBob,
			ExpiresUnix:     now.Unix() + 600,
			DigestAlgorithm: DigestAlgorithmSHA256,
			IntentSignHash:  intentHash,
			ContextHash:     testHash("a"),
			Signature: SignatureMetadata{
				Signer:    testPrincipalAlice,
				Algorithm: SignatureAlgorithmSecp256k1,
				Signature: "sig-intent",
			},
		}
		resp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", testTokenAlice, intentReq)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()

		responseReq := PublishResponseRequest{
			PoolID:           "pool-proposer-matches",
			IntentID:         intentID,
			ResponseID:       responseID,
			Sender:           testPrincipalBob,
			Recipient:        testPrincipalAlice,
			ExpiresUnix:      now.Unix() + 600,
			DigestAlgorithm:  DigestAlgorithmSHA256,
			IntentSignHash:   intentHash,
			ResponseSignHash: responseHash,
			ContextHash:      testHash("b"),
			Signature: SignatureMetadata{
				Signer:    testPrincipalBob,
				Algorithm: SignatureAlgorithmSecp256k1,
				Signature: "sig-response",
			},
		}
		resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", testTokenBob, responseReq)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()
	}

	publishPair("intent-1", "response-1", testHash("1"), testHash("2"))
	publishPair("intent-2", "response-2", testHash("3"), testHash("4"))

	list := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/matches?limit=10", testTokenAlice, nil)
	require.Equal(t, http.StatusOK, list.StatusCode)
	matches := decodeJSONResponse[listProposerMatchesResponse](t, list)
	require.EqualValues(t, 2, matches.TotalPending)
	require.Len(t, matches.Matches, 2)
	require.NotEmpty(t, matches.CanonicalMatchBatchHash)

	rollback := doJSONRequest(t, h, http.MethodPost, "/v1/proposer/matches/commit", testTokenAlice, commitProposerMatchesRequest{
		MatchIDs: []string{matches.Matches[0].MatchID, "missing-match-id"},
	})
	require.Equal(t, http.StatusConflict, rollback.StatusCode)
	rollbackErr := decodeJSONResponse[errorEnvelope](t, rollback)
	require.Equal(t, errorCodeStateConflict, rollbackErr.Error.Code)

	afterRollback := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/matches?limit=10", testTokenAlice, nil)
	require.Equal(t, http.StatusOK, afterRollback.StatusCode)
	afterRollbackPayload := decodeJSONResponse[listProposerMatchesResponse](t, afterRollback)
	require.EqualValues(t, 2, afterRollbackPayload.TotalPending)
	require.Len(t, afterRollbackPayload.Matches, 2)
	require.Equal(t, matches.CanonicalMatchBatchHash, afterRollbackPayload.CanonicalMatchBatchHash)

	commitReq := commitProposerMatchesRequest{
		MatchIDs: []string{
			matches.Matches[0].MatchID,
			matches.Matches[1].MatchID,
		},
	}
	commit := doJSONRequest(t, h, http.MethodPost, "/v1/proposer/matches/commit", testTokenAlice, commitReq)
	require.Equal(t, http.StatusOK, commit.StatusCode)
	commitPayload := decodeJSONResponse[commitProposerMatchesResponse](t, commit)
	require.Equal(t, 2, commitPayload.Committed)
	require.EqualValues(t, 0, commitPayload.Remaining)
	require.Empty(t, commitPayload.CanonicalMatchBatchHash)

	empty := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/matches?limit=10", testTokenAlice, nil)
	require.Equal(t, http.StatusOK, empty.StatusCode)
	emptyPayload := decodeJSONResponse[listProposerMatchesResponse](t, empty)
	require.EqualValues(t, 0, emptyPayload.TotalPending)
	require.Len(t, emptyPayload.Matches, 0)
}

func TestMatchboardProposerMatchesBuildGeneratesDeterministicMsgPayload(t *testing.T) {
	t.Parallel()

	nowUnix := int64(1_710_001_420)
	cert := validMatchCertificateForTest(t, nowUnix, nowUnix+600)
	certBytes, err := matchtypes.DeterministicProtoMarshal(&cert)
	require.NoError(t, err)

	tokenInitiator := "token-initiator"
	tokenResponder := "token-responder"
	h := newMatchboardTestHandler(t, Config{
		TokenPrincipalMap: map[string]string{
			tokenInitiator: cert.Payload.Initiator,
			tokenResponder: cert.Payload.Responder,
		},
		NowFn: nowFn(time.Unix(nowUnix, 0).UTC()),
	})

	intentReq := PublishIntentRequest{
		PoolID:          cert.Payload.PoolId,
		IntentID:        cert.Payload.IntentId,
		Sender:          cert.Payload.Initiator,
		Recipient:       cert.Payload.Responder,
		ExpiresUnix:     cert.Intent.Payload.ExpiresUnix,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  hex.EncodeToString(cert.Payload.IntentSignHash),
		ContextHash:     hex.EncodeToString(cert.Payload.ContextHash),
		Signature: SignatureMetadata{
			Signer:    cert.Payload.Initiator,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: hex.EncodeToString(cert.Intent.Signature.Signature),
		},
	}
	resp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", tokenInitiator, intentReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	responseReq := PublishResponseRequest{
		PoolID:           cert.Payload.PoolId,
		IntentID:         cert.Payload.IntentId,
		ResponseID:       cert.Payload.ResponseId,
		Sender:           cert.Payload.Responder,
		Recipient:        cert.Payload.Initiator,
		ExpiresUnix:      cert.Response.Payload.ExpiresUnix,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		IntentSignHash:   hex.EncodeToString(cert.Payload.IntentSignHash),
		ResponseSignHash: hex.EncodeToString(cert.Payload.ResponseSignHash),
		ContextHash:      hex.EncodeToString(cert.Payload.ContextHash),
		Signature: SignatureMetadata{
			Signer:    cert.Payload.Responder,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: hex.EncodeToString(cert.Response.Signature.Signature),
		},
	}
	resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", tokenResponder, responseReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	finalizeReq := PublishFinalizeRequest{
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
	resp = doJSONRequest(t, h, http.MethodPost, "/v1/finalize", tokenInitiator, finalizeReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	matchesResp := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/matches?limit=5", tokenInitiator, nil)
	require.Equal(t, http.StatusOK, matchesResp.StatusCode)
	matches := decodeJSONResponse[listProposerMatchesResponse](t, matchesResp)
	require.Len(t, matches.Matches, 1)

	buildResp := doJSONRequest(t, h, http.MethodPost, "/v1/proposer/matches/build", tokenInitiator, buildProposerMatchesRequest{
		MatchIDs: []string{matches.Matches[0].MatchID},
	})
	require.Equal(t, http.StatusOK, buildResp.StatusCode)
	build := decodeJSONResponse[buildProposerMatchesResponse](t, buildResp)
	require.Equal(t, cert.Payload.Initiator, build.Submitter)
	require.True(t, build.RequireCertificate)
	require.Len(t, build.Items, 1)
	require.True(t, build.Items[0].HasMatchCertificate)
	require.NotEmpty(t, build.Items[0].MsgSubmitMatchTxPayload)
	require.NotEmpty(t, build.Items[0].MsgPayloadHash)
	require.NotEmpty(t, build.CanonicalBuildHash)

	var msg matchtypes.MsgSubmitMatchCertificate
	require.NoError(t, proto.Unmarshal(build.Items[0].MsgSubmitMatchTxPayload, &msg))
	require.Equal(t, cert.Payload.Initiator, msg.Submitter)
	require.Equal(t, cert.Payload.PoolId, msg.Certificate.Payload.PoolId)
	require.Equal(t, cert.Payload.IntentId, msg.Certificate.Payload.IntentId)
	require.Equal(t, cert.Payload.ResponseId, msg.Certificate.Payload.ResponseId)
	require.Equal(t, cert.Payload.FinalizeId, msg.Certificate.Payload.FinalizeId)
}

func TestMatchboardIntentStreamSSE(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_710_001_500, 0).UTC()
	h, err := NewHandler(Config{
		TokenPrincipalMap: map[string]string{
			testTokenAlice: testPrincipalAlice,
			testTokenBob:   testPrincipalBob,
		},
		NowFn:              nowFn(now),
		EnableIntentStream: true,
	})
	require.NoError(t, err)

	srv := httptest.NewServer(h)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/stream/intents?intent_type=request", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+testTokenBob)

	streamResp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer streamResp.Body.Close()
	require.Equal(t, http.StatusOK, streamResp.StatusCode)

	eventCh := make(chan intentStreamEvent, 1)
	errCh := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(streamResp.Body)
		for {
			line, readErr := reader.ReadString('\n')
			if readErr != nil {
				errCh <- readErr
				return
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			var event intentStreamEvent
			if unmarshalErr := json.Unmarshal([]byte(strings.TrimSpace(strings.TrimPrefix(line, "data: "))), &event); unmarshalErr != nil {
				errCh <- unmarshalErr
				return
			}
			eventCh <- event
			return
		}
	}()

	payload := PublishIntentRequest{
		PoolID:          "pool-stream",
		IntentID:        "intent-stream-1",
		Sender:          testPrincipalAlice,
		Recipient:       testPrincipalBob,
		ExpiresUnix:     now.Unix() + 300,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  testHash("5"),
		ContextHash:     testHash("6"),
		Signature: SignatureMetadata{
			Signer:    testPrincipalAlice,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: "sig-stream",
		},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	postReq, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/intents", bytes.NewReader(body))
	require.NoError(t, err)
	postReq.Header.Set("Authorization", "Bearer "+testTokenAlice)
	postReq.Header.Set("Content-Type", "application/json")
	postResp, err := http.DefaultClient.Do(postReq)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, postResp.StatusCode)
	_, _ = io.Copy(io.Discard, postResp.Body)
	_ = postResp.Body.Close()

	select {
	case event := <-eventCh:
		require.Equal(t, IntentTypeRequest, event.IntentType)
		require.Equal(t, payload.IntentID, event.IntentID)
		require.Equal(t, payload.Sender, event.Requester)
		require.Equal(t, payload.Recipient, event.Responder)
	case readErr := <-errCh:
		require.NoError(t, readErr)
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for stream event")
	}
}

func TestMatchboardGossipEnvelopeDedup(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_710_001_800, 0).UTC()
	h := newMatchboardTestHandler(t, Config{
		TokenPrincipalMap: map[string]string{
			testTokenAlice: testPrincipalAlice,
			testTokenBob:   testPrincipalBob,
		},
		NowFn:              nowFn(now),
		GossipSharedSecret: "gossip-secret",
		GossipSeenTTL:      time.Minute,
	})

	intentReq := PublishIntentRequest{
		PoolID:          "pool-gossip-dedup",
		IntentID:        "intent-gossip-dedup",
		Sender:          testPrincipalAlice,
		Recipient:       testPrincipalBob,
		ExpiresUnix:     now.Unix() + 300,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  testHash("7"),
		ContextHash:     testHash("8"),
		Signature: SignatureMetadata{
			Signer:    testPrincipalAlice,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: "sig-gossip",
		},
	}
	rawPayload, err := json.Marshal(intentReq)
	require.NoError(t, err)

	envelope := GossipEnvelope{
		MessageID:   "msg-1",
		IntentType:  IntentTypeRequest,
		Requester:   intentReq.Sender,
		Responder:   intentReq.Recipient,
		ExpiryUnix:  now.Unix() + 60,
		CreatedUnix: now.Unix(),
		Hops:        0,
		Payload:     rawPayload,
	}
	rawEnvelope, err := json.Marshal(envelope)
	require.NoError(t, err)

	post := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/internal/gossip/intents", bytes.NewReader(rawEnvelope))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(headerGossipSecret, "gossip-secret")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	first := post()
	require.Equal(t, http.StatusCreated, first.Code)
	second := post()
	require.Equal(t, http.StatusOK, second.Code)

	inbox := doJSONRequest(t, h, http.MethodGet, "/v1/inbox", testTokenBob, nil)
	require.Equal(t, http.StatusOK, inbox.StatusCode)
	decoded := decodeJSONResponse[listRecordsResponse](t, inbox)
	require.EqualValues(t, 1, decoded.Total)
	require.Len(t, decoded.Records, 1)
}

func TestMatchboardGossipRateLimit(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_710_001_900, 0).UTC()
	h := newMatchboardTestHandler(t, Config{
		TokenPrincipalMap: map[string]string{
			testTokenAlice: testPrincipalAlice,
			testTokenBob:   testPrincipalBob,
		},
		NowFn:              nowFn(now),
		GossipSharedSecret: "gossip-secret",
		RateLimitRequests:  1,
		RateLimitWindow:    time.Minute,
	})

	intentReq := PublishIntentRequest{
		PoolID:          "pool-gossip-rate",
		IntentID:        "intent-gossip-rate-1",
		Sender:          testPrincipalAlice,
		Recipient:       testPrincipalBob,
		ExpiresUnix:     now.Unix() + 300,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  testHash("d"),
		ContextHash:     testHash("e"),
		Signature: SignatureMetadata{
			Signer:    testPrincipalAlice,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: "sig-gossip-rate",
		},
	}
	makeReq := func(intentID string, msgID string, hashChar string) *httptest.ResponseRecorder {
		intentReq.IntentID = intentID
		intentReq.IntentSignHash = testHash(hashChar)
		payload, marshalErr := json.Marshal(intentReq)
		require.NoError(t, marshalErr)
		envelope := GossipEnvelope{
			MessageID:   msgID,
			IntentType:  IntentTypeRequest,
			Requester:   intentReq.Sender,
			Responder:   intentReq.Recipient,
			ExpiryUnix:  now.Unix() + 60,
			CreatedUnix: now.Unix(),
			Hops:        0,
			Payload:     payload,
		}
		wire, marshalErr := json.Marshal(envelope)
		require.NoError(t, marshalErr)

		req := httptest.NewRequest(http.MethodPost, "/v1/internal/gossip/intents", bytes.NewReader(wire))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(headerGossipSecret, "gossip-secret")
		req.Header.Set(headerGossipOrigin, "node-rate")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	first := makeReq("intent-gossip-rate-1", "rate-msg-1", "a")
	require.Equal(t, http.StatusCreated, first.Code)
	second := makeReq("intent-gossip-rate-2", "rate-msg-2", "b")
	require.Equal(t, http.StatusTooManyRequests, second.Code)
}

func TestMatchboardExpiredArtifactsCleanedFromMatcherAndProposer(t *testing.T) {
	t.Parallel()
	t.Cleanup(ClearABCIProposedOperationsForTest)
	ClearABCIProposedOperationsForTest()

	clock := &testClock{now: time.Unix(1_710_002_100, 0).UTC()}
	h := newMatchboardTestHandler(t, Config{
		TokenPrincipalMap: map[string]string{
			testTokenAlice: testPrincipalAlice,
			testTokenBob:   testPrincipalBob,
		},
		NowFn:                 clock.Now,
		EnableABCIProposerOps: true,
	})

	intentReq := PublishIntentRequest{
		PoolID:          "pool-expired",
		IntentID:        "intent-expired",
		Sender:          testPrincipalAlice,
		Recipient:       testPrincipalBob,
		ExpiresUnix:     clock.now.Unix() + 1,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  testHash("9"),
		ContextHash:     testHash("a"),
		Signature: SignatureMetadata{
			Signer:    testPrincipalAlice,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: "sig-exp-intent",
		},
	}
	respReq := PublishResponseRequest{
		PoolID:           intentReq.PoolID,
		IntentID:         intentReq.IntentID,
		ResponseID:       "response-expired",
		Sender:           testPrincipalBob,
		Recipient:        testPrincipalAlice,
		ExpiresUnix:      clock.now.Unix() + 1,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		IntentSignHash:   intentReq.IntentSignHash,
		ResponseSignHash: testHash("b"),
		ContextHash:      testHash("c"),
		Signature: SignatureMetadata{
			Signer:    testPrincipalBob,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: "sig-exp-resp",
		},
	}

	resp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", testTokenAlice, intentReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()
	resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", testTokenBob, respReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	clock.Advance(2 * time.Second)

	matchers := doJSONRequest(t, h, http.MethodGet, "/v1/matcher/candidates", testTokenAlice, nil)
	require.Equal(t, http.StatusOK, matchers.StatusCode)
	matcherPayload := decodeJSONResponse[listMatchCandidatesResponse](t, matchers)
	require.EqualValues(t, 0, matcherPayload.Total)
	require.Len(t, matcherPayload.Candidates, 0)

	ops := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/operations", testTokenAlice, nil)
	require.Equal(t, http.StatusOK, ops.StatusCode)
	opPayload := decodeJSONResponse[listProposedOperationsResponse](t, ops)
	require.EqualValues(t, 0, opPayload.TotalPending)
	require.Len(t, opPayload.Operations, 0)
}

func nowFn(now time.Time) func() time.Time {
	return func() time.Time { return now }
}
