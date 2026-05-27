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

func TestOSPFInstallsInterAreaRoutesThroughABR(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			ospfAreaNode("r1", "10.255.1.1/32", map[string]string{"eth1": "198.51.100.0/31"}, map[string]string{"lo": "1", "eth1": "1"}, map[string]int{"eth1": 1}),
			ospfAreaNode("r2", "10.255.2.2/32", map[string]string{"eth1": "198.51.100.1/31", "eth2": "198.51.100.2/31"}, map[string]string{"lo": "0", "eth1": "1", "eth2": "0"}, map[string]int{"eth1": 1, "eth2": 1}),
			ospfAreaNode("r3", "10.255.3.3/32", map[string]string{"eth1": "198.51.100.3/31", "eth2": "198.51.100.4/31"}, map[string]string{"lo": "0", "eth1": "0", "eth2": "2"}, map[string]int{"eth1": 1, "eth2": 1}),
			ospfAreaNode("r4", "10.255.4.4/32", map[string]string{"eth1": "198.51.100.5/31"}, map[string]string{"lo": "2", "eth1": "2"}, map[string]int{"eth1": 1}),
		},
		Links: []model.Link{
			{Name: "r1-r2", A: "r1", AIntf: "eth1", B: "r2", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.0/31"},
			{Name: "r2-r3", A: "r2", AIntf: "eth2", B: "r3", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.2/31"},
			{Name: "r3-r4", A: "r3", AIntf: "eth2", B: "r4", BIntf: "eth1", Cost: 1, Subnet: "198.51.100.4/31"},
		},
	}
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	rib := map[string]map[string][]RIBEntry{}
	NewEngine(idx, rib).Simulate()
	routes := rib["r1"]["10.255.4.4/32"]
	if len(routes) == 0 {
		t.Fatalf("r1 did not learn r4 loopback")
	}
	best := routes[0].Normalize()
	if best.SourceKind != model.RouteSourceOSPF || best.RouteSource.OSPFRouteType != "inter-area" || best.RouteSource.Metric != 3 || best.NextHop != "r2" {
		t.Fatalf("best route = %#v, want inter-area OSPF metric 3 via r2", best)
	}
	if got := rib["r1"]["198.51.100.2/31"]; len(got) == 0 || got[0].Normalize().RouteSource.OSPFRouteType != "inter-area" {
		t.Fatalf("r1 backbone link route = %#v, want inter-area route", got)
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

func ospfAreaNode(name, loopback string, ifaces map[string]string, areas map[string]string, costs map[string]int) model.Node {
	interfaces := []model.Interface{{Name: "lo", Address: loopback}}
	ospfIfaces := map[string]model.OSPFInterface{
		"lo": {Name: "lo", Area: areas["lo"], Passive: true},
	}
	networks := []model.OSPFNetwork{{Prefix: model.MustPrefix(loopback), Area: areas["lo"]}}
	for name, addr := range ifaces {
		interfaces = append(interfaces, model.Interface{Name: name, Address: addr})
		ospfIfaces[name] = model.OSPFInterface{Name: name, Area: areas[name], Cost: costs[name]}
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
			RouterID:          loopback[:len(loopback)-3],
			Networks:          networks,
			PassiveInterfaces: []string{"lo"},
			Interfaces:        ospfIfaces,
		},
	}
}
