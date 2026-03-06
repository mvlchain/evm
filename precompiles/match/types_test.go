package match

import (
	"fmt"
	"testing"

	cmn "github.com/cosmos/evm/precompiles/common"

	"github.com/stretchr/testify/require"
)

func TestParseReplayArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []any
		wantErr    bool
		errMessage string
		want       ReplayArgs
	}{
		{
			name:    "valid",
			args:    []any{"pool-1", "intent-1"},
			wantErr: false,
			want: ReplayArgs{
				PoolID:   "pool-1",
				IntentID: "intent-1",
			},
		},
		{
			name:       "no arguments",
			args:       []any{},
			wantErr:    true,
			errMessage: fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 2, 0),
		},
		{
			name:       "wrong first type",
			args:       []any{123, "intent-1"},
			wantErr:    true,
			errMessage: "invalid type for poolId",
		},
		{
			name:       "wrong second type",
			args:       []any{"pool-1", 456},
			wantErr:    true,
			errMessage: "invalid type for intentId",
		},
		{
			name:       "empty pool id",
			args:       []any{"   ", "intent-1"},
			wantErr:    true,
			errMessage: "poolId cannot be empty",
		},
		{
			name:       "empty intent id",
			args:       []any{"pool-1", ""},
			wantErr:    true,
			errMessage: "intentId cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseReplayArgs(tt.args)

			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMessage)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParseSubmitCertificateArgs(t *testing.T) {
	tests := []struct {
		name       string
		args       []any
		wantErr    bool
		errMessage string
		wantLen    int
	}{
		{
			name:    "valid",
			args:    []any{[]byte{0x01, 0x02, 0x03}},
			wantErr: false,
			wantLen: 3,
		},
		{
			name:       "no arguments",
			args:       []any{},
			wantErr:    true,
			errMessage: fmt.Sprintf(cmn.ErrInvalidNumberOfArgs, 1, 0),
		},
		{
			name:       "wrong type",
			args:       []any{"abc"},
			wantErr:    true,
			errMessage: "invalid type for certificate",
		},
		{
			name:       "empty bytes",
			args:       []any{[]byte{}},
			wantErr:    true,
			errMessage: "certificate cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseSubmitCertificateArgs(tt.args)

			if tt.wantErr {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errMessage)
				return
			}

			require.NoError(t, err)
			require.Len(t, got.Certificate, tt.wantLen)
		})
	}
}
