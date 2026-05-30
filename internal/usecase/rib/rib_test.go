package rib

import (
	"path/filepath"
	"reflect"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	observationrib "github.com/81ueman/hoyan-lab/internal/domain/observation/rib"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

func TestExpectedRoutesIncludesMultipleBgpPaths(t *testing.T) {
	topo, err := topology.LoadTopology(filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"))
	if err != nil {
		t.Fatalf("LoadLabTopology() error = %v", err)
	}
	routes := Expected(topo)
	if len(routes) == 0 {
		t.Fatalf("Expected() returned no routes")
	}
	for _, r := range routes {
		if r.Node == "bj-edge1" && r.Prefix == "10.4.1.10/32" {
			if len(r.Paths) < 2 {
				t.Fatalf("bj-edge1 route paths = %#v, want multiple BGP paths", r.Paths)
			}
			return
		}
	}
	t.Fatalf("expected bj-edge1 route to hz customer host")
}

func TestExpectedRoutesIncludesStaticAndConnectedSources(t *testing.T) {
	staticPrefix := model.MustPrefix("203.0.113.0/24")
	blackholePrefix := model.MustPrefix("198.51.100.0/24")
	topo := &model.Topology{
		Nodes: []model.Node{{
			Name:       "r1",
			Kind:       model.KindFRR,
			Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.1/30"}},
			Routes: []model.ConfiguredRoute{
				{Prefix: staticPrefix, Kind: model.RouteSourceStatic, NextHop: "192.0.2.2"},
				{Prefix: blackholePrefix, Kind: model.RouteSourceBlackhole, Interface: "Null0"},
			},
		}, {
			Name:       "r2",
			Kind:       model.KindFRR,
			Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.2/30"}},
		}},
		Links: []model.Link{{Name: "r1-r2", A: "r1", B: "r2", AIntf: "eth1", BIntf: "eth1", Cost: 1}},
	}
	routes := Expected(topo)
	if routeByPrefixProtocol(routes, "192.0.2.0/30", "connected") == nil {
		t.Fatalf("connected route missing from expected RIB routes: %#v", routes)
	}
	if routeByPrefixProtocol(routes, staticPrefix.String(), "static") == nil {
		t.Fatalf("static route missing from expected RIB routes: %#v", routes)
	}
	if routeByPrefixProtocol(routes, blackholePrefix.String(), "blackhole") == nil {
		t.Fatalf("blackhole route missing from expected RIB routes: %#v", routes)
	}
}

func TestExpectedRoutesKeepsBGPNetworkAndLocalBlackholeSeparate(t *testing.T) {
	prefix := model.MustPrefix("203.0.113.0/24")
	topo := &model.Topology{Nodes: []model.Node{{
		Name:     "r1",
		Kind:     model.KindFRR,
		ASN:      65000,
		Prefixes: []model.Prefix{prefix},
		Routes:   []model.ConfiguredRoute{{Prefix: prefix, Kind: model.RouteSourceBlackhole, Interface: "Null0"}},
	}}}
	routes := Expected(topo)
	if routeByPrefixProtocol(routes, prefix.String(), "bgp") == nil {
		t.Fatalf("BGP network route missing: %#v", routes)
	}
	if routeByPrefixProtocol(routes, prefix.String(), "blackhole") == nil {
		t.Fatalf("local blackhole route missing: %#v", routes)
	}
}

func TestExpectedConnectedRoutesCarryClassAndIncludeLoopbackService(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{{
			Name: "svc",
			Kind: model.KindFRR,
			Role: "customer",
			Interfaces: []model.Interface{
				{Name: "lo", Address: "10.0.0.10/32"},
				{Name: "eth1", Address: "192.0.2.1/31"},
			},
		}, {
			Name:       "r2",
			Kind:       model.KindFRR,
			Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.0/31"}},
		}},
		Links: []model.Link{{Name: "svc-r2", A: "svc", B: "r2", AIntf: "eth1", BIntf: "eth1", Cost: 1}},
	}
	routes := Expected(topo)
	link := routeByPrefixProtocol(routes, "192.0.2.0/31", "connected")
	if link == nil || link.ConnectedClass != model.ConnectedRouteClassLink {
		t.Fatalf("link connected route = %#v", link)
	}
	service := routeByPrefixProtocol(routes, "10.0.0.10/32", "connected")
	if service == nil || service.ConnectedClass != model.ConnectedRouteClassService {
		t.Fatalf("service connected route = %#v", service)
	}
}

func TestExpectedOSPFRoutesIncludeLocalAndSelectedRemoteRoutes(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			ospfExpectedNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31", "eth2": "198.51.100.7/31"}, map[string]int{"eth1": 10, "eth2": 1}),
			ospfExpectedNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31"}, map[string]int{"eth1": 10, "eth2": 1}),
			ospfExpectedNode("r3", "10.255.3.3/32", map[string]string{"eth1": "198.51.100.3/31", "eth2": "198.51.100.4/31"}, map[string]int{"eth1": 1, "eth2": 1}),
			ospfExpectedNode("r4", "10.255.4.4/32", map[string]string{"eth1": "198.51.100.6/31", "eth2": "198.51.100.5/31"}, map[string]int{"eth1": 1, "eth2": 1}),
		},
		Links: []model.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "r2-r3", A: "r2", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
			{Name: "r3-r4", A: "r3", AIntf: "eth2", B: "r4", BIntf: "eth2", Cost: 1, Subnet: "198.51.100.4/31"},
			{Name: "r4-r1", A: "r4", AIntf: "eth1", B: "r1", BIntf: "eth2", Cost: 1, Subnet: "198.51.100.6/31"},
		},
	}
	routes := Expected(topo)
	r1ToR2 := routeByNodePrefixProtocol(routes, "r1", "10.255.2.2/32", "ospf")
	if r1ToR2 == nil || len(r1ToR2.Paths) != 1 || r1ToR2.Paths[0].NextHop != "198.51.100.6" {
		t.Fatalf("r1 OSPF route to r2 loopback = %#v, want selected remote route via r4", r1ToR2)
	}
	if local := routeByNodePrefixProtocol(routes, "r1", "10.255.1.1/32", "ospf"); local == nil || len(local.Paths) != 1 || local.Paths[0].NextHop != "" {
		t.Fatalf("local OSPF loopback route = %#v, want directly connected OSPF route", local)
	}
	if connected := routeByNodePrefixProtocol(routes, "r1", "198.51.100.0/31", "ospf"); connected == nil || len(connected.Paths) != 1 || connected.Paths[0].NextHop != "" {
		t.Fatalf("local connected OSPF network = %#v, want directly connected OSPF route", connected)
	}
}

