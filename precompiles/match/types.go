package match

import (
	"fmt"
	"strings"

	cmn "github.com/cosmos/evm/precompiles/common"
)

// ReplayArgs represents method inputs that identify a replay index entry.
type ReplayArgs struct {
	PoolID   string
	IntentID string
}

// SubmitCertificateArgs represents method inputs used to submit a match certificate.
type SubmitCertificateArgs struct {
	Certificate []byte
}

// ParseReplayArgs parses common replay query arguments.
func ParseReplayArgs(args []interface{}) (ReplayArgs, error) {
	if len(args) != 2 {
		return ReplayArgs{}, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 2, len(args))
	}

	poolID, ok := args[0].(string)
	if !ok {
		return ReplayArgs{}, fmt.Errorf(cmn.ErrInvalidType, "poolId", "", args[0])
	}
	if strings.TrimSpace(poolID) == "" {
		return ReplayArgs{}, fmt.Errorf("poolId cannot be empty")
	}

	intentID, ok := args[1].(string)
	if !ok {
		return ReplayArgs{}, fmt.Errorf(cmn.ErrInvalidType, "intentId", "", args[1])
	}
	if strings.TrimSpace(intentID) == "" {
		return ReplayArgs{}, fmt.Errorf("intentId cannot be empty")
	}

	return ReplayArgs{
		PoolID:   poolID,
		IntentID: intentID,
	}, nil
}

// ParseSubmitCertificateArgs parses submitMatchCertificate arguments.
func ParseSubmitCertificateArgs(args []interface{}) (SubmitCertificateArgs, error) {
	if len(args) != 1 {
		return SubmitCertificateArgs{}, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 1, len(args))
	}

	certificate, ok := args[0].([]byte)
	if !ok {
		return SubmitCertificateArgs{}, fmt.Errorf(cmn.ErrInvalidType, "certificate", []byte{}, args[0])
	}
	if len(certificate) == 0 {
		return SubmitCertificateArgs{}, fmt.Errorf("certificate cannot be empty")
	}

	return SubmitCertificateArgs{
		Certificate: certificate,
	}, nil
}
