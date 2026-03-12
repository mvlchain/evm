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
		NowFn: nowFn(now),
	})

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	intentHash := testHash("1")
	intentReq := PublishIntentRequest{
		PoolID:          "pool-matcher",
		IntentID:        "intent-1",
		Sender:          alice.address,
		Recipient:       bob.address,
		ExpiresUnix:     now.Unix() + 300,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  intentHash,
		ContextHash:     testHash("a"),
		Signature: SignatureMetadata{
			Signer:    alice.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(alice.priv, intentHash),
		},
	}
	resp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", intentReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	r1Hash := testHash("2")
	r2Hash := testHash("3")

	response1 := PublishResponseRequest{
		PoolID:           "pool-matcher",
		IntentID:         "intent-1",
		ResponseID:       "response-1",
		Sender:           bob.address,
		Recipient:        alice.address,
		ExpiresUnix:      now.Unix() + 300,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		ResponseType:     "ACCEPT",
		IntentSignHash:   intentHash,
		ResponseSignHash: r1Hash,
		ContextHash:      testHash("b"),
		Signature: SignatureMetadata{
			Signer:    bob.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(bob.priv, r1Hash),
		},
	}
	response2 := response1
	response2.ResponseID = "response-2"
	response2.ResponseSignHash = r2Hash
	response2.Signature.Signature = mustSignHashHex(bob.priv, r2Hash)

	resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", response1)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NoError(t, resp.Body.Close())
	resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", response2)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	list := doJSONRequest(t, h, http.MethodGet, "/v1/matcher/candidates?address="+alice.address, nil)
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

	now := time.Unix(1_710_001_260, 0).UTC()
	h := newMatchboardTestHandler(t, Config{
		NowFn:             nowFn(now),
		MatcherShardCount: 8,
	})

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()
	charlie := mustNewEthereumIdentity()

	bobHash := testHash("2")
	charlieHash := testHash("3")

	for i := 0; i < 20; i++ {
		intentID := "intent-shard-" + strings.Repeat("x", i%3) + strconv.Itoa(i)
		intentHash := testHash("1")
		intentReq := PublishIntentRequest{
			PoolID:          "pool-sharded",
			IntentID:        intentID,
			Sender:          alice.address,
			Recipient:       bob.address,
			ExpiresUnix:     now.Unix() + 600,
			DigestAlgorithm: DigestAlgorithmSHA256,
			IntentSignHash:  intentHash,
			ContextHash:     testHash("a"),
			Signature: SignatureMetadata{
				Signer:    alice.address,
				Algorithm: SignatureAlgorithmSecp256k1,
				Signature: mustSignHashHex(alice.priv, intentHash),
			},
		}
		resp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", intentReq)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()

		responseBob := PublishResponseRequest{
			PoolID:           "pool-sharded",
			IntentID:         intentID,
			ResponseID:       "r-bob-" + strconv.Itoa(i),
			Sender:           bob.address,
			Recipient:        alice.address,
			ExpiresUnix:      now.Unix() + 600,
			DigestAlgorithm:  DigestAlgorithmSHA256,
			ResponseType:     "ACCEPT",
			IntentSignHash:   intentHash,
			ResponseSignHash: bobHash,
			ContextHash:      testHash("b"),
			Signature: SignatureMetadata{
				Signer:    bob.address,
				Algorithm: SignatureAlgorithmSecp256k1,
				Signature: mustSignHashHex(bob.priv, bobHash),
			},
		}
		responseCharlie := PublishResponseRequest{
			PoolID:           "pool-sharded",
			IntentID:         intentID,
			ResponseID:       "r-charlie-" + strconv.Itoa(i),
			Sender:           charlie.address,
			Recipient:        alice.address,
			ExpiresUnix:      now.Unix() + 600,
			DigestAlgorithm:  DigestAlgorithmSHA256,
			ResponseType:     "ACCEPT",
			IntentSignHash:   intentHash,
			ResponseSignHash: charlieHash,
			ContextHash:      testHash("c"),
			Signature: SignatureMetadata{
				Signer:    charlie.address,
				Algorithm: SignatureAlgorithmSecp256k1,
				Signature: mustSignHashHex(charlie.priv, charlieHash),
			},
		}

		resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", responseBob)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()
		resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", responseCharlie)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()
	}

	var baseline listMatchCandidatesResponse
	for i := 0; i < 5; i++ {
		list := doJSONRequest(t, h, http.MethodGet, "/v1/matcher/candidates?limit=100", nil)
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
		NowFn: nowFn(now),
	})

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	publishPair := func(intentID, responseID, intentHash, responseHash string) {
		intentReq := PublishIntentRequest{
			PoolID:          "pool-proposer-matches",
			IntentID:        intentID,
			Sender:          alice.address,
			Recipient:       bob.address,
			ExpiresUnix:     now.Unix() + 600,
			DigestAlgorithm: DigestAlgorithmSHA256,
			IntentSignHash:  intentHash,
			ContextHash:     testHash("a"),
			Signature: SignatureMetadata{
				Signer:    alice.address,
				Algorithm: SignatureAlgorithmSecp256k1,
				Signature: mustSignHashHex(alice.priv, intentHash),
			},
		}
		resp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", intentReq)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()

		responseReq := PublishResponseRequest{
			PoolID:           "pool-proposer-matches",
			IntentID:         intentID,
			ResponseID:       responseID,
			Sender:           bob.address,
			Recipient:        alice.address,
			ExpiresUnix:      now.Unix() + 600,
			DigestAlgorithm:  DigestAlgorithmSHA256,
			ResponseType:     "ACCEPT",
			IntentSignHash:   intentHash,
			ResponseSignHash: responseHash,
			ContextHash:      testHash("b"),
			Signature: SignatureMetadata{
				Signer:    bob.address,
				Algorithm: SignatureAlgorithmSecp256k1,
				Signature: mustSignHashHex(bob.priv, responseHash),
			},
		}
		resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", responseReq)
		require.Equal(t, http.StatusCreated, resp.StatusCode)
		_ = resp.Body.Close()
	}

	publishPair("intent-1", "response-1", testHash("1"), testHash("2"))
	publishPair("intent-2", "response-2", testHash("3"), testHash("4"))

	list := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/matches?limit=10", nil)
	require.Equal(t, http.StatusOK, list.StatusCode)
	matches := decodeJSONResponse[listProposerMatchesResponse](t, list)
	require.EqualValues(t, 2, matches.TotalPending)
	require.Len(t, matches.Matches, 2)
	require.NotEmpty(t, matches.CanonicalMatchBatchHash)

	rollback := doJSONRequest(t, h, http.MethodPost, "/v1/proposer/matches/commit", commitProposerMatchesRequest{
		MatchIDs: []string{matches.Matches[0].MatchID, "missing-match-id"},
	})
	require.Equal(t, http.StatusConflict, rollback.StatusCode)
	rollbackErr := decodeJSONResponse[errorEnvelope](t, rollback)
	require.Equal(t, errorCodeStateConflict, rollbackErr.Error.Code)

	afterRollback := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/matches?limit=10", nil)
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
	commit := doJSONRequest(t, h, http.MethodPost, "/v1/proposer/matches/commit", commitReq)
	require.Equal(t, http.StatusOK, commit.StatusCode)
	commitPayload := decodeJSONResponse[commitProposerMatchesResponse](t, commit)
	require.Equal(t, 2, commitPayload.Committed)
	require.EqualValues(t, 0, commitPayload.Remaining)
	require.Empty(t, commitPayload.CanonicalMatchBatchHash)

	empty := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/matches?limit=10", nil)
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

	h := newMatchboardTestHandler(t, Config{
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
	resp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", intentReq)
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
		ResponseType:     "ACCEPT",
		IntentSignHash:   hex.EncodeToString(cert.Payload.IntentSignHash),
		ResponseSignHash: hex.EncodeToString(cert.Payload.ResponseSignHash),
		ContextHash:      hex.EncodeToString(cert.Payload.ContextHash),
		Signature: SignatureMetadata{
			Signer:    cert.Payload.Responder,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: hex.EncodeToString(cert.Response.Signature.Signature),
		},
	}
	resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", responseReq)
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
	resp = doJSONRequest(t, h, http.MethodPost, "/v1/finalize", finalizeReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	matchesResp := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/matches?limit=5", nil)
	require.Equal(t, http.StatusOK, matchesResp.StatusCode)
	matches := decodeJSONResponse[listProposerMatchesResponse](t, matchesResp)
	require.Len(t, matches.Matches, 1)

	buildResp := doJSONRequest(t, h, http.MethodPost, "/v1/proposer/matches/build", buildProposerMatchesRequest{
		MatchIDs:  []string{matches.Matches[0].MatchID},
		Submitter: cert.Payload.Initiator,
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
		NowFn:              nowFn(now),
		EnableIntentStream: true,
	})
	require.NoError(t, err)

	srv := httptest.NewServer(h)
	defer srv.Close()

	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/stream/intents?intent_type=request", nil)
	require.NoError(t, err)

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

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	streamHash := testHash("5")
	payload := PublishIntentRequest{
		PoolID:          "pool-stream",
		IntentID:        "intent-stream-1",
		Sender:          alice.address,
		Recipient:       bob.address,
		ExpiresUnix:     now.Unix() + 300,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  streamHash,
		ContextHash:     testHash("6"),
		Signature: SignatureMetadata{
			Signer:    alice.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(alice.priv, streamHash),
		},
	}
	body, err := json.Marshal(payload)
	require.NoError(t, err)
	postReq, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/intents", bytes.NewReader(body))
	require.NoError(t, err)
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
		NowFn:              nowFn(now),
		GossipSharedSecret: "gossip-secret",
		GossipSeenTTL:      time.Minute,
	})

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	intentHash := testHash("7")
	intentReq := PublishIntentRequest{
		PoolID:          "pool-gossip-dedup",
		IntentID:        "intent-gossip-dedup",
		Sender:          alice.address,
		Recipient:       bob.address,
		ExpiresUnix:     now.Unix() + 300,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  intentHash,
		ContextHash:     testHash("8"),
		Signature: SignatureMetadata{
			Signer:    alice.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(alice.priv, intentHash),
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

	inbox := doJSONRequest(t, h, http.MethodGet, "/v1/inbox?recipient="+bob.address, nil)
	require.Equal(t, http.StatusOK, inbox.StatusCode)
	decoded := decodeJSONResponse[listRecordsResponse](t, inbox)
	require.EqualValues(t, 1, decoded.Total)
	require.Len(t, decoded.Records, 1)
}

func TestMatchboardGossipRateLimit(t *testing.T) {
	t.Parallel()

	now := time.Unix(1_710_001_900, 0).UTC()
	h := newMatchboardTestHandler(t, Config{
		NowFn:              nowFn(now),
		GossipSharedSecret: "gossip-secret",
		RateLimitRequests:  1,
		RateLimitWindow:    time.Minute,
	})

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	makeGossipReq := func(intentID string, msgID string, hashChar string) *httptest.ResponseRecorder {
		signHash := testHash(hashChar)
		intentReq := PublishIntentRequest{
			PoolID:          "pool-gossip-rate",
			IntentID:        intentID,
			Sender:          alice.address,
			Recipient:       bob.address,
			ExpiresUnix:     now.Unix() + 300,
			DigestAlgorithm: DigestAlgorithmSHA256,
			IntentSignHash:  signHash,
			ContextHash:     testHash("e"),
			Signature: SignatureMetadata{
				Signer:    alice.address,
				Algorithm: SignatureAlgorithmSecp256k1,
				Signature: mustSignHashHex(alice.priv, signHash),
			},
		}
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

	first := makeGossipReq("intent-gossip-rate-1", "rate-msg-1", "a")
	require.Equal(t, http.StatusCreated, first.Code)
	second := makeGossipReq("intent-gossip-rate-2", "rate-msg-2", "b")
	require.Equal(t, http.StatusTooManyRequests, second.Code)
}

func TestMatchboardExpiredArtifactsCleanedFromMatcherAndProposer(t *testing.T) {
	t.Parallel()
	t.Cleanup(ClearABCIProposedOperationsForTest)
	ClearABCIProposedOperationsForTest()

	clock := &testClock{now: time.Unix(1_710_002_100, 0).UTC()}
	h := newMatchboardTestHandler(t, Config{
		NowFn:                 clock.Now,
		EnableABCIProposerOps: true,
	})

	alice := mustNewEthereumIdentity()
	bob := mustNewEthereumIdentity()

	intentHash := testHash("9")
	intentReq := PublishIntentRequest{
		PoolID:          "pool-expired",
		IntentID:        "intent-expired",
		Sender:          alice.address,
		Recipient:       bob.address,
		ExpiresUnix:     clock.now.Unix() + 1,
		DigestAlgorithm: DigestAlgorithmSHA256,
		IntentSignHash:  intentHash,
		ContextHash:     testHash("a"),
		Signature: SignatureMetadata{
			Signer:    alice.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(alice.priv, intentHash),
		},
	}
	responseHash := testHash("b")
	respReq := PublishResponseRequest{
		PoolID:           intentReq.PoolID,
		IntentID:         intentReq.IntentID,
		ResponseID:       "response-expired",
		Sender:           bob.address,
		Recipient:        alice.address,
		ExpiresUnix:      clock.now.Unix() + 1,
		DigestAlgorithm:  DigestAlgorithmSHA256,
		ResponseType:     "ACCEPT",
		IntentSignHash:   intentReq.IntentSignHash,
		ResponseSignHash: responseHash,
		ContextHash:      testHash("c"),
		Signature: SignatureMetadata{
			Signer:    bob.address,
			Algorithm: SignatureAlgorithmSecp256k1,
			Signature: mustSignHashHex(bob.priv, responseHash),
		},
	}

	resp := doJSONRequest(t, h, http.MethodPost, "/v1/intents", intentReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()
	resp = doJSONRequest(t, h, http.MethodPost, "/v1/responses", respReq)
	require.Equal(t, http.StatusCreated, resp.StatusCode)
	_ = resp.Body.Close()

	clock.Advance(2 * time.Second)

	matchers := doJSONRequest(t, h, http.MethodGet, "/v1/matcher/candidates", nil)
	require.Equal(t, http.StatusOK, matchers.StatusCode)
	matcherPayload := decodeJSONResponse[listMatchCandidatesResponse](t, matchers)
	require.EqualValues(t, 0, matcherPayload.Total)
	require.Len(t, matcherPayload.Candidates, 0)

	ops := doJSONRequest(t, h, http.MethodGet, "/v1/proposer/operations", nil)
	require.Equal(t, http.StatusOK, ops.StatusCode)
	opPayload := decodeJSONResponse[listProposedOperationsResponse](t, ops)
	require.EqualValues(t, 0, opPayload.TotalPending)
	require.Len(t, opPayload.Operations, 0)
}
