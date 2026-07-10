package traffic

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestBuildTDG_LinearTopology(t *testing.T) {
	fibs := FIBTable{
		"router1": []TrafficFIBEntry{
			{
				Prefix: model.MustPrefix("10.0.0.0/24").NetIP(),
				NextHops: []TrafficNextHop{
					{Node: "router2", Weight: 1.0},
				},
			},
		},
		"router2": []TrafficFIBEntry{
			{
				Prefix: model.MustPrefix("10.0.0.0/24").NetIP(),
				NextHops: []TrafficNextHop{
					{Node: "router3", Weight: 1.0},
				},
			},
		},
	}

	tdg := BuildTDG("router1", model.PacketClass{DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")}}, fibs)
	if tdg == nil {
		t.Fatal("BuildTDG returned nil")
	}
	if tdg.Root == nil {
		t.Fatal("expected non-nil root")
	}
	if tdg.Root.Node != "router1" {
		t.Errorf("expected root router1, got %s", tdg.Root.Node)
	}

	if len(tdg.Nodes) != 3 {
		t.Errorf("expected 3 nodes, got %d", len(tdg.Nodes))
	}

	if len(tdg.Edges) != 2 {
		t.Errorf("expected 2 edges, got %d", len(tdg.Edges))
	}

	for _, edge := range tdg.Edges {
		if edge.Weight != 1.0 {
			t.Errorf("expected weight 1.0 for single-path edge %s->%s, got %f",
				edge.From.Node, edge.To.Node, edge.Weight)
		}
	}

	order := tdg.TopologicalOrder()
	if len(order) != 3 {
		t.Errorf("expected 3 nodes in topological order, got %d", len(order))
	}
	if order[0].Node != "router1" {
		t.Errorf("expected first node router1, got %s", order[0].Node)
	}
	if order[2].Node != "router3" {
		t.Errorf("expected last node router3, got %s", order[2].Node)
	}

	if len(tdg.Sinks) == 0 {
		t.Errorf("expected at least one sink")
	}
}

func TestBuildTDG_ECMPTopology(t *testing.T) {
	fibs := FIBTable{
		"router1": []TrafficFIBEntry{
			{
				Prefix: model.MustPrefix("10.0.0.0/24").NetIP(),
				NextHops: []TrafficNextHop{
					{Node: "router2", Weight: 0.5},
					{Node: "router3", Weight: 0.5},
				},
			},
		},
		"router2": []TrafficFIBEntry{
			{
				Prefix: model.MustPrefix("10.0.0.0/24").NetIP(),
				NextHops: []TrafficNextHop{
					{Node: "router4", Weight: 1.0},
				},
			},
		},
		"router3": []TrafficFIBEntry{
			{
				Prefix: model.MustPrefix("10.0.0.0/24").NetIP(),
				NextHops: []TrafficNextHop{
					{Node: "router4", Weight: 1.0},
				},
			},
		},
	}

	tdg := BuildTDG("router1", model.PacketClass{DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")}}, fibs)
	if tdg == nil {
		t.Fatal("BuildTDG returned nil")
	}

	if len(tdg.Nodes) != 4 {
		t.Errorf("expected 4 nodes, got %d", len(tdg.Nodes))
	}

	if len(tdg.Edges) != 4 {
		t.Errorf("expected 4 edges, got %d", len(tdg.Edges))
	}

	outEdges := tdg.OutEdges("router1")
	if len(outEdges) != 2 {
		t.Errorf("expected 2 out edges from router1, got %d", len(outEdges))
	}
	totalWeight := 0.0
	for _, edge := range outEdges {
		totalWeight += edge.Weight
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Errorf("expected total ECMP weight ~1.0, got %f", totalWeight)
	}
}

func TestBuildTDG_NoMatch(t *testing.T) {
	fibs := FIBTable{
		"router1": []TrafficFIBEntry{
			{
				Prefix: model.MustPrefix("10.0.0.0/24").NetIP(),
				NextHops: []TrafficNextHop{
					{Node: "router2", Weight: 1.0},
				},
			},
		},
	}

	tdg := BuildTDG("router1", model.PacketClass{DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("192.168.0.0/16")}}, fibs)
	if tdg == nil {
		t.Fatal("BuildTDG returned nil")
	}
	if tdg.Root == nil {
		t.Fatal("expected non-nil root even for no match")
	}
	if len(tdg.Nodes) < 1 {
		t.Errorf("expected at least root node, got %d", len(tdg.Nodes))
	}
}

func TestBuildTDG_DuplicatePrevention(t *testing.T) {
	fibs := FIBTable{
		"router1": []TrafficFIBEntry{
			{
				Prefix: model.MustPrefix("10.0.0.0/24").NetIP(),
				NextHops: []TrafficNextHop{
					{Node: "router2", Weight: 1.0},
				},
			},
		},
		"router2": []TrafficFIBEntry{
			{
				Prefix: model.MustPrefix("10.0.0.0/24").NetIP(),
				NextHops: []TrafficNextHop{
					{Node: "router3", Weight: 1.0},
				},
			},
		},
	}

	tdg := BuildTDG("router1", model.PacketClass{DstSet: model.ExactPrefixSet{Prefix: model.MustPrefix("10.0.0.0/24")}}, fibs)
	if len(tdg.Nodes) != 3 {
		t.Errorf("expected 3 unique nodes, got %d", len(tdg.Nodes))
	}

	names := make(map[string]bool)
	for _, n := range tdg.Nodes {
		if names[n.Node] {
			t.Errorf("duplicate node name: %s", n.Node)
		}
		names[n.Node] = true
	}
}
