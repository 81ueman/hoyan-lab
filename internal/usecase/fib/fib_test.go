package fib

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	observationfib "github.com/81ueman/hoyan-lab/internal/domain/observation/fib"
)

func TestComparableRoutesCanonicalizesNextHopInterfaceByDeviceProfile(t *testing.T) {
	topo := &model.Topology{Nodes: []model.Node{{Name: "srl1", Kind: model.KindSRLinux}}}
	routes := observationfib.ComparableRoutes(topo, []observationfib.NormalizedFIBRoute{{
		Node:      "srl1",
		VRF:       "default",
		AFI:       "ipv4",
		Prefix:    "203.0.113.0/24",
		Protocol:  "static",
		NextHops:  []observationfib.NormalizedFIBNextHop{{Address: "192.0.2.1", Interface: "e1-4"}},
		Installed: true,
	}}, observationfib.Options{})
	if len(routes) != 1 || len(routes[0].NextHops) != 1 {
		t.Fatalf("routes = %#v", routes)
	}
	if got := routes[0].NextHops[0].Interface; got != "ethernet-1/4" {
		t.Fatalf("canonical next-hop interface = %q, want ethernet-1/4", got)
	}
}
func TestComparableRoutesIncludesConnectedClasses(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			{Name: "r1", Kind: model.KindFRR, Interfaces: []model.Interface{
				{Name: "lo", Address: "10.255.0.1/32"},
				{Name: "eth1", Address: "192.0.2.1/31"},
			}},
			{Name: "r2", Kind: model.KindFRR, Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.0/31"}}},
		},
		Links: []model.Link{{Name: "r1-r2", A: "r1", B: "r2", AIntf: "eth1", BIntf: "eth1"}},
	}
	routes := []observationfib.NormalizedFIBRoute{
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "192.0.2.0/31", Protocol: "connected", NextHops: []observationfib.NormalizedFIBNextHop{{Interface: "eth1"}}},
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.255.0.1/32", Protocol: "kernel", NextHops: []observationfib.NormalizedFIBNextHop{{Interface: "lo"}}},
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "203.0.113.1/32", Protocol: "kernel", NextHops: []observationfib.NormalizedFIBNextHop{{Interface: "dummy0"}}},
	}
	filtered := observationfib.ComparableRoutes(topo, routes, observationfib.Options{})
	if len(filtered) != 2 {
		t.Fatalf("filtered routes = %#v", filtered)
	}
	if route := routeByPrefix(filtered, "192.0.2.0/31"); route == nil || route.ConnectedClass != model.ConnectedRouteClassLink {
		t.Fatalf("link route = %#v", route)
	}
	if route := routeByPrefix(filtered, "10.255.0.1/32"); route == nil || route.ConnectedClass != model.ConnectedRouteClassLoopback {
		t.Fatalf("loopback route = %#v", route)
	}
	if route := routeByPrefix(filtered, "192.0.2.0/31"); route == nil || len(route.NextHops) != 1 || route.NextHops[0].Address != "" {
		t.Fatalf("connected route next-hop should compare by interface only: %#v", route)
	}
}

func TestExpectedForNodesNormalizesModeledFIB(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			{Name: "src", Kind: model.KindFRR, ASN: 65000, Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.1/31"}}, Neighbors: []model.BGPNeighbor{{
				PeerNode:  "dst",
				RemoteAS:  65001,
				Activated: true,
			}}},
			{Name: "dst", Kind: model.KindFRR, ASN: 65001, Prefixes: model.MustPrefixes("10.0.0.0/24"), Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.0/31"}}, Neighbors: []model.BGPNeighbor{{
				PeerNode:  "src",
				RemoteAS:  65000,
				Activated: true,
			}}},
		},
		Links: []model.Link{{Name: "src-dst", A: "src", B: "dst", AIntf: "eth1", BIntf: "eth1", Cost: 7, Subnet: "192.0.2.0/31"}},
	}
	routes := ExpectedForNodes(topo, []model.Node{topo.Nodes[0]})
	route := routeByPrefix(routes, "10.0.0.0/24")
	if route == nil {
		t.Fatalf("routes = %#v, want 10.0.0.0/24", routes)
	}
	wantHop := observationfib.NormalizedFIBNextHop{Address: "192.0.2.0", Interface: "eth1"}
	if !reflect.DeepEqual(route.NextHops, []observationfib.NormalizedFIBNextHop{wantHop}) || route.Protocol != "bgp" || route.Metric != 7 {
		t.Fatalf("route = %#v", route)
	}
}

