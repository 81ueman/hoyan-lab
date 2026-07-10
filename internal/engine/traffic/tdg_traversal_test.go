package traffic

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestTraverseLinear(t *testing.T) {
	tdg := model.NewTDG()
	tdg.AddNode("router1", "default", "ingress_acl", 0)
	tdg.AddNode("router2", "default", "fib_lookup", 0)
	tdg.AddNode("router3", "default", "next_hop", 0)
	tdg.SetRoot("router1")
	tdg.AddEdge("router1", "router2", 1.0)
	tdg.AddEdge("router2", "router3", 1.0)
	tdg.AddSink("router3")

	// Edge naming: "router1->router2", "router2->router3"
	linkBytes := Traverse(tdg, 1000)
	if len(linkBytes) != 2 {
		t.Errorf("expected 2 links, got %d", len(linkBytes))
	}
	if linkBytes["router1->router2"] != 1000 {
		t.Errorf("expected 1000 bytes on router1->router2, got %d", linkBytes["router1->router2"])
	}
	if linkBytes["router2->router3"] != 1000 {
		t.Errorf("expected 1000 bytes on router2->router3, got %d", linkBytes["router2->router3"])
	}
}

func TestTraverseECMP(t *testing.T) {
	tdg := model.NewTDG()
	tdg.AddNode("router1", "default", "ingress_acl", 0)
	tdg.AddNode("router2", "default", "fib_lookup", 0)
	tdg.AddNode("router3", "default", "fib_lookup", 0)
	tdg.AddNode("router4", "default", "next_hop", 0)
	tdg.SetRoot("router1")
	tdg.AddEdge("router1", "router2", 0.4)
	tdg.AddEdge("router1", "router3", 0.6)
	tdg.AddEdge("router2", "router4", 1.0)
	tdg.AddEdge("router3", "router4", 1.0)
	tdg.AddSink("router4")

	linkBytes := Traverse(tdg, 1000)
	if len(linkBytes) != 4 {
		t.Errorf("expected 4 links, got %d", len(linkBytes))
	}
	// 40% of 1000 on router1->router2
	if linkBytes["router1->router2"] != 400 {
		t.Errorf("expected 400 bytes on router1->router2, got %d", linkBytes["router1->router2"])
	}
	// 60% of 1000 on router1->router3
	if linkBytes["router1->router3"] != 600 {
		t.Errorf("expected 600 bytes on router1->router3, got %d", linkBytes["router1->router3"])
	}
	// Both converge on router2->router4 and router3->router4
	if linkBytes["router2->router4"] != 400 {
		t.Errorf("expected 400 bytes on router2->router4, got %d", linkBytes["router2->router4"])
	}
	if linkBytes["router3->router4"] != 600 {
		t.Errorf("expected 600 bytes on router3->router4, got %d", linkBytes["router3->router4"])
	}
}

func TestTraverseChainedECMP(t *testing.T) {
	tdg := model.NewTDG()
	tdg.AddNode("router1", "default", "ingress_acl", 0)
	tdg.AddNode("router2a", "default", "fib_lookup", 0)
	tdg.AddNode("router2b", "default", "fib_lookup", 0)
	tdg.AddNode("router3a", "default", "next_hop", 0)
	tdg.AddNode("router3b", "default", "next_hop", 0)
	tdg.AddNode("router4", "default", "next_hop", 0)

	tdg.SetRoot("router1")
	tdg.AddEdge("router1", "router2a", 0.5)
	tdg.AddEdge("router1", "router2b", 0.5)
	tdg.AddEdge("router2a", "router3a", 1.0)
	tdg.AddEdge("router2b", "router3b", 1.0)
	tdg.AddEdge("router3a", "router4", 1.0)
	tdg.AddEdge("router3b", "router4", 1.0)
	tdg.AddSink("router4")

	linkBytes := Traverse(tdg, 1000)
	// router1->router2a: 500, router1->router2b: 500
	// router2a->router3a: 500, router2b->router3b: 500
	// router3a->router4: 500, router3b->router4: 500
	if linkBytes["router1->router2a"] != 500 {
		t.Errorf("expected 500 on router1->router2a, got %d", linkBytes["router1->router2a"])
	}
	if linkBytes["router3a->router4"] != 500 {
		t.Errorf("expected 500 on router3a->router4, got %d", linkBytes["router3a->router4"])
	}
	if linkBytes["router3b->router4"] != 500 {
		t.Errorf("expected 500 on router3b->router4, got %d", linkBytes["router3b->router4"])
	}
}

