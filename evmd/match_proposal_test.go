package evmd

import (
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/evm/server/matchboard"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log/v2"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

func TestMatchProposalHandlerPrepareInjectsOperations(t *testing.T) {
	t.Cleanup(matchboard.ClearABCIProposedOperationsForTest)
	matchboard.ClearABCIProposedOperationsForTest()

	baseTx := []byte{0x01, 0x02}
	prepare := func(_ sdk.Context, _ *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
		return &abci.ResponsePrepareProposal{Txs: [][]byte{baseTx}}, nil
	}
	process := func(_ sdk.Context, _ *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
		return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil
	}

	op := matchboard.ProposedOperation{
		RecordType:     matchboard.RecordTypeIntent,
		PoolID:         "pool-1",
		IntentID:       "intent-1",
		Sender:         "alice",
		Recipient:      "bob",
		CreatedUnix:    1,
		IntentSignHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	op.OperationID = matchboard.BuildOperationIDFromProposedOperation(op)
	matchboard.PublishABCIProposedOperation(op)

	h := newMatchProposalHandler(prepare, process, log.NewNopLogger(), defaultInjectedMatchOpsLimit)
	resp, err := h.PrepareProposalHandler()(sdk.Context{}, &abci.RequestPrepareProposal{MaxTxBytes: 1_000_000, Height: 1})
	require.NoError(t, err)
	require.Len(t, resp.Txs, 3)
	require.Equal(t, baseTx, resp.Txs[0])

	meta, metaMatched, metaErr := matchboard.DecodeABCIInjectedBatchMeta(resp.Txs[1])
	require.True(t, metaMatched)
	require.NoError(t, metaErr)
	require.EqualValues(t, 1, meta.OperationCount)

	_, matched, decErr := matchboard.DecodeABCIInjectedOperation(resp.Txs[2])
	require.True(t, matched)
	require.NoError(t, decErr)

	remaining, _, total := matchboard.SnapshotABCIProposedOperations(10)
	require.EqualValues(t, 1, total)
	require.Len(t, remaining, 1)
}

func TestMatchProposalHandlerProcessRejectsMalformedOrder(t *testing.T) {
	t.Cleanup(matchboard.ClearABCIProposedOperationsForTest)
	matchboard.ClearABCIProposedOperationsForTest()

	processCalled := false
	prepare := func(_ sdk.Context, _ *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
		return &abci.ResponsePrepareProposal{}, nil
	}
	process := func(_ sdk.Context, _ *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
		processCalled = true
		return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil
	}

	op1 := matchboard.ProposedOperation{
		RecordType:     matchboard.RecordTypeIntent,
		PoolID:         "pool-1",
		IntentID:       "intent-1",
		Sender:         "alice",
		Recipient:      "bob",
		CreatedUnix:    1,
		IntentSignHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	op1.OperationID = matchboard.BuildOperationIDFromProposedOperation(op1)
	op2 := op1
	op2.IntentID = "intent-2"
	op2.OperationID = matchboard.BuildOperationIDFromProposedOperation(op2)

	enc1, err := matchboard.EncodeABCIInjectedOperation(op1)
	require.NoError(t, err)
	enc2, err := matchboard.EncodeABCIInjectedOperation(op2)
	require.NoError(t, err)
	encMeta, err := matchboard.EncodeABCIInjectedBatchMeta(matchboard.InjectedBatchMeta{
		CanonicalBatchHash: matchboard.BuildCanonicalBatchHash([]matchboard.ProposedOperation{op1, op2}),
		OperationCount:     2,
	})
	require.NoError(t, err)

	h := newMatchProposalHandler(prepare, process, log.NewNopLogger(), defaultInjectedMatchOpsLimit)
	resp, err := h.ProcessProposalHandler()(sdk.Context{}, &abci.RequestProcessProposal{
		Height: 1,
		Txs: [][]byte{
			encMeta,
			enc2, // reverse order should reject
			enc1,
		},
	})
	require.NoError(t, err)
	require.Equal(t, abci.ResponseProcessProposal_REJECT, resp.Status)
	require.False(t, processCalled)
}

func TestMatchProposalHandlerProcessRejectsBatchHashMismatch(t *testing.T) {
	processCalled := false
	prepare := func(_ sdk.Context, _ *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
		return &abci.ResponsePrepareProposal{}, nil
	}
	process := func(_ sdk.Context, _ *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
		processCalled = true
		return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil
	}

	op := matchboard.ProposedOperation{
		RecordType:     matchboard.RecordTypeIntent,
		PoolID:         "pool-1",
		IntentID:       "intent-1",
		Sender:         "alice",
		Recipient:      "bob",
		CreatedUnix:    1,
		IntentSignHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	op.OperationID = matchboard.BuildOperationIDFromProposedOperation(op)
	encOp, err := matchboard.EncodeABCIInjectedOperation(op)
	require.NoError(t, err)

	encMeta, err := matchboard.EncodeABCIInjectedBatchMeta(matchboard.InjectedBatchMeta{
		CanonicalBatchHash: "deadbeef",
		OperationCount:     1,
	})
	require.NoError(t, err)

	h := newMatchProposalHandler(prepare, process, log.NewNopLogger(), defaultInjectedMatchOpsLimit)
	resp, err := h.ProcessProposalHandler()(sdk.Context{}, &abci.RequestProcessProposal{
		Height: 1,
		Txs:    [][]byte{encMeta, encOp},
	})
	require.NoError(t, err)
	require.Equal(t, abci.ResponseProcessProposal_REJECT, resp.Status)
	require.False(t, processCalled)
}

func TestMatchProposalHandlerProcessRejectsMissingBatchMeta(t *testing.T) {
	processCalled := false
	prepare := func(_ sdk.Context, _ *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
		return &abci.ResponsePrepareProposal{}, nil
	}
	process := func(_ sdk.Context, _ *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
		processCalled = true
		return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil
	}

	op := matchboard.ProposedOperation{
		RecordType:     matchboard.RecordTypeIntent,
		PoolID:         "pool-1",
		IntentID:       "intent-1",
		Sender:         "alice",
		Recipient:      "bob",
		CreatedUnix:    1,
		IntentSignHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
	}
	op.OperationID = matchboard.BuildOperationIDFromProposedOperation(op)
	encOp, err := matchboard.EncodeABCIInjectedOperation(op)
	require.NoError(t, err)

	h := newMatchProposalHandler(prepare, process, log.NewNopLogger(), defaultInjectedMatchOpsLimit)
	resp, err := h.ProcessProposalHandler()(sdk.Context{}, &abci.RequestProcessProposal{
		Height: 1,
		Txs:    [][]byte{encOp},
	})
	require.NoError(t, err)
	require.Equal(t, abci.ResponseProcessProposal_REJECT, resp.Status)
	require.False(t, processCalled)
}

func TestMatchProposalHandlerProcessRejectsInvalidOperationCertificate(t *testing.T) {
	processCalled := false
	prepare := func(_ sdk.Context, _ *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
		return &abci.ResponsePrepareProposal{}, nil
	}
	process := func(_ sdk.Context, _ *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
		processCalled = true
		return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil
	}

	op := matchboard.ProposedOperation{
		RecordType:       matchboard.RecordTypeFinalize,
		PoolID:           "pool-1",
		IntentID:         "intent-1",
		ResponseID:       "response-1",
		FinalizeID:       "finalize-1",
		Sender:           "alice",
		Recipient:        "bob",
		CreatedUnix:      1,
		IntentSignHash:   "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		ResponseSignHash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		FinalizeSignHash: "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
		MatchCertificate: []byte{0x01, 0x02, 0x03},
	}
	op.OperationID = matchboard.BuildOperationIDFromProposedOperation(op)

	encoded, err := matchboard.EncodeABCIInjectedOperation(op)
	require.NoError(t, err)
	encMeta, err := matchboard.EncodeABCIInjectedBatchMeta(matchboard.InjectedBatchMeta{
		CanonicalBatchHash: matchboard.BuildCanonicalBatchHash([]matchboard.ProposedOperation{op}),
		OperationCount:     1,
	})
	require.NoError(t, err)

	h := newMatchProposalHandler(prepare, process, log.NewNopLogger(), defaultInjectedMatchOpsLimit)
	resp, err := h.ProcessProposalHandler()(sdk.Context{}, &abci.RequestProcessProposal{
		Height: 1,
		Txs:    [][]byte{encMeta, encoded},
	})
	require.NoError(t, err)
	require.Equal(t, abci.ResponseProcessProposal_REJECT, resp.Status)
	require.False(t, processCalled)
}

func TestMatchProposalHandlerProcessRejectsSubmitMsgWithoutCertificate(t *testing.T) {
	processCalled := false
	prepare := func(_ sdk.Context, _ *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
		return &abci.ResponsePrepareProposal{}, nil
	}
	process := func(_ sdk.Context, _ *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
		processCalled = true
		return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_ACCEPT}, nil
	}

	op := matchboard.ProposedOperation{
		RecordType:            matchboard.RecordTypeIntent,
		PoolID:                "pool-1",
		IntentID:              "intent-1",
		Sender:                "alice",
		Recipient:             "bob",
		CreatedUnix:           1,
		IntentSignHash:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		MatchSubmitMsgPayload: []byte{0x01, 0x02, 0x03},
	}
	op.OperationID = matchboard.BuildOperationIDFromProposedOperation(op)

	encoded, err := matchboard.EncodeABCIInjectedOperation(op)
	require.NoError(t, err)
	encMeta, err := matchboard.EncodeABCIInjectedBatchMeta(matchboard.InjectedBatchMeta{
		CanonicalBatchHash: matchboard.BuildCanonicalBatchHash([]matchboard.ProposedOperation{op}),
		OperationCount:     1,
	})
	require.NoError(t, err)

	h := newMatchProposalHandler(prepare, process, log.NewNopLogger(), defaultInjectedMatchOpsLimit)
	resp, err := h.ProcessProposalHandler()(sdk.Context{}, &abci.RequestProcessProposal{
		Height: 1,
		Txs:    [][]byte{encMeta, encoded},
	})
	require.NoError(t, err)
	require.Equal(t, abci.ResponseProcessProposal_REJECT, resp.Status)
	require.False(t, processCalled)
}
