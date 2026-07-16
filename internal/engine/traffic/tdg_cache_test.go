package traffic

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestTDGCacheGetOrBuild(t *testing.T) {
	cache := NewTDGCache()

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

	// First call should build the TDG
	tdg1 := cache.GetOrBuild("router1", pc, fibs)
	if tdg1 == nil {
		t.Fatal("GetOrBuild returned nil")
	}
	if tdg1.Root == nil || tdg1.Root.Node != "router1" {
		t.Errorf("expected root router1, got %v", tdg1.Root)
	}

	// Second call with same parameters should return cached TDG (same pointer)
	tdg2 := cache.GetOrBuild("router1", pc, fibs)
	if tdg1 != tdg2 {
		t.Errorf("expected cached TDG, got different pointer")
	}
}

func TestTDGCacheDifferentIngress(t *testing.T) {
	cache := NewTDGCache()

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

	// Different ingress nodes should produce different cache entries
	tdg1 := cache.GetOrBuild("router1", pc, fibs)
	tdg2 := cache.GetOrBuild("router2", pc, fibs)
	if tdg1 == tdg2 {
		t.Errorf("expected different TDG for different ingress")
	}
}

func TestTDGCacheDifferentPacketClass(t *testing.T) {
	cache := NewTDGCache()

	fibs := FIBTable{
		"router1": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router2", Weight: 1.0}}},
			{Prefix: model.MustPrefix("192.168.0.0/16").NetIP(), NextHops: []TrafficNextHop{{Node: "router4", Weight: 1.0}}},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router3", Weight: 1.0}}},
		},
		"router4": {
			{Prefix: model.MustPrefix("192.168.0.0/16").NetIP(), NextHops: []TrafficNextHop{{Node: "router5", Weight: 1.0}}},
		},
	}

	pc1 := model.PacketClass{
		PrefixClassID: 1,
		DstSet:        model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}
	pc2 := model.PacketClass{
		PrefixClassID: 2,
		DstSet:        model.ExactPrefixSet{Prefix: model.MustPrefix("192.168.0.0/16")},
	}

	tdg1 := cache.GetOrBuild("router1", pc1, fibs)
	tdg2 := cache.GetOrBuild("router1", pc2, fibs)
	if tdg1 == tdg2 {
		t.Errorf("expected different TDG for different packet class")
	}
}

func TestApplyFailureRemovesLink(t *testing.T) {
	cache := NewTDGCache()

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

	tdg := cache.GetOrBuild("router1", pc, fibs)
	if len(tdg.Edges) != 2 {
		t.Fatalf("expected 2 edges in base TDG, got %d", len(tdg.Edges))
	}

	// Apply failure: link router2->router3 is down
	failSet := failure.Links("router2->router3")
	failed := cache.ApplyFailure(tdg, failSet)
	if failed == tdg {
		t.Errorf("ApplyFailure should return a new clone, not the original")
	}

	// Verify the failed edge is removed
	for _, edge := range failed.Edges {
		if edge.From.Node == "router2" && edge.To.Node == "router3" {
			t.Errorf("edge router2->router3 should have been removed")
		}
	}

	// Original TDG should be unchanged
	origEdgeCount := 0
	for _, edge := range tdg.Edges {
		if edge.From.Node == "router2" && edge.To.Node == "router3" {
			origEdgeCount++
		}
	}
	if origEdgeCount != 1 {
		t.Errorf("original TDG should still have router2->router3 edge")
	}
}

func TestApplyFailureNodeFailure(t *testing.T) {
	cache := NewTDGCache()

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

	tdg := cache.GetOrBuild("router1", pc, fibs)

	// Apply failure: node router2 is down
	failSet := failure.Nodes("router2")
	failed := cache.ApplyFailure(tdg, failSet)

	// All edges to/from router2 should be removed
	for _, edge := range failed.Edges {
		if edge.From.Node == "router2" || edge.To.Node == "router2" {
			t.Errorf("edge involving failed node router2 should be removed: %s->%s",
				edge.From.Node, edge.To.Node)
		}
	}
}

