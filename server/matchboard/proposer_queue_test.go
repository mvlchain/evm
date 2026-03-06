package matchboard

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestABCIProposerQueueSnapshotAndCommit(t *testing.T) {
	t.Cleanup(ClearABCIProposedOperationsForTest)
	ClearABCIProposedOperationsForTest()

	op1 := ProposedOperation{
		RecordType:     RecordTypeIntent,
		PoolID:         "pool-1",
		IntentID:       "intent-1",
		Sender:         "alice",
		Recipient:      "bob",
		CreatedUnix:    100,
		IntentSignHash: testHash("a"),
	}
	op1.OperationID = BuildOperationIDFromProposedOperation(op1)

	op2 := ProposedOperation{
		RecordType:       RecordTypeResponse,
		PoolID:           "pool-1",
		IntentID:         "intent-1",
		ResponseID:       "response-1",
		Sender:           "bob",
		Recipient:        "alice",
		CreatedUnix:      101,
		IntentSignHash:   testHash("a"),
		ResponseSignHash: testHash("b"),
	}
	op2.OperationID = BuildOperationIDFromProposedOperation(op2)

	PublishABCIProposedOperation(op2)
	PublishABCIProposedOperation(op1)

	ops, canonicalHash, total := SnapshotABCIProposedOperations(10)
	require.EqualValues(t, 2, total)
	require.Len(t, ops, 2)
	require.NotEmpty(t, canonicalHash)
	require.Less(t, ops[0].OperationID, ops[1].OperationID)

	committed, remaining, postHash, err := CommitABCIProposedOperations([]string{ops[0].OperationID})
	require.NoError(t, err)
	require.Equal(t, 1, committed)
	require.EqualValues(t, 1, remaining)
	require.NotEmpty(t, postHash)
}

func TestABCIInjectedOperationEncodeDecode(t *testing.T) {
	op := ProposedOperation{
		RecordType:            RecordTypeFinalize,
		PoolID:                "pool-2",
		IntentID:              "intent-2",
		ResponseID:            "response-2",
		FinalizeID:            "finalize-2",
		Sender:                "alice",
		Recipient:             "bob",
		CreatedUnix:           12345,
		IntentSignHash:        testHash("1"),
		ResponseSignHash:      testHash("2"),
		FinalizeSignHash:      testHash("3"),
		MatchCertificate:      []byte{0x01, 0x02, 0x03, 0x04},
		MatchSubmitMsgPayload: []byte{0xaa, 0xbb, 0xcc},
	}
	op.OperationID = BuildOperationIDFromProposedOperation(op)

	encoded, err := EncodeABCIInjectedOperation(op)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, matched, err := DecodeABCIInjectedOperation(encoded)
	require.True(t, matched)
	require.NoError(t, err)
	require.Equal(t, op, decoded)
}

func TestABCIInjectedBatchMetaEncodeDecode(t *testing.T) {
	meta := InjectedBatchMeta{
		CanonicalBatchHash: testHash("z"),
		OperationCount:     3,
	}

	encoded, err := EncodeABCIInjectedBatchMeta(meta)
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, matched, err := DecodeABCIInjectedBatchMeta(encoded)
	require.True(t, matched)
	require.NoError(t, err)
	require.Equal(t, meta, decoded)
}

func TestBuildCanonicalMatchBuildHashDeterministic(t *testing.T) {
	op1 := ProposedOperation{
		RecordType:            RecordTypeFinalize,
		PoolID:                "pool-1",
		IntentID:              "intent-1",
		ResponseID:            "response-1",
		FinalizeID:            "finalize-1",
		Sender:                "alice",
		Recipient:             "bob",
		IntentSignHash:        testHash("1"),
		ResponseSignHash:      testHash("2"),
		FinalizeSignHash:      testHash("3"),
		MatchCertificate:      []byte{0x01, 0x02},
		MatchSubmitMsgPayload: []byte{0xaa, 0xbb},
	}
	op1.OperationID = BuildOperationIDFromProposedOperation(op1)

	op2 := ProposedOperation{
		RecordType:       RecordTypeIntent,
		PoolID:           "pool-1",
		IntentID:         "intent-2",
		Sender:           "alice",
		Recipient:        "carol",
		IntentSignHash:   testHash("4"),
		CreatedUnix:      123,
		MatchCertificate: nil,
	}
	op2.OperationID = BuildOperationIDFromProposedOperation(op2)

	opsA := []ProposedOperation{op1, op2}
	opsB := []ProposedOperation{op1, op2}
	hashA := BuildCanonicalMatchBuildHash(opsA)
	hashB := BuildCanonicalMatchBuildHash(opsB)
	require.Equal(t, hashA, hashB)

	op2Changed := op2
	op2Changed.MatchSubmitMsgPayload = []byte{0x55}
	hashChanged := BuildCanonicalMatchBuildHash([]ProposedOperation{op1, op2Changed})
	require.NotEqual(t, hashA, hashChanged)
}
