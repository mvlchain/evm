package evmd

import (
	"fmt"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/evm/server/matchboard"

	"cosmossdk.io/log/v2"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const defaultInjectedMatchOpsLimit = 128

type matchProposalHandler struct {
	defaultPrepare sdk.PrepareProposalHandler
	defaultProcess sdk.ProcessProposalHandler

	logger log.Logger
	maxOps int
}

func newMatchProposalHandler(
	defaultPrepare sdk.PrepareProposalHandler,
	defaultProcess sdk.ProcessProposalHandler,
	logger log.Logger,
	maxOps int,
) *matchProposalHandler {
	if maxOps <= 0 {
		maxOps = defaultInjectedMatchOpsLimit
	}
	return &matchProposalHandler{
		defaultPrepare: defaultPrepare,
		defaultProcess: defaultProcess,
		logger:         logger,
		maxOps:         maxOps,
	}
}

func (h *matchProposalHandler) PrepareProposalHandler() sdk.PrepareProposalHandler {
	return func(ctx sdk.Context, req *abci.RequestPrepareProposal) (*abci.ResponsePrepareProposal, error) {
		baseResp, err := h.defaultPrepare(ctx, req)
		if err != nil {
			return nil, err
		}

		ops, canonicalHash, _ := matchboard.SnapshotABCIProposedOperations(h.maxOps)
		if len(ops) == 0 {
			return baseResp, nil
		}

		txs := make([][]byte, 0, len(baseResp.Txs)+len(ops))
		txs = append(txs, baseResp.Txs...)

		var usedBytes int64
		for _, tx := range txs {
			usedBytes += int64(len(tx))
		}

		maxBytes := req.MaxTxBytes
		injectedCount := 0
		for _, op := range ops {
			encoded, encErr := matchboard.EncodeABCIInjectedOperation(op)
			if encErr != nil {
				h.logger.Error("failed to encode injected match operation", "operation_id", op.OperationID, "error", encErr)
				continue
			}
			if maxBytes > 0 && usedBytes+int64(len(encoded)) > maxBytes {
				break
			}

			txs = append(txs, encoded)
			usedBytes += int64(len(encoded))
			injectedCount++
		}

		if injectedCount > 0 {
			h.logger.Info(
				"prepared proposal with injected match operations",
				"height", req.Height,
				"injected_ops", injectedCount,
				"canonical_batch_hash", canonicalHash,
			)
		}

		return &abci.ResponsePrepareProposal{Txs: txs}, nil
	}
}

func (h *matchProposalHandler) ProcessProposalHandler() sdk.ProcessProposalHandler {
	return func(ctx sdk.Context, req *abci.RequestProcessProposal) (*abci.ResponseProcessProposal, error) {
		normalTxs := make([][]byte, 0, len(req.Txs))
		var (
			lastOperationID string
			injectedStarted bool
			injectedCount   int
		)

		for _, txBz := range req.Txs {
			op, matched, err := matchboard.DecodeABCIInjectedOperation(txBz)
			if !matched {
				if injectedStarted {
					return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
				}
				normalTxs = append(normalTxs, txBz)
				continue
			}

			injectedStarted = true
			if err != nil {
				h.logger.Error("rejecting proposal due to malformed injected match operation", "error", err)
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}

			if op.OperationID == "" {
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}
			expectedID := matchboard.BuildOperationIDFromProposedOperation(op)
			if op.OperationID != expectedID {
				h.logger.Error(
					"rejecting proposal due to injected match operation id mismatch",
					"operation_id", op.OperationID,
					"expected_operation_id", expectedID,
				)
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}
			if lastOperationID != "" && op.OperationID <= lastOperationID {
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}
			if len(op.MatchCertificate) > 0 {
				if _, certErr := matchboard.DecodeOperationCertificate(op); certErr != nil {
					h.logger.Error(
						"rejecting proposal due to invalid injected operation certificate",
						"operation_id", op.OperationID,
						"error", certErr,
					)
					return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
				}
			}
			lastOperationID = op.OperationID
			injectedCount++
		}

		normalReq := *req
		normalReq.Txs = normalTxs
		resp, err := h.defaultProcess(ctx, &normalReq)
		if err != nil {
			return nil, err
		}
		if resp.Status != abci.ResponseProcessProposal_ACCEPT {
			return resp, nil
		}

		if injectedCount > 0 {
			h.logger.Info("processed proposal with injected match operations", "height", req.Height, "injected_ops", injectedCount)
		}

		return resp, nil
	}
}

func (h *matchProposalHandler) String() string {
	return fmt.Sprintf("matchProposalHandler(maxOps=%d)", h.maxOps)
}
