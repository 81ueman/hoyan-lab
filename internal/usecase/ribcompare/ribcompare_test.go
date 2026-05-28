package ribcompare

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

type runnerFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f(ctx, name, args...)
}

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

func TestParseFRRRouteTableOSPFInterArea(t *testing.T) {
	data := []byte(`{
  "10.255.4.4/32": [
    {
      "prefix": "10.255.4.4/32",
      "protocol": "ospf",
      "routeType": "O IA",
      "selected": true,
      "nexthops": [{"ip": "198.51.100.1", "interfaceName": "eth1"}]
    }
  ]
}`)
	routes, err := ParseFRRRouteTable("r1", data)
	if err != nil {
		t.Fatalf("ParseFRRRouteTable() error = %v", err)
	}
	if len(routes) != 1 || routes[0].Protocol != "ospf-ia" || routes[0].Paths[0].NextHop != "198.51.100.1" {
		t.Fatalf("routes = %#v, want OSPF inter-area route with next-hop", routes)
	}
}

func TestParseFRRRouteTableUsesOSPFRouteTypes(t *testing.T) {
	routeData := []byte(`{
  "10.255.4.4/32": [
    {
      "prefix": "10.255.4.4/32",
      "protocol": "ospf",
      "selected": true,
      "nexthops": [{"ip": "198.51.100.1", "interfaceName": "eth1"}]
    }
  ]
}`)
	ospfData := []byte(`{
  "10.255.4.4/32": {
    "routeType": "N IA",
    "cost": 3,
    "area": "0.0.0.1",
    "nexthops": [{"ip": "198.51.100.1", "via": "eth1"}]
  }
}`)
	routes, err := ParseFRRRouteTableWithOSPF("r1", routeData, ospfData)
	if err != nil {
		t.Fatalf("ParseFRRRouteTableWithOSPF() error = %v", err)
	}
	if len(routes) != 1 || routes[0].Protocol != "ospf-ia" {
		t.Fatalf("routes = %#v, want OSPF inter-area protocol from OSPF table", routes)
	}
}

func TestCollectIncludesInstalledStaticAndConnectedRoutes(t *testing.T) {
	runner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch cmd {
		case "docker exec -i r1 vtysh -c show ip bgp json":
			return []byte(`{}`), nil
		case "docker exec -i r1 ip -j link show type vrf":
			return []byte(`[]`), nil
		case "docker exec -i r1 vtysh -c show ip route vrf all json":
			return []byte(`{
			  "192.0.2.0/30":[{"protocol":"connected","interfaceName":"eth1"}],
			  "203.0.113.0/24":[{"protocol":"static","nexthops":[{"ip":"192.0.2.2","interfaceName":"eth1"}]}]
			}`), nil
		case "docker exec -i r1 vtysh -c show ip ospf route json":
			return []byte(`{}`), nil
		default:
			t.Fatalf("unexpected command: %s", cmd)
			return nil, nil
		}
	})
	routes, err := Collect(context.Background(), runner, []model.Node{{Name: "r1", Kind: model.KindFRR, ContainerName: "r1"}})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	if routeByPrefixProtocol(routes, "192.0.2.0/30", "connected") == nil {
		t.Fatalf("connected route missing from collected routes: %#v", routes)
	}
	if routeByPrefixProtocol(routes, "203.0.113.0/24", "static") == nil {
		t.Fatalf("static route missing from collected routes: %#v", routes)
	}
}

func TestCollectSkipsBGPCommandsForNodesWithoutASN(t *testing.T) {
	runner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		if strings.Contains(cmd, "show ip bgp") || strings.Contains(cmd, "protocols bgp") {
			t.Fatalf("unexpected BGP collection command for OSPF-only node: %s", cmd)
		}
		switch cmd {
		case "docker exec -i ceos1 Cli -p 15 -c show ip route vrf all | json":
			return []byte(`{"vrfs":{"default":{"routes":{"10.255.2.2/32":{"routeType":"ospfInternal","vias":[{"nexthopAddr":"198.51.100.2","interface":"Ethernet2"}]}}}}}`), nil
		default:
			t.Fatalf("unexpected command: %s", cmd)
			return nil, nil
		}
	})
	routes, err := Collect(context.Background(), runner, []model.Node{{Name: "ceos", Kind: model.KindCEOS, ContainerName: "ceos1"}})
	if err != nil {
		t.Fatalf("Collect() error = %v", err)
	}
	ospf := routeByPrefixProtocol(routes, "10.255.2.2/32", "ospf")
	if ospf == nil || len(ospf.Paths) != 1 || ospf.Paths[0].NextHop != "198.51.100.2" {
		t.Fatalf("OSPF route = %#v in %#v", ospf, routes)
	}
}

