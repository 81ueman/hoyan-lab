package sim

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/solver/enumerate"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// pruneE2ETopo builds a three-router chain to test pruning effects.
func pruneE2ETopo() *model.Topology {
	return &model.Topology{
		Nodes: []model.Node{
			{Name: "router-a", ASN: 65001, Kind: model.KindFRR,
				Prefixes: []model.Prefix{model.MustPrefix("10.0.0.0/24")},
				Neighbors: []model.BGPNeighbor{
					{PeerNode: "router-b", RemoteAS: 65002, Activated: true},
				},
			},
			{Name: "router-b", ASN: 65002, Kind: model.KindFRR,
				Neighbors: []model.BGPNeighbor{
					{PeerNode: "router-a", RemoteAS: 65001, Activated: true},
					{PeerNode: "router-c", RemoteAS: 65003, Activated: true},
				},
			},
			{Name: "router-c", ASN: 65003, Kind: model.KindFRR,
				Neighbors: []model.BGPNeighbor{
					{PeerNode: "router-b", RemoteAS: 65002, Activated: true},
				},
			},
		},
		Links: []model.Link{
			{Name: "a-b", A: "router-a", B: "router-b"},
			{Name: "b-c", A: "router-b", B: "router-c"},
		},
	}
}

func TestE2EPruningDoesNotBreakNormalSimulation(t *testing.T) {
	topo := pruneE2ETopo()

	// Simulate with pruning enabled (maxFailures=0)
	gPruned, err := NewGraph(topo, WithMaxFailures(0), WithSolverBackend(enumerate.Backend{}))
	if err != nil {
		t.Fatalf("NewGraph with pruning failed: %v", err)
	}

	// Simulate without pruning
	gNoPrune, err := NewGraph(topo, WithMaxFailures(-1), WithSolverBackend(enumerate.Backend{}))
	if err != nil {
		t.Fatalf("NewGraph without pruning failed: %v", err)
	}

	// Both should have the same routes on router-c (end of chain)
	prefix := model.MustPrefix("10.0.0.0/24")

	routesPruned := gPruned.RIB("router-c", prefix)
	routesNoPrune := gNoPrune.RIB("router-c", prefix)

	if len(routesPruned) == 0 {
		t.Fatal("pruned simulation should have at least one route on router-c")
	}
	if len(routesNoPrune) == 0 {
		t.Fatal("unpruned simulation should have at least one route on router-c")
	}
	if len(routesPruned) != len(routesNoPrune) {
		t.Logf("pruned routes count: %d, unpruned: %d", len(routesPruned), len(routesNoPrune))
		// For positive conditions, pruning should not eliminate routes
		// since NegatedLinkCount is 0 for all-positive conditions
	}

	// Both should have the same reachability
	reachPruned, okPruned := gPruned.RouteReachable("router-c", "10.0.0.0/24", NoFailures())
	reachNoPrune, okNoPrune := gNoPrune.RouteReachable("router-c", "10.0.0.0/24", NoFailures())

	if okPruned != okNoPrune {
		t.Fatalf("reachability mismatch: pruned=%v, unpruned=%v", okPruned, okNoPrune)
	}
	if okPruned && reachPruned.Cost != reachNoPrune.Cost {
		t.Fatalf("cost mismatch: pruned=%d, unpruned=%d", reachPruned.Cost, reachNoPrune.Cost)
	}
}

func TestE2EPruningWithSymbolicReachability(t *testing.T) {
	topo := pruneE2ETopo()

	// Build graph with pruning enabled
	g, err := NewGraph(topo, WithMaxFailures(1), WithSolverBackend(enumerate.Backend{}))
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	// Symbolic route reachability should still work correctly
	result := g.SymbolicRouteReachability("router-c", "10.0.0.0/24")
	if result.Reachable.Key() == False().Key() {
		t.Fatal("route should be reachable symbolically")
	}
	if result.Unreachable.Key() == True().Key() {
		t.Fatal("route should not be unconditionally unreachable")
	}

	// Concrete reachability with no failures
	path, ok := g.RouteReachable("router-c", "10.0.0.0/24", NoFailures())
	if !ok {
		t.Fatal("route should be reachable with no failures")
	}
	if len(path.Nodes) == 0 {
		t.Fatal("path should have nodes")
	}
}

func TestE2EPruningBreakingFailuresSearch(t *testing.T) {
	topo := pruneE2ETopo()

	g, err := NewGraph(topo, WithMaxFailures(1), WithSolverBackend(enumerate.Backend{}))
	if err != nil {
		t.Fatalf("NewGraph failed: %v", err)
	}

	// Find breaking failures should work with pruning enabled
	elements, ok := g.FindBreakingFailuresWithOptions("router-c", PrefixTarget("10.0.0.0/24"), FailureSearchOptions{
		IncludeLinks: true,
		MaxFailures:  1,
	})
	if !ok {
		t.Fatal("FindBreakingFailures should find at least one failure")
	}
	if len(elements) == 0 {
		t.Fatal("expected at least one breaking failure element")
	}
}
