package controlplane

import (
	"fmt"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

func TestOSPFPrefersLowerMetricAndKeepsFallback(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			ospfNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31", "eth2": "198.51.100.7/31"}, map[string]int{"eth1": 10, "eth2": 1}),
			ospfNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31"}, map[string]int{"eth1": 10, "eth2": 1}),
			ospfNode("r3", "10.255.3.3/32", map[string]string{"eth1": "198.51.100.3/31", "eth2": "198.51.100.4/31"}, map[string]int{"eth1": 1, "eth2": 1}),
			ospfNode("r4", "10.255.4.4/32", map[string]string{"eth1": "198.51.100.6/31", "eth2": "198.51.100.5/31"}, map[string]int{"eth1": 1, "eth2": 1}),
		},
		Links: []model.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "r2-r3", A: "r2", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
			{Name: "r3-r4", A: "r3", AIntf: "eth2", B: "r4", BIntf: "eth2", Cost: 1, Subnet: "198.51.100.4/31"},
			{Name: "r4-r1", A: "r4", AIntf: "eth1", B: "r1", BIntf: "eth2", Cost: 1, Subnet: "198.51.100.6/31"},
		},
	}
	rib := simulateOSPFTestRIB(t, topo)
	routes := rib["r1"][model.MustPrefix("10.255.2.2/32")]
	if len(routes) < 2 {
		t.Fatalf("r1 routes to r2 loopback = %#v, want primary and fallback", routes)
	}
	best := routes[0].Normalize()
	if best.SourceKind != model.RouteSourceOSPF || best.RouteSource.Metric != 3 || best.ForwardingNextHop.Node != "r4" {
		t.Fatalf("best route = %#v, want OSPF metric 3 via r4", best)
	}
	var fallbackFound bool
	for _, route := range routes {
		route = route.Normalize()
		if route.ForwardingNextHop.Node == "r2" && route.RouteSource.Metric == 10 {
			fallbackFound = true
		}
	}
	if !fallbackFound {
		t.Fatalf("routes = %#v, want fallback via r2 metric 10", routes)
	}
}

func TestOSPFInstallsInterAreaRoutesThroughABR(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"lo": "1", "eth1": "1"}, nil),
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31"}, map[string]string{"lo": "0", "eth1": "1", "eth2": "0"}, nil),
			ospfAreaNode("r3", "10.255.3.3/32", map[string]string{"eth1": "198.51.100.3/31", "eth2": "198.51.100.4/31"}, map[string]string{"lo": "0", "eth1": "0", "eth2": "2"}, nil),
			ospfAreaNode("r4", "10.255.4.4/32", map[string]string{"eth1": "198.51.100.5/31"}, map[string]string{"lo": "2", "eth1": "2"}, nil),
		},
		Links: []model.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "r2-r3", A: "r2", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
			{Name: "r3-r4", A: "r3", AIntf: "eth2", B: "r4", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.4/31"},
		},
	}
	rib := simulateOSPFTestRIB(t, topo)
	routes := rib["r1"][model.MustPrefix("10.255.4.4/32")]
	if len(routes) == 0 {
		t.Fatalf("r1 did not learn r4 loopback")
	}
	best := routes[0].Normalize()
	if best.SourceKind != model.RouteSourceOSPF || best.RouteSource.OSPFRouteType != "inter-area" || best.RouteSource.Metric != 3 || best.ForwardingNextHop.Node != "r2" {
		t.Fatalf("best route = %#v, want inter-area OSPF metric 3 via r2", best)
	}
	if got := rib["r1"][model.MustPrefix("198.51.100.2/31")]; len(got) == 0 || got[0].Normalize().RouteSource.OSPFRouteType != "inter-area" {
		t.Fatalf("r1 backbone link route = %#v, want inter-area route", got)
	}
}