func TestParseFRRRouteTableStaticAndConnected(t *testing.T) {
	data := []byte(`{
	  "192.0.2.0/30":[{"protocol":"connected","interfaceName":"eth1"}],
	  "203.0.113.0/24":[{"protocol":"static","nexthops":[{"ip":"192.0.2.2","interfaceName":"eth1"}]}],
	  "198.51.100.0/24":[{"protocol":"static","nexthops":[{"blackhole":true,"unreachable":true}]}],
	  "10.0.0.0/24":[{"protocol":"bgp","nexthops":[{"ip":"198.51.100.1"}]}]
	}`)
	routes, err := ParseFRRRouteTable("r1", data)
	if err != nil {
		t.Fatalf("ParseFRRRouteTable() error = %v", err)
	}
	if routeByPrefixProtocol(routes, "192.0.2.0/30", "connected") == nil {
		t.Fatalf("connected route missing: %#v", routes)
	}
	static := routeByPrefixProtocol(routes, "203.0.113.0/24", "static")
	if static == nil || len(static.Paths) != 1 || static.Paths[0].NextHop != "192.0.2.2" {
		t.Fatalf("static route = %#v", static)
	}
	blackhole := routeByPrefixProtocol(routes, "198.51.100.0/24", "blackhole")
	if blackhole == nil || len(blackhole.Paths) != 1 || blackhole.Paths[0].NextHop != "" {
		t.Fatalf("blackhole route = %#v", blackhole)
	}
	if routeByPrefixProtocol(routes, "10.0.0.0/24", "bgp") != nil {
		t.Fatalf("BGP route table entry should be excluded: %#v", routes)
	}
}

func TestParseFRRRouteTableOSPF(t *testing.T) {
	data := []byte(`{
  "10.255.2.2/32": [
    {
      "prefix": "10.255.2.2/32",
      "protocol": "ospf",
      "selected": true,
      "nexthops": [{"ip": "198.51.100.6", "interfaceName": "eth2"}]
    }
  ]
}`)
	routes, err := ParseFRRRouteTable("r1", data)
	if err != nil {
		t.Fatalf("ParseFRRRouteTable() error = %v", err)
	}
	if len(routes) != 1 || routes[0].Protocol != "ospf" || routes[0].Paths[0].NextHop != "198.51.100.6" {
		t.Fatalf("routes = %#v, want OSPF route with next-hop", routes)
	}
}

func TestParseFRRRouteTableNormalizesLocalOSPFNextHop(t *testing.T) {
	data := []byte(`{
  "10.10.0.1/32": [
    {
      "prefix": "10.10.0.1/32",
      "protocol": "ospf",
      "selected": true,
      "nexthops": [{"ip": "0.0.0.0", "interfaceName": "a-svc"}]
    }
  ]
}`)
	routes, err := ParseFRRRouteTable("r2", data)
	if err != nil {
		t.Fatalf("ParseFRRRouteTable() error = %v", err)
	}
	if len(routes) != 1 || routes[0].Protocol != "ospf" || routes[0].Paths[0].NextHop != "" {
		t.Fatalf("routes = %#v, want local OSPF route with empty next-hop", routes)
	}
}