func TestExpectedOSPFSuppressesNonFRRLocalRoutes(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			ospfExpectedNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]int{"eth1": 1}),
			ospfExpectedNode("r2", "10.255.2.2/32", map[string]string{"Ethernet1": "198.51.100.1/31"}, map[string]int{"Ethernet1": 1}),
		},
		Links: []model.Link{{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"}},
	}
	topo.Nodes[1].Kind = model.KindCEOS
	routes := Expected(topo)
	if local := routeByNodePrefixProtocol(routes, "r2", "10.255.2.2/32", "ospf"); local != nil {
		t.Fatalf("non-FRR local OSPF route should not be expected live: %#v", local)
	}
	if remote := routeByNodePrefixProtocol(routes, "r2", "10.255.1.1/32", "ospf"); remote == nil {
		t.Fatalf("remote OSPF route missing from expected routes: %#v", routes)
	}
}

func TestExpectedOSPFInterAreaRouteProtocol(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			ospfExpectedAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"lo": "1", "eth1": "1"}),
			ospfExpectedAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31"}, map[string]string{"lo": "0", "eth1": "1", "eth2": "0"}),
			ospfExpectedAreaNode("r3", "10.255.3.3/32", map[string]string{"eth1": "198.51.100.3/31", "eth2": "198.51.100.4/31"}, map[string]string{"lo": "0", "eth1": "0", "eth2": "2"}),
			ospfExpectedAreaNode("r4", "10.255.4.4/32", map[string]string{"eth1": "198.51.100.5/31"}, map[string]string{"lo": "2", "eth1": "2"}),
		},
		Links: []model.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "r2-r3", A: "r2", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
			{Name: "r3-r4", A: "r3", AIntf: "eth2", B: "r4", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.4/31"},
		},
	}
	routes := Expected(topo)
	route := routeByNodePrefixProtocol(routes, "r1", "10.255.4.4/32", "ospf-ia")
	if route == nil || len(route.Paths) != 1 || route.Paths[0].NextHop != "198.51.100.1" {
		t.Fatalf("r1 OSPF inter-area route to r4 loopback = %#v", route)
	}
}

