package model

import (
	"testing"
)

func TestTDGNodeCreation(t *testing.T) {
	node := &TDGNode{
		ID:       1,
		Node:     "router1",
		VRF:      "default",
		Stage:    "fib_lookup",
		PacketClassID: PrefixClassID(0),
	}
	if node.ID != 1 {
		t.Errorf("expected ID 1, got %d", node.ID)
	}
	if node.Node != "router1" {
		t.Errorf("expected Node router1, got %s", node.Node)
	}
	if node.Stage != "fib_lookup" {
		t.Errorf("expected Stage fib_lookup, got %s", node.Stage)
	}
}

func TestTDGEdgeCreation(t *testing.T) {
	from := &TDGNode{ID: 1, Node: "router1", Stage: "fib_lookup"}
	to := &TDGNode{ID: 2, Node: "router2", Stage: "next_hop"}
	edge := &TDGEdge{
		From:   from,
		To:     to,
		Weight: 1.0,
	}
	if edge.From != from {
		t.Errorf("expected From router1")
	}
	if edge.To != to {
		t.Errorf("expected To router2")
	}
	if edge.Weight != 1.0 {
		t.Errorf("expected Weight 1.0, got %f", edge.Weight)
	}
}

func TestTDGEmpty(t *testing.T) {
	tdg := NewTDG()
	if tdg == nil {
		t.Fatal("NewTDG() returned nil")
	}
	if len(tdg.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(tdg.Nodes))
	}
	if len(tdg.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d", len(tdg.Edges))
	}
	if tdg.Root != nil {
		t.Errorf("expected nil root, got %v", tdg.Root)
	}
}

func TestTDGAddNode(t *testing.T) {
	tdg := NewTDG()
	node := tdg.AddNode("router1", "default", "fib_lookup", PrefixClassID(0))
	if node == nil {
		t.Fatal("AddNode returned nil")
	}
	if node.ID != 0 {
		t.Errorf("expected ID 0, got %d", node.ID)
	}
	if node.Node != "router1" {
		t.Errorf("expected Node router1, got %s", node.Node)
	}
	if len(tdg.Nodes) != 1 {
		t.Errorf("expected 1 node, got %d", len(tdg.Nodes))
	}

	// Add duplicate should return same node
	node2 := tdg.AddNode("router1", "default", "fib_lookup", PrefixClassID(0))
	if node2 != node {
		t.Errorf("expected same node for duplicate AddNode call")
	}
	if len(tdg.Nodes) != 1 {
		t.Errorf("expected 1 node after duplicate, got %d", len(tdg.Nodes))
	}
}

func TestTDGAddEdge(t *testing.T) {
	tdg := NewTDG()
	from := tdg.AddNode("router1", "default", "fib_lookup", PrefixClassID(0))
	to := tdg.AddNode("router2", "default", "next_hop", PrefixClassID(0))

	edge, err := tdg.AddEdge("router1", "router2", 0.5)
	if err != nil {
		t.Fatalf("AddEdge failed: %v", err)
	}
	if edge.From != from {
		t.Errorf("expected From router1")
	}
	if edge.To != to {
		t.Errorf("expected To router2")
	}
	if edge.Weight != 0.5 {
		t.Errorf("expected Weight 0.5, got %f", edge.Weight)
	}
	if len(tdg.Edges) != 1 {
		t.Errorf("expected 1 edge, got %d", len(tdg.Edges))
	}

	// Add edge to non-existent node should fail
	_, err = tdg.AddEdge("router1", "router3", 1.0)
	if err == nil {
		t.Errorf("expected error for non-existent target node")
	}
}

func TestTDGSetRoot(t *testing.T) {
	tdg := NewTDG()
	tdg.AddNode("router1", "default", "ingress_acl", PrefixClassID(0))
	err := tdg.SetRoot("router1")
	if err != nil {
		t.Fatalf("SetRoot failed: %v", err)
	}
	if tdg.Root == nil || tdg.Root.Node != "router1" {
		t.Errorf("expected root router1")
	}

	// Set root to non-existent node
	err = tdg.SetRoot("router2")
	if err == nil {
		t.Errorf("expected error for non-existent root")
	}
}