func TestOSPFSharedBroadcastSegmentInstallsRoutes(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			ospfBroadcastNode("r1", "10.255.1.1/32", "198.51.100.1/29", 5),
			ospfBroadcastNode("r2", "10.255.2.2/32", "198.51.100.2/29", 2),
			ospfBroadcastNode("r3", "10.255.3.3/32", "198.51.100.3/29", 3),
		},
		Links: []model.Link{
			{Name: "sw1-r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/29"},
			{Name: "sw1-r1-r3", A: "r1", AIntf: "eth1", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/29"},
			{Name: "sw1-r2-r3", A: "r2", AIntf: "eth1", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/29"},
		},
	}
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	engine := NewEngine(idx, domainroute.RIBTable{})
	states := engine.ospfInterfaceStates(model.NetworkInstanceDefault, engine.ospfProcesses(model.NetworkInstanceDefault))
	adjs := engine.ospfAdjacencies("r1", states, func(fromState, toState InterfaceState) (string, bool) {
		if fromState.Area != toState.Area {
			return "", false
		}
		return fromState.Area, true
	})
	if len(adjs) != 2 {
		t.Fatalf("r1 adjacencies = %#v, want r2 and r3 on shared segment", adjs)
	}

	rib := simulateOSPFTestRIB(t, topo)
	routes := rib["r1"][model.MustPrefix("10.255.3.3/32")]
	if len(routes) == 0 {
		t.Fatalf("r1 did not learn r3 loopback")
	}
	best := routes[0].Normalize()
	if best.SourceKind != model.RouteSourceOSPF || best.RouteSource.Metric != 5 || best.ForwardingNextHop.Node != "r3" || best.RouteSource.OSPFRouteType != "intra-area" {
		t.Fatalf("best route = %#v, want OSPF metric 5 via r3", best)
	}
}

func TestOSPFStubSuppressesExternalAndInstallsDefault(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"eth1": "0"}, nil),
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31"}, map[string]string{"eth1": "0", "eth2": "1"}, map[string]model.OSPFArea{"1": {ID: "1", Kind: model.OSPFAreaStub}}),
			ospfAreaNode("r3", "10.255.3.3/32", map[string]string{"eth1": "198.51.100.3/31"}, map[string]string{"eth1": "1"}, map[string]model.OSPFArea{"1": {ID: "1", Kind: model.OSPFAreaStub}}),
		},
		Links: []model.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "r2-r3", A: "r2", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
		},
	}
	topo.Nodes[0].Routes = []model.ConfiguredRoute{{Prefix: model.MustPrefix("203.0.113.0/24"), Kind: model.RouteSourceStatic, NextHop: "192.0.2.254"}}
	topo.Nodes[0].OSPF.Redistribute = []model.OSPFRedistribution{{Kind: model.RouteSourceStatic}}

	rib := simulateOSPFTestRIB(t, topo)
	if routes := rib["r3"][model.MustPrefix("203.0.113.0/24")]; len(routes) != 0 {
		t.Fatalf("r3 external routes = %#v, want suppressed in stub area", routes)
	}
	if routes := rib["r3"][model.MustPrefix("0.0.0.0/0")]; len(routes) == 0 {
		t.Fatalf("r3 default route missing, want stub default from ABR")
	}
	if routes := rib["r2"][model.MustPrefix("0.0.0.0/0")]; len(routes) != 0 {
		t.Fatalf("r2 default routes = %#v, want no default originated by non-ABR stub router", routes)
	}
}

