package traffic

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/solver"
)

func TestKFailAnalyzerBasic(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			{Name: "router1", Role: "core"},
			{Name: "router2", Role: "core"},
			{Name: "router3", Role: "core"},
		},
		Links: []model.Link{
			{Name: "router1->router2", A: "router1", B: "router2", Role: "core"},
			{Name: "router2->router3", A: "router2", B: "router3", Role: "core"},
		},
	}

	// Build FIB table
	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router3", Weight: 1.0}}},
		},
	}

	analyzer := NewKFailAnalyzer(DefaultSimulatorConfig())
	pc := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	ec := FlowEquivalenceClass{
		Key:        FlowEquivalenceClassKeyFromPacketClass(pc, DSCPDefault),
		DstSet:     pc.DstSet,
		TotalBytes: 1000,
	}

	// Test with simple threshold (no link should be overloaded in base case with small traffic)
	result := analyzer.Analyze("router1", topo, fibs, []FlowEquivalenceClass{ec}, 80, 1)
	if result == nil {
		t.Fatal("Analyze returned nil")
	}

	// With only 1000 bytes, nothing should be overloaded
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings for low traffic, got %d", len(result.Findings))
	}
}

func TestKFailAnalyzerWithOverload(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			{Name: "router1", Role: "core"},
			{Name: "router2", Role: "core"},
			{Name: "router3", Role: "core"},
		},
		Links: []model.Link{
			{Name: "router1->router2", A: "router1", B: "router2", Role: "core", Bandwidth: 1000},    // very low bandwidth
			{Name: "router2->router3", A: "router2", B: "router3", Role: "core", Bandwidth: 1000000}, // high bandwidth
		},
	}

	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router3", Weight: 1.0}}},
		},
	}

	analyzer := NewKFailAnalyzer(DefaultSimulatorConfig())
	pc := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	ec := FlowEquivalenceClass{
		Key:        FlowEquivalenceClassKeyFromPacketClass(pc, DSCPDefault),
		DstSet:     pc.DstSet,
		TotalBytes: 500, // 500 bytes on router1->router2 with capacity of 1000 = 50%
	}

	// Threshold 80% - nothing should be over threshold since 50% < 80%
	result := analyzer.Analyze("router1", topo, fibs, []FlowEquivalenceClass{ec}, 80, 1)
	if result == nil {
		t.Fatal("Analyze returned nil")
	}
	if len(result.Findings) != 0 {
		t.Errorf("expected 0 findings for 50%% load < 80%% threshold, got %d", len(result.Findings))
	}

	// Threshold 40% - router1->router2 should be over threshold (50% > 40%)
	result = analyzer.Analyze("router1", topo, fibs, []FlowEquivalenceClass{ec}, 40, 1)
	if len(result.Findings) == 0 {
		t.Errorf("expected findings for 50%% load > 40%% threshold")
	}
}

func TestKFailAnalyzerFindElementCombo(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			{Name: "router1", Role: "core"},
			{Name: "router2", Role: "core"},
			{Name: "router3", Role: "core"},
		},
		Links: []model.Link{
			{Name: "router1->router2", A: "router1", B: "router2", Role: "core"},
		},
	}

	// Test failure.SearchElements and failure.FindElementCombo
	elements := failure.SearchElements(topo, failure.SearchOptions{
		IncludeLinks: true,
		IncludeNodes: true,
		MaxFailures:  1,
	})

	if len(elements) == 0 {
		t.Errorf("expected at least 1 failure element")
	}

	// FindElementCombo should find at least one combination
	found := false
	failure.FindElementCombo(elements, 1, 0, nil, func(combo []solver.FailureElement) bool {
		found = true
		return false // continue searching
	})
	if !found {
		t.Errorf("FindElementCombo should find at least one combo")
	}
}

func TestKFailAnalyzerResultFormat(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			{Name: "router1", Role: "core"},
			{Name: "router2", Role: "core"},
			{Name: "router3", Role: "core"},
		},
		Links: []model.Link{
			{Name: "router1->router2", A: "router1", B: "router2", Role: "core", Bandwidth: 1000},
		},
	}

	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router3", Weight: 1.0}}},
		},
	}

	analyzer := NewKFailAnalyzer(DefaultSimulatorConfig())
	pc := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	ec := FlowEquivalenceClass{
		Key:        FlowEquivalenceClassKeyFromPacketClass(pc, DSCPDefault),
		DstSet:     pc.DstSet,
		TotalBytes: 900, // 90% of bandwidth (capacity 1000)
	}

	// Should find that router1->router2 is overloaded with k=0 (no additional failure needed)
	result := analyzer.Analyze("router1", topo, fibs, []FlowEquivalenceClass{ec}, 80, 1)
	if len(result.Findings) == 0 {
		t.Errorf("expected finding for base overload")
	}
	if result.Findings[0].K != 0 {
		t.Errorf("expected k=0 for base overload, got k=%d", result.Findings[0].K)
	}
}
