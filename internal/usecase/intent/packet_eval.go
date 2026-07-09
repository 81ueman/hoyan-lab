package intent

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

// ---------------------------------------------------------------------------
// PacketReachableExpr
// ---------------------------------------------------------------------------

func evalPacketReachable(e *PacketReachableExpr, snapshot SnapshotContext) (string, Actual) {
	spec := model.PacketSpec{
		Protocol: e.Protocol,
		DstPort:  model.ExactPort(e.DstPort),
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