func ospfExpectedNode(name, loopback string, ifaces map[string]string, costs map[string]int) model.Node {
	interfaces := []model.Interface{{Name: "lo", Address: loopback}}
	ospfIfaces := map[string]model.OSPFInterface{"lo": {Name: "lo", Area: "0", Passive: true}}
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
			Networks:          networks,
			PassiveInterfaces: []string{"lo"},
			Interfaces:        ospfIfaces,
		},
	}
}

func ospfExpectedAreaNode(name, loopback string, ifaces map[string]string, areas map[string]string) model.Node {
	interfaces := []model.Interface{{Name: "lo", Address: loopback}}
	ospfIfaces := map[string]model.OSPFInterface{"lo": {Name: "lo", Area: areas["lo"], Passive: true}}
	networks := []model.OSPFNetwork{{Prefix: model.MustPrefix(loopback), Area: areas["lo"]}}
	for name, addr := range ifaces {
		interfaces = append(interfaces, model.Interface{Name: name, Address: addr})
		ospfIfaces[name] = model.OSPFInterface{Name: name, Area: areas[name], Cost: 1}
		networks = append(networks, model.OSPFNetwork{Prefix: model.MustPrefix(addr), Area: areas[name]})
	}
	return model.Node{
		Name:       name,
		Kind:       model.KindFRR,
		Loopback:   loopback,
		Prefixes:   model.MustPrefixes(loopback),
		Interfaces: interfaces,
		OSPF: model.OSPFProcess{
			Enabled:           true,
			Networks:          networks,
			PassiveInterfaces: []string{"lo"},
			Interfaces:        ospfIfaces,
		},
	}
}
func TestExpectedPathUsesModeledAttributes(t *testing.T) {
	topo := &model.Topology{}
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	node := model.Node{Name: "r1", Kind: "frr"}
	ctx := sim.FailureContext{}
	path := expectedPath(idx, node, sim.RIBEntry{
		Attrs:        domainroute.BGPAttributes{ASPath: []uint32{65001}, LocalPref: 175, MED: 42},
		Condition:    sim.True(),
		SelectedCond: sim.True(),
	}, ctx)
	if !path.Best || path.LocalPref != 175 || path.MED != 42 || path.Origin != "igp" || !reflect.DeepEqual(path.ASPath, []uint32{65001}) {
		t.Fatalf("path = %#v", path)
	}
}

func TestExpectedPathUsesDeviceBehaviorValidity(t *testing.T) {
	idx, err := model.BuildTopologyIndex(&model.Topology{})
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	node := model.Node{Name: "ceos", Kind: model.KindCEOS}
	path := expectedPath(idx, node, sim.RIBEntry{
		Provenance:        domainroute.Provenance{FromNode: "peer"},
		ForwardingNextHop: domainroute.NextHop{Node: "remote"},
		Condition:         sim.True(),
		SelectedCond:      sim.False(),
	}, sim.FailureContext{})
	if path.Valid {
		t.Fatalf("cEOS unresolved next-hop expected path should be invalid: %#v", path)
	}
}

