package doubleratchet

import (
	"encoding/binary"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/require"
)

func TestValidateEnvelope(t *testing.T) {
	precompile, err := NewPrecompile(6_000)
	require.NoError(t, err)

	dhPub := common.HexToHash("0x1111111111111111111111111111111111111111111111111111111111111111")
	adHash := common.HexToHash("0x2222222222222222222222222222222222222222222222222222222222222222")
	header := buildHeader(dhPub.Bytes(), 7, 9, adHash.Bytes())
	ciphertext := []byte{0xaa, 0xbb, 0xcc}

	method, ok := precompile.ABI.Methods[ValidateEnvelopeMethod]
	require.True(t, ok)

	out, err := precompile.ValidateEnvelope(&method, []interface{}{
		header,
		ciphertext,
		uint32(256),
		uint32(512),
	})
	require.NoError(t, err)

	values, err := method.Outputs.Unpack(out)
	require.NoError(t, err)
	require.Len(t, values, 7)

	valid := values[0].(bool)
	version := values[2].(uint8)
	outDhPub := values[3].([32]byte)
	pn := values[4].(uint32)
	n := values[5].(uint32)
	outAdHash := values[6].([32]byte)

	require.True(t, valid)
	require.Equal(t, uint8(1), version)
	require.Equal(t, dhPub.Bytes(), outDhPub[:])
	require.Equal(t, adHash.Bytes(), outAdHash[:])
	require.Equal(t, uint32(7), pn)
	require.Equal(t, uint32(9), n)
}

func TestValidateEnvelopeInvalidHeader(t *testing.T) {
	precompile, err := NewPrecompile(6_000)
	require.NoError(t, err)

	method, ok := precompile.ABI.Methods[ValidateEnvelopeMethod]
	require.True(t, ok)

	_, err = precompile.ValidateEnvelope(&method, []interface{}{
		[]byte{0x01},
		[]byte{0xaa},
		uint32(256),
		uint32(512),
	})
	require.Error(t, err)
}

func buildHeader(dhPub []byte, pn uint32, n uint32, adHash []byte) []byte {
	header := make([]byte, 0, headerLength)
	header = append(header, headerVersion)
	header = append(header, dhPub...)
	pnBz := make([]byte, 4)
	nBz := make([]byte, 4)
	binary.BigEndian.PutUint32(pnBz, pn)
	binary.BigEndian.PutUint32(nBz, n)
	header = append(header, pnBz...)
	header = append(header, nBz...)
	header = append(header, adHash...)
	return header
}
