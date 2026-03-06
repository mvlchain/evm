package evmd

import (
	"bytes"
	"fmt"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/evm/server/matchboard"
	matchtypes "github.com/cosmos/evm/x/match/types"

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

		ops, _, _ := matchboard.SnapshotABCIProposedOperations(h.maxOps)
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
		injectedOps := make([]matchboard.ProposedOperation, 0, len(ops))
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
			injectedOps = append(injectedOps, op)
		}

		if injectedCount > 0 {
			canonicalHash := matchboard.BuildCanonicalBatchHash(injectedOps)
			buildHash := matchboard.BuildCanonicalMatchBuildHash(injectedOps)
			metaBz, metaErr := matchboard.EncodeABCIInjectedBatchMeta(matchboard.InjectedBatchMeta{
				CanonicalBatchHash: canonicalHash,
				OperationCount:     uint32(len(injectedOps)),
			})
			if metaErr != nil {
				h.logger.Error("failed to encode injected match batch metadata", "error", metaErr)
				return baseResp, nil
			}

			if maxBytes > 0 && usedBytes+int64(len(metaBz)) > maxBytes {
				h.logger.Warn(
					"skipping injected match operations due to batch metadata max bytes limit",
					"height", req.Height,
					"injected_ops", injectedCount,
					"canonical_batch_hash", canonicalHash,
					"canonical_match_build_hash", buildHash,
				)
				return baseResp, nil
			}

			withMeta := make([][]byte, 0, len(txs)+1)
			withMeta = append(withMeta, txs[:len(baseResp.Txs)]...)
			withMeta = append(withMeta, metaBz)
			withMeta = append(withMeta, txs[len(baseResp.Txs):]...)
			txs = withMeta

			h.logger.Info(
				"prepared proposal with injected match operations",
				"height", req.Height,
				"injected_ops", injectedCount,
				"canonical_batch_hash", canonicalHash,
				"canonical_match_build_hash", buildHash,
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
			metaSeen        bool
			injectedCount   int
			meta            matchboard.InjectedBatchMeta
			decodedOps      []matchboard.ProposedOperation
		)

		for _, txBz := range req.Txs {
			batchMeta, batchMatched, batchErr := matchboard.DecodeABCIInjectedBatchMeta(txBz)
			if batchMatched {
				injectedStarted = true
				if batchErr != nil {
					incrMatchProposalReject("batch_meta_decode_failed")
					h.logger.Error("rejecting proposal due to malformed injected match batch metadata", "error", batchErr)
					return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
				}
				if metaSeen {
					incrMatchProposalReject("batch_meta_duplicate")
					return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
				}
				metaSeen = true
				meta = batchMeta
				continue
			}

			op, matched, err := matchboard.DecodeABCIInjectedOperation(txBz)
			if !matched {
				if injectedStarted {
					incrMatchProposalReject("non_injected_tx_after_injected_sequence")
					return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
				}
				normalTxs = append(normalTxs, txBz)
				continue
			}

			injectedStarted = true
			if !metaSeen {
				incrMatchProposalReject("missing_batch_meta")
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}
			if err != nil {
				incrMatchProposalReject("operation_decode_failed")
				h.logger.Error("rejecting proposal due to malformed injected match operation", "error", err)
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}

			if op.OperationID == "" {
				incrMatchProposalReject("operation_id_empty")
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}
			expectedID := matchboard.BuildOperationIDFromProposedOperation(op)
			if op.OperationID != expectedID {
				incrMatchProposalReject("operation_id_mismatch")
				h.logger.Error(
					"rejecting proposal due to injected match operation id mismatch",
					"operation_id", op.OperationID,
					"expected_operation_id", expectedID,
				)
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}
			if lastOperationID != "" && op.OperationID <= lastOperationID {
				incrMatchProposalReject("operation_order_violation")
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}
			if len(op.MatchCertificate) > 0 {
				if _, certErr := matchboard.DecodeOperationCertificate(op); certErr != nil {
					incrMatchProposalReject("operation_certificate_invalid")
					h.logger.Error(
						"rejecting proposal due to invalid injected operation certificate",
						"operation_id", op.OperationID,
						"error", certErr,
					)
					return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
				}
			}
			if len(op.MatchSubmitMsgPayload) > 0 {
				msg, msgErr := matchboard.DecodeSubmitMatchCertificateMsgPayload(op.MatchSubmitMsgPayload)
				if msgErr != nil {
					incrMatchProposalReject("operation_submit_msg_invalid")
					h.logger.Error(
						"rejecting proposal due to invalid injected submit match msg payload",
						"operation_id", op.OperationID,
						"error", msgErr,
					)
					return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
				}
				if len(op.MatchCertificate) == 0 {
					incrMatchProposalReject("operation_submit_msg_missing_certificate")
					return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
				}
				certBz, marshalErr := matchtypes.DeterministicProtoMarshal(&msg.Certificate)
				if marshalErr != nil {
					incrMatchProposalReject("operation_submit_msg_certificate_marshal_failed")
					return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
				}
				if !bytes.Equal(certBz, op.MatchCertificate) {
					incrMatchProposalReject("operation_submit_msg_certificate_mismatch")
					return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
				}
			}
			lastOperationID = op.OperationID
			injectedCount++
			decodedOps = append(decodedOps, op)
		}

		if metaSeen {
			if injectedCount == 0 {
				incrMatchProposalReject("batch_meta_without_operations")
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}
			if uint32(injectedCount) != meta.OperationCount {
				incrMatchProposalReject("batch_operation_count_mismatch")
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}
			expectedHash := matchboard.BuildCanonicalBatchHash(decodedOps)
			if meta.CanonicalBatchHash != expectedHash {
				incrMatchProposalReject("batch_hash_mismatch")
				h.logger.Error(
					"rejecting proposal due to injected match result hash mismatch",
					"expected_canonical_batch_hash", expectedHash,
					"provided_canonical_batch_hash", meta.CanonicalBatchHash,
					"injected_ops", injectedCount,
				)
				return &abci.ResponseProcessProposal{Status: abci.ResponseProcessProposal_REJECT}, nil
			}
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
			h.logger.Info(
				"processed proposal with injected match operations",
				"height", req.Height,
				"injected_ops", injectedCount,
				"canonical_match_build_hash", matchboard.BuildCanonicalMatchBuildHash(decodedOps),
			)
		}

		return resp, nil
	}
}

func (h *matchProposalHandler) String() string {
	return fmt.Sprintf("matchProposalHandler(maxOps=%d)", h.maxOps)
}