func TestTraverseNoRoutes(t *testing.T) {
	tdg := model.NewTDG()
	tdg.AddNode("router1", "default", "ingress_acl", 0)
	_ = tdg.SetRoot("router1")

	linkBytes := Traverse(tdg, 1000)
	if len(linkBytes) != 0 {
		t.Errorf("expected 0 links for isolated node, got %d", len(linkBytes))
	}
}

func TestTraverseZeroBytes(t *testing.T) {
	tdg := model.NewTDG()
	tdg.AddNode("router1", "default", "ingress_acl", 0)
	tdg.AddNode("router2", "default", "next_hop", 0)
	_ = tdg.SetRoot("router1")
	_, _ = tdg.AddEdge("router1", "router2", 1.0)
	tdg.AddSink("router2")

	linkBytes := Traverse(tdg, 0)
	if len(linkBytes) != 0 {
		t.Errorf("expected 0 links for zero bytes, got %d", len(linkBytes))
	}
}

func TestTraverseThreeWayECMP(t *testing.T) {
	tdg := model.NewTDG()
	tdg.AddNode("router1", "default", "ingress_acl", 0)
	tdg.AddNode("router2a", "default", "fib_lookup", 0)
	tdg.AddNode("router2b", "default", "fib_lookup", 0)
	tdg.AddNode("router2c", "default", "fib_lookup", 0)
	tdg.AddNode("router3", "default", "next_hop", 0)

	tdg.SetRoot("router1")
	tdg.AddEdge("router1", "router2a", 0.33)
	tdg.AddEdge("router1", "router2b", 0.33)
	tdg.AddEdge("router1", "router2c", 0.34)
	tdg.AddEdge("router2a", "router3", 1.0)
	tdg.AddEdge("router2b", "router3", 1.0)
	tdg.AddEdge("router2c", "router3", 1.0)
	tdg.AddSink("router3")

	linkBytes := Traverse(tdg, 10000)
	// router1 out edges
	if linkBytes["router1->router2a"] != 3300 {
		t.Errorf("expected 3300 on router1->router2a, got %d", linkBytes["router1->router2a"])
	}
	if linkBytes["router1->router2b"] != 3300 {
		t.Errorf("expected 3300 on router1->router2b, got %d", linkBytes["router1->router2b"])
	}
	if linkBytes["router1->router2c"] != 3400 {
		t.Errorf("expected 3400 on router1->router2c, got %d", linkBytes["router1->router2c"])
	}
	// router2/3 out edges carry what came in
	if linkBytes["router2a->router3"] != 3300 {
		t.Errorf("expected 3300 on router2a->router3, got %d", linkBytes["router2a->router3"])
	}
	if linkBytes["router2b->router3"] != 3300 {
		t.Errorf("expected 3300 on router2b->router3, got %d", linkBytes["router2b->router3"])
	}
	if linkBytes["router2c->router3"] != 3400 {
		t.Errorf("expected 3400 on router2c->router3, got %d", linkBytes["router2c->router3"])
	}
	// Total traffic on all links: 20000
	total := uint64(0)
	for _, bytes := range linkBytes {
		total += bytes
	}
	if total != 20000 {
		t.Errorf("expected total 20000, got %d", total)
	}
}
