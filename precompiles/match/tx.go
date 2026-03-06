package match

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"
	matchtypes "github.com/cosmos/evm/x/match/types"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

const (
	// SubmitMatchCertificateMethod defines the ABI method name for certificate submission.
	SubmitMatchCertificateMethod = "submitMatchCertificate"
)

// SubmitMatchCertificate verifies and stores a submitted certificate in canonical on-chain state.
func (p Precompile) SubmitMatchCertificate(
	ctx sdk.Context,
	contract *vm.Contract,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	req, err := ParseSubmitCertificateArgs(args)
	if err != nil {
		return nil, err
	}

	var certificate matchtypes.MatchCertificate
	if err := certificate.Unmarshal(req.Certificate); err != nil {
		return nil, fmt.Errorf("invalid certificate bytes: %w", err)
	}

	if certificate.Payload == nil || strings.TrimSpace(certificate.Payload.Initiator) == "" {
		return nil, fmt.Errorf("certificate payload initiator is required")
	}
	if !common.IsHexAddress(certificate.Payload.Initiator) {
		return nil, fmt.Errorf("certificate payload initiator must be an ethereum address: %s", certificate.Payload.Initiator)
	}

	msgSender := contract.Caller()
	initiator := common.HexToAddress(certificate.Payload.Initiator)
	if msgSender != initiator {
		return nil, fmt.Errorf(cmn.ErrRequesterIsNotMsgSender, msgSender.String(), initiator.String())
	}

	resp, err := p.matchKeeper.SubmitMatchCertificate(ctx, &matchtypes.MsgSubmitMatchCertificate{
		Submitter:   msgSender.Hex(),
		Certificate: certificate,
	})
	if err != nil {
		return nil, err
	}

	return method.Outputs.Pack(resp.MatchId, resp.ReplayKey, resp.CertificateHash)
}
