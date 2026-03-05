package match

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type mockMatchKeeper struct {
	hasReplay bool
	matchID   string
	found     bool
	requester string
	responder string
	parties   bool

	lastPoolID   string
	lastIntentID string
}

func (m *mockMatchKeeper) HasReplay(_ sdk.Context, poolID, intentID string) bool {
	m.lastPoolID = poolID
	m.lastIntentID = intentID
	return m.hasReplay
}

func (m *mockMatchKeeper) GetReplayMatchID(_ sdk.Context, poolID, intentID string) (string, bool) {
	m.lastPoolID = poolID
	m.lastIntentID = intentID
	return m.matchID, m.found
}

func (m *mockMatchKeeper) GetReplayParties(_ sdk.Context, poolID, intentID string) (string, string, bool) {
	m.lastPoolID = poolID
	m.lastIntentID = intentID
	return m.requester, m.responder, m.parties
}

func TestHasReplay(t *testing.T) {
	keeper := &mockMatchKeeper{hasReplay: true}
	p := NewPrecompile(keeper)
	method := p.Methods[HasReplayMethod]

	bz, err := p.HasReplay(sdk.Context{}, &method, []interface{}{"pool-1", "intent-1"})
	require.NoError(t, err)

	out, err := method.Outputs.Unpack(bz)
	require.NoError(t, err)
	require.Len(t, out, 1)
	require.Equal(t, true, out[0].(bool))
	require.Equal(t, "pool-1", keeper.lastPoolID)
	require.Equal(t, "intent-1", keeper.lastIntentID)
}

func TestGetReplay(t *testing.T) {
	tests := []struct {
		name        string
		matchID     string
		found       bool
		wantFound   bool
		wantMatchID string
	}{
		{
			name:        "found",
			matchID:     "match-1",
			found:       true,
			wantFound:   true,
			wantMatchID: "match-1",
		},
		{
			name:        "not found",
			matchID:     "",
			found:       false,
			wantFound:   false,
			wantMatchID: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keeper := &mockMatchKeeper{
				matchID: tc.matchID,
				found:   tc.found,
			}
			p := NewPrecompile(keeper)
			method := p.Methods[GetReplayMethod]

			bz, err := p.GetReplay(sdk.Context{}, &method, []interface{}{"pool-1", "intent-1"})
			require.NoError(t, err)

			out, err := method.Outputs.Unpack(bz)
			require.NoError(t, err)
			require.Len(t, out, 2)
			require.Equal(t, tc.wantFound, out[0].(bool))
			require.Equal(t, tc.wantMatchID, out[1].(string))
			require.Equal(t, "pool-1", keeper.lastPoolID)
			require.Equal(t, "intent-1", keeper.lastIntentID)
		})
	}
}

func TestGetReplayParties(t *testing.T) {
	tests := []struct {
		name          string
		matchID       string
		found         bool
		requester     string
		responder     string
		partiesFound  bool
		wantFound     bool
		wantMatchID   string
		wantRequester string
		wantResponder string
	}{
		{
			name:          "found with parties",
			matchID:       "match-1",
			found:         true,
			requester:     "0xrequester",
			responder:     "0xresponder",
			partiesFound:  true,
			wantFound:     true,
			wantMatchID:   "match-1",
			wantRequester: "0xrequester",
			wantResponder: "0xresponder",
		},
		{
			name:          "found without parties metadata",
			matchID:       "match-2",
			found:         true,
			partiesFound:  false,
			wantFound:     true,
			wantMatchID:   "match-2",
			wantRequester: "",
			wantResponder: "",
		},
		{
			name:          "not found",
			matchID:       "",
			found:         false,
			partiesFound:  false,
			wantFound:     false,
			wantMatchID:   "",
			wantRequester: "",
			wantResponder: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keeper := &mockMatchKeeper{
				matchID:   tc.matchID,
				found:     tc.found,
				requester: tc.requester,
				responder: tc.responder,
				parties:   tc.partiesFound,
			}
			p := NewPrecompile(keeper)
			method := p.Methods[GetReplayPartiesMethod]

			bz, err := p.GetReplayParties(sdk.Context{}, &method, []interface{}{"pool-1", "intent-1"})
			require.NoError(t, err)

			out, err := method.Outputs.Unpack(bz)
			require.NoError(t, err)
			require.Len(t, out, 4)
			require.Equal(t, tc.wantFound, out[0].(bool))
			require.Equal(t, tc.wantMatchID, out[1].(string))
			require.Equal(t, tc.wantRequester, out[2].(string))
			require.Equal(t, tc.wantResponder, out[3].(string))
			require.Equal(t, "pool-1", keeper.lastPoolID)
			require.Equal(t, "intent-1", keeper.lastIntentID)
		})
	}
}
