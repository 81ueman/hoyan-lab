package sim

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/solver/enumerate"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// benchmarkTopo builds a larger topology suitable for benchmarking pruning effects.
// A chain topology where routes propagate over multiple hops.
func benchmarkTopo() *model.Topology {
	nodes := make([]model.Node, 0, 6)
	links := make([]model.Link, 0, 5)

	// Router-a is origin, rest form a chain
	for i := 0; i < 6; i++ {
		name := string(rune('a' + i))
		asn := 65001 + uint32(i)
		neighbors := make([]model.BGPNeighbor, 0, 2)
		if i > 0 {
			prev := string(rune('a' + i - 1))
			neighbors = append(neighbors, model.BGPNeighbor{
				PeerNode: prev, RemoteAS: 65001 + uint32(i-1), Activated: true,
			})
		}
		if i < 5 {
			next := string(rune('a' + i + 1))
			neighbors = append(neighbors, model.BGPNeighbor{
				PeerNode: next, RemoteAS: 65001 + uint32(i+1), Activated: true,
			})
		}
		prefixes := []model.Prefix{}
		if i == 0 {
			prefixes = []model.Prefix{model.MustPrefix("10.0.0.0/24")}
		}
		nodes = append(nodes, model.Node{
			Name: name, ASN: asn, Kind: model.KindFRR,
			Prefixes:  prefixes,
			Neighbors: neighbors,
		})
	}
	for i := 0; i < 5; i++ {
		a := string(rune('a' + i))
		b := string(rune('a' + i + 1))
		links = append(links, model.Link{Name: a + "-" + b, A: a, B: b})
	}

	return &model.Topology{Nodes: nodes, Links: links}
}

func BenchmarkGraphConstructionWithoutPruning(b *testing.B) {
	topo := benchmarkTopo()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g, err := NewGraph(topo, WithMaxFailures(-1), WithSolverBackend(enumerate.Backend{}))
		if err != nil {
			b.Fatalf("NewGraph failed: %v", err)
		}
		_ = g
	}
}

func BenchmarkGraphConstructionWithPruning(b *testing.B) {
	topo := benchmarkTopo()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		g, err := NewGraph(topo, WithMaxFailures(0), WithSolverBackend(enumerate.Backend{}))
		if err != nil {
			b.Fatalf("NewGraph failed: %v", err)
		}
		_ = g
	}
}

func BenchmarkSymbolicReachabilityWithoutPruning(b *testing.B) {
	topo := benchmarkTopo()
	g, err := NewGraph(topo, WithMaxFailures(-1), WithSolverBackend(enumerate.Backend{}))
	if err != nil {
		b.Fatalf("NewGraph failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.SymbolicRouteReachability("router-f", "10.0.0.0/24")
	}
}

func BenchmarkSymbolicReachabilityWithPruning(b *testing.B) {
	topo := benchmarkTopo()
	g, err := NewGraph(topo, WithMaxFailures(0), WithSolverBackend(enumerate.Backend{}))
	if err != nil {
		b.Fatalf("NewGraph failed: %v", err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = g.SymbolicRouteReachability("router-f", "10.0.0.0/24")
	}
}
