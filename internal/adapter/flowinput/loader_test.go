package flowinput_test

import (
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/flowinput"
)

func TestLoadJSONParsesValidFlows(t *testing.T) {
	input := `[
		{
			"src_ip": "10.1.0.1",
			"dst_ip": "10.4.1.10",
			"protocol": "tcp",
			"src_port": 12345,
			"dst_port": 443,
			"bytes": 1000000,
			"ingress": "cust-bj"
		},
		{
			"src_ip": "10.2.0.1",
			"dst_ip": "10.4.1.10",
			"protocol": "tcp",
			"src_port": 23456,
			"dst_port": 443,
			"bytes": 2000000,
			"ingress": "cust-sh"
		}
	]`

	flows, err := flowinput.LoadJSON(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 2 {
		t.Fatalf("expected 2 flows, got %d", len(flows))
	}

	// Check first flow
	f1 := flows[0]
	if f1.Flow.SrcIP.String() != "10.1.0.1" {
		t.Errorf("flow[0] src_ip = %s, want 10.1.0.1", f1.Flow.SrcIP)
	}
	if f1.Flow.DstIP.String() != "10.4.1.10" {
		t.Errorf("flow[0] dst_ip = %s, want 10.4.1.10", f1.Flow.DstIP)
	}
	if f1.Flow.Protocol != "tcp" {
		t.Errorf("flow[0] protocol = %s, want tcp", f1.Flow.Protocol)
	}
	if f1.Flow.SrcPort != 12345 {
		t.Errorf("flow[0] src_port = %d, want 12345", f1.Flow.SrcPort)
	}
	if f1.Flow.DstPort != 443 {
		t.Errorf("flow[0] dst_port = %d, want 443", f1.Flow.DstPort)
	}
	if f1.Bytes != 1000000 {
		t.Errorf("flow[0] bytes = %d, want 1000000", f1.Bytes)
	}
	if f1.IngressNode != "cust-bj" {
		t.Errorf("flow[0] ingress = %s, want cust-bj", f1.IngressNode)
	}

	// Check second flow
	f2 := flows[1]
	if f2.Flow.SrcIP.String() != "10.2.0.1" {
		t.Errorf("flow[1] src_ip = %s, want 10.2.0.1", f2.Flow.SrcIP)
	}
	if f2.Flow.DstIP.String() != "10.4.1.10" {
		t.Errorf("flow[1] dst_ip = %s, want 10.4.1.10", f2.Flow.DstIP)
	}
	if f2.Flow.Protocol != "tcp" {
		t.Errorf("flow[1] protocol = %s, want tcp", f2.Flow.Protocol)
	}
	if f2.Bytes != 2000000 {
		t.Errorf("flow[1] bytes = %d, want 2000000", f2.Bytes)
	}
	if f2.IngressNode != "cust-sh" {
		t.Errorf("flow[1] ingress = %s, want cust-sh", f2.IngressNode)
	}
}

func TestLoadJSONEmptyArray(t *testing.T) {
	flows, err := flowinput.LoadJSON(strings.NewReader("[]"))
	if err != nil {
		t.Fatal(err)
	}
	if len(flows) != 0 {
		t.Errorf("expected empty flows, got %d", len(flows))
	}
}

func TestLoadJSONInvalidJSON(t *testing.T) {
	_, err := flowinput.LoadJSON(strings.NewReader("not json"))
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoadJSONInvalidIP(t *testing.T) {
	input := `[{"src_ip": "not-an-ip", "dst_ip": "10.4.1.10", "protocol": "tcp", "src_port": 1, "dst_port": 80, "bytes": 100, "ingress": "node-a"}]`
	_, err := flowinput.LoadJSON(strings.NewReader(input))
	if err == nil {
		t.Fatal("expected error for invalid source IP")
	}
}

func TestSplitIngress(t *testing.T) {
	node, intf := flowinput.SplitIngress("cust-bj:eth0")
	if node != "cust-bj" || intf != "eth0" {
		t.Errorf("SplitIngress(\"cust-bj:eth0\") = (%q, %q), want (cust-bj, eth0)", node, intf)
	}

	node, intf = flowinput.SplitIngress("cust-bj")
	if node != "cust-bj" || intf != "" {
		t.Errorf("SplitIngress(\"cust-bj\") = (%q, %q), want (cust-bj, \"\")", node, intf)
	}

	node, intf = flowinput.SplitIngress("")
	if node != "" || intf != "" {
		t.Errorf("SplitIngress(\"\") = (%q, %q), want (\"\", \"\")", node, intf)
	}
}
