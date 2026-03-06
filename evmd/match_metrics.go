package evmd

import (
	"strings"

	"github.com/cosmos/cosmos-sdk/telemetry"
	metrics "github.com/hashicorp/go-metrics"
)

func incrMatchProposalReject(reason string) {
	reason = normalizeMatchMetricReason(reason)
	telemetry.IncrCounterWithLabels( //nolint:staticcheck // TODO: fix
		[]string{"matchboard", "proposal", "reject", "total"},
		1,
		[]metrics.Label{
			telemetry.NewLabel("reason", reason), //nolint:staticcheck // TODO: fix
		},
	)
}

func incrMatchFinalizeRollback(reason string) {
	reason = normalizeMatchMetricReason(reason)
	telemetry.IncrCounterWithLabels( //nolint:staticcheck // TODO: fix
		[]string{"matchboard", "finalize_block", "rollback", "total"},
		1,
		[]metrics.Label{
			telemetry.NewLabel("reason", reason), //nolint:staticcheck // TODO: fix
		},
	)
}

func normalizeMatchMetricReason(reason string) string {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "unknown"
	}
	return reason
}