func TestExpectedForNodesResolvesAddressOnlyRecursiveBGPNextHop(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			{
				Name:       "hz-edge1",
				Kind:       model.KindFRR,
				ASN:        65004,
				Interfaces: []model.Interface{{Name: "eth2", Address: "198.18.2.6/31"}},
				Routes: []model.ConfiguredRoute{{
					Prefix:  model.MustPrefix("10.119.2.2/32"),
					NextHop: "198.18.2.7",
					Kind:    model.RouteSourceStatic,
				}},
				Neighbors: []model.BGPNeighbor{{
					PeerNode:  "hz-edge2",
					RemoteAS:  65004,
					Activated: true,
				}},
			},
			{
				Name:       "hz-edge2",
				Kind:       model.KindFRR,
				ASN:        65004,
				Prefixes:   model.MustPrefixes("10.119.0.0/24"),
				Interfaces: []model.Interface{{Name: "eth1", Address: "198.18.2.7/31"}},
				Neighbors: []model.BGPNeighbor{{
					PeerNode:     "hz-edge1",
					RemoteAS:     65004,
					Activated:    true,
					ExportPolicy: "SET-RECURSIVE-NH",
				}},
				RoutePolicies: []model.RoutePolicy{{
					Name: "SET-RECURSIVE-NH",
					Rules: []model.RoutePolicyRule{{
						Seq:        10,
						Action:     "permit",
						SetNextHop: "10.119.2.2",
					}},
				}},
			},
		},
		Links: []model.Link{{Name: "hz1-hz2", A: "hz-edge1", B: "hz-edge2", AIntf: "eth2", BIntf: "eth1", Subnet: "198.18.2.6/31"}},
	}
	routes := ExpectedForNodes(topo, []model.Node{topo.Nodes[0]})
	route := routeByPrefix(routes, "10.119.0.0/24")
	if route == nil {
		t.Fatalf("routes = %#v, want recursive BGP route", routes)
	}
	wantHop := observationfib.NormalizedFIBNextHop{Address: "198.18.2.7", Interface: "eth2"}
	if !reflect.DeepEqual(route.NextHops, []observationfib.NormalizedFIBNextHop{wantHop}) {
		t.Fatalf("next-hops = %#v, want %#v; routes = %#v", route.NextHops, []observationfib.NormalizedFIBNextHop{wantHop}, routes)
	}
}

func TestExpectedForNodesSuppressesSRLinuxLoopbackConnectedFIB(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{{
			Name: "srl",
			Kind: model.KindSRLinux,
			Interfaces: []model.Interface{
				{Name: "lo0.0", Address: "10.255.0.1/32"},
				{Name: "ethernet-1/1.0", Address: "192.0.2.1/31"},
			},
		}},
	}
	routes := ExpectedForNodes(topo, topo.Nodes)
	if routeByPrefix(routes, "10.255.0.1/32") != nil {
		t.Fatalf("SR Linux loopback connected route should not be expected in live FIB: %#v", routes)
	}
	if routeByPrefix(routes, "192.0.2.0/31") == nil {
		t.Fatalf("SR Linux link connected route missing from expected FIB: %#v", routes)
	}
}

func TestExpectedForNodesKeepsLocalBlackholeAndSuppressesSamePrefixBGPFIB(t *testing.T) {
	prefix := model.MustPrefix("203.0.113.0/24")
	topo := &model.Topology{Nodes: []model.Node{{
		Name:     "r1",
		Kind:     model.KindFRR,
		ASN:      65000,
		Prefixes: []model.Prefix{prefix},
		Routes:   []model.ConfiguredRoute{{Prefix: prefix, Kind: model.RouteSourceBlackhole, Interface: "Null0"}},
	}}}
	routes := ExpectedForNodes(topo, topo.Nodes)
	if route := routeByPrefix(routes, prefix.String()); route == nil || route.Protocol != "blackhole" || len(route.NextHops) != 0 {
		t.Fatalf("blackhole FIB route = %#v in %#v", route, routes)
	}
	for _, route := range routes {
		if route.Prefix == prefix.String() && route.Protocol == "bgp" {
			t.Fatalf("same-prefix BGP route should not be expected in local FIB: %#v", routes)
		}
	}
}