func TestRouteNextHopAddressUsesPeerInterfaceAddress(t *testing.T) {
	idx, err := model.BuildTopologyIndex(&model.Topology{
		Nodes: []model.Node{
			{Name: "local", Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.10/24"}}},
			{Name: "peer", Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.20/24"}}},
		},
		Links: []model.Link{{
			Name:   "local-peer",
			A:      "local",
			B:      "peer",
			AIntf:  "eth1",
			BIntf:  "eth1",
			Cost:   1,
			Subnet: "192.0.2.0/24",
		}},
	})
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	got := routeNextHopAddress(idx, "local", sim.RIBEntry{ForwardingNextHop: domainroute.NextHop{Node: "peer"}})
	if got != "192.0.2.20" {
		t.Fatalf("routeNextHopAddress() = %q, want peer interface address 192.0.2.20", got)
	}
}

func TestRouteNextHopAddressUsesRecursiveHopInterfaceAddress(t *testing.T) {
	idx, err := model.BuildTopologyIndex(&model.Topology{
		Nodes: []model.Node{
			{Name: "origin", Interfaces: []model.Interface{{Name: "eth1", Address: "198.51.100.30/24"}}},
			{Name: "hop", Interfaces: []model.Interface{{Name: "eth1", Address: "198.51.100.40/24"}, {Name: "eth2", Address: "203.0.113.50/24"}}},
			{Name: "local", Interfaces: []model.Interface{{Name: "eth1", Address: "203.0.113.60/24"}}},
		},
		Links: []model.Link{
			{Name: "origin-hop", A: "origin", B: "hop", AIntf: "eth1", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/24"},
			{Name: "hop-local", A: "hop", B: "local", AIntf: "eth2", BIntf: "eth1", Cost: 1, Subnet: "203.0.113.0/24"},
		},
	})
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	got := routeNextHopAddress(idx, "local", sim.RIBEntry{
		ForwardingNextHop: domainroute.NextHop{Node: "hop"},
		Provenance:        domainroute.Provenance{PathNodes: []string{"origin", "hop", "local"}},
	})
	if got != "203.0.113.50" {
		t.Fatalf("routeNextHopAddress() = %q, want recursive hop interface address 203.0.113.50", got)
	}
}

func TestExpectedReflectsRouteMapAttributes(t *testing.T) {
	lp := 225
	med := 33
	topo := &model.Topology{
		Nodes: []model.Node{
			{
				Name:     "origin",
				Kind:     "frr",
				ASN:      65001,
				Prefixes: model.MustPrefixes("10.0.0.0/24"),
				PrefixLists: []model.PrefixList{{
					Name:  "PL-OUT",
					Rules: []model.PrefixListRule{{Action: "permit", Prefix: "10.0.0.0/24"}},
				}},
				RoutePolicies: []model.RoutePolicy{{
					Name:  "SET-MED",
					Rules: []model.RoutePolicyRule{{Action: "permit", MatchPrefixList: "PL-OUT", SetMED: &med}},
				}},
				Neighbors: []model.BGPNeighbor{{
					PeerNode:     "rx",
					RemoteAS:     65002,
					Activated:    true,
					ExportPolicy: "SET-MED",
				}},
			},
			{
				Name: "rx",
				Kind: "frr",
				ASN:  65002,
				PrefixLists: []model.PrefixList{{
					Name:  "PL-IN",
					Rules: []model.PrefixListRule{{Action: "permit", Prefix: "10.0.0.0/24"}},
				}},
				RoutePolicies: []model.RoutePolicy{{
					Name:  "SET-LP",
					Rules: []model.RoutePolicyRule{{Action: "permit", MatchPrefixList: "PL-IN", SetLocalPref: &lp}},
				}},
				Neighbors: []model.BGPNeighbor{{
					PeerNode:     "origin",
					RemoteAS:     65001,
					Activated:    true,
					ImportPolicy: "SET-LP",
				}},
			},
		},
		Links: []model.Link{{Name: "origin-rx", A: "origin", B: "rx", Cost: 1, Subnet: "192.0.2.0/31"}},
	}
	routes := ExpectedForNodes(topo, []model.Node{{Name: "rx", Kind: "frr"}})
	route := routeByPrefix(routes, "10.0.0.0/24")
	if route == nil || len(route.Paths) != 1 {
		t.Fatalf("routes = %#v", routes)
	}
	if route.Paths[0].LocalPref != 225 || route.Paths[0].MED != 33 {
		t.Fatalf("path = %#v, want local-pref 225 MED 33", route.Paths[0])
	}
}

func TestCompareRoutes(t *testing.T) {
	base := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
		path(false, true, "192.0.2.2", []uint32{65002}, 100, 0),
	)}
	tests := []struct {
		name string
		exp  []observationrib.NormalizedRoute
		act  []observationrib.NormalizedRoute
		want func(observationrib.CompareResult) bool
	}{
		{"exact", base, base, func(r observationrib.CompareResult) bool { return r.OK }},
		{"missing prefix", base, nil, func(r observationrib.CompareResult) bool { return len(r.MissingPrefixes) == 1 }},
		{"unexpected prefix", nil, base, func(r observationrib.CompareResult) bool { return len(r.UnexpectedPrefixes) == 1 }},
		{"missing path", base, []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", base[0].Paths[0])}, func(r observationrib.CompareResult) bool { return len(r.MissingPaths) == 1 }},
		{"unexpected path", []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", base[0].Paths[0])}, base, func(r observationrib.CompareResult) bool { return len(r.UnexpectedPaths) == 1 }},
		{"as path mismatch", []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 0))}, []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65009}, 100, 0))}, func(r observationrib.CompareResult) bool {
			return len(r.MissingPaths) == 1 && len(r.UnexpectedPaths) == 1
		}},
		{"local-pref mismatch", []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 200, 0))}, []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 0))}, mismatch("local_pref")},
		{"med mismatch", []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 10))}, []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 20))}, mismatch("med")},
		{"best mismatch", []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 0))}, []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", path(false, true, "192.0.2.1", []uint32{65001}, 100, 0))}, mismatch("best")},
		{"valid mismatch", []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 0))}, []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", path(true, false, "192.0.2.1", []uint32{65001}, 100, 0))}, mismatch("valid")},
		{"path order ignored", base, []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24", base[0].Paths[1], base[0].Paths[0])}, func(r observationrib.CompareResult) bool { return r.OK }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := observationrib.Compare(tt.exp, tt.act); !tt.want(got) {
				t.Fatalf("observationrib.Compare() = %#v", got)
			}
		})
	}
}

