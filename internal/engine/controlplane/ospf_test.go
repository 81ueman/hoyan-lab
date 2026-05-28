package controlplane

import (
	"github.com/81ueman/hoyan-lab/internal/core/netaddr"
	"fmt"
	"github.com/81ueman/hoyan-lab/internal/core/predicate"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

func TestOSPFPrefersLowerMetricAndKeepsFallback(t *testing.T) {
	topo := &topology.Topology{
		Nodes: []topology.Node{
			ospfNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31", "eth2": "198.51.100.7/31"}, map[string]int{"eth1": 10, "eth2": 1}),
			ospfNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31"}, map[string]int{"eth1": 10, "eth2": 1}),
			ospfNode("r3", "10.255.3.3/32", map[string]string{"eth1": "198.51.100.3/31", "eth2": "198.51.100.4/31"}, map[string]int{"eth1": 1, "eth2": 1}),
			ospfNode("r4", "10.255.4.4/32", map[string]string{"eth1": "198.51.100.6/31", "eth2": "198.51.100.5/31"}, map[string]int{"eth1": 1, "eth2": 1}),
		},
		Links: []topology.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "r2-r3", A: "r2", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
			{Name: "r3-r4", A: "r3", AIntf: "eth2", B: "r4", BIntf: "eth2", Cost: 1, Subnet: "198.51.100.4/31"},
			{Name: "r4-r1", A: "r4", AIntf: "eth1", B: "r1", BIntf: "eth2", Cost: 1, Subnet: "198.51.100.6/31"},
		},
	}
	rib := simulateOSPFTestRIB(t, topo)
	routes := rib["r1"]["10.255.2.2/32"]
	if len(routes) < 2 {
		t.Fatalf("r1 routes to r2 loopback = %#v, want primary and fallback", routes)
	}
	best := routes[0].Normalize()
	if best.SourceKind != topology.RouteSourceOSPF || best.RouteSource.Metric != 3 || best.ForwardingNextHop.Node != "r4" {
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
	topo := &topology.Topology{
		Nodes: []topology.Node{
			ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"lo": "1", "eth1": "1"}, nil),
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31"}, map[string]string{"lo": "0", "eth1": "1", "eth2": "0"}, nil),
			ospfAreaNode("r3", "10.255.3.3/32", map[string]string{"eth1": "198.51.100.3/31", "eth2": "198.51.100.4/31"}, map[string]string{"lo": "0", "eth1": "0", "eth2": "2"}, nil),
			ospfAreaNode("r4", "10.255.4.4/32", map[string]string{"eth1": "198.51.100.5/31"}, map[string]string{"lo": "2", "eth1": "2"}, nil),
		},
		Links: []topology.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "r2-r3", A: "r2", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
			{Name: "r3-r4", A: "r3", AIntf: "eth2", B: "r4", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.4/31"},
		},
	}
	rib := simulateOSPFTestRIB(t, topo)
	routes := rib["r1"]["10.255.4.4/32"]
	if len(routes) == 0 {
		t.Fatalf("r1 did not learn r4 loopback")
	}
	best := routes[0].Normalize()
	if best.SourceKind != topology.RouteSourceOSPF || best.RouteSource.OSPFRouteType != "inter-area" || best.RouteSource.Metric != 3 || best.ForwardingNextHop.Node != "r2" {
		t.Fatalf("best route = %#v, want inter-area OSPF metric 3 via r2", best)
	}
	if got := rib["r1"]["198.51.100.2/31"]; len(got) == 0 || got[0].Normalize().RouteSource.OSPFRouteType != "inter-area" {
		t.Fatalf("r1 backbone link route = %#v, want inter-area route", got)
	}
}

