package sim

import (
	"path/filepath"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/solver/enumerate"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

func TestDetectRacingDetectsBgpRacing(t *testing.T) {
	topo, err := topology.LoadTopology(filepath.Join("..", "..", "..", "labs", "bgp-racing", "hoyan.clab.yml"))
	if err != nil {
		t.Fatalf("LoadTopology() error = %v", err)
	}
	g, err := NewGraph(topo, WithSolverBackend(enumerate.Backend{}))
	if err != nil {
		t.Fatalf("NewGraph() error = %v", err)
	}

	prefix := model.MustPrefix("10.0.1.0/24")
	result, err := g.DetectRacing(prefix)
	if err != nil {
		t.Fatalf("DetectRacing() error = %v", err)
	}
	if !result.Racing {
		t.Errorf("DetectRacing(%s): expected Racing=true, got false", prefix.String())
	}
	if len(result.Routers) == 0 {
		t.Fatal("DetectRacing(): expected non-empty Routers")
	}

	// Verify at least one router has racing_found=true.
	racingRouters := 0
	for _, rr := range result.Routers {
		if rr.RacingFound {
			racingRouters++
		}
	}
	if racingRouters == 0 {
		t.Error("DetectRacing(): expected at least one router with RacingFound=true")
	}
}

func TestDetectRacingNoFalsePositiveForStablePrefix(t *testing.T) {
	topo, err := topology.LoadTopology(filepath.Join("..", "..", "..", "labs", "bgp-ospf-igp", "hoyan.clab.yml"))
	if err != nil {
		t.Fatalf("LoadTopology() error = %v", err)
	}
	g, err := NewGraph(topo, WithSolverBackend(enumerate.Backend{}))
	if err != nil {
		t.Fatalf("NewGraph() error = %v", err)
	}

	results := g.DetectAllRacing()

	// The bgp-ospf-igp lab should have no racing (no prefixes with multiple BGP origins).
	if len(results) > 0 {
		t.Errorf("DetectAllRacing(): expected 0 racing results for bgp-ospf-igp, got %d", len(results))
		for _, r := range results {
			if r.Racing {
				t.Errorf("  unexpected racing for prefix %s", r.Prefix)
			}
		}
	}
}
