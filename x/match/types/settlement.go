package types

import (
	"fmt"
	"io"
)

// SettlementInstruction encodes the atomic swap parameters for on-chain execution.
// The initiator sends AssetIn to the responder; the responder sends AssetOut back.
// Asset values are ERC-20 contract addresses (hex) or "native" for the chain coin.
// Amount values are uint256-range decimal strings.
type SettlementInstruction struct {
	Initiator string `protobuf:"bytes,1,opt,name=initiator,proto3" json:"initiator,omitempty"`
	Responder string `protobuf:"bytes,2,opt,name=responder,proto3" json:"responder,omitempty"`
	AssetIn   string `protobuf:"bytes,3,opt,name=asset_in,json=assetIn,proto3" json:"asset_in,omitempty"`
	AmountIn  string `protobuf:"bytes,4,opt,name=amount_in,json=amountIn,proto3" json:"amount_in,omitempty"`
	AssetOut  string `protobuf:"bytes,5,opt,name=asset_out,json=assetOut,proto3" json:"asset_out,omitempty"`
	AmountOut string `protobuf:"bytes,6,opt,name=amount_out,json=amountOut,proto3" json:"amount_out,omitempty"`
}

func (m *SettlementInstruction) Reset()      { *m = SettlementInstruction{} }
func (*SettlementInstruction) ProtoMessage() {}
func (m *SettlementInstruction) String() string {
	return "settlement_instruction{" + m.Initiator + "->" + m.Responder + " " + m.AssetIn + ":" + m.AmountIn + "," + m.AssetOut + ":" + m.AmountOut + "}"
}

// Descriptor returns nil — this type is serialisation-compatible but not registered in the file descriptor.
func (*SettlementInstruction) Descriptor() ([]byte, []int) { return nil, nil }

func (m *SettlementInstruction) GetInitiator() string {
	if m != nil {
		return m.Initiator
	}
	return ""
}
func (m *SettlementInstruction) GetResponder() string {
	if m != nil {
		return m.Responder
	}
	return ""
}
func (m *SettlementInstruction) GetAssetIn() string {
	if m != nil {
		return m.AssetIn
	}
	return ""
}
func (m *SettlementInstruction) GetAmountIn() string {
	if m != nil {
		return m.AmountIn
	}
	return ""
}
func (m *SettlementInstruction) GetAssetOut() string {
	if m != nil {
		return m.AssetOut
	}
	return ""
}
func (m *SettlementInstruction) GetAmountOut() string {
	if m != nil {
		return m.AmountOut
	}
	return ""
}

// Marshal encodes m to protobuf binary using field numbers 1-6.
func (m *SettlementInstruction) Marshal() (dAtA []byte, err error) {
	size := m.Size()
	dAtA = make([]byte, size)
	n, err := m.MarshalToSizedBuffer(dAtA[:size])
	if err != nil {
		return nil, err
	}
	return dAtA[:n], nil
}

func (m *SettlementInstruction) MarshalTo(dAtA []byte) (int, error) {
	size := m.Size()
	return m.MarshalToSizedBuffer(dAtA[:size])
}

// MarshalToSizedBuffer writes fields in reverse field-number order (gogoproto convention).
func (m *SettlementInstruction) MarshalToSizedBuffer(dAtA []byte) (int, error) {
	i := len(dAtA)
	if len(m.AmountOut) > 0 {
		i -= len(m.AmountOut)
		copy(dAtA[i:], m.AmountOut)
		i = encodeVarintTypes(dAtA, i, uint64(len(m.AmountOut)))
		i--
		dAtA[i] = 0x32 // field 6, wire type 2
	}
	if len(m.AssetOut) > 0 {
		i -= len(m.AssetOut)
		copy(dAtA[i:], m.AssetOut)
		i = encodeVarintTypes(dAtA, i, uint64(len(m.AssetOut)))
		i--
		dAtA[i] = 0x2a // field 5, wire type 2
	}
	if len(m.AmountIn) > 0 {
		i -= len(m.AmountIn)
		copy(dAtA[i:], m.AmountIn)
		i = encodeVarintTypes(dAtA, i, uint64(len(m.AmountIn)))
		i--
		dAtA[i] = 0x22 // field 4, wire type 2
	}
	if len(m.AssetIn) > 0 {
		i -= len(m.AssetIn)
		copy(dAtA[i:], m.AssetIn)
		i = encodeVarintTypes(dAtA, i, uint64(len(m.AssetIn)))
		i--
		dAtA[i] = 0x1a // field 3, wire type 2
	}
	if len(m.Responder) > 0 {
		i -= len(m.Responder)
		copy(dAtA[i:], m.Responder)
		i = encodeVarintTypes(dAtA, i, uint64(len(m.Responder)))
		i--
		dAtA[i] = 0x12 // field 2, wire type 2
	}
	if len(m.Initiator) > 0 {
		i -= len(m.Initiator)
		copy(dAtA[i:], m.Initiator)
		i = encodeVarintTypes(dAtA, i, uint64(len(m.Initiator)))
		i--
		dAtA[i] = 0x0a // field 1, wire type 2
	}
	return len(dAtA) - i, nil
}

