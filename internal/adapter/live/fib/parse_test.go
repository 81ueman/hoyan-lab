package fib

import (
	"reflect"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestParseLinuxIPRoute(t *testing.T) {
	data := []byte(`[
	  {"dst":"10.0.0.0/24","gateway":"192.0.2.1","dev":"eth1","protocol":"bgp","metric":20},
	  {"dst":"10.0.0.10","gateway":"192.0.2.9","dev":"eth9","protocol":"bgp"},
	  {"dst":"10.0.1.0/24","protocol":"bgp","metric":30,"nexthops":[{"gateway":"192.0.2.1","dev":"eth1","weight":1},{"gateway":"192.0.2.2","dev":"eth2","weight":1}]},
	  {"dst":"default","gateway":"198.51.100.1","dev":"eth0","protocol":"static","metric":100},
	  {"type":"blackhole","dst":"203.0.113.0/24","protocol":"static","metric":10},
	  {"dst":"2001:db8::/64","dev":"eth3","protocol":"kernel"}
	]`)
	routes, err := ParseLinuxIPRoute("r1", data)
	if err != nil {
		t.Fatalf("ParseLinuxIPRoute() error = %v", err)
	}
	// Now includes IPv6 routes (was 5 before AFI-awareness change)
	if len(routes) != 6 {
		t.Fatalf("routes = %#v", routes)
	}
	if host := routeByPrefix(routes, "10.0.0.10/32"); host == nil {
		t.Fatalf("routes = %#v, want host route normalized to /32", routes)
	}
	ecmp := routeByPrefix(routes, "10.0.1.0/24")
	if ecmp == nil || len(ecmp.NextHops) != 2 {
		t.Fatalf("ecmp route = %#v", ecmp)
	}
	if !reflect.DeepEqual(ecmp.NextHops[0], NextHop{Address: "192.0.2.1", Interface: "eth1", Weight: 1}) {
		t.Fatalf("first next-hop = %#v, want %#v", ecmp.NextHops[0], NextHop{Address: "192.0.2.1", Interface: "eth1", Weight: 1})
	}
	if def := routeByPrefix(routes, "0.0.0.0/0"); def == nil || def.Source.Protocol != "static" || def.Metric != 100 {
		t.Fatalf("default route = %#v", def)
	}
	if blackhole := routeByPrefix(routes, "203.0.113.0/24"); blackhole == nil || blackhole.Source.Protocol != "blackhole" || len(blackhole.NextHops) != 0 {
		t.Fatalf("blackhole route = %#v", blackhole)
	}
	// Verify IPv6 route inclusion
	if v6 := routeByPrefix(routes, "2001:db8::/64"); v6 == nil {
		t.Fatalf("IPv6 route missing from parsed routes")
	} else if v6.AFI != model.AFIIPv6 {
		t.Fatalf("IPv6 route AFI = %q, want %q", v6.AFI, model.AFIIPv6)
	} else if v6.Source.Protocol != "connected" {
		t.Fatalf("IPv6 route protocol = %q, want connected", v6.Source.Protocol)
	}
}

func TestParseLinuxIPRouteCanonicalizesConnectedProtocol(t *testing.T) {
	routes, err := ParseLinuxIPRoute("r1", []byte(`[{"dst":"192.0.2.0/31","dev":"eth1","protocol":"kernel"}]`))
	if err != nil {
		t.Fatalf("ParseLinuxIPRoute() error = %v", err)
	}
	route := routeByPrefix(routes, "192.0.2.0/31")
	if route == nil || route.Source.Protocol != "connected" {
		t.Fatalf("route = %#v", route)
	}
}

func TestParseCEOSRoutes(t *testing.T) {
	data := []byte(`{
	  "vrfs": {"default": {"routes": {
	    "10.0.0.0/24": {
	      "kernelProgrammed": true,
	      "hardwareProgrammed": true,
	      "routeType": "eBGP",
	      "preference": 200,
	      "metric": 10,
	      "vias": [{"nexthopAddr":"192.0.2.1","interface":"Ethernet1"}]
	    },
	    "198.51.100.0/31": {
	      "kernelProgrammed": true,
	      "routeType": "connected",
	      "vias": [{"interface":"Ethernet2"}]
	    },
	    "10.255.2.2/32": {
	      "kernelProgrammed": true,
	      "routeType": "ospfInternal",
	      "preference": 110,
	      "metric": 20,
	      "vias": [{"nexthopAddr":"198.51.100.2","interface":"Ethernet3"}]
	    },
	    "203.0.113.0/24": {
	      "kernelProgrammed": true,
	      "routeType": "static",
	      "vias": [{"interface":"Null0"}]
	    }
	  }}}
	}`)
	routes, err := ParseCEOSRoutes("ceos1", data)
	if err != nil {
		t.Fatalf("ParseCEOSRoutes() error = %v", err)
	}
	route := routeByPrefix(routes, "10.0.0.0/24")
	if route == nil {
		t.Fatalf("routes = %#v", routes)
	}
	if route.Source.Protocol != "bgp" || route.Preference != 200 || route.Metric != 10 {
		t.Fatalf("route attrs = %#v", route)
	}
	if got, want := route.NextHops, []NextHop{{Address: "192.0.2.1", Interface: "Ethernet1"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("next-hops = %#v, want %#v", got, want)
	}
	connected := routeByPrefix(routes, "198.51.100.0/31")
	if connected == nil || connected.Source.Protocol != "connected" {
		t.Fatalf("connected route = %#v", connected)
	}
	ospf := routeByPrefix(routes, "10.255.2.2/32")
	if ospf == nil || ospf.Source.Protocol != "ospf" || ospf.Preference != 110 || ospf.Metric != 20 {
		t.Fatalf("OSPF route = %#v", ospf)
	}
	blackhole := routeByPrefix(routes, "203.0.113.0/24")
	if blackhole == nil || blackhole.Source.Protocol != "blackhole" || len(blackhole.NextHops) != 0 {
		t.Fatalf("blackhole route = %#v", blackhole)
	}
}

func TestParseCEOSRoutesMultipleVRFs(t *testing.T) {
	data := []byte(`{"vrfs":{
	  "tenant-a":{"routes":{"10.255.0.1/32":{"kernelProgrammed":true,"routeType":"static","vias":[{"nexthopAddr":"192.0.2.2","interface":"Ethernet1"}]}}},
	  "tenant-b":{"routes":{"10.255.0.1/32":{"kernelProgrammed":true,"routeType":"static","vias":[{"nexthopAddr":"192.0.2.2","interface":"Ethernet2"}]}}}
	}}`)
	fibs, err := ParseCEOSFIBs("ceos1", data)
	if err != nil {
		t.Fatalf("ParseCEOSFIBs() error = %v", err)
	}
	for _, vrf := range []string{"tenant-a", "tenant-b"} {
		route := routeByVRFPrefix(fibs, vrf, "10.255.0.1/32")
		if route == nil || route.Source.Protocol != "static" {
			t.Fatalf("%s route = %#v, fibs=%#v", vrf, route, fibs)
		}
	}
}

func TestParseSRLinuxRoutesNetworkInstance(t *testing.T) {
	data := []byte(`{"instance":[{"ip route":[{"Prefix":"10.255.0.1/32","Route Type":"static","Active":"True","Next-hop (Type)":"192.0.2.2/32 (direct)","Next-hop Interface":"ethernet-1/1.0"}]}]}`)
	routes, err := ParseSRLinuxRoutesNetworkInstance("srl1", "tenant-a", data)
	if err != nil {
		t.Fatalf("ParseSRLinuxRoutesNetworkInstance() error = %v", err)
	}
	route := routeByPrefix(routes, "10.255.0.1/32")
	if route == nil || route.Source.Protocol != "static" {
		t.Fatalf("route = %#v, routes=%#v", route, routes)
	}
}

func TestParseSRLinuxRoutes(t *testing.T) {
	data := []byte("\x00noise\r\n" + `{
	  "instance": [{
	    "Name": "default",
	    "ip route": [
	      {"Prefix":"10.0.0.0/24","Route Type":"bgp","Active":"True","Metric":0,"Pref":170,"Next-hop (Type)":"192.0.2.1/31 (indirect/local)","Next-hop Interface":"ethernet-1/1.0 "},
	      {"Prefix":"198.51.100.0/31","Route Type":"local","Active":"True","Metric":0,"Pref":0,"Next-hop (Type)":"198.51.100.1 (direct)","Next-hop Interface":"ethernet-1/2.0 "},
	      {"Prefix":"198.51.100.0/24","Route Type":"blackhole","Active":"True","Metric":0,"Pref":1,"Next-hop (Type)":"None"},
	      {"Prefix":"10.255.2.2/32","Route Type":"ospf-internal","Active":"True","Metric":20,"Pref":110,"Next-hop (Type)":"198.51.100.2/32 (direct)","Next-hop Interface":"ethernet-1/4.0 "},
	      {"Prefix":"203.0.113.0/24","Route Type":"bgp","Active":"False","Next-hop (Type)":"192.0.2.2/31 (indirect/local)","Next-hop Interface":"ethernet-1/3.0 "}
	    ]
	  }]
	}` + "\r\n")
	routes, err := ParseSRLinuxRoutes("srl1", data)
	if err != nil {
		t.Fatalf("ParseSRLinuxRoutes() error = %v", err)
	}
	if routeByPrefix(routes, "203.0.113.0/24") != nil {
		t.Fatalf("inactive route was parsed: %#v", routes)
	}
	route := routeByPrefix(routes, "10.0.0.0/24")
	if route == nil {
		t.Fatalf("routes = %#v", routes)
	}
	if route.Source.Protocol != "bgp" || route.Preference != 170 {
		t.Fatalf("route attrs = %#v", route)
	}
	if got, want := route.NextHops, []NextHop{{Address: "192.0.2.1", Interface: "ethernet-1/1.0"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("next-hops = %#v, want %#v", got, want)
	}
	connected := routeByPrefix(routes, "198.51.100.0/31")
	if connected == nil || connected.Source.Protocol != "connected" {
		t.Fatalf("connected route = %#v", connected)
	}
	ospf := routeByPrefix(routes, "10.255.2.2/32")
	if ospf == nil || ospf.Source.Protocol != "ospf" || ospf.Preference != 110 || ospf.Metric != 20 {
		t.Fatalf("OSPF route = %#v", ospf)
	}
	blackhole := routeByPrefix(routes, "198.51.100.0/24")
	if blackhole == nil || blackhole.Source.Protocol != "blackhole" || len(blackhole.NextHops) != 0 {
		t.Fatalf("blackhole route = %#v", blackhole)
	}
}

func TestParseSRLinuxRoutesIgnoresBackupNextHop(t *testing.T) {
	data := []byte(`{
	  "instance": [{
	    "Name": "default",
	    "ip route": [{
	      "Prefix":"10.10.0.0/24",
	      "Route Type":"bgp",
	      "Active":"True",
	      "Metric":0,
	      "Pref":170,
	      "Next-hop (Type)":"192.0.2.1/32 (direct)",
	      "Next-hop Interface":"ethernet-1/1.0 ",
	      "Backup Next-hop (Type)":"192.0.2.254/32 (direct)",
	      "Backup Next-hop Interface":"ethernet-1/99.0 "
	    }]
	  }]
	}`)
	routes, err := ParseSRLinuxRoutes("srl1", data)
	if err != nil {
		t.Fatalf("ParseSRLinuxRoutes() error = %v", err)
	}
	route := routeByPrefix(routes, "10.10.0.0/24")
	if route == nil {
		t.Fatalf("routes = %#v", routes)
	}
	want := []NextHop{{Address: "192.0.2.1", Interface: "ethernet-1/1.0"}}
	if !reflect.DeepEqual(route.NextHops, want) {
		t.Fatalf("next-hops = %#v, want %#v", route.NextHops, want)
	}
}

func TestParseSRLinuxRouteDetailsNormalizesPeerGateway(t *testing.T) {
	data := []byte(`{
	  "instance": [{
	    "Name": "default",
	    "ip route": [{
	      "Destination": "10.4.0.0/16",
	      "ID": 0,
	      "Route Type": "bgp",
	      "Route Owner": "bgp_mgr",
	      "Origin Network Instance": "default",
	      "Metric": 0,
	      "Preference": 170,
	      "Active": true,
	      "ip route nexthop": {
	        "Next Hop Count": 1,
	        "Next hops": "198.18.20.5 (indirect) resolved by route to 198.18.20.4/31 (local)\n  via 198.18.20.5 (direct) via [ethernet-1/4.0]"
	      },
	      "ip route backup nexthop": {
	        "Backup Next Hop Count": 0,
	        "Backup Next hops": ""
	      }
	    }]
	  }]
	}`)
	routes, err := ParseSRLinuxRouteDetails("core-gz", data)
	if err != nil {
		t.Fatalf("ParseSRLinuxRouteDetails() error = %v", err)
	}
	route := routeByPrefix(routes, "10.4.0.0/16")
	if route == nil {
		t.Fatalf("routes = %#v", routes)
	}
	if route.Source.Protocol != "bgp" || route.Preference != 170 {
		t.Fatalf("route attrs = %#v", route)
	}
	want := []NextHop{{Address: "198.18.20.5", Interface: "ethernet-1/4.0"}}
	if !reflect.DeepEqual(route.NextHops, want) {
		t.Fatalf("next-hops = %#v, want %#v", route.NextHops, want)
	}
}

func TestParseSRLinuxRouteDetailsIgnoresBackupNextHop(t *testing.T) {
	data := []byte(`{
	  "instance": [{
	    "Name": "default",
	    "ip route": [{
	      "Destination": "10.20.0.0/24",
	      "Route Type": "bgp",
	      "Metric": 0,
	      "Preference": 170,
	      "Active": true,
	      "ip route nexthop": {
	        "Next Hop Count": 1,
	        "Next hops": "192.0.2.1 (direct) via [ethernet-1/1.0]"
	      },
	      "ip route backup nexthop": {
	        "Backup Next Hop Count": 1,
	        "Backup Next hops": "192.0.2.254 (direct) via [ethernet-1/99.0]"
	      }
	    }]
	  }]
	}`)
	routes, err := ParseSRLinuxRouteDetails("srl1", data)
	if err != nil {
		t.Fatalf("ParseSRLinuxRouteDetails() error = %v", err)
	}
	route := routeByPrefix(routes, "10.20.0.0/24")
	if route == nil {
		t.Fatalf("routes = %#v", routes)
	}
	want := []NextHop{{Address: "192.0.2.1", Interface: "ethernet-1/1.0"}}
	if !reflect.DeepEqual(route.NextHops, want) {
		t.Fatalf("next-hops = %#v, want %#v", route.NextHops, want)
	}
}

func routeByPrefix(routes []FIBEntry, prefix string) *FIBEntry {
	for i := range routes {
		if routes[i].Prefix == prefix {
			return &routes[i]
		}
	}
	return nil
}

func routeByVRFPrefix(fibs []FIB, vrf, prefix string) *FIBEntry {
	for fi := range fibs {
		if string(fibs[fi].VRF) != vrf {
			continue
		}
		for ri := range fibs[fi].Entries {
			if fibs[fi].Entries[ri].Prefix == prefix {
				return &fibs[fi].Entries[ri]
			}
		}
	}
	return nil
}