func TestParseCEOSRouteTableStaticAndConnected(t *testing.T) {
	data := []byte(`{
	  "vrfs":{"default":{"routes":{
	    "192.0.2.0/30":{"routeType":"connected","vias":[{"interface":"Ethernet1"}]},
	    "203.0.113.0/24":{"routeType":"static","vias":[{"nexthopAddr":"192.0.2.2","interface":"Ethernet1"}]},
	    "198.51.100.0/24":{"routeType":"static","vias":[{"interface":"Null0"}]},
	    "10.255.2.2/32":{"routeType":"ospfInternal","vias":[{"nexthopAddr":"198.51.100.2","interface":"Ethernet2"}]},
	    "10.0.0.0/24":{"routeType":"eBGP","vias":[{"nexthopAddr":"198.51.100.1"}]}
	  }}}
	}`)
	routes, err := ParseCEOSRouteTable("ceos1", data)
	if err != nil {
		t.Fatalf("ParseCEOSRouteTable() error = %v", err)
	}
	if routeByPrefixProtocol(routes, "192.0.2.0/30", "connected") == nil {
		t.Fatalf("connected route missing: %#v", routes)
	}
	static := routeByPrefixProtocol(routes, "203.0.113.0/24", "static")
	if static == nil || len(static.Paths) != 1 || static.Paths[0].NextHop != "192.0.2.2" {
		t.Fatalf("static route = %#v", static)
	}
	blackhole := routeByPrefixProtocol(routes, "198.51.100.0/24", "blackhole")
	if blackhole == nil || len(blackhole.Paths) != 1 || blackhole.Paths[0].NextHop != "" {
		t.Fatalf("blackhole route = %#v", blackhole)
	}
	if routeByPrefixProtocol(routes, "10.0.0.0/24", "bgp") != nil {
		t.Fatalf("BGP route table entry should be excluded: %#v", routes)
	}
	ospf := routeByPrefixProtocol(routes, "10.255.2.2/32", "ospf")
	if ospf == nil || len(ospf.Paths) != 1 || ospf.Paths[0].NextHop != "198.51.100.2" {
		t.Fatalf("OSPF route = %#v", ospf)
	}
}

func TestParseCEOSRouteTableMultipleVRFs(t *testing.T) {
	data := []byte(`{"vrfs":{
	  "tenant-a":{"routes":{"10.255.0.1/32":{"routeType":"static","vias":[{"nexthopAddr":"192.0.2.2","interface":"Ethernet1"}]}}},
	  "tenant-b":{"routes":{"10.255.0.1/32":{"routeType":"static","vias":[{"nexthopAddr":"192.0.2.2","interface":"Ethernet2"}]}}}
	}}`)
	routes, err := ParseCEOSRouteTable("ceos1", data)
	if err != nil {
		t.Fatalf("ParseCEOSRouteTable() error = %v", err)
	}
	for _, vrf := range []string{"tenant-a", "tenant-b"} {
		route := routeByVRFPrefixProtocol(routes, vrf, "10.255.0.1/32", "static")
		if route == nil {
			t.Fatalf("%s static route missing: %#v", vrf, routes)
		}
	}
}

func TestParseSRLinuxRouteTableNetworkInstance(t *testing.T) {
	data := []byte(`{"instance":[{"ip route":[{"Prefix":"10.255.0.1/32","Route Type":"static","Active":"True","Next-hop (Type)":"192.0.2.2/32 (direct)","Next-hop Interface":"ethernet-1/1.0"}]}]}`)
	routes, err := ParseSRLinuxRouteTableNetworkInstance("srl1", "tenant-a", data)
	if err != nil {
		t.Fatalf("ParseSRLinuxRouteTableNetworkInstance() error = %v", err)
	}
	if routeByVRFPrefixProtocol(routes, "tenant-a", "10.255.0.1/32", "static") == nil {
		t.Fatalf("tenant-a static route missing: %#v", routes)
	}
}