// Size returns the number of bytes required to marshal m.
func (m *SettlementInstruction) Size() (n int) {
	if m == nil {
		return 0
	}
	var l int
	if l = len(m.Initiator); l > 0 {
		n += 1 + l + sovTypes(uint64(l))
	}
	if l = len(m.Responder); l > 0 {
		n += 1 + l + sovTypes(uint64(l))
	}
	if l = len(m.AssetIn); l > 0 {
		n += 1 + l + sovTypes(uint64(l))
	}
	if l = len(m.AmountIn); l > 0 {
		n += 1 + l + sovTypes(uint64(l))
	}
	if l = len(m.AssetOut); l > 0 {
		n += 1 + l + sovTypes(uint64(l))
	}
	if l = len(m.AmountOut); l > 0 {
		n += 1 + l + sovTypes(uint64(l))
	}
	return n
}

// Unmarshal decodes m from protobuf binary dAtA.
func (m *SettlementInstruction) Unmarshal(dAtA []byte) error {
	l := len(dAtA)
	iNdEx := 0
	for iNdEx < l {
		preIndex := iNdEx
		var wire uint64
		for shift := uint(0); ; shift += 7 {
			if shift >= 64 {
				return ErrIntOverflowTypes
			}
			if iNdEx >= l {
				return io.ErrUnexpectedEOF
			}
			b := dAtA[iNdEx]
			iNdEx++
			wire |= uint64(b&0x7F) << shift
			if b < 0x80 {
				break
			}
		}
		fieldNum := int32(wire >> 3)
		wireType := int(wire & 0x7)
		if wireType == 4 {
			return errSettlementEndGroup
		}
		if fieldNum <= 0 {
			return errSettlementIllegalTag
		}
		switch fieldNum {
		case 1, 2, 3, 4, 5, 6:
			if wireType != 2 {
				return errSettlementWireType
			}
			var stringLen uint64
			for shift := uint(0); ; shift += 7 {
				if shift >= 64 {
					return ErrIntOverflowTypes
				}
				if iNdEx >= l {
					return io.ErrUnexpectedEOF
				}
				b := dAtA[iNdEx]
				iNdEx++
				stringLen |= uint64(b&0x7F) << shift
				if b < 0x80 {
					break
				}
			}
			intStringLen := int(stringLen)
			if intStringLen < 0 {
				return ErrInvalidLengthTypes
			}
			postIndex := iNdEx + intStringLen
			if postIndex < 0 {
				return ErrInvalidLengthTypes
			}
			if postIndex > l {
				return io.ErrUnexpectedEOF
			}
			s := string(dAtA[iNdEx:postIndex])
			switch fieldNum {
			case 1:
				m.Initiator = s
			case 2:
				m.Responder = s
			case 3:
				m.AssetIn = s
			case 4:
				m.AmountIn = s
			case 5:
				m.AssetOut = s
			case 6:
				m.AmountOut = s
			}
			iNdEx = postIndex
		default:
			iNdEx = preIndex
			skippy, err := skipTypes(dAtA[iNdEx:])
			if err != nil {
				return err
			}
			if (skippy < 0) || (iNdEx+skippy) < 0 {
				return ErrInvalidLengthTypes
			}
			if (iNdEx + skippy) > l {
				return io.ErrUnexpectedEOF
			}
			iNdEx += skippy
		}
	}
	if iNdEx > l {
		return io.ErrUnexpectedEOF
	}
	return nil
}

var (
	errSettlementEndGroup   = fmt.Errorf("proto: SettlementInstruction: wiretype end group for non-group")
	errSettlementIllegalTag = fmt.Errorf("proto: SettlementInstruction: illegal tag")
	errSettlementWireType   = fmt.Errorf("proto: SettlementInstruction: wrong wire type for string field")
)