func TestOSPFNSSAAllowsLocalExternalAndBlocksNormalExternal(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"eth1": "0"}, nil),
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31"}, map[string]string{"eth1": "0", "eth2": "2"}, map[string]model.OSPFArea{"2": {ID: "2", Kind: model.OSPFAreaNSSA, DefaultInformationOriginate: true}}),
			ospfAreaNode("r3", "10.255.3.3/32", map[string]string{"eth1": "198.51.100.3/31"}, map[string]string{"eth1": "2"}, map[string]model.OSPFArea{"2": {ID: "2", Kind: model.OSPFAreaNSSA}}),
		},
		Links: []model.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "r2-r3", A: "r2", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
		},
	}
	topo.Nodes[0].Routes = []model.ConfiguredRoute{{Prefix: model.MustPrefix("203.0.113.0/24"), Kind: model.RouteSourceStatic, NextHop: "192.0.2.254"}}
	topo.Nodes[0].OSPF.Redistribute = []model.OSPFRedistribution{{Kind: model.RouteSourceStatic}}
	topo.Nodes[2].Routes = []model.ConfiguredRoute{{Prefix: model.MustPrefix("198.18.3.0/24"), Kind: model.RouteSourceStatic, NextHop: "192.0.2.253"}}
	topo.Nodes[2].OSPF.Redistribute = []model.OSPFRedistribution{{Kind: model.RouteSourceStatic}}

	rib := simulateOSPFTestRIB(t, topo)
	if routes := rib["r1"][model.MustPrefix("198.18.3.0/24")]; len(routes) == 0 {
		t.Fatalf("r1 NSSA external route missing, want translated NSSA external from r3")
	}
	if routes := rib["r3"][model.MustPrefix("203.0.113.0/24")]; len(routes) != 0 {
		t.Fatalf("r3 normal external routes = %#v, want blocked from NSSA", routes)
	}
	if routes := rib["r3"][model.MustPrefix("0.0.0.0/0")]; len(routes) == 0 {
		t.Fatalf("r3 default route missing, want NSSA default-information-originate from ABR")
	}
}

func TestOSPFRedistributesConnectedWithRouteMapAndType1Metric(t *testing.T) {
	r1 := ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"lo": "0", "eth1": "0"}, nil)
	r1.Interfaces = append(r1.Interfaces, model.Interface{Name: "svc0", Address: "198.18.1.1/24"})
	r1.OSPF.Redistribute = []model.OSPFRedistribution{{Kind: model.RouteSourceConnected, RouteMap: "CONN-TO-OSPF", MetricType: 1}}
	r1.PrefixLists = []model.PrefixList{{
		Name: "ONLY-SVC",
		Rules: []model.PrefixListRule{{
			Seq: 10, Action: "permit", Prefix: "198.18.1.0/24", Match: model.ExactPrefixSet{Prefix: model.MustPrefix("198.18.1.0/24")},
		}},
	}}
	r1.RoutePolicies = []model.RoutePolicy{{
		Name: "CONN-TO-OSPF",
		Rules: []model.RoutePolicyRule{{
			Seq: 10, Action: "permit", MatchPrefixList: "ONLY-SVC", SetMED: testIntPtr(7),
		}},
	}}
	topo := &model.Topology{
		Nodes: []model.Node{
			r1,
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31"}, map[string]string{"lo": "0", "eth1": "0"}, nil),
		},
		Links: []model.Link{{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"}},
	}

	rib := simulateOSPFTestRIB(t, topo)
	route := bestOSPFTestRoute(t, rib, "r2", model.MustPrefix("198.18.1.0/24"))
	if route.RouteSource.OSPFRouteType != RouteTypeExternal1 || route.RouteSource.Metric != 8 || route.ForwardingNextHop.Node != "r1" {
		t.Fatalf("redistributed connected route = %#v, want E1 metric 8 via r1", route)
	}
	if routes := rib["r2"][model.MustPrefix("10.255.1.1/32")]; len(routes) == 0 || routes[0].Normalize().RouteSource.OSPFRouteType != RouteTypeIntraArea {
		t.Fatalf("r1 loopback route = %#v, want normal intra-area route unaffected by route-map", routes)
	}
}

func TestOSPFRedistributesStaticType2MetricWithoutPathCost(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"lo": "0", "eth1": "0"}, nil),
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31"}, map[string]string{"lo": "0", "eth1": "0"}, nil),
		},
		Links: []model.Link{{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"}},
	}
	topo.Nodes[0].Routes = []model.ConfiguredRoute{{Prefix: model.MustPrefix("203.0.113.0/24"), Kind: model.RouteSourceStatic, NextHop: "192.0.2.254"}}
	topo.Nodes[0].OSPF.Redistribute = []model.OSPFRedistribution{{Kind: model.RouteSourceStatic, Metric: 33, MetricType: 2}}

	rib := simulateOSPFTestRIB(t, topo)
	route := bestOSPFTestRoute(t, rib, "r2", model.MustPrefix("203.0.113.0/24"))
	if route.RouteSource.OSPFRouteType != RouteTypeExternal2 || route.RouteSource.Metric != 33 {
		t.Fatalf("redistributed static route = %#v, want E2 metric 33", route)
	}
}