func TestParseSRLinuxRouteTableStaticAndConnected(t *testing.T) {
	data := []byte(`noise
	{"instance":[{"ip route":[
	  {"Prefix":"192.0.2.0/30","Route Type":"local","Active":"True","Next-hop Interface":"ethernet-1/1.0"},
	  {"Prefix":"203.0.113.0/24","Route Type":"static","Active":"True","Next-hop (Type)":"192.0.2.2/32 (direct)","Next-hop Interface":"ethernet-1/1.0"},
	  {"Prefix":"198.51.100.0/24","Route Type":"blackhole","Active":"True","Next-hop (Type)":"None"},
	  {"Prefix":"10.255.2.2/32","Route Type":"ospf-internal","Active":"True","Next-hop (Type)":"198.51.100.2/32 (direct)","Next-hop Interface":"ethernet-1/2.0"},
	  {"Prefix":"10.0.0.0/24","Route Type":"bgp","Active":"True","Next-hop (Type)":"198.51.100.1/32 (indirect)"}
	]}]}`)
	routes, err := ParseSRLinuxRouteTable("srl1", data)
	if err != nil {
		t.Fatalf("ParseSRLinuxRouteTable() error = %v", err)
	}
	if routeByPrefixProtocol(routes, "192.0.2.0/30", "connected") == nil {
		t.Fatalf("connected route missing: %#v", routes)
	}
	static := routeByPrefixProtocol(routes, "203.0.113.0/24", "static")
	if static == nil || len(static.Paths) != 1 || static.Paths[0].NextHop != "192.0.2.2" {
		t.Fatalf("static route = %#v", static)
	}
	blackhole := routeByPrefixProtocol(routes, "198.51.100.0/24", "blackhole")
	if blackhole == nil || len(blackhole.Paths) != 1 || blackhole.Paths[0].NextHop != "" {
		t.Fatalf("blackhole route = %#v", blackhole)
	}
	if routeByPrefixProtocol(routes, "10.0.0.0/24", "bgp") != nil {
		t.Fatalf("BGP route table entry should be excluded: %#v", routes)
	}
	ospf := routeByPrefixProtocol(routes, "10.255.2.2/32", "ospf")
	if ospf == nil || len(ospf.Paths) != 1 || ospf.Paths[0].NextHop != "198.51.100.2" {
		t.Fatalf("OSPF route = %#v", ospf)
	}
}

