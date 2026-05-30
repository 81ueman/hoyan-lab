package verify

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/query"
	"github.com/81ueman/hoyan-lab/internal/domain/solver"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

func appendFailureCheckResults(report *Report, topo *model.Topology, checks []query.FailureCheck, g *sim.Graph, universe model.PrefixUniverse) {
	for _, q := range checks {
		vrf := string(model.NormalizeNetworkInstance(q.VRF))
		expected := true
		if q.ExpectReachable != nil {
			expected = *q.ExpectReachable
		}
		ports := q.DstPortValues()
		for _, port := range ports {
			for _, classID := range failureClasses(topo, universe, q) {
				class, ok := prefixClass(universe, classID)
				if !ok {
					continue
				}
				target := sim.PacketClassTarget{Universe: universe, ClassID: classID, Protocol: q.Protocol, DstPort: port, VRF: vrf}
				symbolic := g.SymbolicPacketReachabilityForPrefixSetSpecVRF(q.From, vrf, class.Space, target.Spec())
				result := classResult(universe, class, NewFailureResult(queryResultName(q.Name, port, len(ports)), true, expected, symbolic.Reason))
				result.SetConditions(symbolic.Reachable.String(), symbolic.Unreachable.String())
				if cut, ok := findBreakingFailures(g, q.From, target, failureSearchOptions(q.MaxFailures, q.FailureDomain), &result); ok {
					result.Metadata.Reachable = false
					result.SetCounterexample(formatFailureElements(cut))
					result.Metadata.Reason = "counterexample within failure budget"
				}
				report.Results = append(report.Results, result)
			}
		}
	}
}

func failureSearchOptions(maxFailures int, domain model.FailureDomain) sim.FailureSearchOptions {
	return sim.FailureSearchOptions{
		IncludeLinks: true,
		MaxFailures:  maxFailures,
		Domain:       domain,
	}
}

func findBreakingFailures(g *sim.Graph, from string, target sim.SymbolicTarget, opts sim.FailureSearchOptions, result *Result) ([]solver.FailureElement, bool) {
	search, err := g.FindBreakingFailuresSymbolic(from, target, opts)
	result.Solver = &search.Solver
	if err != nil {
		result.Metadata.Reason = appendReason(result.Metadata.Reason, "failure search error: "+err.Error())
		return nil, false
	}
	if !search.Sat {
		return nil, false
	}
	return search.Failures, true
}

func appendReason(existing, extra string) string {
	if existing == "" {
		return extra
	}
	return existing + "; " + extra
}

func formatFailureElements(elements []solver.FailureElement) []string {
	out := make([]string, 0, len(elements))
	for _, element := range elements {
		if element.Kind == solver.FailureLink {
			out = append(out, element.Name)
			continue
		}
		out = append(out, element.String())
	}
	return out
}
