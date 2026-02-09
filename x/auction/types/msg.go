package types

import (
	"fmt"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

var _ sdk.Msg = &MsgConfirmAuction{}

func (m *MsgConfirmAuction) ValidateBasic() error {
	if m.AuctionId == "" {
		return fmt.Errorf("auction_id is required")
	}
	if !common.IsHexAddress(m.Seller) || !common.IsHexAddress(m.Winner) {
		return ErrInvalidAddress
	}
	if m.Denom == "" {
		return fmt.Errorf("denom is required")
	}
	if m.EndHeight <= 0 {
		return fmt.Errorf("end_height must be > 0")
	}
	if err := validatePrice(m.Price); err != nil {
		return err
	}
	if m.AskSig == "" || m.BidSig == "" || m.SellerSig == "" {
		return fmt.Errorf("signatures are required")
	}
	if err := validateSigFormat(m.AskSig); err != nil {
		return err
	}
	if err := validateSigFormat(m.BidSig); err != nil {
		return err
	}
	if err := validateSellerSig(m); err != nil {
		return err
	}
	return nil
}

func (m *MsgConfirmAuction) GetSigners() []sdk.AccAddress {
	addr := common.HexToAddress(m.Seller)
	return []sdk.AccAddress{sdk.AccAddress(addr.Bytes())}
}

func validatePrice(price string) error {
	if price == "" {
		return ErrInvalidPrice
	}
	for _, r := range price {
		if r < '0' || r > '9' {
			return ErrInvalidPrice
		}
	}
	if len(strings.TrimLeft(price, "0")) == 0 {
		return ErrInvalidPrice
	}
	return nil
}

func validateSigFormat(sig string) error {
	bz, err := hexutil.Decode(sig)
	if err != nil || len(bz) != 65 {
		return ErrInvalidSignature
	}
	return nil
}

func validateSellerSig(m *MsgConfirmAuction) error {
	msg := confirmPayload(m)
	hash := accounts.TextHash([]byte(msg))
	sig, err := hexutil.Decode(m.SellerSig)
	if err != nil || len(sig) != 65 {
		return ErrInvalidSignature
	}
	if sig[64] >= 27 {
		sig[64] -= 27
	}
	pub, err := crypto.SigToPub(hash, sig)
	if err != nil {
		return ErrInvalidSignature
	}
	recovered := crypto.PubkeyToAddress(*pub).Hex()
	if strings.ToLower(recovered) != strings.ToLower(m.Seller) {
		return ErrInvalidSignature
	}
	return nil
}

func confirmPayload(m *MsgConfirmAuction) string {
	return strings.Join([]string{
		m.AuctionId,
		strings.ToLower(m.Seller),
		strings.ToLower(m.Winner),
		m.Price,
		m.Denom,
		fmt.Sprintf("%d", m.EndHeight),
		m.AskSig,
		m.BidSig,
	}, "|")
}
