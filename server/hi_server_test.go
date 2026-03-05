package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"

	evmtypes "github.com/cosmos/evm/x/vm/types"
)

type stubSubmitter struct {
	called bool
	err    error
}

func (s *stubSubmitter) Submit(_ *evmtypes.MsgEthereumTx) error {
	s.called = true
	return s.err
}

func TestHiHandlerWithName(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/hi?name=triggy", nil)
	rr := httptest.NewRecorder()

	hiHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp hiResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Msg != "Hi triggy" {
		t.Fatalf("expected msg %q, got %q", "Hi triggy", resp.Msg)
	}
	if resp.Tx == nil {
		t.Fatalf("expected tx response")
	}
	if !resp.Tx.System {
		t.Fatalf("expected system tx")
	}
	if resp.Tx.GasPrice != "0" {
		t.Fatalf("expected gas_price %q, got %q", "0", resp.Tx.GasPrice)
	}
	if resp.Tx.Value != "0" {
		t.Fatalf("expected value %q, got %q", "0", resp.Tx.Value)
	}
	if resp.Tx.Data != hexutil.Encode([]byte("Hi triggy")) {
		t.Fatalf("unexpected tx data %q", resp.Tx.Data)
	}
	if resp.Tx.Hash == "" {
		t.Fatalf("expected tx hash")
	}
	if resp.Tx.MempoolAdded {
		t.Fatalf("expected mempool_added to be false in direct handler mode")
	}
}

func TestHiHandlerWithoutName(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/hi", nil)
	rr := httptest.NewRecorder()

	hiHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp hiResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Msg != "Hi world" {
		t.Fatalf("expected msg %q, got %q", "Hi world", resp.Msg)
	}
	if resp.Tx == nil {
		t.Fatalf("expected tx response")
	}
}

func TestHiHandlerWithMessage(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/hi?msg=hello-evm", nil)
	rr := httptest.NewRecorder()

	hiHandler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}

	var resp hiResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Msg != "hello-evm" {
		t.Fatalf("expected msg %q, got %q", "hello-evm", resp.Msg)
	}
	if resp.Tx == nil {
		t.Fatalf("expected tx response")
	}
	if resp.Tx.Data != hexutil.Encode([]byte("hello-evm")) {
		t.Fatalf("unexpected tx data %q", resp.Tx.Data)
	}
}

func TestHiHandlerWithSubmitter(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/hi?msg=hello-evm", nil)
	rr := httptest.NewRecorder()

	submitter := &stubSubmitter{}
	hiHandlerWithSubmitter(submitter).ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if !submitter.called {
		t.Fatalf("expected submitter to be called")
	}

	var resp hiResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Tx == nil {
		t.Fatalf("expected tx response")
	}
	if !resp.Tx.MempoolAdded {
		t.Fatalf("expected tx to be marked as mempool_added")
	}
}

func TestHiHandlerWithSubmitterError(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/hi?msg=hello-evm", nil)
	rr := httptest.NewRecorder()

	submitter := &stubSubmitter{err: http.ErrHandlerTimeout}
	hiHandlerWithSubmitter(submitter).ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

func TestHiHandlerMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/hi?name=triggy", nil)
	rr := httptest.NewRecorder()

	hiHandler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected status %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}
