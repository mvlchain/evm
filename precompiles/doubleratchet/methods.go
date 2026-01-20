package doubleratchet

import (
	"encoding/binary"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	ValidateEnvelopeMethod = "validateEnvelope"
	headerLength           = 73
	headerVersion          = 1
)

func (p Precompile) ValidateEnvelope(method *abi.Method, args []interface{}) ([]byte, error) {
	if len(args) != 4 {
		return nil, fmt.Errorf("invalid number of args: expected 4, got %d", len(args))
	}

	header, ok := args[0].([]byte)
	if !ok {
		return nil, fmt.Errorf("invalid header type")
	}
	ciphertext, ok := args[1].([]byte)
	if !ok {
		return nil, fmt.Errorf("invalid ciphertext type")
	}
	maxHeaderBytes, ok := args[2].(uint32)
	if !ok {
		return nil, fmt.Errorf("invalid maxHeaderBytes type")
	}
	maxCiphertextBytes, ok := args[3].(uint32)
	if !ok {
		return nil, fmt.Errorf("invalid maxCiphertextBytes type")
	}

	if len(header) != headerLength {
		return nil, fmt.Errorf("invalid header length")
	}
	if maxHeaderBytes > 0 && len(header) > int(maxHeaderBytes) {
		return nil, fmt.Errorf("header too large")
	}
	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("empty ciphertext")
	}
	if maxCiphertextBytes > 0 && len(ciphertext) > int(maxCiphertextBytes) {
		return nil, fmt.Errorf("ciphertext too large")
	}

	version := header[0]
	if version != headerVersion {
		return nil, fmt.Errorf("unsupported header version")
	}

	dhPub := common.BytesToHash(header[1:33])
	pn := binary.BigEndian.Uint32(header[33:37])
	n := binary.BigEndian.Uint32(header[37:41])
	adHash := common.BytesToHash(header[41:73])

	payload := make([]byte, 0, len(header)+len(ciphertext))
	payload = append(payload, header...)
	payload = append(payload, ciphertext...)
	envelopeHash := crypto.Keccak256Hash(payload)

	return method.Outputs.Pack(true, envelopeHash, version, dhPub, pn, n, adHash)
}