func TestDefaultCompareRejectsBestPathMismatch(t *testing.T) {
	expected := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	actual := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		path(false, true, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	result := observationrib.CompareRoutes(expected, actual, observationrib.DefaultCompareOptions())
	if result.OK || len(result.Mismatched) != 1 || result.Mismatched[0].Field != "best" {
		t.Fatalf("observationrib.CompareRoutes() = %#v, want best mismatch", result)
	}
}

func TestDefaultCompareRejectsUnexpectedExtraPath(t *testing.T) {
	expected := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	actual := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
		path(false, true, "192.0.2.2", []uint32{65002}, 100, 0),
	)}
	result := observationrib.CompareRoutes(expected, actual, observationrib.DefaultCompareOptions())
	if result.OK || len(result.UnexpectedPaths) != 1 {
		t.Fatalf("observationrib.CompareRoutes() = %#v, want unexpected path", result)
	}
}

func TestCompareAllowsIdenticalDuplicatePaths(t *testing.T) {
	expected := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	actual := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	if result := observationrib.CompareRoutes(expected, actual, observationrib.DefaultCompareOptions()); !result.OK {
		t.Fatalf("observationrib.CompareRoutes() = %#v, want identical duplicate paths accepted", result)
	}
}

func TestCompareReportsDuplicatePathConflictForAttributeDifference(t *testing.T) {
	expected := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 10),
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 20),
	)}
	actual := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 10),
	)}
	result := observationrib.CompareRoutes(expected, actual, observationrib.DefaultCompareOptions())
	if result.OK || len(result.DuplicatePathConflicts) != 1 {
		t.Fatalf("observationrib.CompareRoutes() = %#v, want duplicate path conflict", result)
	}
	conflict := result.DuplicatePathConflicts[0]
	if conflict.RouteKey != "r1|default|ipv4|10.0.0.0/24" || conflict.PathKey != "nh=192.0.2.1|as=65001" || conflict.Side != "expected" || len(conflict.Paths) != 2 {
		t.Fatalf("conflict = %#v", conflict)
	}
}

