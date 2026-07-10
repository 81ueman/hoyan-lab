package traffic

import (
	"math"
	"net/netip"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestTrafficSimulatorBasic(t *testing.T) {
	sim := NewTrafficSimulator(DefaultSimulatorConfig())

	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router3", Weight: 1.0}}},
		},
	}

	packetClass := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	linkLoads := sim.SimulateClass("router1", packetClass, fibs, 1000)
	if len(linkLoads) != 2 {
		t.Errorf("expected 2 links, got %d", len(linkLoads))
	}
	if linkLoads["router1->router2"] != 1000 {
		t.Errorf("expected 1000 on router1->router2, got %d", linkLoads["router1->router2"])
	}
	if linkLoads["router2->router3"] != 1000 {
		t.Errorf("expected 1000 on router2->router3, got %d", linkLoads["router2->router3"])
	}
}

func TestTrafficSimulatorWithFlows(t *testing.T) {
	sim := NewTrafficSimulator(SimulatorConfig{ECMPMode: ECMPModeHash})

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

	packetClass := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	flows := []Flow{
		{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp", SrcPort: 10001, DstPort: 80},
		{SrcIP: netip.MustParseAddr("10.0.0.3"), DstIP: netip.MustParseAddr("10.0.0.4"), Protocol: "udp", SrcPort: 53, DstPort: 443},
	}

	linkLoads := sim.SimulateClassWithFlows("router1", packetClass, fibs, flows)
	if len(linkLoads) == 0 {
		t.Errorf("expected non-zero link loads")
	}
	// Each flow traverses 2 links, carrying 1500 bytes per hop
	// Total = flows * hops * 1500 = 2 * 2 * 1500
	totalBytes := uint64(0)
	for _, bytes := range linkLoads {
		totalBytes += bytes
	}
	expectedTotal := uint64(len(flows)) * 2 * 1500
	if totalBytes != expectedTotal {
		t.Errorf("expected total bytes %d, got %d", expectedTotal, totalBytes)
	}
}

func TestMultiSnapshotSameTopology(t *testing.T) {
	sim := NewTrafficSimulator(DefaultSimulatorConfig())

	packetClass := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	// Same FIB for both snapshots
	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router3", Weight: 1.0}}},
		},
	}

	snapshots := []SnapshotDef{
		{Label: "baseline", FIBs: fibs, TotalBytes: 1000},
		{Label: "after-update", FIBs: fibs, TotalBytes: 1000},
	}

	result := sim.SimulateMultiSnapshot("router1", packetClass, snapshots, nil)
	if len(result.Snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(result.Snapshots))
	}
	if result.Snapshots[0].Label != "baseline" {
		t.Errorf("expected first snapshot label 'baseline', got %s", result.Snapshots[0].Label)
	}
	if result.Snapshots[1].Label != "after-update" {
		t.Errorf("expected second snapshot label 'after-update', got %s", result.Snapshots[1].Label)
	}

	// No diffs expected since they use the same FIB and traffic load
	if len(result.Diffs) != 0 {
		t.Errorf("expected 0 diffs for identical snapshots, got %d", len(result.Diffs))
	}
}

func TestMultiSnapshotTopologyChange(t *testing.T) {
	sim := NewTrafficSimulator(DefaultSimulatorConfig())

	packetClass := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	// Snapshot 1: router1 -> router2 -> router3
	fibs1 := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router3", Weight: 1.0}}},
		},
	}

	// Snapshot 2: router1 -> router4 -> router3 (rerouted)
	fibs2 := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router4", Weight: 1.0}}},
		},
		"router4": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router3", Weight: 1.0}}},
		},
	}

	snapshots := []SnapshotDef{
		{Label: "baseline", FIBs: fibs1, TotalBytes: 1000},
		{Label: "reroute", FIBs: fibs2, TotalBytes: 1000},
	}

	result := sim.SimulateMultiSnapshot("router1", packetClass, snapshots, nil)
	if len(result.Snapshots) != 2 {
		t.Errorf("expected 2 snapshots, got %d", len(result.Snapshots))
	}

	// Should have diffs for changed links
	if len(result.Diffs) == 0 {
		t.Errorf("expected non-zero diffs for changed topology")
	}

	// Check specific diffs
	for _, diff := range result.Diffs {
		switch diff.LinkName {
		case "router1->router2":
			if diff.Before != 1000 || diff.After != 0 {
				t.Errorf("router1->router2: expected before=1000 after=0, got before=%d after=%d", diff.Before, diff.After)
			}
			if diff.ChangePct != -100 {
				t.Errorf("router1->router2: expected -100%% change, got %f%%", diff.ChangePct)
			}
		case "router1->router4":
			if diff.Before != 0 || diff.After != 1000 {
				t.Errorf("router1->router4: expected before=0 after=1000, got before=%d after=%d", diff.Before, diff.After)
			}
			if !math.IsInf(diff.ChangePct, 1) {
				t.Errorf("router1->router4: expected +Inf change, got %f", diff.ChangePct)
			}
		case "router2->router3":
			if diff.Before != 1000 || diff.After != 0 {
				t.Errorf("router2->router3: expected before=1000 after=0, got before=%d after=%d", diff.Before, diff.After)
			}
			if diff.ChangePct != -100 {
				t.Errorf("router2->router3: expected -100%% change, got %f%%", diff.ChangePct)
			}
		case "router4->router3":
			if diff.Before != 0 || diff.After != 1000 {
				t.Errorf("router4->router3: expected before=0 after=1000, got before=%d after=%d", diff.Before, diff.After)
			}
			if !math.IsInf(diff.ChangePct, 1) {
				t.Errorf("router4->router3: expected +Inf change, got %f", diff.ChangePct)
			}
		}
	}
}

