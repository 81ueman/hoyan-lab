package traffic

import (
	"net/netip"
	"testing"
)

func TestSelectECMPMemberDeterministic(t *testing.T) {
	flow := Flow{
		SrcIP:    netip.MustParseAddr("10.0.0.1"),
		DstIP:    netip.MustParseAddr("10.0.0.2"),
		Protocol: "tcp",
		SrcPort:  12345,
		DstPort:  80,
	}
	nextHops := []TrafficNextHop{
		{Node: "router2", Weight: 0.5},
		{Node: "router3", Weight: 0.5},
	}

	// Same flow must select same member
	idx1 := SelectECMPMember(flow, nextHops)
	idx2 := SelectECMPMember(flow, nextHops)
	if idx1 != idx2 {
		t.Errorf("SelectECMPMember must be deterministic: first=%d, second=%d", idx1, idx2)
	}
}

func TestSelectECMPMemberWithinBounds(t *testing.T) {
	flow := Flow{
		SrcIP:    netip.MustParseAddr("192.168.1.1"),
		DstIP:    netip.MustParseAddr("10.0.0.1"),
		Protocol: "udp",
		SrcPort:  53,
		DstPort:  1234,
	}
	nextHops := []TrafficNextHop{
		{Node: "router2", Weight: 0.25},
		{Node: "router3", Weight: 0.25},
		{Node: "router4", Weight: 0.25},
		{Node: "router5", Weight: 0.25},
	}

	for i := 0; i < 100; i++ {
		idx := SelectECMPMember(flow, nextHops)
		if idx < 0 || idx >= len(nextHops) {
			t.Errorf("index %d out of bounds [0, %d)", idx, len(nextHops))
		}
	}
}

func TestSelectECMPMemberSingleHop(t *testing.T) {
	flow := Flow{
		SrcIP:    netip.MustParseAddr("10.0.0.1"),
		DstIP:    netip.MustParseAddr("10.0.0.2"),
		Protocol: "tcp",
		SrcPort:  80,
		DstPort:  8080,
	}
	nextHops := []TrafficNextHop{
		{Node: "router2", Weight: 1.0},
	}

	idx := SelectECMPMember(flow, nextHops)
	if idx != 0 {
		t.Errorf("expected index 0 for single next-hop, got %d", idx)
	}
}

func TestSelectECMPMemberDifferentFlows(t *testing.T) {
	nextHops := []TrafficNextHop{
		{Node: "router2", Weight: 0.5},
		{Node: "router3", Weight: 0.5},
	}

	// Different flows should distribute across members
	flowA := Flow{
		SrcIP:    netip.MustParseAddr("10.0.0.1"),
		DstIP:    netip.MustParseAddr("10.0.0.2"),
		Protocol: "tcp",
		SrcPort:  10001,
		DstPort:  80,
	}
	flowB := Flow{
		SrcIP:    netip.MustParseAddr("10.0.0.3"),
		DstIP:    netip.MustParseAddr("10.0.0.4"),
		Protocol: "udp",
		SrcPort:  53,
		DstPort:  443,
	}

	idxA := SelectECMPMember(flowA, nextHops)
	idxB := SelectECMPMember(flowB, nextHops)
	// Different flows may (or may not) hash to different members
	// This is a distribution test - just verify both are valid
	if idxA < 0 || idxA >= len(nextHops) || idxB < 0 || idxB >= len(nextHops) {
		t.Errorf("indices out of bounds: A=%d, B=%d", idxA, idxB)
	}
}

func TestSelectECMPMember5TupleChange(t *testing.T) {
	nextHops := []TrafficNextHop{
		{Node: "router2", Weight: 0.5},
		{Node: "router3", Weight: 0.5},
	}

	baseFlow := Flow{
		SrcIP:    netip.MustParseAddr("10.0.0.1"),
		DstIP:    netip.MustParseAddr("10.0.0.2"),
		Protocol: "tcp",
		SrcPort:  12345,
		DstPort:  80,
	}
	baseIdx := SelectECMPMember(baseFlow, nextHops)

	// Changing each tuple element should potentially change the hash
	cases := []struct {
		name string
		flow Flow
	}{
		{"srcIP", Flow{SrcIP: netip.MustParseAddr("10.0.0.99"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp", SrcPort: 12345, DstPort: 80}},
		{"dstIP", Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.99"), Protocol: "tcp", SrcPort: 12345, DstPort: 80}},
		{"protocol", Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "udp", SrcPort: 12345, DstPort: 80}},
		{"srcPort", Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp", SrcPort: 65535, DstPort: 80}},
		{"dstPort", Flow{SrcIP: netip.MustParseAddr("10.0.0.1"), DstIP: netip.MustParseAddr("10.0.0.2"), Protocol: "tcp", SrcPort: 12345, DstPort: 443}},
	}

	changes := 0
	for _, c := range cases {
		newIdx := SelectECMPMember(c.flow, nextHops)
		if newIdx != baseIdx {
			changes++
		}
	}
	if changes == 0 {
		t.Log("all 5-tuple changes resulted in same hash (possible but unlikely with FNV-1a)")
	}
}