func TestOSPFRedistributesLearnedBGPRoute(t *testing.T) {
	r1 := ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth2": "198.51.100.0/31"}, map[string]string{"lo": "0", "eth2": "0"}, nil)
	r1.ASN = 65001
	r1.Interfaces = append(r1.Interfaces, model.Interface{Name: "eth1", Address: "192.0.2.1/31"})
	r1.Neighbors = []model.BGPNeighbor{{Address: "192.0.2.0", RemoteAS: 65000, Activated: true, PeerNode: "r0"}}
	r1.OSPF.Redistribute = []model.OSPFRedistribution{{Kind: model.RouteSourceBGP, Metric: 12, MetricType: 2}}
	topo := &model.Topology{
		Nodes: []model.Node{
			{
				Name:       "r0",
				Kind:       model.KindFRR,
				ASN:        65000,
				Prefixes:   model.MustPrefixes("172.16.0.0/24"),
				Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.0/31"}},
				Neighbors:  []model.BGPNeighbor{{Address: "192.0.2.1", RemoteAS: 65001, Activated: true, PeerNode: "r1"}},
			},
			r1,
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31"}, map[string]string{"lo": "0", "eth1": "0"}, nil),
		},
		Links: []model.Link{
			{Name: "r0-r1", A: "r0", AIntf: "eth1", B: "r1", BIntf: "eth1", Cost: 1, Subnet: "192.0.2.0/31"},
			{Name: "r1-r2", A: "r1", AIntf: "eth2", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
		},
	}

	rib := simulateOSPFTestRIB(t, topo)
	route := bestOSPFTestRoute(t, rib, "r2", model.MustPrefix("172.16.0.0/24"))
	if route.RouteSource.OSPFRouteType != RouteTypeExternal2 || route.RouteSource.Metric != 12 || route.Provenance.OriginNode != "r1" {
		t.Fatalf("redistributed BGP route = %#v, want OSPF E2 from r1 metric 12", route)
	}
}

func bestOSPFTestRoute(t *testing.T, rib map[model.NodeID]map[model.Prefix][]domainroute.RIBEntry, node model.NodeID, prefix model.Prefix) domainroute.RIBEntry {
	t.Helper()
	routes := rib[node][prefix]
	if len(routes) == 0 {
		t.Fatalf("%s route to %s missing", node, prefix)
	}
	return routes[0].Normalize()
}

func testIntPtr(v int) *int {
	return &v
}

func TestOSPFProcessesStaySeparatedByVRF(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			ospfVRFNode("r1", map[string]model.NetworkInstanceID{"eth1": "tenant-a", "eth2": "tenant-b"}, map[string]string{"eth1": "192.0.2.1/30", "eth2": "198.51.100.1/30"}, nil),
			ospfVRFNode("r2", map[string]model.NetworkInstanceID{"eth1": "tenant-a", "a-svc": "tenant-a"}, map[string]string{"eth1": "192.0.2.2/30", "a-svc": "10.10.0.1/32"}, map[string]bool{"a-svc": true}),
			ospfVRFNode("r3", map[string]model.NetworkInstanceID{"eth1": "tenant-b", "b-svc": "tenant-b"}, map[string]string{"eth1": "198.51.100.2/30", "b-svc": "10.20.0.1/32"}, map[string]bool{"b-svc": true}),
		},
		Links: []model.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "192.0.2.0/30"},
			{Name: "r1-r3", A: "r1", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/30"},
		},
	}
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	rib := domainroute.RIBTable{}
	if err := NewEngine(idx, rib).Simulate(); err != nil {
		t.Fatalf("Simulate() error = %v", err)
	}
	if routes := rib["r1"]["tenant-a"][model.MustPrefix("10.10.0.1/32")]; len(routes) == 0 || routes[0].Normalize().SourceKind != model.RouteSourceOSPF {
		t.Fatalf("r1 tenant-a route to 10.10.0.1/32 = %#v, want OSPF", routes)
	}
	if routes := rib["r1"]["tenant-a"][model.MustPrefix("10.20.0.1/32")]; len(routes) != 0 {
		t.Fatalf("r1 tenant-a leaked tenant-b service route: %#v", routes)
	}
	if routes := rib["r1"]["tenant-b"][model.MustPrefix("10.20.0.1/32")]; len(routes) == 0 || routes[0].Normalize().SourceKind != model.RouteSourceOSPF {
		t.Fatalf("r1 tenant-b route to 10.20.0.1/32 = %#v, want OSPF", routes)
	}
	if routes := rib["r1"]["tenant-b"][model.MustPrefix("10.10.0.1/32")]; len(routes) != 0 {
		t.Fatalf("r1 tenant-b leaked tenant-a service route: %#v", routes)
	}
}

