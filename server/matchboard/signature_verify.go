package matchboard

import (
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func validateEthereumSignature(field, signHashHex, expectedSigner string, sig SignatureMetadata) error {
	if !strings.EqualFold(strings.TrimSpace(sig.Algorithm), SignatureAlgorithmSecp256k1) {
		return &validationError{
			code:    errorCodeInvalidSignature,
			field:   field + ".algorithm",
			message: "signer requires secp256k1 signature",
		}
	}

	sigBytes, err := decodeHexBytes(sig.Signature)
	if err != nil {
		return &validationError{
			code:    errorCodeInvalidSignature,
			field:   field + ".signature",
			message: "signature must be hex encoded",
			detail:  err.Error(),
		}
	}

	sigBytes, err = normalizeRecoverySignature(sigBytes)
	if err != nil {
		return &validationError{
			code:    errorCodeInvalidSignature,
			field:   field + ".signature",
			message: "invalid secp256k1 signature format",
			detail:  err.Error(),
		}
	}

	hashBytes, err := hex.DecodeString(signHashHex)
	if err != nil {
		return &validationError{
			code:    errorCodeInvalidRequest,
			field:   field + ".sign_hash",
			message: "invalid sign hash",
			detail:  err.Error(),
		}
	}

	pub, err := crypto.SigToPub(hashBytes, sigBytes)
	if err != nil {
		return &validationError{
			code:    errorCodeInvalidSignature,
			field:   field + ".signature",
			message: "failed to recover ethereum signer",
			detail:  err.Error(),
		}
	}

	recovered := crypto.PubkeyToAddress(*pub)
	expected := common.HexToAddress(expectedSigner)
	if recovered != expected {
		return &validationError{
			code:    errorCodeSignerMismatch,
			field:   field + ".signer",
			message: fmt.Sprintf("signature signer mismatch: expected %s got %s", expected.Hex(), recovered.Hex()),
		}
	}

	return nil
}

func decodeHexBytes(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty value")
	}
	trimmed = strings.TrimPrefix(trimmed, "0x")
	trimmed = strings.TrimPrefix(trimmed, "0X")
	return hex.DecodeString(trimmed)
}

func normalizeRecoverySignature(signature []byte) ([]byte, error) {
	if len(signature) != crypto.SignatureLength {
		return nil, fmt.Errorf("signature must be %d bytes, got %d", crypto.SignatureLength, len(signature))
	}

	out := make([]byte, len(signature))
	copy(out, signature)

	switch out[crypto.RecoveryIDOffset] {
	case 0, 1:
		return out, nil
	case 27, 28:
		out[crypto.RecoveryIDOffset] -= 27
		return out, nil
	default:
		return nil, fmt.Errorf("recovery id must be 0/1 or 27/28, got %d", out[crypto.RecoveryIDOffset])
	}
}
