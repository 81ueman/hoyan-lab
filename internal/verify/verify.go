package verify

import (
	"fmt"
	"github.com/81ueman/hoyan-lab/internal/core/predicate"
	"net/netip"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/check/query"
	"github.com/81ueman/hoyan-lab/internal/config/routing"
	"github.com/81ueman/hoyan-lab/internal/core/solver"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/engine/space"
)

type VerifyOptions struct {
	CollapseEquivalentResults bool
	MaxPrefixClasses          int
}

func Run(topo *topology.Topology, routes routing.TopologyRouting, queries *query.Queries) Report {
	return RunWithOptions(topo, routes, queries, VerifyOptions{})
}

func RunWithOptions(topo *topology.Topology, routes routing.TopologyRouting, queries *query.Queries, opts VerifyOptions) Report {
	return runPrefixClasses(topo, routes, queries, opts)
}

func runPrefixClasses(topo *topology.Topology, routes routing.TopologyRouting, queries *query.Queries, opts VerifyOptions) Report {
	g := sim.NewGraphWithRouting(topo, routes)
	universe, err := prefixUniverseForGraph(topo, routes, queries, g, nil)
	if err != nil {
		return Report{Results: []Result{NewSetupResult("prefix-universe", true, err.Error())}}
	}
	if err := checkPrefixClassLimit(universe, opts.MaxPrefixClasses); err != nil {
		stats := universe.Stats
		return Report{Stats: &stats, Results: []Result{NewSetupResult("prefix-universe", true, err.Error())}}
	}
	stats := universe.Stats
	report := Report{Stats: &stats}
	for _, q := range queries.RouteChecks {
		vrf := string(topology.NormalizeNetworkInstance(q.VRF))
		classes := universe.ClassesMatching(predicate.ExactPrefixSet{Prefix: q.Prefix})
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
	for _, q := range queries.PacketChecks {
		vrf := string(topology.NormalizeNetworkInstance(q.VRF))
		expected := true
		if q.ExpectReachable != nil {
			expected = *q.ExpectReachable
		}
		ports := q.DstPortValues()
		for _, port := range ports {
			spec := predicate.PacketSpec{Protocol: q.Protocol, DstPort: predicate.ExactPort(port)}
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
	for _, q := range queries.FailureChecks {
		vrf := string(topology.NormalizeNetworkInstance(q.VRF))
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
	if opts.CollapseEquivalentResults {
		report.Results = collapseResults(report.Results)
	}
	return report
}

func queryResultName(name string, port int, portCount int) string {
	if portCount <= 1 || port <= 0 {
		return name
	}
	return fmt.Sprintf("%s:dst-port-%d", name, port)
}

func prefixUniverseForGraph(topo *topology.Topology, routes routing.TopologyRouting, queries *query.Queries, g *sim.Graph, extra []space.PrefixPredicate) (space.PrefixUniverse, error) {
	predicates := space.CollectPrefixPredicateMetadata(topo, routes, queries)
	predicates = append(predicates, sim.CollectRIBPrefixPredicates(g)...)
	predicates = append(predicates, sim.CollectFIBPrefixPredicates(g)...)
	predicates = append(predicates, extra...)
	return space.BuildPrefixUniverseFromPredicates(predicates)
}

func checkPrefixClassLimit(universe space.PrefixUniverse, maxClasses int) error {
	if maxClasses <= 0 || universe.Stats.ClassCount <= maxClasses {
		return nil
	}
	return fmt.Errorf("prefix universe class count %d exceeds --max-prefix-classes %d", universe.Stats.ClassCount, maxClasses)
}

func packetClasses(topo *topology.Topology, universe space.PrefixUniverse, to string) []space.PrefixClassID {
	if addr, err := netip.ParseAddr(to); err == nil {
		return classesForAddr(universe, addr)
	}
	return classesForDestinationNode(topo, universe, to)
}

func failureClasses(topo *topology.Topology, universe space.PrefixUniverse, q query.FailureCheck) []space.PrefixClassID {
	if !q.Prefix.IsZero() {
		return universe.ClassesMatching(predicate.ExactPrefixSet{Prefix: q.Prefix})
	}
	return packetClasses(topo, universe, q.To)
}

func classesForDestinationNode(topo *topology.Topology, universe space.PrefixUniverse, to string) []space.PrefixClassID {
	if topo == nil {
		return nil
	}
	node, ok := topo.Node(to)
	if !ok {
		return nil
	}
	seen := map[space.PrefixClassID]bool{}
	var out []space.PrefixClassID
	for _, prefix := range node.Prefixes {
		for _, id := range universe.ClassesMatching(predicate.ExactPrefixSet{Prefix: prefix}) {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

func classesForAddr(universe space.PrefixUniverse, addr netip.Addr) []space.PrefixClassID {
	for _, class := range universe.Classes {
		if predicate.AddressSpaceContains(class.Space, addr) {
			return []space.PrefixClassID{class.ID}
		}
	}
	return nil
}

func prefixClass(universe space.PrefixUniverse, id space.PrefixClassID) (space.PrefixClass, bool) {
	for _, class := range universe.Classes {
		if class.ID == id {
			return class, true
		}
	}
	return space.PrefixClass{}, false
}

func classResult(universe space.PrefixUniverse, class space.PrefixClass, result Result) Result {
	id := class.ID
	result.PrefixClass = &PrefixClassMetadata{
		ClassID:           &id,
		ClassIDs:          []space.PrefixClassID{id},
		Space:             class.Space.String(),
		Spaces:            []string{class.Space.String()},
		MatchedPredicates: matchedPredicates(universe, class),
	}
	return result
}

func matchedPredicates(universe space.PrefixUniverse, class space.PrefixClass) []string {
	byID := map[space.PrefixPredicateID]string{}
	for _, predicate := range universe.Predicates {
		byID[predicate.ID] = predicate.Source
	}
	out := make([]string, 0, len(class.MatchingPredicates))
	seen := map[string]bool{}
	for _, id := range class.MatchingPredicates {
		if source := byID[id]; source != "" {
			if seen[source] {
				continue
			}
			seen[source] = true
			out = append(out, source)
		}
	}
	sort.Strings(out)
	return out
}

func collapseResults(results []Result) []Result {
	type aggregate struct {
		result Result
		seen   map[string]bool
	}
	groups := map[string]*aggregate{}
	var order []string
	for _, result := range results {
		key := collapseKey(result)
		group, ok := groups[key]
		if !ok {
			cp := result
			cp.PrefixClass = &PrefixClassMetadata{}
			group = &aggregate{result: cp, seen: map[string]bool{}}
			groups[key] = group
			order = append(order, key)
		}
		if result.PrefixClass != nil && result.PrefixClass.ClassID != nil {
			classKey := fmt.Sprintf("%d", *result.PrefixClass.ClassID)
			if !group.seen[classKey] {
				group.seen[classKey] = true
				group.result.PrefixClass.ClassIDs = append(group.result.PrefixClass.ClassIDs, *result.PrefixClass.ClassID)
			}
		}
		if result.PrefixClass != nil {
			if result.PrefixClass.Space != "" && !containsString(group.result.PrefixClass.Spaces, result.PrefixClass.Space) {
				group.result.PrefixClass.Spaces = append(group.result.PrefixClass.Spaces, result.PrefixClass.Space)
			}
			for _, predicate := range result.PrefixClass.MatchedPredicates {
				if !containsString(group.result.PrefixClass.MatchedPredicates, predicate) {
					group.result.PrefixClass.MatchedPredicates = append(group.result.PrefixClass.MatchedPredicates, predicate)
				}
			}
		}
	}
	out := make([]Result, 0, len(order))
	for _, key := range order {
		result := groups[key].result
		if result.PrefixClass != nil {
			sort.Slice(result.PrefixClass.ClassIDs, func(i, j int) bool {
				return result.PrefixClass.ClassIDs[i] < result.PrefixClass.ClassIDs[j]
			})
			sort.Strings(result.PrefixClass.Spaces)
			sort.Strings(result.PrefixClass.MatchedPredicates)
		}
		out = append(out, result)
	}
	return out
}

func collapseKey(result Result) string {
	return strings.Join([]string{
		result.Name,
		string(result.Type),
		fmt.Sprint(result.Metadata.Reachable),
		fmt.Sprint(result.Metadata.Expected),
		strings.Join(result.Counterexample(), ","),
		result.Metadata.Reason,
		result.ReachableCondition(),
		result.UnreachableCondition(),
	}, "\x00")
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func failureSearchOptions(maxFailures int, domain topology.FailureDomain) sim.FailureSearchOptions {
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

func (r Report) OK() bool {
	for _, result := range r.Results {
		if result.Metadata.Reachable != result.Metadata.Expected {
			return false
		}
	}
	return true
}
