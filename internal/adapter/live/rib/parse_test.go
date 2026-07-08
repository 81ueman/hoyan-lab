package rib

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/live/device"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type runnerFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

func (f runnerFunc) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f(ctx, name, args...)
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
	if len(routes) != 1 || routes[0].OSPF == nil || routes[0].OSPF.RouteType != observation.OSPFRouteTypeInterArea || firstNextHop(routes[0]) != "198.51.100.1" {
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
	if len(routes) != 1 || routes[0].OSPF == nil || routes[0].OSPF.RouteType != observation.OSPFRouteTypeInterArea {
		t.Fatalf("routes = %#v, want OSPF inter-area protocol from OSPF table", routes)
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
	if static == nil || pathCount(static) != 1 || firstNextHop(static) != "192.0.2.2" {
		t.Fatalf("static route = %#v", static)
	}
	blackhole := routeByPrefixProtocol(routes, "198.51.100.0/24", "blackhole")
	if blackhole == nil || pathCount(blackhole) != 1 || firstNextHop(blackhole) != "" {
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
	if len(routes) != 1 || string(routes[0].Common.Protocol) != "ospf" || firstNextHop(routes[0]) != "198.51.100.6" {
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
	if len(routes) != 1 || string(routes[0].Common.Protocol) != "ospf" || firstNextHop(routes[0]) != "" {
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
	if static == nil || pathCount(static) != 1 || firstNextHop(static) != "192.0.2.2" {
		t.Fatalf("static route = %#v", static)
	}
	blackhole := routeByPrefixProtocol(routes, "198.51.100.0/24", "blackhole")
	if blackhole == nil || pathCount(blackhole) != 1 || firstNextHop(blackhole) != "" {
		t.Fatalf("blackhole route = %#v", blackhole)
	}
	if routeByPrefixProtocol(routes, "10.0.0.0/24", "bgp") != nil {
		t.Fatalf("BGP route table entry should be excluded: %#v", routes)
	}
	ospf := routeByPrefixProtocol(routes, "10.255.2.2/32", "ospf")
	if ospf == nil || pathCount(ospf) != 1 || firstNextHop(ospf) != "198.51.100.2" {
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
	if static == nil || pathCount(static) != 1 || firstNextHop(static) != "192.0.2.2" {
		t.Fatalf("static route = %#v", static)
	}
	blackhole := routeByPrefixProtocol(routes, "198.51.100.0/24", "blackhole")
	if blackhole == nil || pathCount(blackhole) != 1 || firstNextHop(blackhole) != "" {
		t.Fatalf("blackhole route = %#v", blackhole)
	}
	if routeByPrefixProtocol(routes, "10.0.0.0/24", "bgp") != nil {
		t.Fatalf("BGP route table entry should be excluded: %#v", routes)
	}
	ospf := routeByPrefixProtocol(routes, "10.255.2.2/32", "ospf")
	if ospf == nil || pathCount(ospf) != 1 || firstNextHop(ospf) != "198.51.100.2" {
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
		for _, path := range route.BGP.Paths {
			if route.Common.Prefix == "10.4.1.10/32" && path.NextHop.Address == "198.18.10.1" && path.Best && len(path.ASPath) == 2 {
				foundRemote = true
			}
			if route.Common.Prefix == "10.1.0.0/16" && path.NextHop.Address == "" && path.Best {
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
	if len(routes) != 1 || routes[0].Common.Prefix != "10.255.0.1/32" {
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
	if remote == nil || pathCount(remote) != 2 {
		t.Fatalf("remote route = %#v", remote)
	}
	best := pathByNextHop(remote.BGP.Paths, "192.0.2.1")
	if best == nil {
		t.Fatalf("paths = %#v, want next-hop 192.0.2.1", remote.BGP.Paths)
	}
	if !best.Best || !best.Eligible || best.LocalPref != 150 || best.MED != 10 || !reflect.DeepEqual(best.ASPath, []uint32{65001, 65002}) || best.Peer != "192.0.2.1" || best.PeerAS != 65001 {
		t.Fatalf("best path = %#v", best)
	}
	if !reflect.DeepEqual(best.Communities, []string{"65000:1", "no-export"}) || !reflect.DeepEqual(best.LargeCommunities, []string{"65000:100:1"}) {
		t.Fatalf("best path communities = %#v large=%#v", best.Communities, best.LargeCommunities)
	}
	backup := pathByNextHop(remote.BGP.Paths, "192.0.2.2")
	if backup == nil || backup.Best || backup.LocalPref != 120 || backup.MED != 20 || !reflect.DeepEqual(backup.ASPath, []uint32{65003, 65004}) || backup.PeerAS != 65003 {
		t.Fatalf("backup path = %#v", backup)
	}
	local := routeByPrefix(routes, "10.0.1.0/24")
	if local == nil || pathCount(local) != 1 || firstNextHop(local) != "" {
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
	if len(routes) != 1 || pathCount(routes[0]) != 2 {
		t.Fatalf("routes = %#v", routes)
	}
	best := pathByNextHop(routes[0].BGP.Paths, "192.0.2.1")
	if best == nil || !best.Best || !best.Eligible || best.LocalPref != 150 || best.MED != 0 || !reflect.DeepEqual(best.ASPath, []uint32{65001, 65002}) || best.Peer != "192.0.2.1" || best.PeerAS != 65001 {
		t.Fatalf("best path = %#v", best)
	}
	if !reflect.DeepEqual(best.Communities, []string{"65000:1", "no-export"}) {
		t.Fatalf("best communities = %#v", best.Communities)
	}
	backup := pathByNextHop(routes[0].BGP.Paths, "192.0.2.2")
	if backup == nil || backup.Best || !backup.Eligible || backup.LocalPref != 120 || backup.MED != 30 || !reflect.DeepEqual(backup.ASPath, []uint32{65003, 65004}) || backup.Peer != "192.0.2.2" || backup.PeerAS != 65003 {
		t.Fatalf("backup path = %#v", backup)
	}
	if pathByNextHop(routes[0].BGP.Paths, "203.0.113.1") != nil || pathByNextHop(routes[0].BGP.Paths, "203.0.113.2") != nil {
		t.Fatalf("advertised/non-route sections were parsed: %#v", routes[0].BGP.Paths)
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
	rib, err := NewCollector(runner).CollectRIB(context.Background(), model.Node{Name: "r1", Kind: model.KindFRR, ContainerName: "r1"}, model.NetworkInstanceDefault, observation.CollectOptions{IncludeInactive: true})
	if err != nil {
		t.Fatalf("CollectRIB() error = %v", err)
	}
	routes := rib.Routes
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
	rib, err := NewCollector(runner).CollectRIB(context.Background(), model.Node{Name: "ceos", Kind: model.KindCEOS, ContainerName: "ceos1"}, model.NetworkInstanceDefault, observation.CollectOptions{IncludeInactive: true})
	if err != nil {
		t.Fatalf("CollectRIB() error = %v", err)
	}
	routes := rib.Routes
	ospf := routeByPrefixProtocol(routes, "10.255.2.2/32", "ospf")
	if ospf == nil || pathCount(ospf) != 1 || firstNextHop(ospf) != "198.51.100.2" {
		t.Fatalf("OSPF route = %#v in %#v", ospf, routes)
	}
}

func routeByPrefix(routes []RIBRoute, prefix string) *RIBRoute {
	for i := range routes {
		if routes[i].Common.Prefix == prefix {
			return &routes[i]
		}
	}
	return nil
}

func routeByPrefixProtocol(routes []RIBRoute, prefix, protocol string) *RIBRoute {
	for i := range routes {
		if routes[i].Common.Prefix == prefix && string(routes[i].Common.Protocol) == protocol {
			return &routes[i]
		}
	}
	return nil
}

func routeByVRFPrefixProtocol(routes []RIBRoute, vrf, prefix, protocol string) *RIBRoute {
	normalizedVRF := string(model.NormalizeNetworkInstance(vrf))
	for i := range routes {
		routeVRF := string(model.NormalizeNetworkInstance(string(routes[i].Common.VRF)))
		if routeVRF == normalizedVRF && routes[i].Common.Prefix == prefix && string(routes[i].Common.Protocol) == protocol {
			return &routes[i]
		}
	}
	return nil
}

func pathByNextHop(paths []observation.BGPPath, nextHop string) *observation.BGPPath {
	for i := range paths {
		if paths[i].NextHop.Address == nextHop {
			return &paths[i]
		}
	}
	return nil
}

func pathCount(route any) int {
	switch r := route.(type) {
	case RIBRoute:
		return pathCount(&r)
	case *RIBRoute:
		switch {
		case r == nil:
			return 0
		case r.BGP != nil:
			return len(r.BGP.Paths)
		case r.OSPF != nil:
			return len(r.OSPF.Paths)
		case r.Static != nil:
			return len(r.Static.NextHops)
		case r.Connected != nil, r.Blackhole != nil:
			return 1
		default:
			return 0
		}
	default:
		return 0
	}
}

func firstNextHop(route any) string {
	switch r := route.(type) {
	case RIBRoute:
		return firstNextHop(&r)
	case *RIBRoute:
		switch {
		case r == nil:
			return ""
		case r.BGP != nil && len(r.BGP.Paths) > 0:
			return r.BGP.Paths[0].NextHop.Address
		case r.OSPF != nil && len(r.OSPF.Paths) > 0:
			return r.OSPF.Paths[0].NextHop.Address
		case r.Static != nil && len(r.Static.NextHops) > 0:
			return r.Static.NextHops[0].Address
		default:
			return ""
		}
	default:
		return ""
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
	session := device.NewDockerSession(runner, "clab-test-core-gz")
	data, err := RunSRLinuxJSON(context.Background(), session, "show", "version")
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
	session := device.NewDockerSession(runner, "clab-test-core-gz")
	_, err := RunSRLinuxJSON(context.Background(), session, "show", "version")
	if err == nil {
		t.Fatalf("RunSRLinuxJSON() succeeded unexpectedly")
	}
	for _, want := range []string{"malformed JSON", "bytes=10", `preview="{\"header\":"`} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
}

// --- Cross-VRF leakage tests ---

func TestCollectRIB_FRR_FiltersByVRF_NoCrossVRFLeakage(t *testing.T) {
	// FRR "show ip route vrf all json" returns routes from multiple VRFs.
	// CollectRIB must only return routes for the requested VRF.
	runner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch cmd {
		case "docker exec -i r1 vtysh -c show ip bgp json":
			return []byte(`{}`), nil
		case "docker exec -i r1 ip -j link show type vrf":
			return []byte(`[]`), nil
		case "docker exec -i r1 vtysh -c show ip route vrf all json":
			return []byte(`{
			  "vrfs": {
			    "default": {
			      "routes": {
			        "10.0.0.0/24": [{"protocol":"connected","interfaceName":"eth0"}]
			      }
			    },
			    "tenant-a": {
			      "routes": {
			        "10.1.0.0/24": [{"protocol":"static","nexthops":[{"ip":"192.0.2.1"}]}]
			      }
			    },
			    "tenant-b": {
			      "routes": {
			        "10.2.0.0/24": [{"protocol":"static","nexthops":[{"ip":"192.0.2.2"}]}]
			      }
			    }
			  }
			}`), nil
		case "docker exec -i r1 vtysh -c show ip ospf route json":
			return []byte(`{}`), nil
		default:
			t.Fatalf("unexpected command: %s", cmd)
			return nil, nil
		}
	})
	collector := NewCollector(runner)

	// Collect for "default" VRF - should only get 10.0.0.0/24
	rib, err := collector.CollectRIB(context.Background(), model.Node{Name: "r1", Kind: model.KindFRR, ContainerName: "r1"}, model.NetworkInstanceDefault, observation.CollectOptions{IncludeInactive: true})
	if err != nil {
		t.Fatalf("CollectRIB(default) error = %v", err)
	}
	if routeByPrefixProtocol(rib.Routes, "10.0.0.0/24", "connected") == nil {
		t.Fatalf("default VRF: connected route 10.0.0.0/24 missing")
	}
	if routeByPrefixProtocol(rib.Routes, "10.1.0.0/24", "static") != nil {
		t.Fatalf("default VRF: tenant-a route 10.1.0.0/24 leaked!")
	}
	if routeByPrefixProtocol(rib.Routes, "10.2.0.0/24", "static") != nil {
		t.Fatalf("default VRF: tenant-b route 10.2.0.0/24 leaked!")
	}

	// Collect for "tenant-a" VRF - should only get 10.1.0.0/24
	rib, err = collector.CollectRIB(context.Background(), model.Node{Name: "r1", Kind: model.KindFRR, ContainerName: "r1"}, model.NetworkInstanceID("tenant-a"), observation.CollectOptions{IncludeInactive: true})
	if err != nil {
		t.Fatalf("CollectRIB(tenant-a) error = %v", err)
	}
	if len(rib.Routes) != 1 || rib.Routes[0].Common.Prefix != "10.1.0.0/24" {
		t.Fatalf("tenant-a VRF: expected 1 route (10.1.0.0/24), got %d: %#v", len(rib.Routes), rib.Routes)
	}

	// Collect for "tenant-b" VRF - should only get 10.2.0.0/24
	rib, err = collector.CollectRIB(context.Background(), model.Node{Name: "r1", Kind: model.KindFRR, ContainerName: "r1"}, model.NetworkInstanceID("tenant-b"), observation.CollectOptions{IncludeInactive: true})
	if err != nil {
		t.Fatalf("CollectRIB(tenant-b) error = %v", err)
	}
	if len(rib.Routes) != 1 || rib.Routes[0].Common.Prefix != "10.2.0.0/24" {
		t.Fatalf("tenant-b VRF: expected 1 route (10.2.0.0/24), got %d: %#v", len(rib.Routes), rib.Routes)
	}
}

func TestCollectRIB_CEOS_FiltersByVRF_NoCrossVRFLeakage(t *testing.T) {
	// cEOS "show ip bgp vrf all | json" returns routes from multiple VRFs.
	runner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch cmd {
		case "docker exec -i ceos1 Cli -p 15 -c show ip bgp vrf all | json":
			return []byte(`{
			  "vrfs": {
			    "default": {
			      "bgpRouteEntries": {
			        "10.0.0.0/24": {
			          "bgpRoutePaths": [{
			            "routeType": {"active": true, "valid": true},
			            "nextHop": "192.0.2.1",
			            "peerEntry": {"peerAddr": "192.0.2.1", "peerAS": 65001},
			            "routeOrigin": "igp"
			          }]
			        }
			      }
			    },
			    "tenant-a": {
			      "bgpRouteEntries": {
			        "10.1.0.0/24": {
			          "bgpRoutePaths": [{
			            "routeType": {"active": true, "valid": true},
			            "nextHop": "192.0.2.2",
			            "peerEntry": {"peerAddr": "192.0.2.2", "peerAS": 65002},
			            "routeOrigin": "igp"
			          }]
			        }
			      }
			    }
			  }
			}`), nil
		case "docker exec -i ceos1 Cli -p 15 -c show ip route vrf all | json":
			return []byte(`{"vrfs":{"default":{"routes":{}}}}`), nil
		default:
			t.Fatalf("unexpected command: %s", cmd)
			return nil, nil
		}
	})
	collector := NewCollector(runner)
	ceosNode := model.Node{Name: "ceos1", Kind: model.KindCEOS, ContainerName: "ceos1", ASN: 65000}

	// Collect for "default" VRF - should only get 10.0.0.0/24
	rib, err := collector.CollectRIB(context.Background(), ceosNode, model.NetworkInstanceDefault, observation.CollectOptions{IncludeInactive: true})
	if err != nil {
		t.Fatalf("CollectRIB(default) error = %v", err)
	}
	bgp := routeByPrefixProtocol(rib.Routes, "10.0.0.0/24", "bgp")
	if bgp == nil {
		t.Fatalf("default VRF: BGP route 10.0.0.0/24 missing in %#v", rib.Routes)
	}
	if routeByPrefixProtocol(rib.Routes, "10.1.0.0/24", "bgp") != nil {
		t.Fatalf("default VRF: tenant-a route 10.1.0.0/24 leaked!")
	}

	// Collect for "tenant-a" VRF - should only get 10.1.0.0/24
	rib, err = collector.CollectRIB(context.Background(), ceosNode, model.NetworkInstanceID("tenant-a"), observation.CollectOptions{IncludeInactive: true})
	if err != nil {
		t.Fatalf("CollectRIB(tenant-a) error = %v", err)
	}
	if len(rib.Routes) != 1 || rib.Routes[0].Common.Prefix != "10.1.0.0/24" {
		t.Fatalf("tenant-a VRF: expected 1 route (10.1.0.0/24), got %d: %#v", len(rib.Routes), rib.Routes)
	}
}

func TestParseFRRRouteTableMultipleVRFs(t *testing.T) {
	// Test that ParseFRRRouteTable returns routes with correct VRF set when
	// the JSON has a "vrfs" key (from "show ip route vrf all json").
	data := []byte(`{
	  "vrfs": {
	    "default": {
	      "routes": {
	        "10.0.0.0/24": [{"protocol":"connected","interfaceName":"eth0"}]
	      }
	    },
	    "tenant-a": {
	      "routes": {
	        "10.1.0.0/24": [{"protocol":"static","nexthops":[{"ip":"192.0.2.1"}]}]
	      }
	    }
	  }
	}`)
	routes, err := ParseFRRRouteTable("r1", data)
	if err != nil {
		t.Fatalf("ParseFRRRouteTable() error = %v", err)
	}
	// Check default VRF route
	defRoute := routeByVRFPrefixProtocol(routes, "default", "10.0.0.0/24", "connected")
	if defRoute == nil {
		t.Fatalf("default route missing: %#v", routes)
	}
	// Check tenant-a route
	tenantRoute := routeByVRFPrefixProtocol(routes, "tenant-a", "10.1.0.0/24", "static")
	if tenantRoute == nil {
		t.Fatalf("tenant-a route missing: %#v", routes)
	}
	// Ensure no route leaks between VRFs
	if routeByVRFPrefixProtocol(routes, "default", "10.1.0.0/24", "static") != nil {
		t.Fatalf("tenant-a route leaked into default VRF")
	}
	if routeByVRFPrefixProtocol(routes, "tenant-a", "10.0.0.0/24", "connected") != nil {
		t.Fatalf("default route leaked into tenant-a VRF")
	}
}

func TestParseCEOSRouteTableMultipleVRFs_VRFAware(t *testing.T) {
	data := []byte(`{"vrfs":{
	  "default": {"routes":{"10.0.0.0/24":{"routeType":"connected","vias":[{"interface":"eth0"}]}}},
	  "tenant-a": {"routes":{"10.1.0.0/24":{"routeType":"static","vias":[{"nexthopAddr":"192.0.2.1","interface":"eth1"}]}}},
	  "tenant-b": {"routes":{"10.2.0.0/24":{"routeType":"static","vias":[{"nexthopAddr":"192.0.2.2","interface":"eth2"}]}}}
	}}`)
	routes, err := ParseCEOSRouteTable("ceos1", data)
	if err != nil {
		t.Fatalf("ParseCEOSRouteTable() error = %v", err)
	}
	// Each route should have its correct VRF
	if r := routeByVRFPrefixProtocol(routes, "default", "10.0.0.0/24", "connected"); r == nil {
		t.Fatalf("default route missing or wrong VRF")
	}
	if r := routeByVRFPrefixProtocol(routes, "tenant-a", "10.1.0.0/24", "static"); r == nil {
		t.Fatalf("tenant-a route missing or wrong VRF")
	}
	if r := routeByVRFPrefixProtocol(routes, "tenant-b", "10.2.0.0/24", "static"); r == nil {
		t.Fatalf("tenant-b route missing or wrong VRF")
	}
	// Cross-VRF negative checks
	if routeByVRFPrefixProtocol(routes, "default", "10.1.0.0/24", "static") != nil {
		t.Fatalf("tenant-a route leaked into default VRF")
	}
	if routeByVRFPrefixProtocol(routes, "tenant-a", "10.2.0.0/24", "static") != nil {
		t.Fatalf("tenant-b route leaked into tenant-a VRF")
	}
}

func TestParseCEOSMultipleVREBGP(t *testing.T) {
	// Test that ParseCEOS correctly sets VRF on BGP routes from multi-VRF output.
	data := []byte(`{
	  "vrfs": {
	    "default": {
	      "bgpRouteEntries": {
	        "10.0.0.0/24": {
	          "bgpRoutePaths": [{
	            "routeType": {"active": true, "valid": true},
	            "nextHop": "192.0.2.1",
	            "peerEntry": {"peerAddr": "192.0.2.1", "peerAS": 65001},
	            "routeOrigin": "igp"
	          }]
	        }
	      }
	    },
	    "tenant-a": {
	      "bgpRouteEntries": {
	        "10.1.0.0/24": {
	          "bgpRoutePaths": [{
	            "routeType": {"active": true, "valid": true},
	            "nextHop": "192.0.2.2",
	            "peerEntry": {"peerAddr": "192.0.2.2", "peerAS": 65002},
	            "routeOrigin": "igp"
	          }]
	        }
	      }
	    }
	  }
	}`)
	routes, err := ParseCEOS("ceos1", data)
	if err != nil {
		t.Fatalf("ParseCEOS() error = %v", err)
	}
	// Check VRF attribution
	if r := routeByVRFPrefixProtocol(routes, "default", "10.0.0.0/24", "bgp"); r == nil {
		t.Fatalf("default route missing or wrong VRF")
	}
	if r := routeByVRFPrefixProtocol(routes, "tenant-a", "10.1.0.0/24", "bgp"); r == nil {
		t.Fatalf("tenant-a route missing or wrong VRF")
	}
	// Cross-VRF negative check
	if routeByVRFPrefixProtocol(routes, "default", "10.1.0.0/24", "bgp") != nil {
		t.Fatalf("tenant-a BGP route leaked into default VRF")
	}
}

func TestParseSRLinuxRouteTableVRFSet(t *testing.T) {
	data := []byte(`{"instance":[{"ip route":[{"Prefix":"10.1.0.0/24","Route Type":"static","Active":"True","Next-hop (Type)":"192.0.2.1/32 (direct)","Next-hop Interface":"ethernet-1/1.0"}]}]}`)
	routes, err := ParseSRLinuxRouteTableNetworkInstance("srl1", "tenant-a", data)
	if err != nil {
		t.Fatalf("ParseSRLinuxRouteTableNetworkInstance() error = %v", err)
	}
	// Verify VRF is set correctly
	if r := routeByVRFPrefixProtocol(routes, "tenant-a", "10.1.0.0/24", "static"); r == nil {
		t.Fatalf("route not found with VRF tenant-a: %#v", routes)
	}
}

func TestParseSRLinuxBGPDetailVRFSet(t *testing.T) {
	detail := []byte(`{
	  "routes": [
	    {"status":"<Best,Valid,Used>","next-hop":"192.0.2.1","neighbor":"192.0.2.1","peer-as":"65001","local pref":"150","as path":"65001","origin":"igp"}
	  ]
	}`)
	routes, err := ParseSRLinuxDetailNetworkInstance("srl1", "tenant-a", "10.1.0.0/24", detail)
	if err != nil {
		t.Fatalf("ParseSRLinuxDetailNetworkInstance() error = %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	// Verify VRF is set correctly
	if string(routes[0].Common.VRF) != "tenant-a" {
		t.Fatalf("expected VRF tenant-a, got %q", routes[0].Common.VRF)
	}
}

func TestCollectRIB_SRLinux_FiltersByVRF(t *testing.T) {
	// The SR Linux collection uses sr_cli --output-format json --pagination off -- ...
	runner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch cmd {
		case "docker exec -i srl1 sr_cli --output-format json --pagination off -- show network-instance default protocols bgp routes ipv4 summary":
			return []byte(`{"network-instance":[{"routes":[{"prefix":"10.0.0.0/24"}]}]}`), nil
		case "docker exec -i srl1 sr_cli --output-format json --pagination off -- show network-instance default protocols bgp routes ipv4 prefix 10.0.0.0/24 detail":
			return []byte(`{"routes":[{"status":"<Best,Valid,Used>","next-hop":"192.0.2.1","neighbor":"192.0.2.1","peer-as":"65001","as path":"65001","origin":"igp"}]}`), nil
		case "docker exec -i srl1 sr_cli --output-format json --pagination off -- show network-instance default route-table ipv4-unicast summary":
			return []byte(`{"instance":[{"ip route":[{"Prefix":"10.0.0.0/24","Route Type":"connected","Active":"True","Next-hop Interface":"eth0"}]}]}`), nil
		default:
			t.Fatalf("unexpected command: %s", cmd)
			return nil, nil
		}
	})
	collector := NewCollector(runner)
	rib, err := collector.CollectRIB(context.Background(), model.Node{Name: "srl1", Kind: model.KindSRLinux, ContainerName: "srl1"}, model.NetworkInstanceDefault, observation.CollectOptions{IncludeInactive: true})
	if err != nil {
		t.Fatalf("CollectRIB(default) error = %v", err)
	}
	if len(rib.Routes) == 0 {
		t.Fatalf("expected routes for default VRF, got none")
	}
	// All routes should have VRF set to "default" (or empty which we treat as default)
	for i, r := range rib.Routes {
		vrf := string(r.Common.VRF)
		if vrf != "" && vrf != "default" {
			t.Fatalf("route %d has VRF %q, expected default", i, vrf)
		}
	}
}

// Test that the FRR BGP VRF command also sets VRF correctly.
func TestCollectRIB_FRR_BGP_PerVRFCommandSetsVRF(t *testing.T) {
	runner := runnerFunc(func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch cmd {
		case "docker exec -i r1 vtysh -c show ip bgp json":
			return []byte(`{"routes":{"10.0.0.0/24":[{"valid":true,"bestpath":true,"nexthops":[{"ip":"192.0.2.1"}],"path":"65001","origin":"i"}]}}`), nil
		case "docker exec -i r1 ip -j link show type vrf":
			return []byte(`[{"ifindex":10,"link":"eth1","ifname":"tenant-a"}]`), nil
		case "docker exec -i r1 vtysh -c show bgp vrf tenant-a ipv4 unicast json":
			return []byte(`{"routes":{"10.1.0.0/24":[{"valid":true,"bestpath":true,"nexthops":[{"ip":"192.0.2.2"}],"path":"65002","origin":"i"}]}}`), nil
		case "docker exec -i r1 vtysh -c show ip route vrf all json":
			return []byte(`{}`), nil
		case "docker exec -i r1 vtysh -c show ip ospf route json":
			return []byte(`{}`), nil
		default:
			t.Fatalf("unexpected command: %s", cmd)
			return nil, nil
		}
	})
	collector := NewCollector(runner)

	// Collect default VRF
	rib, err := collector.CollectRIB(context.Background(), model.Node{
		Name: "r1", Kind: model.KindFRR, ContainerName: "r1",
		Interfaces: []model.Interface{{
			Name: "eth1", VRF: model.NetworkInstanceID("tenant-a"),
		}},
	}, model.NetworkInstanceDefault, observation.CollectOptions{IncludeInactive: true})
	if err != nil {
		t.Fatalf("CollectRIB(default) error = %v", err)
	}
	// Should only have 10.0.0.0/24 (default VRF BGP route)
	if routeByPrefixProtocol(rib.Routes, "10.0.0.0/24", "bgp") == nil {
		t.Fatalf("default VRF: BGP route 10.0.0.0/24 missing")
	}
	if routeByPrefixProtocol(rib.Routes, "10.1.0.0/24", "bgp") != nil {
		t.Fatalf("default VRF: tenant-a BGP route leaked!")
	}

	// Collect tenant-a VRF
	rib, err = collector.CollectRIB(context.Background(), model.Node{
		Name: "r1", Kind: model.KindFRR, ContainerName: "r1",
		Interfaces: []model.Interface{{
			Name: "eth1", VRF: model.NetworkInstanceID("tenant-a"),
		}},
	}, model.NetworkInstanceID("tenant-a"), observation.CollectOptions{IncludeInactive: true})
	if err != nil {
		t.Fatalf("CollectRIB(tenant-a) error = %v", err)
	}
	// Should only have 10.1.0.0/24 (tenant-a VRF BGP route)
	if routeByPrefixProtocol(rib.Routes, "10.1.0.0/24", "bgp") == nil {
		t.Fatalf("tenant-a VRF: BGP route 10.1.0.0/24 missing")
	}
	if routeByPrefixProtocol(rib.Routes, "10.0.0.0/24", "bgp") != nil {
		t.Fatalf("tenant-a VRF: default route leaked!")
	}
}