func TestComputeDiffsNoSnapshots(t *testing.T) {
	diffs := ComputeDiffs(nil)
	if len(diffs) != 0 {
		t.Errorf("expected 0 diffs for nil, got %d", len(diffs))
	}
}

func TestComputeDiffsSingleSnapshot(t *testing.T) {
	diffs := ComputeDiffs([]model.TrafficResult{
		{Label: "single", LinkLoads: map[string]uint64{"link1": 100}},
	})
	if len(diffs) != 0 {
		t.Errorf("expected 0 diffs for single snapshot, got %d", len(diffs))
	}
}

func TestSimulateParallel(t *testing.T) {
	sim := NewTrafficSimulator(DefaultSimulatorConfig())

	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router3", Weight: 1.0}}},
		},
	}

	ecs := []FlowEquivalenceClass{
		{
			Key: FlowEquivalenceClassKey{
				PrefixClassID: 1,
			},
			DstSet:     model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
			TotalBytes: 1000,
		},
		{
			Key: FlowEquivalenceClassKey{
				PrefixClassID: 2,
			},
			DstSet:     model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
			TotalBytes: 2000,
		},
	}

	result := sim.SimulateParallel("router1", ecs, fibs, 2)
	if len(result) == 0 {
		t.Errorf("expected non-zero links")
	}
	// Total bytes: 1000 + 2000 = 3000 on each of 2 links
	total := uint64(0)
	for _, bytes := range result {
		total += bytes
	}
	expectedTotal := uint64(3000 * 2) // 3000 per class * 2 links
	if total != expectedTotal {
		t.Errorf("expected total bytes %d, got %d", expectedTotal, total)
	}
}

func TestSimulateParallelWithFlows(t *testing.T) {
	sim := NewTrafficSimulator(SimulatorConfig{ECMPMode: ECMPModeHash})

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

	ecs := []FlowEquivalenceClass{
		{
			Key: FlowEquivalenceClassKey{
				PrefixClassID: 1,
			},
			DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
			Flows: []SampledFlow{
				{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.100"), Protocol: "tcp", SrcPort: 80, DstPort: 80}, DSCP: 0},
				{Flow: Flow{SrcIP: netip.MustParseAddr("10.0.0.2"), DstIP: netip.MustParseAddr("10.0.0.100"), Protocol: "tcp", SrcPort: 81, DstPort: 80}, DSCP: 0},
			},
		},
	}

	result := sim.SimulateParallel("router1", ecs, fibs, 2)
	if len(result) == 0 {
		t.Errorf("expected non-zero links for parallel flow simulation")
	}
}

func TestSimulateParallelZeroWorkers(t *testing.T) {
	sim := NewTrafficSimulator(DefaultSimulatorConfig())

	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
		},
	}

	ecs := []FlowEquivalenceClass{
		{
			Key:        FlowEquivalenceClassKey{PrefixClassID: 1},
			DstSet:     model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
			TotalBytes: 500,
		},
	}

	// 0 workers should default to GOMAXPROCS
	result := sim.SimulateParallel("router1", ecs, fibs, 0)
	if len(result) == 0 {
		t.Errorf("expected non-zero links")
	}
}
