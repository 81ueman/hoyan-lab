package verify

import (
	"fmt"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/query"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

func appendPacketCheckResults(report *Report, topo *model.Topology, checks []query.PacketCheck, g *sim.Graph, universe model.PrefixUniverse) {
	for _, q := range checks {
		vrf := string(model.NormalizeNetworkInstance(q.VRF))
		expected := true
		if q.ExpectReachable != nil {
			expected = *q.ExpectReachable
		}
		ports := q.DstPortValues()
		for _, port := range ports {
			spec := model.PacketSpec{Protocol: q.Protocol, DstPort: model.ExactPort(port)}
			for _, classID := range packetClasses(topo, universe, q.To) {
				class, ok := prefixClass(universe, classID)
				if !ok {
					continue
				}
				symbolic := g.SymbolicPacketReachabilityForPrefixSetSpecVRF(q.From, vrf, class.Space, spec)
				reachable := symbolic.Reachable.Eval(g.FailureContext(sim.NoFailures()))
				result := classResult(universe, class, NewPacketResult(queryResultName(q.Name, port, len(ports)), reachable, expected, sim.Path{}, symbolic.Reason))
				result.SetConditions(symbolic.Reachable.String(), symbolic.Unreachable.String())
				if expected && reachable {
					target := sim.PacketClassTarget{Universe: universe, ClassID: classID, Protocol: q.Protocol, DstPort: port, VRF: vrf}
					if cut, ok := findBreakingFailures(g, q.From, target, failureSearchOptions(q.MaxFailures, q.FailureDomain), &result); ok {
						result.SetCounterexample(formatFailureElements(cut))
						result.Metadata.Reason = "reachable now but not resilient to requested failure budget"
					}
				}
				report.Results = append(report.Results, result)
			}
		}
	}
}

func queryResultName(name string, port int, portCount int) string {
	if portCount <= 1 || port <= 0 {
		return name
	}
	return fmt.Sprintf("%s:dst-port-%d", name, port)
}
