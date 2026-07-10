package controlplane

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// simpleTopoForPrune creates a minimal two-node topology for prune testing.
func simpleTopoForPrune() *model.Topology {
	return &model.Topology{
		Nodes: []model.Node{
			{Name: "router-a", ASN: 65001, Kind: model.KindFRR,
				Prefixes: []model.Prefix{model.MustPrefix("10.0.0.0/24")},
				Neighbors: []model.BGPNeighbor{
					{PeerNode: "router-b", RemoteAS: 65002, Activated: true},
				},
			},
			{Name: "router-b", ASN: 65002, Kind: model.KindFRR, Neighbors: []model.BGPNeighbor{
				{PeerNode: "router-a", RemoteAS: 65001, Activated: true},
			}},
		},
		Links: []model.Link{
			{Name: "a-b", A: "router-a", B: "router-b"},
		},
	}
}

func TestEngineWithMaxFailuresPrunesImpossibleConditions(t *testing.T) {
	topo := simpleTopoForPrune()
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatal(err)
	}
	// Enable pruning with maxFailures=0 (any negated link causes pruning)
	engine := NewEngine(idx, nil, WithMaxFailures(0))

	// Simulate should complete successfully with pruning enabled
	if err := engine.Simulate(); err != nil {
		t.Fatalf("Simulate() with pruning enabled failed: %v", err)
	}

	// Verify routes are present
	nodeAEntries := engine.rib[model.NodeID("router-a")][model.NetworkInstanceDefault]
	if len(nodeAEntries) == 0 {
		t.Fatal("expected routes on router-a after simulation with pruning")
	}
}

func TestEngineDefaultNoPruning(t *testing.T) {
	topo := simpleTopoForPrune()
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatal(err)
	}
	// No option = pruning disabled
	engine := NewEngine(idx, nil)

	if err := engine.Simulate(); err != nil {
		t.Fatalf("Simulate() failed: %v", err)
	}

	// Count total RIB entries
	totalEntries := 0
	for _, byVRF := range engine.rib {
		for _, byPrefix := range byVRF {
			for _, routes := range byPrefix {
				totalEntries += len(routes)
			}
		}
	}
	if totalEntries == 0 {
		t.Fatal("expected at least some RIB entries")
	}
}

func TestEngineWithMaxFailuresNegativeDisablesPruning(t *testing.T) {
	topo := simpleTopoForPrune()
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(idx, nil, WithMaxFailures(-1))

	if err := engine.Simulate(); err != nil {
		t.Fatalf("Simulate() failed: %v", err)
	}
}

func TestEngineOptionsAreApplied(t *testing.T) {
	topo := simpleTopoForPrune()
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(idx, nil, WithMaxFailures(3))
	if engine.maxFailures != 3 {
		t.Fatalf("expected maxFailures=3, got %d", engine.maxFailures)
	}
}

// Test that the Prune callback correctly uses CheckPrune from bgp package
// by creating an Engine that would prune impossible conditions.
func TestEngineBgPropagationContextHasPruneCallback(t *testing.T) {
	topo := simpleTopoForPrune()
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatal(err)
	}
	engine := NewEngine(idx, nil, WithMaxFailures(1))

	// Verify the bgpPropagationContext has a Prune callback set
	ctx := engine.bgpPropagationContext()
	if ctx.Prune == nil {
		t.Fatal("expected Prune callback to be set when maxFailures >= 0")
	}
}