func TestCompareDuplicateBestValidDoesNotHideDiff(t *testing.T) {
	expected := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
		path(false, false, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	actual := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		path(false, false, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	result := observationrib.CompareRoutes(expected, actual, observationrib.DefaultCompareOptions())
	if result.OK || len(result.Mismatched) != 2 || result.Mismatched[0].Field != "best" || result.Mismatched[1].Field != "valid" || len(result.DuplicatePathConflicts) != 0 {
		t.Fatalf("observationrib.CompareRoutes() = %#v, want merged best/valid duplicate to expose mismatches", result)
	}
}

func TestComparePeerOptionSeparatesDuplicateIdentity(t *testing.T) {
	opts := observationrib.DefaultCompareOptions()
	opts.ComparePeer = true
	expected := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		pathWithPeer(path(true, true, "192.0.2.1", []uint32{65001}, 100, 10), "192.0.2.10"),
		pathWithPeer(path(true, true, "192.0.2.1", []uint32{65001}, 100, 20), "192.0.2.20"),
	)}
	actual := []observationrib.NormalizedRoute{route("r1", "10.0.0.0/24",
		pathWithPeer(path(true, true, "192.0.2.1", []uint32{65001}, 100, 10), "192.0.2.10"),
		pathWithPeer(path(true, true, "192.0.2.1", []uint32{65001}, 100, 20), "192.0.2.20"),
	)}
	result := observationrib.CompareRoutes(expected, actual, opts)
	if !result.OK {
		t.Fatalf("observationrib.CompareRoutes() = %#v, want peer to distinguish path identity", result)
	}
}

func TestFormatDiffsIncludesDuplicatePathConflict(t *testing.T) {
	result := observationrib.CompareResult{DuplicatePathConflicts: []observationrib.DuplicatePathConflict{{
		RouteKey: "r1|default|ipv4|10.0.0.0/24",
		PathKey:  "nh=192.0.2.1|as=65001",
		Side:     "actual",
		Paths: []observationrib.NormalizedPath{
			path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
			path(false, true, "192.0.2.1", []uint32{65001}, 100, 0),
		},
	}}}
	lines := observationrib.FormatDiffs(result)
	want := "[DIFF] r1|default|ipv4|10.0.0.0/24 path nh=192.0.2.1|as=65001 duplicate path conflict side=actual paths=2"
	if len(lines) != 1 || lines[0] != want {
		t.Fatalf("observationrib.FormatDiffs() = %#v, want %#v", lines, want)
	}
}

func mismatch(field string) func(observationrib.CompareResult) bool {
	return func(r observationrib.CompareResult) bool {
		return len(r.Mismatched) == 1 && r.Mismatched[0].Field == field
	}
}

func route(node, prefix string, paths ...observationrib.NormalizedPath) observationrib.NormalizedRoute {
	return observationrib.NormalizedRoute{Node: node, NetworkInstance: "default", AFI: "ipv4", Prefix: prefix, Paths: paths}
}

func path(best, valid bool, nextHop string, asPath []uint32, localPref, med int) observationrib.NormalizedPath {
	return observationrib.NormalizedPath{Best: best, Valid: valid, NextHop: nextHop, ASPath: asPath, Origin: "igp", LocalPref: localPref, MED: med}
}

func pathWithPeer(p observationrib.NormalizedPath, peer string) observationrib.NormalizedPath {
	p.Peer = peer
	return p
}

func routeByPrefix(routes []observationrib.NormalizedRoute, prefix string) *observationrib.NormalizedRoute {
	for i := range routes {
		if routes[i].Prefix == prefix {
			return &routes[i]
		}
	}
	return nil
}

func routeByPrefixProtocol(routes []observationrib.NormalizedRoute, prefix, protocol string) *observationrib.NormalizedRoute {
	for i := range routes {
		if routes[i].Prefix == prefix && observationrib.NormalizeRoute(routes[i]).Protocol == protocol {
			return &routes[i]
		}
	}
	return nil
}

func routeByNodePrefixProtocol(routes []observationrib.NormalizedRoute, node, prefix, protocol string) *observationrib.NormalizedRoute {
	for i := range routes {
		if routes[i].Node == node && routes[i].Prefix == prefix && observationrib.NormalizeRoute(routes[i]).Protocol == protocol {
			return &routes[i]
		}
	}
	return nil
}
