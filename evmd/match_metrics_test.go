package evmd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNormalizeMatchMetricReason(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "keeps reason", input: "batch_hash_mismatch", expected: "batch_hash_mismatch"},
		{name: "trims spaces", input: "  missing_batch_meta  ", expected: "missing_batch_meta"},
		{name: "empty to unknown", input: "", expected: "unknown"},
		{name: "spaces to unknown", input: "   ", expected: "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, normalizeMatchMetricReason(tc.input))
		})
	}
}