// alternatePathTopology returns a topology for testing same-first-hop higher-cost
// OSPF alternates:
//
//	r1 --1-- a --1-- x --1-- d   (cost 3, primary)
//	          \
//	           2-- y --2-- d     (cost 5, same first-hop alternate)
//	r1 --10-- b --1-- d          (cost 11, different first-hop alternate)
func alternatePathTopology() *model.Topology {
	return &model.Topology{
		Nodes: []model.Node{
			ospfNode("r1", "10.255.1.1/32",
				map[string]string{"eth1": "198.51.100.0/31", "eth2": "198.51.100.10/31"},
				map[string]int{"eth1": 1, "eth2": 10}),
			ospfNode("a", "10.255.2.2/32",
				map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31", "eth3": "198.51.100.4/31"},
				map[string]int{"eth1": 1, "eth2": 1, "eth3": 2}),
			ospfNode("x", "10.255.4.4/32",
				map[string]string{"eth1": "198.51.100.3/31", "eth2": "198.51.100.6/31"},
				map[string]int{"eth1": 1, "eth2": 1}),
			ospfNode("y", "10.255.5.5/32",
				map[string]string{"eth1": "198.51.100.5/31", "eth2": "198.51.100.8/31"},
				map[string]int{"eth1": 2, "eth2": 2}),
			ospfNode("b", "10.255.3.3/32",
				map[string]string{"eth1": "198.51.100.11/31", "eth2": "198.51.100.12/31"},
				map[string]int{"eth1": 10, "eth2": 1}),
			ospfNode("d", "10.255.6.6/32",
				map[string]string{"eth1": "198.51.100.7/31", "eth2": "198.51.100.9/31", "eth3": "198.51.100.13/31"},
				map[string]int{"eth1": 1, "eth2": 2, "eth3": 1}),
		},
		Links: []model.Link{
			{Name: "r1-a", A: "r1", AIntf: "eth1", B: "a", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "a-x", A: "a", AIntf: "eth2", B: "x", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
			{Name: "a-y", A: "a", AIntf: "eth3", B: "y", BIntf: "eth1", Cost: 2, Subnet: "198.51.100.4/31"},
			{Name: "x-d", A: "x", AIntf: "eth2", B: "d", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.6/31"},
			{Name: "y-d", A: "y", AIntf: "eth2", B: "d", BIntf: "eth2", Cost: 2, Subnet: "198.51.100.8/31"},
			{Name: "r1-b", A: "r1", AIntf: "eth2", B: "b", BIntf: "eth1", Cost: 10, Subnet: "198.51.100.10/31"},
			{Name: "b-d", A: "b", AIntf: "eth2", B: "d", BIntf: "eth3", Cost: 1, Subnet: "198.51.100.12/31"},
		},
	}
}

func TestOSPFInstallsSameFirstHopAlternate(t *testing.T) {
	topo := alternatePathTopology()
	rib := simulateOSPFTestRIB(t, topo)
	routes := rib["r1"][model.MustPrefix("10.255.6.6/32")]
	if len(routes) < 2 {
		t.Fatalf("r1 routes to d = %d, want at least 2 (primary + alternate)", len(routes))
	}

	// Primary: via a, cost 3
	best := routes[0].Normalize()
	if best.RouteSource.Metric != 3 || best.ForwardingNextHop.Node != "a" {
		t.Fatalf("best route = %#v, want metric 3 via a", best)
	}

	// Same-first-hop alternate: via a, cost 5, path r1-a-y-d
	var sameFirstHopAlt bool
	for _, r := range routes {
		r = r.Normalize()
		if r.ForwardingNextHop.Node == "a" && r.RouteSource.Metric == 5 {
			sameFirstHopAlt = true
			if len(r.Provenance.PathNodes) != 4 || r.Provenance.PathNodes[0] != "r1" || r.Provenance.PathNodes[1] != "a" || r.Provenance.PathNodes[2] != "y" || r.Provenance.PathNodes[3] != "d" {
				t.Fatalf("same-first-hop alternate path nodes = %v, want [r1 a y d]", r.Provenance.PathNodes)
			}
			break
		}
	}
	if !sameFirstHopAlt {
		t.Fatalf("routes = %#v, want same-first-hop alternate via a metric 5", routes)
	}

	// Different-first-hop alternate: via b, cost 11
	var diffFirstHopAlt bool
	for _, r := range routes {
		r = r.Normalize()
		if r.ForwardingNextHop.Node == "b" && r.RouteSource.Metric == 11 {
			diffFirstHopAlt = true
			break
		}
	}
	if !diffFirstHopAlt {
		t.Fatalf("routes = %#v, want different-first-hop alternate via b metric 11", routes)
	}
}

func TestOSPFAlternateSelectedUnderPrimaryLinkFailure(t *testing.T) {
	topo := alternatePathTopology()
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	rib := domainroute.RIBTable{}
	if err := NewEngine(idx, rib).Simulate(); err != nil {
		t.Fatalf("Simulate() error = %v", err)
	}

	routes := rib["r1"][model.NetworkInstanceDefault][model.MustPrefix("10.255.6.6/32")]
	if len(routes) < 2 {
		t.Fatalf("r1 routes to d = %d, want at least 2", len(routes))
	}

	// Primary link a-x fails
	ctx := failure.Context{
		Failures: failure.Set{
			Links: map[model.LinkID]bool{"a-x": true},
			Nodes: map[model.NodeID]bool{},
		},
		LinksByName: map[model.LinkID]model.Link{},
	}

	// The alternate via a-y-d should be selected when a-x fails
	var alternateSelected bool
	for _, r := range routes {
		r = r.Normalize()
		if r.ForwardingNextHop.Node == "a" && r.RouteSource.Metric == 5 {
			if r.SelectedCond == nil {
				t.Fatalf("alternate route via a (metric 5) has nil SelectedCond")
			}
			if r.SelectedCond.Eval(ctx) {
				alternateSelected = true
			}
		}
	}
	if !alternateSelected {
		t.Fatalf("alternate route via a-y-d should be selected when a-x fails")
	}

	// The primary via a-x-d should NOT be selected when a-x fails
	var primarySelected bool
	for _, r := range routes {
		r = r.Normalize()
		if r.ForwardingNextHop.Node == "a" && r.RouteSource.Metric == 3 {
			if r.SelectedCond != nil && r.SelectedCond.Eval(ctx) {
				primarySelected = true
			}
		}
	}
	if primarySelected {
		t.Fatalf("primary route via a-x-d should not be selected when a-x fails")
	}
}

func simulateOSPFTestRIB(t *testing.T, topo *model.Topology) map[model.NodeID]map[model.Prefix][]domainroute.RIBEntry {
	t.Helper()
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	rib := domainroute.RIBTable{}
	if err := NewEngine(idx, rib).Simulate(); err != nil {
		t.Fatalf("Simulate() error = %v", err)
	}
	out := map[model.NodeID]map[model.Prefix][]domainroute.RIBEntry{}
	for node, byVRF := range rib {
		out[node] = byVRF[model.NetworkInstanceDefault]
	}
	return out
}

func TestOSPFSPFScalesWithDenseTopology(t *testing.T) {
	topo := denseOSPFTopology(12)
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	rib := domainroute.RIBTable{}
	if err := NewEngine(idx, rib).Simulate(); err != nil {
		t.Fatalf("Simulate() error = %v", err)
	}
	routes := rib["r1"][model.NetworkInstanceDefault][model.MustPrefix("10.255.12.12/32")]
	if len(routes) == 0 {
		t.Fatalf("r1 routes to r12 loopback missing")
	}
	if len(routes) > MaxPathsPerDestination {
		t.Fatalf("r1 routes to r12 loopback = %d, want at most MaxPathsPerDestination (%d)", len(routes), MaxPathsPerDestination)
	}
	best := routes[0].Normalize()
	if best.ForwardingNextHop.Node != "r12" || best.RouteSource.Metric != 1 {
		t.Fatalf("best route = %#v, want direct SPF route to r12", best)
	}
}

func ospfNode(name, loopback string, ifaces map[string]string, costs map[string]int) model.Node {
	interfaces := []model.Interface{{Name: "lo", Address: loopback}}
	ospfIfaces := map[string]model.OSPFInterface{
		"lo": {Name: "lo", Area: "0", Passive: true},
	}
	networks := []model.OSPFNetwork{{Prefix: model.MustPrefix(loopback), Area: "0"}}
	for name, addr := range ifaces {
		interfaces = append(interfaces, model.Interface{Name: name, Address: addr})
		ospfIfaces[name] = model.OSPFInterface{Name: name, Area: "0", Cost: costs[name]}
		networks = append(networks, model.OSPFNetwork{Prefix: model.MustPrefix(addr), Area: "0"})
	}
	return model.Node{
		Name:       name,
		Kind:       model.KindFRR,
		Loopback:   loopback,
		Prefixes:   model.MustPrefixes(loopback),
		Interfaces: interfaces,
		OSPF: model.OSPFProcess{
			Enabled:           true,
			RouterID:          loopback[:len(loopback)-3],
			Networks:          networks,
			PassiveInterfaces: []string{"lo"},
			Interfaces:        ospfIfaces,
		},
	}
}

func ospfAreaNode(name, loopback string, ifaces map[string]string, areasByIface map[string]string, areas map[string]model.OSPFArea) model.Node {
	loopbackArea := areasByIface["lo"]
	if loopbackArea == "" {
		loopbackArea = "0"
		for _, area := range areasByIface {
			loopbackArea = area
			break
		}
	}
	interfaces := []model.Interface{{Name: "lo", Address: loopback}}
	ospfIfaces := map[string]model.OSPFInterface{
		"lo": {Name: "lo", Area: loopbackArea, Passive: true},
	}
	networks := []model.OSPFNetwork{{Prefix: model.MustPrefix(loopback), Area: loopbackArea}}
	for ifName, addr := range ifaces {
		area := areasByIface[ifName]
		interfaces = append(interfaces, model.Interface{Name: ifName, Address: addr})
		ospfIfaces[ifName] = model.OSPFInterface{Name: ifName, Area: area, Cost: 1}
		networks = append(networks, model.OSPFNetwork{Prefix: model.MustPrefix(addr), Area: area})
	}
	if areas == nil {
		areas = map[string]model.OSPFArea{}
	}
	return model.Node{
		Name:       name,
		Kind:       model.KindFRR,
		Loopback:   loopback,
		Prefixes:   model.MustPrefixes(loopback),
		Interfaces: interfaces,
		OSPF: model.OSPFProcess{
			Enabled:           true,
			RouterID:          loopback[:len(loopback)-3],
			Networks:          networks,
			PassiveInterfaces: []string{"lo"},
			Interfaces:        ospfIfaces,
			Areas:             areas,
		},
	}
}

func ospfBroadcastNode(name, loopback, shared string, cost int) model.Node {
	node := ospfNode(name, loopback, map[string]string{"eth1": shared}, map[string]int{"eth1": cost})
	iface := node.OSPF.Interfaces["eth1"]
	iface.NetworkType = "broadcast"
	node.OSPF.Interfaces["eth1"] = iface
	return node
}

func ospfVRFNode(name string, vrfs map[string]model.NetworkInstanceID, addrs map[string]string, passive map[string]bool) model.Node {
	var interfaces []model.Interface
	byVRF := map[model.NetworkInstanceID]model.OSPFProcess{}
	for ifName, addr := range addrs {
		vrf := model.NormalizeNetworkInstance(string(vrfs[ifName]))
		interfaces = append(interfaces, model.Interface{Name: ifName, Address: addr, VRF: vrf})
		process := byVRF[vrf]
		process.Enabled = true
		process.NetworkInstance = vrf
		if process.Interfaces == nil {
			process.Interfaces = map[string]model.OSPFInterface{}
		}
		process.Interfaces[ifName] = model.OSPFInterface{Name: ifName, Area: "0", Cost: 1, Passive: passive[ifName]}
		process.Networks = append(process.Networks, model.OSPFNetwork{Prefix: model.MustPrefix(addr), Area: "0"})
		if passive[ifName] {
			process.PassiveInterfaces = append(process.PassiveInterfaces, ifName)
		}
		byVRF[vrf] = process
	}
	var processes []model.OSPFProcess
	for _, process := range byVRF {
		processes = append(processes, process)
	}
	return model.Node{
		Name:          name,
		Kind:          model.KindFRR,
		Interfaces:    interfaces,
		OSPFProcesses: processes,
	}
}

func denseOSPFTopology(n int) *model.Topology {
	ifaces := map[string]map[string]string{}
	costs := map[string]map[string]int{}
	var links []model.Link
	linkID := 0
	for i := 1; i <= n; i++ {
		ifaces[fmt.Sprintf("r%d", i)] = map[string]string{}
		costs[fmt.Sprintf("r%d", i)] = map[string]int{}
	}
	for i := 1; i <= n; i++ {
		for j := i + 1; j <= n; j++ {
			linkID++
			a := fmt.Sprintf("r%d", i)
			b := fmt.Sprintf("r%d", j)
			aIface := fmt.Sprintf("eth%d", j)
			bIface := fmt.Sprintf("eth%d", i)
			subnet := fmt.Sprintf("198.%d.%d.0/31", linkID/255, linkID%255)
			ifaces[a][aIface] = fmt.Sprintf("198.%d.%d.0/31", linkID/255, linkID%255)
			ifaces[b][bIface] = fmt.Sprintf("198.%d.%d.1/31", linkID/255, linkID%255)
			costs[a][aIface] = 1
			costs[b][bIface] = 1
			links = append(links, model.Link{
				Name:   fmt.Sprintf("%s-%s", a, b),
				A:      a,
				AIntf:  aIface,
				B:      b,
				BIntf:  bIface,
				Cost:   1,
				Subnet: subnet,
			})
		}
	}
	nodes := make([]model.Node, 0, n)
	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("r%d", i)
		nodes = append(nodes, ospfNode(name, fmt.Sprintf("10.255.%d.%d/32", i, i), ifaces[name], costs[name]))
	}
	return &model.Topology{Nodes: nodes, Links: links}
}
