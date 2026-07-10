package model_test

import (
	"net/netip"
	"testing"

	. "github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestFlowCreation(t *testing.T) {
	f := Flow{
		SrcIP:    netip.MustParseAddr("10.1.0.1"),
		DstIP:    netip.MustParseAddr("10.4.1.10"),
		Protocol: "tcp",
		SrcPort:  12345,
		DstPort:  443,
	}
	if f.SrcIP.String() != "10.1.0.1" {
		t.Errorf("unexpected src IP: %s", f.SrcIP)
	}
	if f.DstIP.String() != "10.4.1.10" {
		t.Errorf("unexpected dst IP: %s", f.DstIP)
	}
	if f.Protocol != "tcp" {
		t.Errorf("unexpected protocol: %s", f.Protocol)
	}
	if f.SrcPort != 12345 {
		t.Errorf("unexpected src port: %d", f.SrcPort)
	}
	if f.DstPort != 443 {
		t.Errorf("unexpected dst port: %d", f.DstPort)
	}
}

func TestLocatedFlowCreation(t *testing.T) {
	lf := LocatedFlow{
		Flow: Flow{
			SrcIP:    netip.MustParseAddr("10.1.0.1"),
			DstIP:    netip.MustParseAddr("10.4.1.10"),
			Protocol: "tcp",
			SrcPort:  12345,
			DstPort:  443,
		},
		IngressNode: "cust-bj",
		IngressIntf: "eth0",
		Bytes:       1000000,
	}
	if lf.IngressNode != "cust-bj" {
		t.Errorf("unexpected ingress node: %s", lf.IngressNode)
	}
	if lf.IngressIntf != "eth0" {
		t.Errorf("unexpected ingress interface: %s", lf.IngressIntf)
	}
	if lf.Bytes != 1000000 {
		t.Errorf("unexpected bytes: %d", lf.Bytes)
	}
}

func TestFlowEquivalenceClassCreation(t *testing.T) {
	ec := FlowEquivalenceClass{
		ID:          1,
		IngressNode: "cust-bj",
		IngressIntf: "eth0",
		TotalBytes:  3000000,
		FlowCount:   3,
	}
	if ec.ID != 1 {
		t.Errorf("unexpected EC ID: %d", ec.ID)
	}
	if ec.IngressNode != "cust-bj" {
		t.Errorf("unexpected ingress node: %s", ec.IngressNode)
	}
	if ec.TotalBytes != 3000000 {
		t.Errorf("unexpected total bytes: %d", ec.TotalBytes)
	}
	if ec.FlowCount != 3 {
		t.Errorf("unexpected flow count: %d", ec.FlowCount)
	}
}

func TestLinkLoadCreation(t *testing.T) {
	ll := LinkLoad{
		LinkName: "cust-bj--core-bj-1",
		Bytes:    5000000,
		Capacity: 10000000000, // 10 Gbps
	}
	if ll.LinkName != "cust-bj--core-bj-1" {
		t.Errorf("unexpected link name: %s", ll.LinkName)
	}
	if ll.Bytes != 5000000 {
		t.Errorf("unexpected bytes: %d", ll.Bytes)
	}
	if ll.Capacity != 10000000000 {
		t.Errorf("unexpected capacity: %d", ll.Capacity)
	}
}

func TestTrafficResultCreation(t *testing.T) {
	tr := TrafficResult{
		LinkLoads: map[string]LinkLoad{
			"link-1": {LinkName: "link-1", Bytes: 100, Capacity: 1000},
		},
	}
	if len(tr.LinkLoads) != 1 {
		t.Errorf("unexpected link load count: %d", len(tr.LinkLoads))
	}
	ll, ok := tr.LinkLoads["link-1"]
	if !ok {
		t.Fatal("missing link-1 in result")
	}
	if ll.Bytes != 100 {
		t.Errorf("unexpected bytes: %d", ll.Bytes)
	}
}
