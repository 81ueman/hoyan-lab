package intent

import (
	"fmt"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

// ---------------------------------------------------------------------------
// PacketReachableExpr
// ---------------------------------------------------------------------------

func evalPacketReachable(e *PacketReachableExpr, snapshot SnapshotContext, scenario Scenario) (string, Actual) {
	spec := model.PacketSpec{
		Protocol: e.Protocol,
		DstPort:  model.ExactPort(e.DstPort),
	}

	if scenario.Failures.Max > 0 && e.Expect {
		// Forward check: expect reachable under up to MaxFailures failures.
		target := sim.PacketTarget{
			To:       e.To,
			Protocol: e.Protocol,
			DstPort:  e.DstPort,
			VRF:      e.VRF,
		}
		opts := sim.FailureSearchOptions{
			MaxFailures:  scenario.Failures.Max,
			IncludeLinks: true,
			IncludeNodes: true,
			Domain:       buildFailureDomain(scenario.Failures),
		}
		result, err := snapshot.Graph.FindBreakingFailuresSymbolic(e.From, target, opts)
		if err != nil {
			// Failure search error (e.g. empty domain with no candidate elements):
			// report as info but still run the no-failure check below.
			return "fail", Actual{Reason: fmt.Sprintf("failure search error: %v", err)}
		}
		if result.Sat {
			return "fail", Actual{
				Reachable: &e.Expect,
				Reason:    "reachable under no failures but broken by some failure within scenario constraints",
			}
		}
		return "pass", Actual{Reachable: &e.Expect}
	}

	var reachable bool
	var reason string

	if e.VRF != "" {
		_, reachable, reason = snapshot.Graph.PacketReachableSpecVRF(e.From, e.VRF, e.To, spec, sim.NoFailures())
	} else {
		_, reachable, reason = snapshot.Graph.PacketReachableSpec(e.From, e.To, spec, sim.NoFailures())
	}

	actual := Actual{
		Reachable: &reachable,
		Reason:    reason,
	}

	if reachable == e.Expect {
		return "pass", actual
	}
	return "fail", actual
}

func buildFailureDomain(fc FailureConstraints) model.FailureDomain {
	return model.FailureDomain{
		IncludeLinkRoles: fc.IncludeLinkRoles,
		ExcludeLinkRoles: fc.ExcludeLinkRoles,
		IncludeLinks:     fc.IncludeLinks,
		ExcludeLinks:     fc.ExcludeLinks,
		IncludeNodeRoles: fc.IncludeNodeRoles,
		ExcludeNodeRoles: fc.ExcludeNodeRoles,
		IncludeNodes:     fc.IncludeNodes,
		ExcludeNodes:     fc.ExcludeNodes,
	}
}