func TestOSPFSharedBroadcastSegmentInstallsRoutes(t *testing.T) {
	topo := &topology.Topology{
		Nodes: []topology.Node{
			ospfBroadcastNode("r1", "10.255.1.1/32", "198.51.100.1/29", 5),
			ospfBroadcastNode("r2", "10.255.2.2/32", "198.51.100.2/29", 2),
			ospfBroadcastNode("r3", "10.255.3.3/32", "198.51.100.3/29", 3),
		},
		Links: []topology.Link{
			{Name: "sw1-r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/29"},
			{Name: "sw1-r1-r3", A: "r1", AIntf: "eth1", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/29"},
			{Name: "sw1-r2-r3", A: "r2", AIntf: "eth1", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/29"},
		},
	}
	idx, err := topology.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	engine := NewEngine(idx, map[string]map[string]map[string][]RIBEntry{})
	states := engine.ospfInterfaceStates(topology.NetworkInstanceDefault, engine.ospfProcesses(topology.NetworkInstanceDefault))
	adjs := engine.ospfAdjacencies("r1", states, func(fromState, toState ospfInterfaceState) (string, bool) {
		if fromState.Area != toState.Area {
			return "", false
		}
		return fromState.Area, true
	})
	if len(adjs) != 2 {
		t.Fatalf("r1 adjacencies = %#v, want r2 and r3 on shared segment", adjs)
	}

	rib := simulateOSPFTestRIB(t, topo)
	routes := rib["r1"]["10.255.3.3/32"]
	if len(routes) == 0 {
		t.Fatalf("r1 did not learn r3 loopback")
	}
	best := routes[0].Normalize()
	if best.SourceKind != topology.RouteSourceOSPF || best.RouteSource.Metric != 5 || best.ForwardingNextHop.Node != "r3" || best.RouteSource.OSPFRouteType != "intra-area" {
		t.Fatalf("best route = %#v, want OSPF metric 5 via r3", best)
	}
}

func TestOSPFStubSuppressesExternalAndInstallsDefault(t *testing.T) {
	topo := &topology.Topology{
		Nodes: []topology.Node{
			ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"eth1": "0"}, nil),
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31"}, map[string]string{"eth1": "0", "eth2": "1"}, map[string]topology.OSPFArea{"1": {ID: "1", Kind: topology.OSPFAreaStub}}),
			ospfAreaNode("r3", "10.255.3.3/32", map[string]string{"eth1": "198.51.100.3/31"}, map[string]string{"eth1": "1"}, map[string]topology.OSPFArea{"1": {ID: "1", Kind: topology.OSPFAreaStub}}),
		},
		Links: []topology.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "r2-r3", A: "r2", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
		},
	}
	topo.Nodes[0].Routes = []topology.ConfiguredRoute{{Prefix: netaddr.MustPrefix("203.0.113.0/24"), Kind: topology.RouteSourceStatic, NextHop: "192.0.2.254"}}
	topo.Nodes[0].OSPF.Redistribute = []topology.OSPFRedistribution{{Kind: topology.RouteSourceStatic}}

	rib := simulateOSPFTestRIB(t, topo)
	if routes := rib["r3"]["203.0.113.0/24"]; len(routes) != 0 {
		t.Fatalf("r3 external routes = %#v, want suppressed in stub area", routes)
	}
	if routes := rib["r3"]["0.0.0.0/0"]; len(routes) == 0 {
		t.Fatalf("r3 default route missing, want stub default from ABR")
	}
	if routes := rib["r2"]["0.0.0.0/0"]; len(routes) != 0 {
		t.Fatalf("r2 default routes = %#v, want no default originated by non-ABR stub router", routes)
	}
}

