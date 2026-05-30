package verify

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/query"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

func appendRouteCheckResults(report *Report, checks []query.RouteCheck, g *sim.Graph, universe model.PrefixUniverse) {
	for _, q := range checks {
		vrf := string(model.NormalizeNetworkInstance(q.VRF))
		classes := universe.ClassesMatching(model.ExactPrefixSet{Prefix: q.Prefix})
		for _, classID := range classes {
			class, ok := prefixClass(universe, classID)
			if !ok {
				continue
			}
			symbolic := g.SymbolicRouteReachabilityForPrefixSetVRF(q.From, vrf, class.Space)
			path, reachable := g.RouteReachableForPrefixSetVRF(q.From, vrf, class.Space, sim.NoFailures())
			result := classResult(universe, class, NewRouteResult(q.Name, reachable, true, path, symbolic.Reason))
			result.SetConditions(symbolic.Reachable.String(), symbolic.Unreachable.String())
			if reachable {
				target := sim.RouteClassTarget{Universe: universe, ClassID: classID, VRF: vrf}
				if cut, ok := findBreakingFailures(g, q.From, target, failureSearchOptions(q.MaxFailures, q.FailureDomain), &result); ok {
					result.SetCounterexample(formatFailureElements(cut))
					result.Metadata.Reason = "reachable now but not resilient to requested failure budget"
				}
			}
			report.Results = append(report.Results, result)
		}
	}
}
