package keyregistry

import (
	"encoding/binary"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
)

const (
	PublishKeysV2Method = "publishKeysV2"
	GetKeysMethod       = "getKeys"
	signatureMaxLen     = 96
)

type KeyBundle struct {
	IdentityDhKey   [32]byte
	IdentitySignKey [32]byte
	SignedPreKey    [32]byte
	Signature       []byte
	ExpiresAt       uint64
	UpdatedAt       uint64
}

func (p Precompile) PublishKeysV2(evm *vm.EVM, contract *vm.Contract, method *abi.Method, args []interface{}) ([]byte, error) {
	if len(args) != 5 {
		return nil, fmt.Errorf("invalid number of args")
	}
	identityDhKey, ok := args[0].([32]byte)
	if !ok {
		return nil, fmt.Errorf("invalid identityDhKey type")
	}
	identitySignKey, ok := args[1].([32]byte)
	if !ok {
		return nil, fmt.Errorf("invalid identitySignKey type")
	}
	signedPreKey, ok := args[2].([32]byte)
	if !ok {
		return nil, fmt.Errorf("invalid signedPreKey type")
	}
	signature, ok := args[3].([]byte)
	if !ok {
		return nil, fmt.Errorf("invalid signature type")
	}
	expiresAt, ok := args[4].(uint64)
	if !ok {
		return nil, fmt.Errorf("invalid expiresAt type")
	}
	if isZero32(identityDhKey) || isZero32(identitySignKey) || isZero32(signedPreKey) {
		return nil, fmt.Errorf("empty keys")
	}
	if len(signature) == 0 || len(signature) > signatureMaxLen {
		return nil, fmt.Errorf("invalid signature length")
	}

	now := uint64(evm.Context.Time)
	if expiresAt != 0 && expiresAt <= now {
		return nil, fmt.Errorf("invalid expiry")
	}

	stateDB := evm.StateDB
	owner := contract.Caller()
	base := mappingSlot(owner, 0)

	stateDB.SetState(p.Address(), base, common.BytesToHash(identityDhKey[:]))
	stateDB.SetState(p.Address(), addSlot(base, 1), common.BytesToHash(identitySignKey[:]))
	stateDB.SetState(p.Address(), addSlot(base, 2), common.BytesToHash(signedPreKey[:]))

	sigSlot := addSlot(base, 3)
	stateDB.SetState(p.Address(), sigSlot, uint64ToHash(uint64(len(signature))))
	writeBytes(stateDB, p.Address(), sigSlot, signature)

	stateDB.SetState(p.Address(), addSlot(base, 4), uint64ToHash(expiresAt))
	stateDB.SetState(p.Address(), addSlot(base, 5), uint64ToHash(now))

	return method.Outputs.Pack()
}

func (p Precompile) GetKeys(evm *vm.EVM, method *abi.Method, args []interface{}) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("invalid number of args")
	}
	owner, ok := args[0].(common.Address)
	if !ok {
		return nil, fmt.Errorf("invalid owner type")
	}

	stateDB := evm.StateDB
	base := mappingSlot(owner, 0)

	bundle := KeyBundle{
		IdentityDhKey:   stateDB.GetState(p.Address(), base),
		IdentitySignKey: stateDB.GetState(p.Address(), addSlot(base, 1)),
		SignedPreKey:    stateDB.GetState(p.Address(), addSlot(base, 2)),
		ExpiresAt:       hashToUint64(stateDB.GetState(p.Address(), addSlot(base, 4))),
		UpdatedAt:       hashToUint64(stateDB.GetState(p.Address(), addSlot(base, 5))),
	}

	sigSlot := addSlot(base, 3)
	sigLen := hashToUint64(stateDB.GetState(p.Address(), sigSlot))
	if sigLen > 0 {
		bundle.Signature = readBytes(stateDB, p.Address(), sigSlot, int(sigLen))
	}

	return method.Outputs.Pack(bundle)
}

func mappingSlot(addr common.Address, slot uint64) common.Hash {
	key := common.LeftPadBytes(addr.Bytes(), 32)
	slotBz := common.LeftPadBytes(uint64ToBytes(slot), 32)
	return crypto.Keccak256Hash(append(key, slotBz...))
}

func addSlot(base common.Hash, offset uint64) common.Hash {
	value := new(big.Int).SetBytes(base.Bytes())
	return common.BigToHash(value.Add(value, new(big.Int).SetUint64(offset)))
}

func uint64ToHash(v uint64) common.Hash {
	return common.BigToHash(new(big.Int).SetUint64(v))
}

func hashToUint64(h common.Hash) uint64 {
	return h.Big().Uint64()
}

func uint64ToBytes(v uint64) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, v)
	return b
}

func isZero32(value [32]byte) bool {
	for _, b := range value {
		if b != 0 {
			return false
		}
	}
	return true
}

func writeBytes(stateDB vm.StateDB, addr common.Address, slot common.Hash, data []byte) {
	base := crypto.Keccak256Hash(slot.Bytes())
	for i := 0; i < len(data); i += 32 {
		chunk := data[i:]
		if len(chunk) > 32 {
			chunk = chunk[:32]
		}
		stateDB.SetState(addr, addSlot(base, uint64(i/32)), common.BytesToHash(common.RightPadBytes(chunk, 32)))
	}
}

func readBytes(stateDB vm.StateDB, addr common.Address, slot common.Hash, length int) []byte {
	base := crypto.Keccak256Hash(slot.Bytes())
	out := make([]byte, 0, length)
	for i := 0; i < length; i += 32 {
		chunk := stateDB.GetState(addr, addSlot(base, uint64(i/32))).Bytes()
		if length-i < 32 {
			chunk = chunk[:length-i]
		}
		out = append(out, chunk...)
	}
	return out
}