func TestOSPFNSSAAllowsLocalExternalAndBlocksNormalExternal(t *testing.T) {
	topo := &topology.Topology{
		Nodes: []topology.Node{
			ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"eth1": "0"}, nil),
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31"}, map[string]string{"eth1": "0", "eth2": "2"}, map[string]topology.OSPFArea{"2": {ID: "2", Kind: topology.OSPFAreaNSSA, DefaultInformationOriginate: true}}),
			ospfAreaNode("r3", "10.255.3.3/32", map[string]string{"eth1": "198.51.100.3/31"}, map[string]string{"eth1": "2"}, map[string]topology.OSPFArea{"2": {ID: "2", Kind: topology.OSPFAreaNSSA}}),
		},
		Links: []topology.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "r2-r3", A: "r2", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
		},
	}
	topo.Nodes[0].Routes = []topology.ConfiguredRoute{{Prefix: netaddr.MustPrefix("203.0.113.0/24"), Kind: topology.RouteSourceStatic, NextHop: "192.0.2.254"}}
	topo.Nodes[0].OSPF.Redistribute = []topology.OSPFRedistribution{{Kind: topology.RouteSourceStatic}}
	topo.Nodes[2].Routes = []topology.ConfiguredRoute{{Prefix: netaddr.MustPrefix("198.18.3.0/24"), Kind: topology.RouteSourceStatic, NextHop: "192.0.2.253"}}
	topo.Nodes[2].OSPF.Redistribute = []topology.OSPFRedistribution{{Kind: topology.RouteSourceStatic}}

	rib := simulateOSPFTestRIB(t, topo)
	if routes := rib["r1"]["198.18.3.0/24"]; len(routes) == 0 {
		t.Fatalf("r1 NSSA external route missing, want translated NSSA external from r3")
	}
	if routes := rib["r3"]["203.0.113.0/24"]; len(routes) != 0 {
		t.Fatalf("r3 normal external routes = %#v, want blocked from NSSA", routes)
	}
	if routes := rib["r3"]["0.0.0.0/0"]; len(routes) == 0 {
		t.Fatalf("r3 default route missing, want NSSA default-information-originate from ABR")
	}
}

