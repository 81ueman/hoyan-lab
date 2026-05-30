package verify

import (
	"fmt"
	"net/netip"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/query"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

func prefixUniverseForGraph(topo *model.Topology, queries *query.Queries, g *sim.Graph, extra []model.PrefixPredicate) (model.PrefixUniverse, error) {
	predicates := model.CollectPrefixPredicateMetadata(topo, queries)
	predicates = append(predicates, sim.CollectRIBPrefixPredicates(g)...)
	predicates = append(predicates, sim.CollectFIBPrefixPredicates(g)...)
	predicates = append(predicates, extra...)
	return model.BuildPrefixUniverseFromPredicates(predicates)
}

func checkPrefixClassLimit(universe model.PrefixUniverse, maxClasses int) error {
	if maxClasses <= 0 || universe.Stats.ClassCount <= maxClasses {
		return nil
	}
	return fmt.Errorf("prefix universe class count %d exceeds --max-prefix-classes %d", universe.Stats.ClassCount, maxClasses)
}

func packetClasses(topo *model.Topology, universe model.PrefixUniverse, to string) []model.PrefixClassID {
	if addr, err := netip.ParseAddr(to); err == nil {
		return classesForAddr(universe, addr)
	}
	return classesForDestinationNode(topo, universe, to)
}

func failureClasses(topo *model.Topology, universe model.PrefixUniverse, q query.FailureCheck) []model.PrefixClassID {
	if !q.Prefix.IsZero() {
		return universe.ClassesMatching(model.ExactPrefixSet{Prefix: q.Prefix})
	}
	return packetClasses(topo, universe, q.To)
}

func classesForDestinationNode(topo *model.Topology, universe model.PrefixUniverse, to string) []model.PrefixClassID {
	if topo == nil {
		return nil
	}
	node, ok := topo.Node(to)
	if !ok {
		return nil
	}
	seen := map[model.PrefixClassID]bool{}
	var out []model.PrefixClassID
	for _, prefix := range node.Prefixes {
		for _, id := range universe.ClassesMatching(model.ExactPrefixSet{Prefix: prefix}) {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

func classesForAddr(universe model.PrefixUniverse, addr netip.Addr) []model.PrefixClassID {
	for _, class := range universe.Classes {
		if model.AddressSpaceContains(class.Space, addr) {
			return []model.PrefixClassID{class.ID}
		}
	}
	return nil
}

func prefixClass(universe model.PrefixUniverse, id model.PrefixClassID) (model.PrefixClass, bool) {
	for _, class := range universe.Classes {
		if class.ID == id {
			return class, true
		}
	}
	return model.PrefixClass{}, false
}

func classResult(universe model.PrefixUniverse, class model.PrefixClass, result Result) Result {
	id := class.ID
	result.PrefixClass = &PrefixClassMetadata{
		ClassID:           &id,
		ClassIDs:          []model.PrefixClassID{id},
		Space:             class.Space.String(),
		Spaces:            []string{class.Space.String()},
		MatchedPredicates: matchedPredicates(universe, class),
	}
	return result
}

func matchedPredicates(universe model.PrefixUniverse, class model.PrefixClass) []string {
	byID := map[model.PrefixPredicateID]string{}
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