func TestParseFRR(t *testing.T) {
	data := []byte(`{
	  "totalPrefixCounter": 1,
	  "routes": {
	    "10.4.1.10/32": [
	      {"valid": true, "bestpath": false, "nexthops": [{"ip": "198.18.20.7"}], "path": "65100 4200001004", "origin":"i", "locPrf": 100, "metric": 0, "peerId": "198.18.20.7"},
	      {"valid": true, "bestpath": true, "nexthops": [{"ip": "198.18.10.1"}], "path": "65100 4200001004", "origin":"i", "locPrf": 100}
	    ],
	    "10.1.0.0/16": [
	      {"valid": true, "bestpath": true, "nexthops": [{"ip": "0.0.0.0"}], "path": "", "origin":"i"}
	    ]
	  }
	}`)
	routes, err := ParseFRR("bj-edge1", data)
	if err != nil {
		t.Fatalf("ParseFRR() error = %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("routes = %#v", routes)
	}
	var foundRemote, foundLocal bool
	for _, route := range routes {
		for _, path := range route.Paths {
			if route.Prefix == "10.4.1.10/32" && path.NextHop == "198.18.10.1" && path.Best && len(path.ASPath) == 2 {
				foundRemote = true
			}
			if route.Prefix == "10.1.0.0/16" && path.NextHop == "" && path.Best {
				foundLocal = true
			}
		}
	}
	if !foundRemote || !foundLocal {
		t.Fatalf("routes = %#v", routes)
	}
}

func TestParseFRRVRF(t *testing.T) {
	data := []byte(`{
	  "routes": {
	    "10.255.0.1/32": [
	      {"valid": true, "bestpath": true, "nexthops": [{"ip": "192.0.2.2"}], "path": "65002", "origin":"i"}
	    ]
	  }
	}`)
	routes, err := ParseFRRVRF("r1", "tenant-a", data)
	if err != nil {
		t.Fatalf("ParseFRRVRF() error = %v", err)
	}
	if len(routes) != 1 || routes[0].NetworkInstance != "tenant-a" || routes[0].Prefix != "10.255.0.1/32" {
		t.Fatalf("routes = %#v, want tenant-a route", routes)
	}
}

func TestParseCEOS(t *testing.T) {
	data := []byte(`{
	  "vrfs": {
	    "default": {
	      "bgpRouteEntries": {
	        "10.0.0.0/24": {
	          "bgpRoutePaths": [
	            {
	              "routeType": {"active": true, "valid": true},
	              "localPreference": 150,
	              "med": 10,
	              "weight": 0,
	              "nextHop": "192.0.2.1",
	              "peerEntry": {"peerAddr": "192.0.2.1", "peerAS": 65001},
	              "asPathEntry": {"asPath": "65001 65002", "largeCommunityList": ["65000:100:1"]},
	              "communityList": ["65000:1", "no-export"],
	              "routeOrigin": "igp"
	            },
	            {
	              "routeType": {"active": false, "valid": true},
	              "localPreference": 120,
	              "med": 20,
	              "nextHop": "192.0.2.2",
	              "peerEntry": {"peerAddr": "192.0.2.2", "peerAS": 65003},
	              "asPathEntry": {"asPath": "65003 65004"},
	              "routeOrigin": "egp"
	            }
	          ]
	        },
	        "10.0.1.0/24": {
	          "bgpRoutePaths": [{
	            "routeType": {"active": true, "valid": true},
	            "nextHop": "0.0.0.0",
	            "asPathEntry": {"asPath": ""},
	            "routeOrigin": "igp"
	          }]
	        }
	      }
	    }
	  }
	}`)
	routes, err := ParseCEOS("core-sh", data)
	if err != nil {
		t.Fatalf("ParseCEOS() error = %v", err)
	}
	if len(routes) != 2 {
		t.Fatalf("routes = %#v", routes)
	}
	remote := routeByPrefix(routes, "10.0.0.0/24")
	if remote == nil || len(remote.Paths) != 2 {
		t.Fatalf("remote route = %#v", remote)
	}
	best := pathByNextHop(remote.Paths, "192.0.2.1")
	if best == nil {
		t.Fatalf("paths = %#v, want next-hop 192.0.2.1", remote.Paths)
	}
	if !best.Best || !best.Valid || best.LocalPref != 150 || best.MED != 10 || !reflect.DeepEqual(best.ASPath, []uint32{65001, 65002}) || best.Peer != "192.0.2.1" || best.PeerAS != 65001 {
		t.Fatalf("best path = %#v", best)
	}
	if !reflect.DeepEqual(best.Communities, []string{"65000:1", "no-export"}) || !reflect.DeepEqual(best.LargeCommunities, []string{"65000:100:1"}) {
		t.Fatalf("best path communities = %#v large=%#v", best.Communities, best.LargeCommunities)
	}
	backup := pathByNextHop(remote.Paths, "192.0.2.2")
	if backup == nil || backup.Best || backup.LocalPref != 120 || backup.MED != 20 || !reflect.DeepEqual(backup.ASPath, []uint32{65003, 65004}) || backup.PeerAS != 65003 {
		t.Fatalf("backup path = %#v", backup)
	}
	local := routeByPrefix(routes, "10.0.1.0/24")
	if local == nil || len(local.Paths) != 1 || local.Paths[0].NextHop != "" {
		t.Fatalf("local route = %#v", local)
	}
}

func TestParseSRLinux(t *testing.T) {
	summary := []byte(`{"network-instance":[{"routes":[{"prefix":"10.0.1.0/24"},{"prefix":"10.0.0.0/24"}]}]}`)
	prefixes, err := ParseSRLinuxSummary(summary)
	if err != nil {
		t.Fatalf("ParseSRLinuxSummary() error = %v", err)
	}
	if got, want := prefixes, []string{"10.0.0.0/24", "10.0.1.0/24"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("prefixes = %#v", prefixes)
	}
	detail := []byte(`{
	  "routes": [
	    {"status":"<Best,Valid,Used>","next-hop":"192.0.2.1","neighbor":"192.0.2.1","peer-as":"65001","local pref":"150","med":"-","communities":["65000:1","no-export"],"as path":"65001 65002","origin":"igp"},
	    {"status":"<Valid>","next-hop":"192.0.2.2","peer":"192.0.2.2","peerAS":65003,"local-pref":120,"med":30,"community":"65000:2","as-path":"65003 65004","origin":"incomplete"}
	  ],
	  "advertised": {"routes":[{"status":"<Best,Valid>","next-hop":"203.0.113.1","as-path":"64512"}]},
	  "non-route": {"routes":[{"status":"<Best,Valid>","next-hop":"203.0.113.2","as-path":"64513"}]}
	}`)
	routes, err := ParseSRLinuxDetail("core-gz", "10.0.0.0/24", detail)
	if err != nil {
		t.Fatalf("ParseSRLinuxDetail() error = %v", err)
	}
	if len(routes) != 1 || len(routes[0].Paths) != 2 {
		t.Fatalf("routes = %#v", routes)
	}
	best := pathByNextHop(routes[0].Paths, "192.0.2.1")
	if best == nil || !best.Best || !best.Valid || best.LocalPref != 150 || best.MED != 0 || !reflect.DeepEqual(best.ASPath, []uint32{65001, 65002}) || best.Peer != "192.0.2.1" || best.PeerAS != 65001 {
		t.Fatalf("best path = %#v", best)
	}
	if !reflect.DeepEqual(best.Communities, []string{"65000:1", "no-export"}) {
		t.Fatalf("best communities = %#v", best.Communities)
	}
	backup := pathByNextHop(routes[0].Paths, "192.0.2.2")
	if backup == nil || backup.Best || !backup.Valid || backup.LocalPref != 120 || backup.MED != 30 || !reflect.DeepEqual(backup.ASPath, []uint32{65003, 65004}) || backup.Peer != "192.0.2.2" || backup.PeerAS != 65003 {
		t.Fatalf("backup path = %#v", backup)
	}
	if pathByNextHop(routes[0].Paths, "203.0.113.1") != nil || pathByNextHop(routes[0].Paths, "203.0.113.2") != nil {
		t.Fatalf("advertised/non-route sections were parsed: %#v", routes[0].Paths)
	}
}

func TestRunSRLinuxJSONRetriesEmptyOutput(t *testing.T) {
	calls := 0
	runner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		calls++
		if name != "docker" || strings.Join(args[:4], " ") != "exec -i clab-test-core-gz sr_cli" {
			t.Fatalf("unexpected command: %s %v", name, args)
		}
		if calls == 1 {
			return nil, nil
		}
		return []byte(`{"header":[]}`), nil
	})
	data, err := RunSRLinuxJSON(context.Background(), runner, "clab-test-core-gz", "show", "version")
	if err != nil {
		t.Fatalf("RunSRLinuxJSON() error = %v", err)
	}
	if string(data) != `{"header":[]}` {
		t.Fatalf("data = %q", data)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want retry after empty output", calls)
	}
}

