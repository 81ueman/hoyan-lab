package controlplane

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/model"
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
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	rib := map[string]map[string][]RIBEntry{}
	NewEngine(idx, rib).Simulate()
	routes := rib["r1"]["10.255.2.2/32"]
	if len(routes) < 2 {
		t.Fatalf("r1 routes to r2 loopback = %#v, want primary and fallback", routes)
	}
	best := routes[0].Normalize()
	if best.SourceKind != model.RouteSourceOSPF || best.RouteSource.Metric != 3 || best.NextHop != "r4" {
		t.Fatalf("best route = %#v, want OSPF metric 3 via r4", best)
	}
	var fallbackFound bool
	for _, route := range routes {
		route = route.Normalize()
		if route.NextHop == "r2" && route.RouteSource.Metric == 10 {
			fallbackFound = true
		}
	}
	if !fallbackFound {
		t.Fatalf("routes = %#v, want fallback via r2 metric 10", routes)
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

func simulateOSPFTestRIB(t *testing.T, topo *model.Topology) map[string]map[string][]RIBEntry {
	t.Helper()
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	rib := map[string]map[string][]RIBEntry{}
	NewEngine(idx, rib).Simulate()
	return rib
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
	loopbackArea := "0"
	for _, area := range areasByIface {
		loopbackArea = area
		break
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
