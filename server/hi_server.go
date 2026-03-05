package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"cosmossdk.io/log"
	"golang.org/x/sync/errgroup"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/cosmos/evm/x/vm/types"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client"
)

const defaultHiServerAddress = ":3000"

var systemTxToAddress = common.HexToAddress("0x1000000000000000000000000000000000000000")

type hiResponse struct {
	Msg string       `json:"msg"`
	Tx  *systemEVMTx `json:"tx,omitempty"`
}

type systemEVMTx struct {
	System       bool   `json:"system"`
	MempoolAdded bool   `json:"mempool_added"`
	From         string `json:"from"`
	To           string `json:"to"`
	Nonce        uint64 `json:"nonce"`
	Gas          uint64 `json:"gas"`
	GasPrice     string `json:"gas_price"`
	Value        string `json:"value"`
	Data         string `json:"data"`
	Raw          string `json:"raw"`
	Hash         string `json:"hash"`
}

type hiTxSubmitter interface {
	Submit(msg *types.MsgEthereumTx) error
}

type hiMempoolApp interface {
	GetBaseApp() *baseapp.BaseApp
	GetTxConfig() client.TxConfig
}

type hiMempoolSubmitter struct {
	app hiMempoolApp
}

func (s hiMempoolSubmitter) Submit(msg *types.MsgEthereumTx) error {
	txCfg := s.app.GetTxConfig()
	txBuilder := txCfg.NewTxBuilder()

	if _, err := msg.BuildTx(txBuilder, types.GetEVMCoinDenom()); err != nil {
		return fmt.Errorf("failed to build cosmos tx from ethereum tx: %w", err)
	}

	txBytes, err := txCfg.TxEncoder()(txBuilder.GetTx())
	if err != nil {
		return fmt.Errorf("failed to encode cosmos tx: %w", err)
	}

	res, err := s.app.GetBaseApp().CheckTx(&abci.RequestCheckTx{Tx: txBytes})
	if err != nil {
		return fmt.Errorf("check tx failed: %w", err)
	}

	if res == nil {
		return errors.New("nil check tx response")
	}

	if res.Code != 0 {
		return fmt.Errorf("check tx rejected: code=%d log=%s", res.Code, res.Log)
	}

	return nil
}

func newHiTxSubmitter(logger log.Logger, app any) hiTxSubmitter {
	hiApp, ok := app.(hiMempoolApp)
	if !ok || hiApp == nil {
		logger.Info("hi server tx submission to mempool disabled: app does not expose BaseApp/TxConfig")
		return nil
	}

	return hiMempoolSubmitter{app: hiApp}
}

func hiHandler(w http.ResponseWriter, r *http.Request) {
	hiHandlerWithSubmitter(nil)(w, r)
}

func hiHandlerWithSubmitter(submitter hiTxSubmitter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		message := strings.TrimSpace(r.URL.Query().Get("msg"))
		if message == "" {
			name := strings.TrimSpace(r.URL.Query().Get("name"))
			if name == "" {
				name = "world"
			}
			message = "Hi " + name
		}

		msg, systemTx, err := buildSystemEVMTx(message)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to build evm tx: %v", err), http.StatusInternalServerError)
			return
		}

		if submitter != nil {
			if err := submitter.Submit(msg); err != nil {
				http.Error(w, fmt.Sprintf("failed to submit tx to mempool: %v", err), http.StatusInternalServerError)
				return
			}
			systemTx.MempoolAdded = true
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		if err := json.NewEncoder(w).Encode(hiResponse{Msg: message, Tx: systemTx}); err != nil {
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
		}
	}
}

func buildSystemEVMTx(message string) (*types.MsgEthereumTx, *systemEVMTx, error) {
	data := []byte(message)
	unsignedTx := ethtypes.NewTx(&ethtypes.LegacyTx{
		Nonce:    0,
		To:       &systemTxToAddress,
		Value:    big.NewInt(0),
		Gas:      intrinsicGasForData(data),
		GasPrice: big.NewInt(0),
		Data:     data,
	})

	privKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, nil, err
	}

	chainID := new(big.Int).SetUint64(types.DefaultEVMChainID)
	if chainCfg := types.GetChainConfig(); chainCfg != nil {
		chainID = new(big.Int).SetUint64(chainCfg.ChainId)
	}

	signer := ethtypes.LatestSignerForChainID(chainID)
	signedTx, err := ethtypes.SignTx(unsignedTx, signer, privKey)
	if err != nil {
		return nil, nil, err
	}

	rawTx, err := signedTx.MarshalBinary()
	if err != nil {
		return nil, nil, err
	}

	msg := &types.MsgEthereumTx{}
	if err := msg.FromSignedEthereumTx(signedTx, signer); err != nil {
		return nil, nil, err
	}

	txResp := &systemEVMTx{
		System:       true,
		MempoolAdded: false,
		From:         common.BytesToAddress(msg.From).Hex(),
		To:           systemTxToAddress.Hex(),
		Nonce:        signedTx.Nonce(),
		Gas:          signedTx.Gas(),
		GasPrice:     signedTx.GasPrice().String(),
		Value:        signedTx.Value().String(),
		Data:         hexutil.Encode(signedTx.Data()),
		Raw:          hexutil.Encode(rawTx),
		Hash:         signedTx.Hash().Hex(),
	}

	return msg, txResp, nil
}

func intrinsicGasForData(data []byte) uint64 {
	const (
		baseGas     = uint64(21_000)
		zeroByte    = uint64(4)
		nonZeroByte = uint64(16)
	)

	gas := baseGas
	for _, b := range data {
		if b == 0 {
			gas += zeroByte
		} else {
			gas += nonZeroByte
		}
	}
	return gas
}

func startHiServer(ctx context.Context, g *errgroup.Group, logger log.Logger, addr string, app any) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		logger.Info("hi server disabled")
		return
	}

	submitter := newHiTxSubmitter(logger, app)

	mux := http.NewServeMux()
	mux.HandleFunc("/hi", hiHandlerWithSubmitter(submitter))

	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	g.Go(func() error {
		logger.Info("starting web server", "address", addr)

		errCh := make(chan error, 1)
		go func() {
			errCh <- srv.ListenAndServe()
		}()

		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			if err := srv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.Canceled) {
				logger.Error("failed to shutdown hi server", "error", err.Error())
			}

			err := <-errCh
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		case err := <-errCh:
			if err != nil && !errors.Is(err, http.ErrServerClosed) {
				return err
			}
			return nil
		}
	})
}