func TestApplyFailureECMPRebalancing(t *testing.T) {
	cache := NewTDGCache()

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

	tdg := cache.GetOrBuild("router1", pc, fibs)

	// Fail router2 - router1 should now send 100% to router3
	failSet := failure.Nodes("router2")
	failed := cache.ApplyFailure(tdg, failSet)

	// Check that remaining edge router1->router3 has weight 1.0
	for _, edge := range failed.Edges {
		if edge.From.Node == "router1" && edge.To.Node == "router3" {
			if edge.Weight != 1.0 {
				t.Errorf("expected rebalanced weight 1.0 for router1->router3, got %f", edge.Weight)
			}
		}
		if edge.From.Node == "router1" && edge.To.Node == "router2" {
			t.Errorf("router1->router2 should be removed after node failure")
		}
	}
}

func TestApplyFailureAllECMPMembersFailMarksSink(t *testing.T) {
	cache := NewTDGCache()

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
		"router4": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router5", Weight: 1.0}}},
		},
	}
	pc := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	tdg := cache.GetOrBuild("router1", pc, fibs)

	// Fail both router2 and router3 – router1 loses ALL out-edges
	failSet := failure.Nodes("router2", "router3")
	failed := cache.ApplyFailure(tdg, failSet)

	// router1 should be marked as a sink since all ECMP members failed
	found := false
	for _, sink := range failed.Sinks {
		if sink.Node == "router1" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected router1 to be marked as sink after all ECMP members failed, sinks: %v", sinkNames(failed.Sinks))
	}

	// router1 should have no outgoing edges
	if len(failed.OutEdges("router1")) != 0 {
		t.Errorf("expected router1 to have no out-edges, got %d", len(failed.OutEdges("router1")))
	}
}

func TestApplyFailureAllECMPMembersFailNotSinkIfSomeRemain(t *testing.T) {
	cache := NewTDGCache()

	fibs := FIBTable{
		"router1": {
			{
				Prefix: model.MustPrefix("10.0.0.0/24").NetIP(),
				NextHops: []TrafficNextHop{
					{Node: "router2", Weight: 0.25},
					{Node: "router3", Weight: 0.25},
					{Node: "router4", Weight: 0.25},
					{Node: "router5", Weight: 0.25},
				},
			},
		},
		"router2": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router6", Weight: 1.0}}},
		},
		"router3": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router6", Weight: 1.0}}},
		},
		"router4": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router6", Weight: 1.0}}},
		},
		"router5": {
			{Prefix: model.MustPrefix("10.0.0.0/24").NetIP(), NextHops: []TrafficNextHop{{Node: "router6", Weight: 1.0}}},
		},
	}
	pc := model.PacketClass{
		DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")},
	}

	tdg := cache.GetOrBuild("router1", pc, fibs)

	// Fail router2 and router3 – router1 still has router4 and router5
	failSet := failure.Nodes("router2", "router3")
	failed := cache.ApplyFailure(tdg, failSet)

	// router1 should NOT be a sink
	for _, sink := range failed.Sinks {
		if sink.Node == "router1" {
			t.Errorf("router1 should NOT be a sink when some ECMP members remain")
		}
	}

	// Remaining edges should be rebalanced to sum to 1.0
	outEdges := failed.OutEdges("router1")
	if len(outEdges) != 2 {
		t.Fatalf("expected 2 remaining out-edges, got %d", len(outEdges))
	}
	totalWeight := 0.0
	for _, edge := range outEdges {
		totalWeight += edge.Weight
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Errorf("expected remaining weights to sum to 1.0, got %f", totalWeight)
	}
}

// sinkNames extracts node names from a sink slice for readable test output.
func sinkNames(sinks []*model.TDGNode) []string {
	names := make([]string, len(sinks))
	for i, s := range sinks {
		names[i] = s.Node
	}
	return names
}

func TestApplyFailureNoAffectedEdges(t *testing.T) {
	cache := NewTDGCache()

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

	tdg := cache.GetOrBuild("router1", pc, fibs)

	// Fail a non-existent link
	failSet := failure.Links("nonexistent-link")
	failed := cache.ApplyFailure(tdg, failSet)

	// Should have same number of edges since no edge matches
	if len(failed.Edges) != len(tdg.Edges) {
		t.Errorf("expected same number of edges for unrelated failure, got %d vs %d",
			len(failed.Edges), len(tdg.Edges))
	}
}
