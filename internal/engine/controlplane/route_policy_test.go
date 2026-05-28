package controlplane

import (
	"github.com/81ueman/hoyan-lab/internal/core/netaddr"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/config/routing"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

func TestRouteNextHopForPolicyUsesResolvedPeerAddress(t *testing.T) {
	idx, err := topology.BuildTopologyIndex(&topology.Topology{
		Nodes: []topology.Node{
			{
				Name:       "local",
				Kind:       topology.KindFRR,
				Interfaces: []topology.Interface{{Name: "eth1", Address: "198.51.100.10/24"}},
			},
			{
				Name:       "peer",
				Kind:       topology.KindCEOS,
				Interfaces: []topology.Interface{{Name: "Ethernet1", Address: "198.51.100.20/24"}},
			},
		},
		Links: []topology.Link{
			{Name: "local-peer", A: "local", B: "peer", AIntf: "eth1", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/24"},
		},
	})
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	got := routeNextHopForPolicy(idx, "local", "", testRIB("", withNextHop("peer")))
	if got != "198.51.100.20" {
		t.Fatalf("routeNextHopForPolicy() = %q, want resolved peer address 198.51.100.20", got)
	}
}

func TestRoutePolicyNextHopPrefixListUsesResolvedAddress(t *testing.T) {
	idx, err := topology.BuildTopologyIndex(&topology.Topology{
		Nodes: []topology.Node{
			{
				Name:       "local",
				Kind:       topology.KindFRR,
				Interfaces: []topology.Interface{{Name: "eth1", Address: "198.51.100.10/24"}},
			},
			{
				Name:       "peer",
				Kind:       topology.KindCEOS,
				Interfaces: []topology.Interface{{Name: "Ethernet1", Address: "198.51.100.20/24"}},
			},
		},
		Links: []topology.Link{
			{Name: "local-peer", A: "local", B: "peer", AIntf: "eth1", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/24"},
		},
	})
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	node := topology.Node{
		Name: "local",
		PrefixLists: []topology.PrefixList{{
			Name:  "NH",
			Rules: []topology.PrefixListRule{{Seq: 10, Action: "permit", Prefix: "198.51.100.20/32"}},
		}},
	}
	rule := topology.RoutePolicyRule{MatchNextHopPrefixList: "NH"}
	if !routePolicyRuleMatches(idx, routing.FromTopology(&topology.Topology{Nodes: []topology.Node{node}}).ForNode(node.Name), "", rule, testRIB("", withNextHop("peer"))) {
		t.Fatalf("routePolicyRuleMatches() = false, want next-hop prefix-list to match resolved peer address")
	}
}

func TestRoutePolicySetNextHopSelf(t *testing.T) {
	node := topology.Node{
		Name: "core-gz",
		RoutePolicies: []topology.RoutePolicy{{
			Name: "NH-SELF",
			Rules: []topology.RoutePolicyRule{{
				Seq:            10,
				Action:         "permit",
				SetNextHopSelf: true,
			}},
		}},
	}
	route := RIBEntry{
		NLRI:              RouteNLRI{Prefix: netaddr.MustPrefix("10.3.0.0/16")},
		ForwardingNextHop: RouteNextHop{Node: "gz-edge1", Addr: "198.18.10.8"},
	}
	decision := applyRoutePolicy(nil, node, "core-bj", "NH-SELF", route)
	if !decision.Accept {
		t.Fatalf("decision rejected route: %#v", decision)
	}
	if decision.Route.ForwardingNextHop.Node != "core-gz" || decision.Route.ForwardingNextHop.Addr != "" {
		t.Fatalf("route next-hop = %#v, want core-gz self", decision.Route)
	}
}

func TestRoutePolicySetIPAddressNextHop(t *testing.T) {
	node := topology.Node{
		Name: "local",
		RoutePolicies: []topology.RoutePolicy{{
			Name: "SET-NH",
			Rules: []topology.RoutePolicyRule{{
				Seq:        10,
				Action:     "permit",
				SetNextHop: "192.0.2.1",
			}},
		}},
	}
	decision := applyRoutePolicy(nil, node, "peer", "SET-NH", testRIB("10.0.0.0/24", withNextHop("local")))
	if !decision.Accept {
		t.Fatalf("decision rejected route: %#v", decision)
	}
	if decision.Route.ForwardingNextHop.Node != "" || decision.Route.ForwardingNextHop.Addr != "192.0.2.1" {
		t.Fatalf("route next-hop = %#v, want address-only recursive next-hop", decision.Route)
	}
}

func TestPrefixListRuleMatchesUsesNLRILengthSemantics(t *testing.T) {
	rule := topology.PrefixListRule{Seq: 10, Action: "permit", Prefix: "10.0.0.0/8", Ge: 16, Le: 24}
	if !prefixListRuleMatches(rule, netaddr.MustPrefix("10.4.0.0/16")) {
		t.Fatalf("prefix-list range should match NLRI inside ge/le bounds")
	}
	if prefixListRuleMatches(rule, netaddr.MustPrefix("10.4.1.10/32")) {
		t.Fatalf("prefix-list range should reject NLRI longer than le")
	}
	if prefixListRuleMatches(rule, netaddr.MustPrefix("10.0.0.0/8")) {
		t.Fatalf("prefix-list range should reject NLRI shorter than ge")
	}
}
