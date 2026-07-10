package traffic

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestWhatIfSimulatorBasic(t *testing.T) {
	ws := NewWhatIfSimulator(DefaultSimulatorConfig())

	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router3", Weight: 1.0}}},
		},
	}
	pc := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	cache := NewTDGCache()
	ec := FlowEquivalenceClass{
		Key:        FlowEquivalenceClassKeyFromPacketClass(pc, DSCPDefault),
		DstSet:     pc.DstSet,
		TotalBytes: 1000,
	}

	// Simulate base case (no failure)
	failSet := failure.None()
	result := ws.Simulate(failSet, []FlowEquivalenceClass{ec}, cache, fibs)
	if result == nil {
		t.Fatal("Simulate returned nil")
	}

	// Should have link loads for the two links
	if len(result.LinkLoads) != 2 {
		t.Errorf("expected 2 links with load, got %d", len(result.LinkLoads))
	}
	if len(result.Diffs) != 0 {
		t.Errorf("expected 0 diffs for base case, got %d", len(result.Diffs))
	}
}

func TestWhatIfSimulatorLinkFailure(t *testing.T) {
	ws := NewWhatIfSimulator(DefaultSimulatorConfig())

	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router3", Weight: 1.0}}},
		},
	}
	pc := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	cache := NewTDGCache()
	ec := FlowEquivalenceClass{
		Key:        FlowEquivalenceClassKeyFromPacketClass(pc, DSCPDefault),
		DstSet:     pc.DstSet,
		TotalBytes: 1000,
	}

	// First simulate base to build the cache
	_ = cache.GetOrBuild("router1", pc, fibs)

	// Now simulate with a link failure
	failSet := failure.Links("router2->router3")
	result := ws.Simulate(failSet, []FlowEquivalenceClass{ec}, cache, fibs)
	if result == nil {
		t.Fatal("Simulate returned nil")
	}

	// router2->router3 should have no traffic
	if ll, ok := result.LinkLoads["router2->router3"]; ok && ll.Bytes > 0 {
		t.Errorf("router2->router3 should have 0 bytes after failure, got %d", ll.Bytes)
	}

	// Should have diffs since the failure changes the topology
	if len(result.Diffs) == 0 {
		t.Errorf("expected non-zero diffs for link failure")
	}
}

func TestWhatIfSimulatorNoBase(t *testing.T) {
	ws := NewWhatIfSimulator(DefaultSimulatorConfig())

	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
	}
	pc := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	cache := NewTDGCache()
	ec := FlowEquivalenceClass{
		Key:        FlowEquivalenceClassKeyFromPacketClass(pc, DSCPDefault),
		DstSet:     pc.DstSet,
		TotalBytes: 1000,
	}

	// Simulate with a failure before building base - should build base automatically
	failSet := failure.Links("nonexistent")
	result := ws.Simulate(failSet, []FlowEquivalenceClass{ec}, cache, fibs)
	if result == nil {
		t.Fatal("Simulate returned nil")
	}
	if len(result.LinkLoads) == 0 {
		t.Errorf("expected non-zero link loads")
	}
}

func TestWhatIfSimulatorECMPLoadShift(t *testing.T) {
	ws := NewWhatIfSimulator(DefaultSimulatorConfig())

	fibs := FIBTable{
		"router1": {
			{
				Prefix: model.MustPrefix("10.0.0.0/24").NetIP(),
				NextHops: []TrafficNextHop{
					{Node: "router2", Weight: 0.5},
					{Node: "router3", Weight: 0.5},
				},
			},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router4", Weight: 1.0}}},
		},
		"router3": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router4", Weight: 1.0}}},
		},
	}
	pc := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	cache := NewTDGCache()
	ec := FlowEquivalenceClass{
		Key:        FlowEquivalenceClassKeyFromPacketClass(pc, DSCPDefault),
		DstSet:     pc.DstSet,
		TotalBytes: 1000,
	}

	// Build base
	_ = cache.GetOrBuild("router1", pc, fibs)

	// Fail router2 - all traffic should shift to router3
	failSet := failure.Nodes("router2")
	result := ws.Simulate(failSet, []FlowEquivalenceClass{ec}, cache, fibs)

	// router1->router3 should carry all 1000 bytes
	ll, ok := result.LinkLoads["router1->router3"]
	if !ok {
		t.Errorf("expected router1->router3 to have traffic")
	} else if ll.Bytes != 1000 {
		t.Errorf("expected 1000 bytes on router1->router3, got %d", ll.Bytes)
	}

	// Should see a diff for the load shift
	foundShift := false
	for _, diff := range result.Diffs {
		if diff.LinkName == "router1->router3" && diff.Delta > 0 {
			foundShift = true
			break
		}
	}
	if !foundShift {
		t.Errorf("expected a positive delta diff for router1->router3 showing load shift")
	}
}

func TestWhatIfSimulatorFailureInResult(t *testing.T) {
	ws := NewWhatIfSimulator(DefaultSimulatorConfig())

	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
	}
	pc := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	cache := NewTDGCache()
	ec := FlowEquivalenceClass{
		Key:        FlowEquivalenceClassKeyFromPacketClass(pc, DSCPDefault),
		DstSet:     pc.DstSet,
		TotalBytes: 1000,
	}

	failSet := failure.Links("router1->router2")
	result := ws.Simulate(failSet, []FlowEquivalenceClass{ec}, cache, fibs)

	// Verify the failure set is recorded in the result
	if !result.Failure.Links["router1->router2"] {
		t.Errorf("expected link router1->router2 in failure set")
	}
}
