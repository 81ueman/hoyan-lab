package model_test

import (
	"testing"

	. "github.com/81ueman/hoyan-lab/internal/model"
)

func TestOriginLookupsUseTypedCanonicalPrefixes(t *testing.T) {
	topo := &Topology{Nodes: []Node{
		{Name: "origin", Prefixes: MustPrefixes("10.0.0.1/24")},
	}}
	if err := topo.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	node, ok := topo.OriginForPrefix("10.0.0.0/24")
	if !ok || node != "origin" {
		t.Fatalf("OriginForPrefix() = %q, %v", node, ok)
	}
	node, pfx, ok := topo.OriginForIP("10.0.0.99")
	if !ok || node != "origin" || pfx.String() != "10.0.0.0/24" {
		t.Fatalf("OriginForIP() = %q %s %v", node, pfx, ok)
	}
}

func TestValidateRejectsMissingRoutePolicyReferences(t *testing.T) {
	tests := []struct {
		name     string
		neighbor BGPNeighbor
		want     string
	}{
		{
			name:     "import",
			neighbor: BGPNeighbor{Address: "192.0.2.1", ImportPolicy: "MISSING"},
			want:     "node r1 neighbor 192.0.2.1 import route policy MISSING not found",
		},
		{
			name:     "export",
			neighbor: BGPNeighbor{Address: "192.0.2.1", ExportPolicy: "MISSING"},
			want:     "node r1 neighbor 192.0.2.1 export route policy MISSING not found",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topo := &Topology{
				Nodes: []Node{{Name: "r1", Neighbors: []BGPNeighbor{tt.neighbor}}},
			}
			err := topo.Validate()
			if err == nil || err.Error() != tt.want {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsMissingRouteMapMatchReferences(t *testing.T) {
	tests := []struct {
		name string
		rule RoutePolicyRule
		want string
	}{
		{
			name: "prefix-list",
			rule: RoutePolicyRule{Seq: 10, Action: "permit", MatchPrefixList: "MISSING"},
			want: "node r1 route policy RM rule 10 references missing prefix-list MISSING",
		},
		{
			name: "next-hop-prefix-list",
			rule: RoutePolicyRule{Seq: 10, Action: "permit", MatchNextHopPrefixList: "MISSING"},
			want: "node r1 route policy RM rule 10 references missing next-hop prefix-list MISSING",
		},
		{
			name: "as-path",
			rule: RoutePolicyRule{Seq: 10, Action: "permit", MatchASPathList: "MISSING"},
			want: "node r1 route policy RM rule 10 references missing as-path list MISSING",
		},
		{
			name: "community",
			rule: RoutePolicyRule{Seq: 10, Action: "permit", MatchCommunityList: "MISSING"},
			want: "node r1 route policy RM rule 10 references missing community-list MISSING",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topo := &Topology{Nodes: []Node{{Name: "r1", RoutePolicies: []RoutePolicy{{Name: "RM", Rules: []RoutePolicyRule{tt.rule}}}}}}
			err := topo.Validate()
			if err == nil || err.Error() != tt.want {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsInvalidACLFields(t *testing.T) {
	tests := []struct {
		name string
		acl  ACL
		want string
	}{
		{
			name: "default action",
			acl:  ACL{Name: "a1", Node: "r1", DefaultAction: "drop"},
			want: "acl a1 has invalid default action drop",
		},
		{
			name: "rule action",
			acl:  ACL{Name: "a1", Node: "r1", DefaultAction: ACLDefaultPermit, Rules: []ACLRule{{Seq: 10, Action: "drop"}}},
			want: "acl a1 rule 10 has invalid action drop",
		},
		{
			name: "protocol",
			acl:  ACL{Name: "a1", Node: "r1", DefaultAction: ACLDefaultPermit, Rules: []ACLRule{{Seq: 10, Action: ACLDeny, Match: PacketSpec{Protocol: "gre"}}}},
			want: "acl a1 rule 10 has invalid protocol gre",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topo := &Topology{
				Nodes: []Node{{Name: "r1"}},
				ACLs:  []ACL{tt.acl},
			}
			err := topo.Validate()
			if err == nil || err.Error() != tt.want {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsUnknownACLBindingNode(t *testing.T) {
	topo := &Topology{
		Nodes:       []Node{{Name: "r1"}},
		ACLBindings: []ACLBinding{{Node: "missing", ACLName: "a1", Direction: "egress"}},
	}
	err := topo.Validate()
	if err == nil || err.Error() != "acl binding a1 references unknown node missing" {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidBGPNeighbors(t *testing.T) {
	tests := []struct {
		name string
		node Node
		want string
	}{
		{
			name: "unknown-peer",
			node: Node{Name: "r1", Neighbors: []BGPNeighbor{{PeerNode: "missing", RemoteAS: 65002, Activated: true}}},
			want: "node r1 neighbor missing references unknown peer node missing",
		},
		{
			name: "activated-zero-remote-as",
			node: Node{Name: "r1", Neighbors: []BGPNeighbor{{Address: "192.0.2.1", Activated: true}}},
			want: "node r1 neighbor 192.0.2.1 is activated with remote_as 0",
		},
		{
			name: "duplicate-address",
			node: Node{Name: "r1", Neighbors: []BGPNeighbor{
				{Address: "192.0.2.1", RemoteAS: 65002},
				{Address: "192.0.2.1", RemoteAS: 65002},
			}},
			want: "node r1 has duplicate neighbor address 192.0.2.1",
		},
		{
			name: "duplicate-peer-node",
			node: Node{Name: "r1", Neighbors: []BGPNeighbor{
				{PeerNode: "r2", RemoteAS: 65002},
				{PeerNode: "r2", RemoteAS: 65002},
			}},
			want: "node r1 has duplicate neighbor peer node r2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topo := &Topology{
				Nodes: []Node{
					tt.node,
					{Name: "r2", Interfaces: []Interface{{Name: "eth1", Address: "192.0.2.1/31"}}},
				},
			}
			err := topo.Validate()
			if err == nil || err.Error() != tt.want {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsDuplicatePolicyAndListState(t *testing.T) {
	tests := []struct {
		name string
		node Node
		want string
	}{
		{
			name: "duplicate-prefix-list",
			node: Node{Name: "r1", PrefixLists: []PrefixList{{Name: "PL"}, {Name: "PL"}}},
			want: "node r1 has duplicate prefix-list PL",
		},
		{
			name: "duplicate-prefix-list-seq",
			node: Node{Name: "r1", PrefixLists: []PrefixList{{Name: "PL", Rules: []PrefixListRule{{Seq: 10, Action: "permit"}, {Seq: 10, Action: "deny"}}}}},
			want: "node r1 prefix-list PL has duplicate seq 10",
		},
		{
			name: "duplicate-route-policy",
			node: Node{Name: "r1", RoutePolicies: []RoutePolicy{{Name: "RM"}, {Name: "RM"}}},
			want: "node r1 has duplicate route policy RM",
		},
		{
			name: "duplicate-route-policy-seq",
			node: Node{Name: "r1", RoutePolicies: []RoutePolicy{{Name: "RM", Rules: []RoutePolicyRule{{Seq: 10, Action: "permit"}, {Seq: 10, Action: "deny"}}}}},
			want: "node r1 route policy RM has duplicate seq 10",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			topo := &Topology{Nodes: []Node{tt.node}}
			err := topo.Validate()
			if err == nil || err.Error() != tt.want {
				t.Fatalf("Validate() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestValidateRejectsUnknownLinkInterface(t *testing.T) {
	topo := &Topology{
		Nodes: []Node{
			{Name: "r1", Interfaces: []Interface{{Name: "eth1", Address: "192.0.2.0/31"}}},
			{Name: "r2", Interfaces: []Interface{{Name: "eth1", Address: "192.0.2.1/31"}}},
		},
		Links: []Link{{Name: "r1-r2", A: "r1", B: "r2", AIntf: "eth9", BIntf: "eth1", Cost: 1, Subnet: "192.0.2.0/31"}},
	}
	err := topo.Validate()
	if err == nil || err.Error() != "link r1-r2 references unknown interface eth9 on node r1" {
		t.Fatalf("Validate() error = %v", err)
	}
}
