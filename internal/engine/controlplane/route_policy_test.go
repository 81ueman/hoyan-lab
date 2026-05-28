package controlplane

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/bgp"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

func TestRouteNextHopForPolicyUsesResolvedPeerAddress(t *testing.T) {
	idx, err := model.BuildTopologyIndex(&model.Topology{
		Nodes: []model.Node{
			{
				Name:       "local",
				Kind:       model.KindFRR,
				Interfaces: []model.Interface{{Name: "eth1", Address: "198.51.100.10/24"}},
			},
			{
				Name:       "peer",
				Kind:       model.KindCEOS,
				Interfaces: []model.Interface{{Name: "Ethernet1", Address: "198.51.100.20/24"}},
			},
		},
		Links: []model.Link{
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
	idx, err := model.BuildTopologyIndex(&model.Topology{
		Nodes: []model.Node{
			{
				Name:       "local",
				Kind:       model.KindFRR,
				Interfaces: []model.Interface{{Name: "eth1", Address: "198.51.100.10/24"}},
			},
			{
				Name:       "peer",
				Kind:       model.KindCEOS,
				Interfaces: []model.Interface{{Name: "Ethernet1", Address: "198.51.100.20/24"}},
			},
		},
		Links: []model.Link{
			{Name: "local-peer", A: "local", B: "peer", AIntf: "eth1", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/24"},
		},
	})
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	node := model.Node{
		Name: "local",
		PrefixLists: []model.PrefixList{{
			Name:  "NH",
			Rules: []model.PrefixListRule{{Seq: 10, Action: "permit", Prefix: "198.51.100.20/32"}},
		}},
	}
	rule := model.RoutePolicyRule{MatchNextHopPrefixList: "NH"}
	if !bgp.RoutePolicyRuleMatches(routePolicyResolver{idx: idx}, node, "", rule, testRIB("", withNextHop("peer"))) {
		t.Fatalf("routePolicyRuleMatches() = false, want next-hop prefix-list to match resolved peer address")
	}
}

func TestRoutePolicySetNextHopSelf(t *testing.T) {
	node := model.Node{
		Name: "core-gz",
		RoutePolicies: []model.RoutePolicy{{
			Name: "NH-SELF",
			Rules: []model.RoutePolicyRule{{
				Seq:            10,
				Action:         "permit",
				SetNextHopSelf: true,
			}},
		}},
	}
	route := domainroute.RIBEntry{
		NLRI:              domainroute.NLRI{Prefix: model.MustPrefix("10.3.0.0/16")},
		ForwardingNextHop: domainroute.NextHop{Node: "gz-edge1", Addr: "198.18.10.8"},
	}
	decision := bgp.ApplyRoutePolicy(routePolicyResolver{}, node, "core-bj", "NH-SELF", route)
	if !decision.Accept {
		t.Fatalf("decision rejected route: %#v", decision)
	}
	if decision.Route.ForwardingNextHop.Node != "core-gz" || decision.Route.ForwardingNextHop.Addr != "" {
		t.Fatalf("route next-hop = %#v, want core-gz self", decision.Route)
	}
}

func TestRoutePolicySetIPAddressNextHop(t *testing.T) {
	node := model.Node{
		Name: "local",
		RoutePolicies: []model.RoutePolicy{{
			Name: "SET-NH",
			Rules: []model.RoutePolicyRule{{
				Seq:        10,
				Action:     "permit",
				SetNextHop: "192.0.2.1",
			}},
		}},
	}
	decision := bgp.ApplyRoutePolicy(routePolicyResolver{}, node, "peer", "SET-NH", testRIB("10.0.0.0/24", withNextHop("local")))
	if !decision.Accept {
		t.Fatalf("decision rejected route: %#v", decision)
	}
	if decision.Route.ForwardingNextHop.Node != "" || decision.Route.ForwardingNextHop.Addr != "192.0.2.1" {
		t.Fatalf("route next-hop = %#v, want address-only recursive next-hop", decision.Route)
	}
}

func TestPrefixListRuleMatchesUsesNLRILengthSemantics(t *testing.T) {
	rule := model.PrefixListRule{Seq: 10, Action: "permit", Prefix: "10.0.0.0/8", Ge: 16, Le: 24}
	node := model.Node{PrefixLists: []model.PrefixList{{Name: "PL", Rules: []model.PrefixListRule{rule}}}}
	if !bgp.PrefixListPermitsPrefix(node, "PL", model.MustPrefix("10.4.0.0/16").NetIP()) {
		t.Fatalf("prefix-list range should match NLRI inside ge/le bounds")
	}
	if bgp.PrefixListPermitsPrefix(node, "PL", model.MustPrefix("10.4.1.10/32").NetIP()) {
		t.Fatalf("prefix-list range should reject NLRI longer than le")
	}
	if bgp.PrefixListPermitsPrefix(node, "PL", model.MustPrefix("10.0.0.0/8").NetIP()) {
		t.Fatalf("prefix-list range should reject NLRI shorter than ge")
	}
}