func TestCompareReportsRouteAndNextHopDiffs(t *testing.T) {
	expected := []observationfib.NormalizedFIBRoute{
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", NextHops: []observationfib.NormalizedFIBNextHop{{Address: "192.0.2.1", Interface: "eth1"}}},
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.1.0/24"},
	}
	actual := []observationfib.NormalizedFIBRoute{
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", NextHops: []observationfib.NormalizedFIBNextHop{{Address: "192.0.2.2", Interface: "eth1"}}},
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.2.0/24"},
	}
	result := observationfib.Compare(expected, actual)
	if result.OK {
		t.Fatalf("observationfib.Compare() OK, want diffs")
	}
	if len(result.MissingRoutes) != 1 || len(result.UnexpectedRoutes) != 1 || len(result.MissingNextHops) != 1 || len(result.UnexpectedNextHops) != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestNormalizeRoutesMergesDuplicateNextHops(t *testing.T) {
	routes, conflicts := observationfib.NormalizeRoutes([]observationfib.NormalizedFIBRoute{
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", Installed: true, Preference: 20, NextHops: []observationfib.NormalizedFIBNextHop{{Address: "192.0.2.1", Interface: "eth1"}}},
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", Installed: true, Preference: 20, NextHops: []observationfib.NormalizedFIBNextHop{{Address: "192.0.2.2", Interface: "eth2"}}},
	})
	if len(conflicts) != 0 || len(routes) != 1 {
		t.Fatalf("routes=%#v conflicts=%#v, want one merged route and no conflicts", routes, conflicts)
	}
	want := []observationfib.NormalizedFIBNextHop{
		{Address: "192.0.2.1", Interface: "eth1"},
		{Address: "192.0.2.2", Interface: "eth2"},
	}
	if !reflect.DeepEqual(routes[0].NextHops, want) {
		t.Fatalf("next-hops = %#v, want %#v", routes[0].NextHops, want)
	}
}

func TestCompareReportsDuplicateRouteConflictForPreference(t *testing.T) {
	result := observationfib.Compare([]observationfib.NormalizedFIBRoute{
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", Installed: true, Preference: 20},
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", Installed: true, Preference: 30},
	}, nil)
	if result.OK || len(result.DuplicateRouteConflicts) != 1 {
		t.Fatalf("result = %#v, want duplicate route conflict", result)
	}
	conflict := result.DuplicateRouteConflicts[0]
	if conflict.Side != "expected" || conflict.Reason != "preference mismatch" || len(conflict.Routes) != 2 {
		t.Fatalf("conflict = %#v", conflict)
	}
}

func TestCompareReportsDuplicateRouteConflictForConnectedClass(t *testing.T) {
	result := observationfib.Compare([]observationfib.NormalizedFIBRoute{
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "192.0.2.0/31", Protocol: "connected", ConnectedClass: model.ConnectedRouteClassLink, Installed: true},
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "192.0.2.0/31", Protocol: "connected", ConnectedClass: model.ConnectedRouteClassLoopback, Installed: true},
	}, nil)
	if result.OK || len(result.DuplicateRouteConflicts) != 1 || result.DuplicateRouteConflicts[0].Reason != "connected_class mismatch" {
		t.Fatalf("result = %#v, want connected class duplicate conflict", result)
	}
}

func TestCompareReportsExpectedAndActualDuplicateRouteConflicts(t *testing.T) {
	expected := []observationfib.NormalizedFIBRoute{
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", Installed: true, Preference: 20},
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", Installed: true, Preference: 30},
	}
	actual := []observationfib.NormalizedFIBRoute{
		{Node: "r2", VRF: "default", AFI: "ipv4", Prefix: "10.0.1.0/24", Protocol: "bgp", Installed: true, Metric: 10},
		{Node: "r2", VRF: "default", AFI: "ipv4", Prefix: "10.0.1.0/24", Protocol: "bgp", Installed: true, Metric: 20},
	}
	result := observationfib.Compare(expected, actual)
	if result.OK || len(result.DuplicateRouteConflicts) != 2 {
		t.Fatalf("result = %#v, want expected and actual duplicate conflicts", result)
	}
	if result.DuplicateRouteConflicts[0].Side != "expected" || result.DuplicateRouteConflicts[1].Side != "actual" {
		t.Fatalf("conflicts = %#v", result.DuplicateRouteConflicts)
	}
}

