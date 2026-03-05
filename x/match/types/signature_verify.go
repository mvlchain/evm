package types

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"fmt"

	errorsmod "cosmossdk.io/errors"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

func identitiesEqual(a, b string) bool {
	if common.IsHexAddress(a) && common.IsHexAddress(b) {
		return common.HexToAddress(a) == common.HexToAddress(b)
	}
	return a == b
}

func (c MatchCertificate) validateDeterministicSignatures() error {
	intentHash, err := IntentSignDocHash(c.Intent.Payload)
	if err != nil {
		return errorsmod.Wrapf(ErrInvalidRequest, "failed to build intent sign-doc hash: %v", err)
	}
	if !bytes.Equal(c.Intent.SignBytesHash, intentHash) {
		return errorsmod.Wrap(ErrHashMismatch, "intent.sign_bytes_hash does not match deterministic intent sign-doc hash")
	}

	responseHash, err := ResponseSignDocHash(c.Response.Payload)
	if err != nil {
		return errorsmod.Wrapf(ErrInvalidRequest, "failed to build response sign-doc hash: %v", err)
	}
	if !bytes.Equal(c.Response.SignBytesHash, responseHash) {
		return errorsmod.Wrap(ErrHashMismatch, "response.sign_bytes_hash does not match deterministic response sign-doc hash")
	}

	finalizeHash, err := FinalizeSignDocHash(c.Finalize.Payload)
	if err != nil {
		return errorsmod.Wrapf(ErrInvalidRequest, "failed to build finalize sign-doc hash: %v", err)
	}
	if !bytes.Equal(c.Finalize.SignBytesHash, finalizeHash) {
		return errorsmod.Wrap(ErrHashMismatch, "finalize.sign_bytes_hash does not match deterministic finalize sign-doc hash")
	}

	certificateHash, err := CertificateSignDocHash(c.Payload)
	if err != nil {
		return errorsmod.Wrapf(ErrInvalidRequest, "failed to build certificate sign-doc hash: %v", err)
	}
	if !bytes.Equal(c.SignBytesHash, certificateHash) {
		return errorsmod.Wrap(ErrHashMismatch, "certificate.sign_bytes_hash does not match deterministic certificate sign-doc hash")
	}

	if err := verifySignatureOverHash("intent.signature", c.Intent.Signature, c.Intent.Payload.Initiator, intentHash); err != nil {
		return err
	}
	if err := verifySignatureOverHash("response.signature", c.Response.Signature, c.Response.Payload.Responder, responseHash); err != nil {
		return err
	}
	if err := verifySignatureOverHash("finalize.initiator_signature", c.Finalize.InitiatorSignature, c.Finalize.Payload.Initiator, finalizeHash); err != nil {
		return err
	}
	if err := verifySignatureOverHash("finalize.responder_signature", c.Finalize.ResponderSignature, c.Finalize.Payload.Responder, finalizeHash); err != nil {
		return err
	}
	// Board signature does not need role binding to submitter; submitter is tx fee payer/relayer identity.
	if err := verifySignatureOverHash("board_signature", c.BoardSignature, c.BoardSignature.Signer, certificateHash); err != nil {
		return err
	}

	return nil
}

func verifySignatureOverHash(field string, sig *Signature, expectedSigner string, hash []byte) error {
	if sig == nil {
		return errorsmod.Wrapf(ErrInvalidRequest, "%s is required", field)
	}
	if len(hash) != sha256.Size {
		return errorsmod.Wrapf(ErrInvalidRequest, "%s hash must be %d bytes, got %d", field, sha256.Size, len(hash))
	}

	switch sig.Algorithm {
	case SignatureAlgorithmSecp256k1:
		return verifySecp256k1EthereumSignature(field, sig, expectedSigner, hash)
	case SignatureAlgorithmEd25519:
		return verifyEd25519Signature(field, sig, hash)
	default:
		return errorsmod.Wrapf(ErrInvalidRequest, "%s.algorithm is unsupported: %d", field, sig.Algorithm)
	}
}

func verifySecp256k1EthereumSignature(field string, sig *Signature, expectedSigner string, hash []byte) error {
	if !common.IsHexAddress(expectedSigner) {
		return errorsmod.Wrapf(ErrInvalidSignature, "%s signer must be an ethereum address, got %q", field, expectedSigner)
	}

	normalizedSig, err := normalizeRecoverySignature(sig.Signature)
	if err != nil {
		return errorsmod.Wrapf(ErrInvalidSignature, "%s %v", field, err)
	}

	pub, err := crypto.SigToPub(hash, normalizedSig)
	if err != nil {
		return errorsmod.Wrapf(ErrInvalidSignature, "%s failed to recover signer: %v", field, err)
	}

	recovered := crypto.PubkeyToAddress(*pub)
	expected := common.HexToAddress(expectedSigner)
	if recovered != expected {
		return errorsmod.Wrapf(ErrSignerMismatch, "%s signer mismatch: expected %s got %s", field, expected.Hex(), recovered.Hex())
	}

	return nil
}

func verifyEd25519Signature(field string, sig *Signature, hash []byte) error {
	if len(sig.PublicKey) != ed25519.PublicKeySize {
		return errorsmod.Wrapf(ErrInvalidSignature, "%s.public_key must be %d bytes for ed25519", field, ed25519.PublicKeySize)
	}
	if !ed25519.Verify(ed25519.PublicKey(sig.PublicKey), hash, sig.Signature) {
		return errorsmod.Wrapf(ErrInvalidSignature, "%s signature verification failed", field)
	}
	return nil
}

func normalizeRecoverySignature(raw []byte) ([]byte, error) {
	if len(raw) != crypto.SignatureLength {
		return nil, fmt.Errorf("signature must be %d bytes, got %d", crypto.SignatureLength, len(raw))
	}

	out := make([]byte, len(raw))
	copy(out, raw)

	switch out[crypto.RecoveryIDOffset] {
	case 0, 1:
		return out, nil
	case 27, 28:
		out[crypto.RecoveryIDOffset] -= 27
		return out, nil
	default:
		return nil, fmt.Errorf("signature recovery id must be 0/1 or 27/28, got %d", out[crypto.RecoveryIDOffset])
	}
}