func TestRunSRLinuxJSONReportsMalformedOutput(t *testing.T) {
	runner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return []byte(`{"header":`), nil
	})
	_, err := RunSRLinuxJSON(context.Background(), runner, "clab-test-core-gz", "show", "version")
	if err == nil {
		t.Fatalf("RunSRLinuxJSON() succeeded unexpectedly")
	}
	for _, want := range []string{"malformed JSON", "bytes=10", `preview="{\"header\":"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
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

func TestCompareBgpRib(t *testing.T) {
	base := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
		path(false, true, "192.0.2.2", []uint32{65002}, 100, 0),
	)}
	tests := []struct {
		name string
		exp  []NormalizedBgpRoute
		act  []NormalizedBgpRoute
		want func(BgpRibCompareResult) bool
	}{
		{"exact", base, base, func(r BgpRibCompareResult) bool { return r.OK }},
		{"missing prefix", base, nil, func(r BgpRibCompareResult) bool { return len(r.MissingPrefixes) == 1 }},
		{"unexpected prefix", nil, base, func(r BgpRibCompareResult) bool { return len(r.UnexpectedPrefixes) == 1 }},
		{"missing path", base, []NormalizedBgpRoute{route("r1", "10.0.0.0/24", base[0].Paths[0])}, func(r BgpRibCompareResult) bool { return len(r.MissingPaths) == 1 }},
		{"unexpected path", []NormalizedBgpRoute{route("r1", "10.0.0.0/24", base[0].Paths[0])}, base, func(r BgpRibCompareResult) bool { return len(r.UnexpectedPaths) == 1 }},
		{"as path mismatch", []NormalizedBgpRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 0))}, []NormalizedBgpRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65009}, 100, 0))}, func(r BgpRibCompareResult) bool { return len(r.MissingPaths) == 1 && len(r.UnexpectedPaths) == 1 }},
		{"local-pref mismatch", []NormalizedBgpRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 200, 0))}, []NormalizedBgpRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 0))}, mismatch("local_pref")},
		{"med mismatch", []NormalizedBgpRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 10))}, []NormalizedBgpRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 20))}, mismatch("med")},
		{"best mismatch", []NormalizedBgpRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 0))}, []NormalizedBgpRoute{route("r1", "10.0.0.0/24", path(false, true, "192.0.2.1", []uint32{65001}, 100, 0))}, mismatch("best")},
		{"valid mismatch", []NormalizedBgpRoute{route("r1", "10.0.0.0/24", path(true, true, "192.0.2.1", []uint32{65001}, 100, 0))}, []NormalizedBgpRoute{route("r1", "10.0.0.0/24", path(true, false, "192.0.2.1", []uint32{65001}, 100, 0))}, mismatch("valid")},
		{"path order ignored", base, []NormalizedBgpRoute{route("r1", "10.0.0.0/24", base[0].Paths[1], base[0].Paths[0])}, func(r BgpRibCompareResult) bool { return r.OK }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Compare(tt.exp, tt.act); !tt.want(got) {
				t.Fatalf("Compare() = %#v", got)
			}
		})
	}
}

func TestDefaultCompareRejectsBestPathMismatch(t *testing.T) {
	expected := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	actual := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		path(false, true, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	result := CompareBgpRib(expected, actual, DefaultBgpRibCompareOptions())
	if result.OK || len(result.Mismatched) != 1 || result.Mismatched[0].Field != "best" {
		t.Fatalf("CompareBgpRib() = %#v, want best mismatch", result)
	}
}

func TestDefaultCompareRejectsUnexpectedExtraPath(t *testing.T) {
	expected := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	actual := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
		path(false, true, "192.0.2.2", []uint32{65002}, 100, 0),
	)}
	result := CompareBgpRib(expected, actual, DefaultBgpRibCompareOptions())
	if result.OK || len(result.UnexpectedPaths) != 1 {
		t.Fatalf("CompareBgpRib() = %#v, want unexpected path", result)
	}
}

func TestCompareAllowsIdenticalDuplicatePaths(t *testing.T) {
	expected := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	actual := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	if result := CompareBgpRib(expected, actual, DefaultBgpRibCompareOptions()); !result.OK {
		t.Fatalf("CompareBgpRib() = %#v, want identical duplicate paths accepted", result)
	}
}

func TestCompareReportsDuplicatePathConflictForAttributeDifference(t *testing.T) {
	expected := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 10),
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 20),
	)}
	actual := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 10),
	)}
	result := CompareBgpRib(expected, actual, DefaultBgpRibCompareOptions())
	if result.OK || len(result.DuplicatePathConflicts) != 1 {
		t.Fatalf("CompareBgpRib() = %#v, want duplicate path conflict", result)
	}
	conflict := result.DuplicatePathConflicts[0]
	if conflict.RouteKey != "r1|default|ipv4|10.0.0.0/24" || conflict.PathKey != "nh=192.0.2.1|as=65001" || conflict.Side != "expected" || len(conflict.Paths) != 2 {
		t.Fatalf("conflict = %#v", conflict)
	}
}

func TestCompareDuplicateBestValidDoesNotHideDiff(t *testing.T) {
	expected := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
		path(false, false, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	actual := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		path(false, false, "192.0.2.1", []uint32{65001}, 100, 0),
	)}
	result := CompareBgpRib(expected, actual, DefaultBgpRibCompareOptions())
	if result.OK || len(result.Mismatched) != 2 || result.Mismatched[0].Field != "best" || result.Mismatched[1].Field != "valid" || len(result.DuplicatePathConflicts) != 0 {
		t.Fatalf("CompareBgpRib() = %#v, want merged best/valid duplicate to expose mismatches", result)
	}
}

func TestComparePeerOptionSeparatesDuplicateIdentity(t *testing.T) {
	opts := DefaultBgpRibCompareOptions()
	opts.ComparePeer = true
	expected := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		pathWithPeer(path(true, true, "192.0.2.1", []uint32{65001}, 100, 10), "192.0.2.10"),
		pathWithPeer(path(true, true, "192.0.2.1", []uint32{65001}, 100, 20), "192.0.2.20"),
	)}
	actual := []NormalizedBgpRoute{route("r1", "10.0.0.0/24",
		pathWithPeer(path(true, true, "192.0.2.1", []uint32{65001}, 100, 10), "192.0.2.10"),
		pathWithPeer(path(true, true, "192.0.2.1", []uint32{65001}, 100, 20), "192.0.2.20"),
	)}
	result := CompareBgpRib(expected, actual, opts)
	if !result.OK {
		t.Fatalf("CompareBgpRib() = %#v, want peer to distinguish path identity", result)
	}
}

func TestFormatDiffsIncludesDuplicatePathConflict(t *testing.T) {
	result := BgpRibCompareResult{DuplicatePathConflicts: []DuplicatePathConflict{{
		RouteKey: "r1|default|ipv4|10.0.0.0/24",
		PathKey:  "nh=192.0.2.1|as=65001",
		Side:     "actual",
		Paths: []NormalizedBgpPath{
			path(true, true, "192.0.2.1", []uint32{65001}, 100, 0),
			path(false, true, "192.0.2.1", []uint32{65001}, 100, 0),
		},
	}}}
	lines := FormatDiffs(result)
	want := "[DIFF] r1|default|ipv4|10.0.0.0/24 path nh=192.0.2.1|as=65001 duplicate path conflict side=actual paths=2"
	if len(lines) != 1 || lines[0] != want {
		t.Fatalf("FormatDiffs() = %#v, want %#v", lines, want)
	}
}

func mismatch(field string) func(BgpRibCompareResult) bool {
	return func(r BgpRibCompareResult) bool {
		return len(r.Mismatched) == 1 && r.Mismatched[0].Field == field
	}
}

func route(node, prefix string, paths ...NormalizedBgpPath) NormalizedBgpRoute {
	return NormalizedBgpRoute{Node: node, NetworkInstance: "default", AFI: "ipv4", Prefix: prefix, Paths: paths}
}

func path(best, valid bool, nextHop string, asPath []uint32, localPref, med int) NormalizedBgpPath {
	return NormalizedBgpPath{Best: best, Valid: valid, NextHop: nextHop, ASPath: asPath, Origin: "igp", LocalPref: localPref, MED: med}
}

func pathWithPeer(p NormalizedBgpPath, peer string) NormalizedBgpPath {
	p.Peer = peer
	return p
}

func routeByPrefix(routes []NormalizedBgpRoute, prefix string) *NormalizedBgpRoute {
	for i := range routes {
		if routes[i].Prefix == prefix {
			return &routes[i]
		}
	}
	return nil
}

func routeByPrefixProtocol(routes []NormalizedBgpRoute, prefix, protocol string) *NormalizedBgpRoute {
	for i := range routes {
		if routes[i].Prefix == prefix && normalizeRoute(routes[i]).Protocol == protocol {
			return &routes[i]
		}
	}
	return nil
}

func routeByVRFPrefixProtocol(routes []NormalizedBgpRoute, vrf, prefix, protocol string) *NormalizedBgpRoute {
	for i := range routes {
		if routes[i].NetworkInstance == vrf && routes[i].Prefix == prefix && normalizeRoute(routes[i]).Protocol == protocol {
			return &routes[i]
		}
	}
	return nil
}

func routeByNodePrefixProtocol(routes []NormalizedBgpRoute, node, prefix, protocol string) *NormalizedBgpRoute {
	for i := range routes {
		if routes[i].Node == node && routes[i].Prefix == prefix && normalizeRoute(routes[i]).Protocol == protocol {
			return &routes[i]
		}
	}
	return nil
}

func pathByNextHop(paths []NormalizedBgpPath, nextHop string) *NormalizedBgpPath {
	for i := range paths {
		if paths[i].NextHop == nextHop {
			return &paths[i]
		}
	}
	return nil
}