func TestCompareDuplicateRoutesDoNotSilentlyOverwrite(t *testing.T) {
	expected := []observationfib.NormalizedFIBRoute{{
		Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", Installed: true,
		NextHops: []observationfib.NormalizedFIBNextHop{{Address: "192.0.2.1", Interface: "eth1"}},
	}}
	actual := []observationfib.NormalizedFIBRoute{
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", Installed: true, NextHops: []observationfib.NormalizedFIBNextHop{{Address: "192.0.2.2", Interface: "eth2"}}},
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", Installed: true, NextHops: []observationfib.NormalizedFIBNextHop{{Address: "192.0.2.1", Interface: "eth1"}}},
	}
	result := observationfib.Compare(expected, actual)
	if result.OK || len(result.UnexpectedNextHops) != 1 || result.UnexpectedNextHops[0].NextHopKey != "192.0.2.2|eth2" {
		t.Fatalf("result = %#v, want duplicate next-hop merged into visible diff", result)
	}
}

func TestFormatAndJSONIncludeDuplicateRouteConflict(t *testing.T) {
	result := observationfib.Result{DuplicateRouteConflicts: []observationfib.DuplicateRouteConflict{{
		RouteKey: "r1|default|ipv4|10.0.0.0/24",
		Side:     "expected",
		Reason:   "preference mismatch",
		Routes: []observationfib.NormalizedFIBRoute{
			{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", Preference: 20},
			{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", Preference: 30},
		},
	}}}
	lines := observationfib.FormatDiffs(result)
	if len(lines) != 1 || !strings.Contains(lines[0], "duplicate FIB route conflict") || !strings.Contains(lines[0], "reason=preference mismatch") {
		t.Fatalf("lines = %#v", lines)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(data), "DuplicateRouteConflicts") || !strings.Contains(string(data), "preference mismatch") {
		t.Fatalf("json = %s, want duplicate conflict", data)
	}
}

func TestComparableRoutesFiltersNonBGPAndUnsupportedNextHops(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			{Name: "r1", Kind: model.KindFRR, Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.1/31"}, {Name: "eth2", Address: "198.51.100.1/31"}}},
			{Name: "r2", Kind: model.KindFRR, Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.0/31"}}},
			{Name: "nos1", Kind: model.DeviceKind("unknown"), Interfaces: []model.Interface{{Name: "eth1", Address: "198.51.100.0/31"}}},
		},
		Links: []model.Link{
			{Name: "r1-r2", A: "r1", B: "r2", AIntf: "eth1", BIntf: "eth1"},
			{Name: "r1-nos1", A: "r1", B: "nos1", AIntf: "eth2", BIntf: "eth1"},
		},
	}
	routes := []observationfib.NormalizedFIBRoute{
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "0.0.0.0/0", Protocol: "", NextHops: []observationfib.NormalizedFIBNextHop{{Address: "172.16.0.1", Interface: "eth0"}}},
		{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.0.0.0/24", Protocol: "bgp", NextHops: []observationfib.NormalizedFIBNextHop{{Address: "192.0.2.0", Interface: "eth1"}, {Address: "198.51.100.0", Interface: "eth2"}}},
	}
	filtered := observationfib.ComparableRoutes(topo, routes, observationfib.Options{AllowUnsupported: true})
	if len(filtered) != 1 {
		t.Fatalf("filtered routes = %#v", filtered)
	}
	if got, want := filtered[0].NextHops, []observationfib.NormalizedFIBNextHop{{Address: "192.0.2.0", Interface: "eth1"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("next-hops = %#v, want %#v", got, want)
	}
}

func TestAnalyzeComparableRoutesReportsManagementFallback(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			{Name: "r1", Kind: model.KindFRR, Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.1/31"}}},
			{Name: "r2", Kind: model.KindFRR, Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.0/31"}}},
		},
		Links: []model.Link{{Name: "r1-r2", A: "r1", B: "r2", AIntf: "eth1", BIntf: "eth1"}},
	}
	routes := []observationfib.NormalizedFIBRoute{{
		Node:     "r1",
		VRF:      "default",
		AFI:      "ipv4",
		Prefix:   "10.3.0.0/16",
		Protocol: "bgp",
		NextHops: []observationfib.NormalizedFIBNextHop{{Address: "172.86.191.1", Interface: "eth0"}},
	}}
	result := observationfib.AnalyzeComparableRoutes(topo, routes, observationfib.Options{})
	if len(result.Routes) != 0 {
		t.Fatalf("routes = %#v, want unresolved route excluded", result.Routes)
	}
	if len(result.Unresolved) != 1 {
		t.Fatalf("unresolved = %#v, want one diagnostic", result.Unresolved)
	}
	got := result.Unresolved[0]
	if got.RouteKey != "r1|default|ipv4|10.3.0.0/16" || got.Reason != "unresolved_or_mgmt_fallback" {
		t.Fatalf("diagnostic = %#v", got)
	}
	if len(got.NextHops) != 1 || got.NextHops[0].Reason != "unresolved_or_mgmt_fallback" {
		t.Fatalf("next-hop diagnostic = %#v", got.NextHops)
	}
}

func TestCompareFilterResultsWarnExcludesUnresolvedRoute(t *testing.T) {
	expected := observationfib.FilterResult{Routes: []observationfib.NormalizedFIBRoute{{
		Node:     "r1",
		VRF:      "default",
		AFI:      "ipv4",
		Prefix:   "10.3.0.0/16",
		Protocol: "bgp",
		NextHops: []observationfib.NormalizedFIBNextHop{{Address: "192.0.2.0", Interface: "eth1"}},
	}}}
	actual := observationfib.FilterResult{Unresolved: []observationfib.UnresolvedRoute{{
		RouteKey: "r1|default|ipv4|10.3.0.0/16",
		Node:     "r1",
		VRF:      "default",
		AFI:      "ipv4",
		Prefix:   "10.3.0.0/16",
		Protocol: "bgp",
		Reason:   "unresolved_or_mgmt_fallback",
	}}}
	result := observationfib.CompareFilterResults(expected, actual, observationfib.Options{})
	if !result.OK {
		t.Fatalf("result = %#v, want warning policy to exclude unresolved route from strict comparison", result)
	}

	result = observationfib.CompareFilterResults(expected, actual, observationfib.Options{UnresolvedPolicy: observationfib.UnresolvedPolicyFail})
	if result.OK || len(result.UnresolvedRoutes) != 1 {
		t.Fatalf("result = %#v, want unresolved route as failing diff", result)
	}
}

func TestComparableRoutesKeepsSRLinuxDetailNextHopAddress(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			{Name: "core-gz", Kind: model.KindSRLinux, Interfaces: []model.Interface{{Name: "ethernet-1/4.0", Address: "198.18.20.4/31"}}},
			{Name: "core-hz", Kind: model.KindFRR, Interfaces: []model.Interface{{Name: "eth3", Address: "198.18.20.5/31"}}},
		},
		Links: []model.Link{{Name: "gz-hz", A: "core-gz", B: "core-hz", AIntf: "e1-4", BIntf: "eth3"}},
	}
	routes := []observationfib.NormalizedFIBRoute{{
		Node:     "core-gz",
		VRF:      "default",
		AFI:      "ipv4",
		Prefix:   "10.4.0.0/16",
		Protocol: "bgp",
		NextHops: []observationfib.NormalizedFIBNextHop{{Address: "198.18.20.5", Interface: "ethernet-1/4.0"}},
	}}
	filtered := observationfib.ComparableRoutes(topo, routes, observationfib.Options{})
	if len(filtered) != 1 {
		t.Fatalf("filtered routes = %#v", filtered)
	}
	want := []observationfib.NormalizedFIBNextHop{{Address: "198.18.20.5", Interface: "ethernet-1/4"}}
	if !reflect.DeepEqual(filtered[0].NextHops, want) {
		t.Fatalf("next-hops = %#v, want %#v", filtered[0].NextHops, want)
	}
}

func routeByPrefix(routes []observationfib.NormalizedFIBRoute, prefix string) *observationfib.NormalizedFIBRoute {
	for i := range routes {
		if routes[i].Prefix == prefix {
			return &routes[i]
		}
	}
	return nil
}

func routeByVRFPrefix(routes []observationfib.NormalizedFIBRoute, vrf, prefix string) *observationfib.NormalizedFIBRoute {
	for i := range routes {
		if routes[i].VRF == vrf && routes[i].Prefix == prefix {
			return &routes[i]
		}
	}
	return nil
}
