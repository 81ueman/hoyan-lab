package sim

import (
	"github.com/81ueman/hoyan-lab/internal/core/netaddr"
	"github.com/81ueman/hoyan-lab/internal/core/predicate"
	"reflect"
	"sort"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/core/failure"
	"github.com/81ueman/hoyan-lab/internal/engine/controlplane"
	"github.com/81ueman/hoyan-lab/internal/engine/space"
)

func TestCollectRIBPrefixPredicatesIncludesModeledRIBOnlyPrefix(t *testing.T) {
	prefix := netaddr.MustPrefix("192.0.2.0/24")
	graph := &Graph{rib: map[string]map[string]map[string][]RIBEntry{
		"r1": {
			"default": {
				prefix.String(): {{
					NLRI:       controlplane.RouteNLRI{Prefix: prefix},
					Provenance: controlplane.RouteProvenance{OriginNode: "origin"},
				}},
			},
		},
	}}

	predicates := CollectRIBPrefixPredicates(graph)
	sources := predicateSources(predicates)
	if got, want := sources, []string{"rib:r1:default:192.0.2.0/24:origin=origin"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sources = %#v, want %#v", got, want)
	}

	universe, err := space.BuildPrefixUniverseFromPredicates(predicates)
	if err != nil {
		t.Fatalf("BuildPrefixUniverseFromPredicates() error = %v", err)
	}
	if _, ok := universe.ClassForPrefix(prefix); !ok {
		t.Fatalf("universe did not include RIB-only prefix %s", prefix)
	}
}

func TestCollectFIBPrefixPredicatesIncludesModeledFIBOnlyPrefix(t *testing.T) {
	prefix := netaddr.MustPrefix("198.51.100.0/24")
	graph := &Graph{fib: map[string]map[string][]FIBEntry{
		"r1": {
			"default": {
				{
					Prefix:    prefix.NetIP(),
					NextHop:   "r2",
					Condition: failure.True(),
					GroupID:   "198.51.100.0/24#rank-0",
				},
			},
		},
	}}

	predicates := CollectFIBPrefixPredicates(graph)
	sources := predicateSources(predicates)
	if got, want := sources, []string{"fib:r1:default:198.51.100.0/24:group=198.51.100.0/24#rank-0"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("sources = %#v, want %#v", got, want)
	}

	universe, err := space.BuildPrefixUniverseFromPredicates(predicates)
	if err != nil {
		t.Fatalf("BuildPrefixUniverseFromPredicates() error = %v", err)
	}
	if _, ok := universe.ClassForPrefix(prefix); !ok {
		t.Fatalf("universe did not include FIB-only prefix %s", prefix)
	}
}

func TestRIBAndFIBPredicatesPreserveSourcesWithoutExtraClassBoundaries(t *testing.T) {
	prefix := netaddr.MustPrefix("203.0.113.0/24")
	graph := &Graph{
		rib: map[string]map[string]map[string][]RIBEntry{
			"r1": {
				"default": {
					prefix.String(): {{
						NLRI:       controlplane.RouteNLRI{Prefix: prefix},
						Provenance: controlplane.RouteProvenance{OriginNode: "dst"},
					}},
				},
			},
		},
		fib: map[string]map[string][]FIBEntry{
			"r1": {
				"default": {
					{Prefix: prefix.NetIP(), NextHop: "a", GroupID: "ecmp", Condition: failure.True()},
					{Prefix: prefix.NetIP(), NextHop: "b", GroupID: "ecmp", Condition: failure.True()},
				},
			},
		},
	}

	predicates := []space.PrefixPredicate{{Source: "route:dst", Set: predicate.ExactPrefixSet{Prefix: prefix}}}
	predicates = append(predicates, CollectRIBPrefixPredicates(graph)...)
	predicates = append(predicates, CollectFIBPrefixPredicates(graph)...)
	universe, err := space.BuildPrefixUniverseFromPredicates(predicates)
	if err != nil {
		t.Fatalf("BuildPrefixUniverseFromPredicates() error = %v", err)
	}
	if got, want := len(universe.Classes), 1; got != want {
		t.Fatalf("len(Classes) = %d, want %d", got, want)
	}
	id, ok := universe.ClassForPrefix(prefix)
	if !ok {
		t.Fatalf("ClassForPrefix() did not find %s", prefix)
	}
	class := universe.Classes[id]
	sources := sourcesForClass(universe, class)
	want := []string{
		"fib:r1:default:203.0.113.0/24:group=ecmp",
		"rib:r1:default:203.0.113.0/24:origin=dst",
		"route:dst",
	}
	if !reflect.DeepEqual(sources, want) {
		t.Fatalf("matched sources = %#v, want %#v", sources, want)
	}
}

func predicateSources(predicates []space.PrefixPredicate) []string {
	out := make([]string, 0, len(predicates))
	for _, predicate := range predicates {
		out = append(out, predicate.Source)
	}
	sort.Strings(out)
	return out
}

func sourcesForClass(universe space.PrefixUniverse, class space.PrefixClass) []string {
	byID := map[space.PrefixPredicateID]string{}
	for _, predicate := range universe.Predicates {
		byID[predicate.ID] = predicate.Source
	}
	seen := map[string]bool{}
	var out []string
	for _, id := range class.MatchingPredicates {
		source := byID[id]
		if source == "" || seen[source] {
			continue
		}
		seen[source] = true
		out = append(out, source)
	}
	sort.Strings(out)
	return out
}