func TestOSPFRedistributesConnectedWithRouteMapAndType1Metric(t *testing.T) {
	r1 := ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"lo": "0", "eth1": "0"}, nil)
	r1.Interfaces = append(r1.Interfaces, topology.Interface{Name: "svc0", Address: "198.18.1.1/24"})
	r1.OSPF.Redistribute = []topology.OSPFRedistribution{{Kind: topology.RouteSourceConnected, RouteMap: "CONN-TO-OSPF", MetricType: 1}}
	r1.PrefixLists = []topology.PrefixList{{
		Name: "ONLY-SVC",
		Rules: []topology.PrefixListRule{{
			Seq: 10, Action: "permit", Prefix: "198.18.1.0/24", Match: predicate.ExactPrefixSet{Prefix: netaddr.MustPrefix("198.18.1.0/24")},
		}},
	}}
	r1.RoutePolicies = []topology.RoutePolicy{{
		Name: "CONN-TO-OSPF",
		Rules: []topology.RoutePolicyRule{{
			Seq: 10, Action: "permit", MatchPrefixList: "ONLY-SVC", SetMED: testIntPtr(7),
		}},
	}}
	topo := &topology.Topology{
		Nodes: []topology.Node{
			r1,
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31"}, map[string]string{"lo": "0", "eth1": "0"}, nil),
		},
		Links: []topology.Link{{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"}},
	}

	rib := simulateOSPFTestRIB(t, topo)
	route := bestOSPFTestRoute(t, rib, "r2", "198.18.1.0/24")
	if route.RouteSource.OSPFRouteType != ospfRouteTypeExternal1 || route.RouteSource.Metric != 8 || route.ForwardingNextHop.Node != "r1" {
		t.Fatalf("redistributed connected route = %#v, want E1 metric 8 via r1", route)
	}
	if routes := rib["r2"]["10.255.1.1/32"]; len(routes) == 0 || routes[0].Normalize().RouteSource.OSPFRouteType != ospfRouteTypeIntraArea {
		t.Fatalf("r1 loopback route = %#v, want normal intra-area route unaffected by route-map", routes)
	}
}

func TestOSPFRedistributesStaticType2MetricWithoutPathCost(t *testing.T) {
	topo := &topology.Topology{
		Nodes: []topology.Node{
			ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"lo": "0", "eth1": "0"}, nil),
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31"}, map[string]string{"lo": "0", "eth1": "0"}, nil),
		},
		Links: []topology.Link{{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"}},
	}
	topo.Nodes[0].Routes = []topology.ConfiguredRoute{{Prefix: netaddr.MustPrefix("203.0.113.0/24"), Kind: topology.RouteSourceStatic, NextHop: "192.0.2.254"}}
	topo.Nodes[0].OSPF.Redistribute = []topology.OSPFRedistribution{{Kind: topology.RouteSourceStatic, Metric: 33, MetricType: 2}}

	rib := simulateOSPFTestRIB(t, topo)
	route := bestOSPFTestRoute(t, rib, "r2", "203.0.113.0/24")
	if route.RouteSource.OSPFRouteType != ospfRouteTypeExternal2 || route.RouteSource.Metric != 33 {
		t.Fatalf("redistributed static route = %#v, want E2 metric 33", route)
	}
}

func TestOSPFRedistributesLearnedBGPRoute(t *testing.T) {
	r1 := ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth2": "198.51.100.0/31"}, map[string]string{"lo": "0", "eth2": "0"}, nil)
	r1.ASN = 65001
	r1.Interfaces = append(r1.Interfaces, topology.Interface{Name: "eth1", Address: "192.0.2.1/31"})
	r1.Neighbors = []topology.BGPNeighbor{{Address: "192.0.2.0", RemoteAS: 65000, Activated: true, PeerNode: "r0"}}
	r1.OSPF.Redistribute = []topology.OSPFRedistribution{{Kind: topology.RouteSourceBGP, Metric: 12, MetricType: 2}}
	topo := &topology.Topology{
		Nodes: []topology.Node{
			{
				Name:       "r0",
				Kind:       topology.KindFRR,
				ASN:        65000,
				Prefixes:   netaddr.MustPrefixes("172.16.0.0/24"),
				Interfaces: []topology.Interface{{Name: "eth1", Address: "192.0.2.0/31"}},
				Neighbors:  []topology.BGPNeighbor{{Address: "192.0.2.1", RemoteAS: 65001, Activated: true, PeerNode: "r1"}},
			},
			r1,
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31"}, map[string]string{"lo": "0", "eth1": "0"}, nil),
		},
		Links: []topology.Link{
			{Name: "r0-r1", A: "r0", AIntf: "eth1", B: "r1", BIntf: "eth1", Cost: 1, Subnet: "192.0.2.0/31"},
			{Name: "r1-r2", A: "r1", AIntf: "eth2", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
		},
	}

	rib := simulateOSPFTestRIB(t, topo)
	route := bestOSPFTestRoute(t, rib, "r2", "172.16.0.0/24")
	if route.RouteSource.OSPFRouteType != ospfRouteTypeExternal2 || route.RouteSource.Metric != 12 || route.Provenance.OriginNode != "r1" {
		t.Fatalf("redistributed BGP route = %#v, want OSPF E2 from r1 metric 12", route)
	}
}

func bestOSPFTestRoute(t *testing.T, rib map[string]map[string][]RIBEntry, node, prefix string) RIBEntry {
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
	topo := &topology.Topology{
		Nodes: []topology.Node{
			ospfVRFNode("r1", map[string]topology.NetworkInstanceID{"eth1": "tenant-a", "eth2": "tenant-b"}, map[string]string{"eth1": "192.0.2.1/30", "eth2": "198.51.100.1/30"}, nil),
			ospfVRFNode("r2", map[string]topology.NetworkInstanceID{"eth1": "tenant-a", "a-svc": "tenant-a"}, map[string]string{"eth1": "192.0.2.2/30", "a-svc": "10.10.0.1/32"}, map[string]bool{"a-svc": true}),
			ospfVRFNode("r3", map[string]topology.NetworkInstanceID{"eth1": "tenant-b", "b-svc": "tenant-b"}, map[string]string{"eth1": "198.51.100.2/30", "b-svc": "10.20.0.1/32"}, map[string]bool{"b-svc": true}),
		},
		Links: []topology.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "192.0.2.0/30"},
			{Name: "r1-r3", A: "r1", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/30"},
		},
	}
	idx, err := topology.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	rib := map[string]map[string]map[string][]RIBEntry{}
	NewEngine(idx, rib).Simulate()
	if routes := rib["r1"]["tenant-a"]["10.10.0.1/32"]; len(routes) == 0 || routes[0].Normalize().SourceKind != topology.RouteSourceOSPF {
		t.Fatalf("r1 tenant-a route to 10.10.0.1/32 = %#v, want OSPF", routes)
	}
	if routes := rib["r1"]["tenant-a"]["10.20.0.1/32"]; len(routes) != 0 {
		t.Fatalf("r1 tenant-a leaked tenant-b service route: %#v", routes)
	}
	if routes := rib["r1"]["tenant-b"]["10.20.0.1/32"]; len(routes) == 0 || routes[0].Normalize().SourceKind != topology.RouteSourceOSPF {
		t.Fatalf("r1 tenant-b route to 10.20.0.1/32 = %#v, want OSPF", routes)
	}
	if routes := rib["r1"]["tenant-b"]["10.10.0.1/32"]; len(routes) != 0 {
		t.Fatalf("r1 tenant-b leaked tenant-a service route: %#v", routes)
	}
}

func simulateOSPFTestRIB(t *testing.T, topo *topology.Topology) map[string]map[string][]RIBEntry {
	t.Helper()
	idx, err := topology.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	rib := map[string]map[string]map[string][]RIBEntry{}
	NewEngine(idx, rib).Simulate()
	out := map[string]map[string][]RIBEntry{}
	for node, byVRF := range rib {
		out[node] = byVRF[string(topology.NetworkInstanceDefault)]
	}
	return out
}

func TestOSPFSPFScalesWithDenseTopology(t *testing.T) {
	topo := denseOSPFTopology(12)
	idx, err := topology.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	rib := map[string]map[string]map[string][]RIBEntry{}
	NewEngine(idx, rib).Simulate()
	routes := rib["r1"][string(topology.NetworkInstanceDefault)]["10.255.12.12/32"]
	if len(routes) != 11 {
		t.Fatalf("r1 routes to r12 loopback = %d, want one candidate per first hop", len(routes))
	}
	best := routes[0].Normalize()
	if best.ForwardingNextHop.Node != "r12" || best.RouteSource.Metric != 1 {
		t.Fatalf("best route = %#v, want direct SPF route to r12", best)
	}
	for _, route := range routes {
		route = route.Normalize()
		if len(route.Provenance.PathNodes) > 3 {
			t.Fatalf("route path = %#v, want SPF representative path without enumerated detours", route.Provenance.PathNodes)
		}
	}
}

func ospfNode(name, loopback string, ifaces map[string]string, costs map[string]int) topology.Node {
	interfaces := []topology.Interface{{Name: "lo", Address: loopback}}
	ospfIfaces := map[string]topology.OSPFInterface{
		"lo": {Name: "lo", Area: "0", Passive: true},
	}
	networks := []topology.OSPFNetwork{{Prefix: netaddr.MustPrefix(loopback), Area: "0"}}
	for name, addr := range ifaces {
		interfaces = append(interfaces, topology.Interface{Name: name, Address: addr})
		ospfIfaces[name] = topology.OSPFInterface{Name: name, Area: "0", Cost: costs[name]}
		networks = append(networks, topology.OSPFNetwork{Prefix: netaddr.MustPrefix(addr), Area: "0"})
	}
	return topology.Node{
		Name:       name,
		Kind:       topology.KindFRR,
		Loopback:   loopback,
		Prefixes:   netaddr.MustPrefixes(loopback),
		Interfaces: interfaces,
		OSPF: topology.OSPFProcess{
			Enabled:           true,
			RouterID:          loopback[:len(loopback)-3],
			Networks:          networks,
			PassiveInterfaces: []string{"lo"},
			Interfaces:        ospfIfaces,
		},
	}
}

func ospfAreaNode(name, loopback string, ifaces map[string]string, areasByIface map[string]string, areas map[string]topology.OSPFArea) topology.Node {
	loopbackArea := areasByIface["lo"]
	if loopbackArea == "" {
		loopbackArea = "0"
		for _, area := range areasByIface {
			loopbackArea = area
			break
		}
	}
	interfaces := []topology.Interface{{Name: "lo", Address: loopback}}
	ospfIfaces := map[string]topology.OSPFInterface{
		"lo": {Name: "lo", Area: loopbackArea, Passive: true},
	}
	networks := []topology.OSPFNetwork{{Prefix: netaddr.MustPrefix(loopback), Area: loopbackArea}}
	for ifName, addr := range ifaces {
		area := areasByIface[ifName]
		interfaces = append(interfaces, topology.Interface{Name: ifName, Address: addr})
		ospfIfaces[ifName] = topology.OSPFInterface{Name: ifName, Area: area, Cost: 1}
		networks = append(networks, topology.OSPFNetwork{Prefix: netaddr.MustPrefix(addr), Area: area})
	}
	if areas == nil {
		areas = map[string]topology.OSPFArea{}
	}
	return topology.Node{
		Name:       name,
		Kind:       topology.KindFRR,
		Loopback:   loopback,
		Prefixes:   netaddr.MustPrefixes(loopback),
		Interfaces: interfaces,
		OSPF: topology.OSPFProcess{
			Enabled:           true,
			RouterID:          loopback[:len(loopback)-3],
			Networks:          networks,
			PassiveInterfaces: []string{"lo"},
			Interfaces:        ospfIfaces,
			Areas:             areas,
		},
	}
}

func ospfBroadcastNode(name, loopback, shared string, cost int) topology.Node {
	node := ospfNode(name, loopback, map[string]string{"eth1": shared}, map[string]int{"eth1": cost})
	iface := node.OSPF.Interfaces["eth1"]
	iface.NetworkType = "broadcast"
	node.OSPF.Interfaces["eth1"] = iface
	return node
}

func ospfVRFNode(name string, vrfs map[string]topology.NetworkInstanceID, addrs map[string]string, passive map[string]bool) topology.Node {
	var interfaces []topology.Interface
	byVRF := map[topology.NetworkInstanceID]topology.OSPFProcess{}
	for ifName, addr := range addrs {
		vrf := topology.NormalizeNetworkInstance(string(vrfs[ifName]))
		interfaces = append(interfaces, topology.Interface{Name: ifName, Address: addr, VRF: vrf})
		process := byVRF[vrf]
		process.Enabled = true
		process.NetworkInstance = vrf
		if process.Interfaces == nil {
			process.Interfaces = map[string]topology.OSPFInterface{}
		}
		process.Interfaces[ifName] = topology.OSPFInterface{Name: ifName, Area: "0", Cost: 1, Passive: passive[ifName]}
		process.Networks = append(process.Networks, topology.OSPFNetwork{Prefix: netaddr.MustPrefix(addr), Area: "0"})
		if passive[ifName] {
			process.PassiveInterfaces = append(process.PassiveInterfaces, ifName)
		}
		byVRF[vrf] = process
	}
	var processes []topology.OSPFProcess
	for _, process := range byVRF {
		processes = append(processes, process)
	}
	return topology.Node{
		Name:          name,
		Kind:          topology.KindFRR,
		Interfaces:    interfaces,
		OSPFProcesses: processes,
	}
}

func denseOSPFTopology(n int) *topology.Topology {
	ifaces := map[string]map[string]string{}
	costs := map[string]map[string]int{}
	var links []topology.Link
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
			links = append(links, topology.Link{
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
	nodes := make([]topology.Node, 0, n)
	for i := 1; i <= n; i++ {
		name := fmt.Sprintf("r%d", i)
		nodes = append(nodes, ospfNode(name, fmt.Sprintf("10.255.%d.%d/32", i, i), ifaces[name], costs[name]))
	}
	return &topology.Topology{Nodes: nodes, Links: links}
}
