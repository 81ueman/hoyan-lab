package space

import (
	"github.com/81ueman/hoyan-lab/internal/core/netaddr"
	"github.com/81ueman/hoyan-lab/internal/core/predicate"
	"reflect"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/check/query"
	"github.com/81ueman/hoyan-lab/internal/config/routing"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

func TestHeaderSpaceSplitsTCPDstPorts(t *testing.T) {
	pfx := netaddr.MustPrefix("10.0.0.0/24")
	topo := aclTestTopology("WEB", "r1", "eth1", "egress",
		topology.ACLRule{Seq: 10, Action: topology.ACLDeny, Match: predicate.PacketSpec{Protocol: "tcp", DstSet: predicate.ExactPrefixSet{Prefix: pfx}, DstPort: predicate.ExactPort(80)}},
		topology.ACLRule{Seq: 20, Action: topology.ACLPermit, Match: predicate.PacketSpec{Protocol: "tcp", DstSet: predicate.ExactPrefixSet{Prefix: pfx}, DstPort: predicate.ExactPort(443)}},
	)
	universe, err := NewPrefixUniverse(topo, routing.FromTopology(topo), nil)
	if err != nil {
		t.Fatalf("NewPrefixUniverse() error = %v", err)
	}
	headerSpace := NewHeaderSpace(topo, routing.FromTopology(topo), nil, universe)
	if got, want := len(headerSpace.Classes), 2; got != want {
		t.Fatalf("len(Classes) = %d, want %d: %#v", got, want, headerSpace.Classes)
	}
	gotPorts := []string{headerSpace.Classes[0].DstPort.String(), headerSpace.Classes[1].DstPort.String()}
	if !reflect.DeepEqual(gotPorts, []string{"443", "80"}) {
		t.Fatalf("dst ports = %#v, want [443 80]", gotPorts)
	}
}

func TestHeaderSpaceLinksDstPrefixToPrefixClass(t *testing.T) {
	topo := &topology.Topology{
		Nodes: []topology.Node{{Name: "dst", Prefixes: netaddr.MustPrefixes("10.0.0.0/24", "10.0.1.0/24")}},
		ACLs: []topology.ACL{{Name: "DENY-DST", Node: "r1", DefaultAction: topology.ACLDefaultPermit, Rules: []topology.ACLRule{{
			Seq: 10, Action: topology.ACLDeny, Match: predicate.PacketSpec{Protocol: "tcp", DstSet: predicate.ExactPrefixSet{Prefix: netaddr.MustPrefix("10.0.1.0/24")}},
		}}}},
	}
	universe, err := NewPrefixUniverse(topo, routing.FromTopology(topo), nil)
	if err != nil {
		t.Fatalf("NewPrefixUniverse() error = %v", err)
	}
	headerSpace := NewHeaderSpace(topo, routing.FromTopology(topo), nil, universe)
	if got, want := len(headerSpace.Classes), 1; got != want {
		t.Fatalf("len(Classes) = %d, want %d: %#v", got, want, headerSpace.Classes)
	}
	classID, ok := universe.ClassForPrefix(netaddr.MustPrefix("10.0.1.0/24"))
	if !ok {
		t.Fatalf("ClassForPrefix() did not find policy prefix")
	}
	if got := headerSpace.Classes[0].PrefixClassID; got != classID {
		t.Fatalf("PrefixClassID = %d, want %d", got, classID)
	}
}

func TestHeaderSpaceSplitsIngressInterface(t *testing.T) {
	pfx := netaddr.MustPrefix("10.0.0.0/24")
	topo := &topology.Topology{
		ACLs: []topology.ACL{{Name: "DENY-IN", Node: "r1", DefaultAction: topology.ACLDefaultPermit, Rules: []topology.ACLRule{{
			Seq: 10, Action: topology.ACLDeny, Match: predicate.PacketSpec{Protocol: "tcp", DstSet: predicate.ExactPrefixSet{Prefix: pfx}},
		}}}},
		ACLBindings: []topology.ACLBinding{
			{Node: "r1", Interface: "eth1", Direction: "ingress", ACLName: "DENY-IN"},
			{Node: "r1", Interface: "eth2", Direction: "ingress", ACLName: "DENY-IN"},
		},
	}
	universe, err := NewPrefixUniverse(topo, routing.FromTopology(topo), nil)
	if err != nil {
		t.Fatalf("NewPrefixUniverse() error = %v", err)
	}
	headerSpace := NewHeaderSpace(topo, routing.FromTopology(topo), nil, universe)
	if got, want := len(headerSpace.Classes), 2; got != want {
		t.Fatalf("len(Classes) = %d, want %d: %#v", got, want, headerSpace.Classes)
	}
	gotIfaces := []string{headerSpace.Classes[0].IngressInterface, headerSpace.Classes[1].IngressInterface}
	if !reflect.DeepEqual(gotIfaces, []string{"eth1", "eth2"}) {
		t.Fatalf("ingress interfaces = %#v, want [eth1 eth2]", gotIfaces)
	}
}

func TestHeaderSpaceAvoidsUnusedDimensionCrossProduct(t *testing.T) {
	topo := &topology.Topology{ACLs: []topology.ACL{{Name: "DENY", Node: "r1", DefaultAction: topology.ACLDefaultPermit, Rules: []topology.ACLRule{
		{Seq: 10, Action: topology.ACLDeny, Match: predicate.PacketSpec{DstSet: predicate.ExactPrefixSet{Prefix: netaddr.MustPrefix("10.0.0.0/24")}}},
		{Seq: 20, Action: topology.ACLDeny, Match: predicate.PacketSpec{DstSet: predicate.ExactPrefixSet{Prefix: netaddr.MustPrefix("10.0.1.0/24")}}},
	}}}}
	universe, err := NewPrefixUniverse(topo, routing.FromTopology(topo), nil)
	if err != nil {
		t.Fatalf("NewPrefixUniverse() error = %v", err)
	}
	headerSpace := NewHeaderSpace(topo, routing.FromTopology(topo), nil, universe)
	if got, want := len(headerSpace.Classes), 2; got != want {
		t.Fatalf("len(Classes) = %d, want %d: %#v", got, want, headerSpace.Classes)
	}
	for _, class := range headerSpace.Classes {
		if class.Protocol != "" || class.DstPort != nil || class.IngressInterface != "" || class.EgressInterface != "" {
			t.Fatalf("class contains an unnecessary dimension: %#v", class)
		}
	}
}

func TestCollectHeaderPredicatesIncludesQueries(t *testing.T) {
	topo := &topology.Topology{Nodes: []topology.Node{{Name: "dst", Prefixes: netaddr.MustPrefixes("10.0.0.0/24")}}}
	queries := &query.Queries{PacketChecks: []query.PacketCheck{{Name: "web", To: "dst", Protocol: "tcp", DstPorts: []int{80, 443}}}}
	predicates := CollectHeaderPredicates(topo, routing.FromTopology(topo), queries)
	if got, want := len(predicates), 2; got != want {
		t.Fatalf("len(predicates) = %d, want %d", got, want)
	}
	if got, want := predicates[0].Source, "query-packet:web"; got != want {
		t.Fatalf("Source = %q, want %q", got, want)
	}
	if !predicates[0].DstPort.Contains(80) || !predicates[1].DstPort.Contains(443) {
		t.Fatalf("DstPorts = %#v, %#v; want 80 and 443", predicates[0].DstPort, predicates[1].DstPort)
	}
}

func aclTestTopology(name, node, iface, direction string, rules ...topology.ACLRule) *topology.Topology {
	return &topology.Topology{
		ACLs:        []topology.ACL{{Name: name, Node: node, DefaultAction: topology.ACLDefaultPermit, Rules: rules}},
		ACLBindings: []topology.ACLBinding{{Node: node, Interface: iface, Direction: direction, ACLName: name}},
	}
}