func TestTDGAddSink(t *testing.T) {
	tdg := NewTDG()
	tdg.AddNode("router1", "default", "next_hop", PrefixClassID(0))
	tdg.AddSink("router1")
	if len(tdg.Sinks) != 1 {
		t.Errorf("expected 1 sink, got %d", len(tdg.Sinks))
	}
	if tdg.Sinks[0].Node != "router1" {
		t.Errorf("expected sink router1")
	}
}

func TestTDGOutEdges(t *testing.T) {
	tdg := NewTDG()
	tdg.AddNode("router1", "default", "fib_lookup", PrefixClassID(0))
	tdg.AddNode("router2", "default", "next_hop", PrefixClassID(0))
	tdg.AddNode("router3", "default", "next_hop", PrefixClassID(0))

	_, _ = tdg.AddEdge("router1", "router2", 0.5)
	_, _ = tdg.AddEdge("router1", "router3", 0.5)

	edges := tdg.OutEdges("router1")
	if len(edges) != 2 {
		t.Errorf("expected 2 out edges, got %d", len(edges))
	}

	edges = tdg.OutEdges("router2")
	if len(edges) != 0 {
		t.Errorf("expected 0 out edges from leaf, got %d", len(edges))
	}

	edges = tdg.OutEdges("nonexistent")
	if len(edges) != 0 {
		t.Errorf("expected 0 out edges from non-existent node, got %d", len(edges))
	}
}

func TestTDGTopologicalOrder(t *testing.T) {
	tdg := NewTDG()
	tdg.AddNode("router1", "default", "ingress_acl", PrefixClassID(0))
	tdg.AddNode("router2", "default", "fib_lookup", PrefixClassID(0))
	tdg.AddNode("router3", "default", "fib_lookup", PrefixClassID(0))
	tdg.AddNode("router4", "default", "next_hop", PrefixClassID(0))

	_ = tdg.SetRoot("router1")
	// Linear: router1 -> router2 -> router3 -> router4
	_, _ = tdg.AddEdge("router1", "router2", 1.0)
	_, _ = tdg.AddEdge("router2", "router3", 1.0)
	_, _ = tdg.AddEdge("router3", "router4", 1.0)
	tdg.AddSink("router4")

	order := tdg.TopologicalOrder()
	if len(order) != 4 {
		t.Errorf("expected 4 nodes in topological order, got %d: %v", len(order), order)
	}
	// Verify order: router1 should be first, router4 last
	if len(order) > 0 && order[0].Node != "router1" {
		t.Errorf("expected first node router1, got %s", order[0].Node)
	}
	if len(order) > 3 && order[3].Node != "router4" {
		t.Errorf("expected last node router4, got %s", order[3].Node)
	}
}

func TestTDGTopologicalOrderWithECMP(t *testing.T) {
	tdg := NewTDG()
	tdg.AddNode("router1", "default", "ingress_acl", PrefixClassID(0))
	tdg.AddNode("router2", "default", "fib_lookup", PrefixClassID(0))
	tdg.AddNode("router3", "default", "fib_lookup", PrefixClassID(0))
	tdg.AddNode("router4", "default", "next_hop", PrefixClassID(0))
	tdg.AddNode("router5", "default", "next_hop", PrefixClassID(0))

	_ = tdg.SetRoot("router1")
	// router1 -> router2 -> router4
	// router1 -> router3 -> router4
	_, _ = tdg.AddEdge("router1", "router2", 0.5)
	_, _ = tdg.AddEdge("router1", "router3", 0.5)
	_, _ = tdg.AddEdge("router2", "router4", 1.0)
	_, _ = tdg.AddEdge("router3", "router4", 1.0)
	_, _ = tdg.AddEdge("router4", "router5", 1.0)
	tdg.AddSink("router5")

	order := tdg.TopologicalOrder()
	if len(order) < 4 {
		t.Errorf("expected at least 4 nodes, got %d", len(order))
	}
	// router1 must be before router2 and router3
	if order[0].Node != "router1" {
		t.Errorf("expected router1 first, got %s", order[0].Node)
	}
	// router4 should be after router2 and router3
	router4Pos := -1
	for i, n := range order {
		if n.Node == "router4" {
			router4Pos = i
			break
		}
	}
	for _, n := range order {
		if (n.Node == "router2" || n.Node == "router3") && router4Pos <= 0 {
			t.Errorf("router2/router3 should appear before router4")
			break
		}
	}
}
