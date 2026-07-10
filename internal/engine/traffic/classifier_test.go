package traffic_test

import (
	"net/netip"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/engine/traffic"
)

func TestFlowClassifierClassifiesFlows(t *testing.T) {
	// Build a PrefixUniverse with a known predicate for 10.4.0.0/16
	universe, err := model.BuildPrefixUniverse([]model.PrefixSet{
		model.ExactPrefixSet{Prefix: model.MustPrefix("10.4.0.0/16")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(universe.Classes) == 0 {
		t.Fatal("expected at least one prefix class")
	}
	// Find the class ID for 10.4.1.10/32
	dstPrefix := model.PrefixFromNetIP(netip.PrefixFrom(netip.MustParseAddr("10.4.1.10"), 32))
	_, ok := universe.ClassForPrefix(dstPrefix)
	if !ok {
		t.Fatal("expected 10.4.1.10/32 to match a prefix class")
	}

	flows := []model.LocatedFlow{
		{
			Flow: model.Flow{
				SrcIP:    netip.MustParseAddr("10.1.0.1"),
				DstIP:    netip.MustParseAddr("10.4.1.10"),
				Protocol: "tcp",
				SrcPort:  12345,
				DstPort:  443,
			},
			IngressNode: "cust-bj",
			IngressIntf: "eth0",
			Bytes:       1000000,
		},
		{
			Flow: model.Flow{
				SrcIP:    netip.MustParseAddr("10.2.0.1"),
				DstIP:    netip.MustParseAddr("10.4.1.10"),
				Protocol: "tcp",
				SrcPort:  23456,
				DstPort:  443,
			},
			IngressNode: "cust-bj",
			IngressIntf: "eth0",
			Bytes:       2000000,
		},
		{
			// Different destination prefix (no matching predicate)
			Flow: model.Flow{
				SrcIP:    netip.MustParseAddr("10.3.0.1"),
				DstIP:    netip.MustParseAddr("192.168.1.1"),
				Protocol: "udp",
				SrcPort:  34567,
				DstPort:  53,
			},
			IngressNode: "cust-sh",
			IngressIntf: "eth1",
			Bytes:       500000,
		},
	}

	classifier := traffic.NewFlowClassifier(universe)
	ecs := classifier.Classify(flows)

	if len(ecs) == 0 {
		t.Fatal("expected at least one equivalence class")
	}

	// Flow 1 and 2 should be grouped into the same EC (same DstPrefix, Protocol, DstPort, IngressNode)
	var foundGrouped bool
	for _, ec := range ecs {
		if ec.IngressNode == "cust-bj" && ec.TotalBytes == 3000000 && ec.FlowCount == 2 {
			foundGrouped = true
			if ec.ID < 0 {
				t.Error("expected non-negative EC ID")
			}
		}
	}
	if !foundGrouped {
		t.Errorf("expected EC with ingress cust-bj, 3MB total, 2 flows, got %d ECs", len(ecs))
		for _, ec := range ecs {
			t.Logf("  EC id=%d ingress=%s bytes=%d count=%d", ec.ID, ec.IngressNode, ec.TotalBytes, ec.FlowCount)
		}
	}
}

func TestFlowClassifierEmptyFlows(t *testing.T) {
	classifier := traffic.NewFlowClassifier(model.PrefixUniverse{})
	ecs := classifier.Classify(nil)
	if len(ecs) != 0 {
		t.Errorf("expected empty result for nil flows, got %d ECs", len(ecs))
	}
	ecs = classifier.Classify([]model.LocatedFlow{})
	if len(ecs) != 0 {
		t.Errorf("expected empty result for empty flows, got %d ECs", len(ecs))
	}
}
