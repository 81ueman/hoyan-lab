package controlplane

import (
	"github.com/81ueman/hoyan-lab/internal/core/netaddr"
	"github.com/81ueman/hoyan-lab/internal/core/predicate"
	"reflect"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

type testRIBOption func(*RIBEntry)

func testRIB(prefix string, opts ...testRIBOption) RIBEntry {
	route := RIBEntry{}
	if prefix != "" {
		route.NLRI.Prefix = netaddr.MustPrefix(prefix)
	}
	for _, opt := range opts {
		opt(&route)
	}
	return route
}

func withNextHop(node string) testRIBOption {
	return func(route *RIBEntry) {
		route.ForwardingNextHop = RouteNextHop{Node: node}
	}
}

func withNextHopAddr(addr string) testRIBOption {
	return func(route *RIBEntry) {
		route.ForwardingNextHop = RouteNextHop{Addr: addr}
	}
}

func withASPath(path ...uint32) testRIBOption {
	return func(route *RIBEntry) {
		route.Attrs.ASPath = append([]uint32(nil), path...)
	}
}

func withLocalPref(localPref int) testRIBOption {
	return func(route *RIBEntry) {
		route.Attrs.LocalPref = localPref
	}
}

func withMED(med int) testRIBOption {
	return func(route *RIBEntry) {
		route.Attrs.MED = med
	}
}

func withOrigin(origin string) testRIBOption {
	return func(route *RIBEntry) {
		route.Provenance.OriginNode = origin
	}
}

func withFrom(from string) testRIBOption {
	return func(route *RIBEntry) {
		route.Provenance.FromNode = from
	}
}

func withOriginCode(origin BGPOriginCode) testRIBOption {
	return func(route *RIBEntry) {
		route.Attrs.OriginCode = origin
	}
}

func withIBGP() testRIBOption {
	return func(route *RIBEntry) {
		route.Attrs.LearnedIBGP = true
	}
}

func withPath(nodes, links []string) testRIBOption {
	return func(route *RIBEntry) {
		route.Provenance.PathNodes = append([]string(nil), nodes...)
		route.Provenance.PathLinks = append([]string(nil), links...)
	}
}

func TestRIBEntryNormalizeSeparatesRouteModelFields(t *testing.T) {
	prefix := netaddr.MustPrefix("10.0.0.0/24")
	route := RIBEntry{
		NLRI:              RouteNLRI{Prefix: prefix},
		Attrs:             BGPAttributes{ASPath: []uint32{65100}, OriginCode: BGPOriginEGP, LocalPref: 150, MED: 20, LearnedIBGP: true},
		Provenance:        RouteProvenance{OriginNode: "origin-node", FromNode: "peer-node", PathNodes: []string{"origin-node", "peer-node", "rx"}, PathLinks: []string{"a", "b"}},
		ForwardingNextHop: RouteNextHop{Node: "peer-node"},
	}.Normalize()

	if route.Provenance.OriginNode != "origin-node" || route.Attrs.OriginCode != BGPOriginEGP {
		t.Fatalf("origin node/code = %q/%q, want separated origin-node/egp", route.Provenance.OriginNode, route.Attrs.OriginCode)
	}
	if route.Provenance.OriginNode == string(route.Attrs.OriginCode) {
		t.Fatalf("provenance origin node was mixed with BGP origin-code: %#v", route)
	}
	if route.NLRI.Prefix.String() != prefix.String() || route.ForwardingNextHop.Node != "peer-node" {
		t.Fatalf("route model fields not populated: %#v", route)
	}
	if !reflect.DeepEqual(route.Attrs.ASPath, []uint32{65100}) || route.Attrs.LocalPref != 150 || route.Attrs.MED != 20 || !route.Attrs.LearnedIBGP {
		t.Fatalf("BGP attributes not synchronized: %#v", route)
	}
}

func TestInterfaceMatchesAliases(t *testing.T) {
	for _, tt := range []struct {
		kind   topology.DeviceKind
		policy string
		packet string
	}{
		{kind: topology.KindCEOS, policy: "eth5", packet: "Ethernet5"},
		{kind: topology.KindCEOS, policy: "Ethernet5", packet: "eth5"},
		{kind: topology.KindSRLinux, policy: "ethernet-1/4.0", packet: "e1-4"},
		{kind: topology.KindSRLinux, policy: "e1-4", packet: "ethernet-1/4.0"},
	} {
		if !interfaceMatches(tt.kind, tt.policy, tt.packet) {
			t.Fatalf("interfaceMatches(%s, %q, %q) = false, want true", tt.kind, tt.policy, tt.packet)
		}
	}
	if interfaceMatches(topology.KindFRR, "eth1", "Ethernet1") {
		t.Fatalf("interfaceMatches(FRR, eth1, Ethernet1) = true, want false")
	}
	if interfaceMatches(topology.KindFRR, "eth1", "eth2") {
		t.Fatalf("interfaceMatches(eth1, eth2) = true, want false")
	}
}

func TestEvaluateDataACLFirstMatchAndDefaultAction(t *testing.T) {
	pfx := netaddr.MustPrefix("10.0.0.0/24")
	packet := PacketMessage{Spec: predicate.PacketSpec{
		DstSet:          predicate.ExactPrefixSet{Prefix: pfx},
		Protocol:        "tcp",
		DstPort:         predicate.ExactPort(80),
		EgressInterface: "eth1",
	}}
	node := topology.Node{Name: "r1", Kind: topology.KindCEOS}
	bindings := []topology.ACLBinding{{Node: "r1", Interface: "eth1", Direction: "egress", ACLName: "WEB"}}

	acls := []topology.ACL{{
		Name:          "WEB",
		Node:          "r1",
		Vendor:        topology.KindCEOS,
		DefaultAction: topology.ACLDefaultDeny,
		Rules: []topology.ACLRule{
			{Seq: 10, Action: topology.ACLPermit, Match: packet.Spec},
			{Seq: 20, Action: topology.ACLDeny, Match: packet.Spec},
		},
	}}
	decision := BehaviorFor(topology.KindCEOS).EvaluateDataACL(node, packet, "egress", acls, bindings)
	if decision.Denied || decision.Action != topology.ACLPermit || decision.RuleSeq != 10 {
		t.Fatalf("permit before deny decision = %#v, want permit rule 10", decision)
	}

	acls[0].Rules[0].Action = topology.ACLDeny
	acls[0].Rules[1].Action = topology.ACLPermit
	decision = BehaviorFor(topology.KindCEOS).EvaluateDataACL(node, packet, "egress", acls, bindings)
	if !decision.Denied || decision.Action != topology.ACLDeny || decision.RuleSeq != 10 {
		t.Fatalf("deny before permit decision = %#v, want deny rule 10", decision)
	}

	acls[0].Rules = nil
	decision = BehaviorFor(topology.KindCEOS).EvaluateDataACL(node, packet, "egress", acls, bindings)
	if !decision.Denied || decision.DefaultAction != topology.ACLDefaultDeny {
		t.Fatalf("default deny decision = %#v, want denied default", decision)
	}

	acls[0].DefaultAction = topology.ACLDefaultPermit
	decision = BehaviorFor(topology.KindFRR).EvaluateDataACL(topology.Node{Name: "r1", Kind: topology.KindFRR}, packet, "egress", acls, bindings)
	if decision.Denied || decision.DefaultAction != topology.ACLDefaultPermit {
		t.Fatalf("default permit decision = %#v, want permitted default", decision)
	}
}

func TestBaseBGPExportRoute(t *testing.T) {
	behavior := NewGenericBehavior("generic")
	ebgpFrom := topology.Node{Name: "r1", ASN: 65001}
	ebgpTo := topology.Node{Name: "r2", ASN: 65002}
	ibgpTo := topology.Node{Name: "r3", ASN: 65001}

	tests := []struct {
		name        string
		from        topology.Node
		to          topology.Node
		session     topology.BGPNeighbor
		route       RIBEntry
		accept      bool
		nextHop     string
		nextHopNode string
		nextHopAddr string
		asPath      []uint32
		learnedIBG  bool
	}{
		{
			name:        "ebgp prepends local ASN and rewrites next-hop",
			from:        ebgpFrom,
			to:          ebgpTo,
			route:       testRIB("10.0.0.0/24", withNextHop("original"), withASPath(65100)),
			accept:      true,
			nextHop:     "r1",
			nextHopNode: "r1",
			asPath:      []uint32{65001, 65100},
		},
		{
			name:        "ibgp preserves next-hop",
			from:        ebgpFrom,
			to:          ibgpTo,
			route:       testRIB("10.0.0.0/24", withNextHopAddr("192.0.2.1"), withASPath(65100)),
			accept:      true,
			nextHop:     "192.0.2.1",
			nextHopAddr: "192.0.2.1",
			asPath:      []uint32{65100},
			learnedIBG:  true,
		},
		{
			name:        "ibgp next-hop-self rewrites next-hop",
			from:        ebgpFrom,
			to:          ibgpTo,
			session:     topology.BGPNeighbor{NextHopSelf: true},
			route:       testRIB("10.0.0.0/24", withNextHopAddr("192.0.2.1"), withASPath(65100)),
			accept:      true,
			nextHop:     "r1",
			nextHopNode: "r1",
			asPath:      []uint32{65100},
			learnedIBG:  true,
		},
		{
			name:        "ibgp empty next-hop is set to exporter",
			from:        ebgpFrom,
			to:          ibgpTo,
			route:       testRIB("10.0.0.0/24", withASPath(65100)),
			accept:      true,
			nextHop:     "r1",
			nextHopNode: "r1",
			asPath:      []uint32{65100},
			learnedIBG:  true,
		},
		{
			name:   "ibgp learned route is not readvertised to ibgp",
			from:   ebgpFrom,
			to:     ibgpTo,
			route:  testRIB("10.0.0.0/24", withIBGP(), withNextHop("edge"), withASPath(65100)),
			accept: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := behavior.ExportRoute(tt.from, tt.to, tt.session, tt.route)
			if got.Accept != tt.accept {
				t.Fatalf("Accept = %v, want %v, reason=%s", got.Accept, tt.accept, got.Reason)
			}
			if !tt.accept {
				return
			}
			gotNextHop := got.Route.ForwardingNextHop.Node
			if gotNextHop == "" {
				gotNextHop = got.Route.ForwardingNextHop.Addr
			}
			if gotNextHop != tt.nextHop {
				t.Fatalf("next-hop = %q, want %q", gotNextHop, tt.nextHop)
			}
			if !reflect.DeepEqual(got.Route.Attrs.ASPath, tt.asPath) {
				t.Fatalf("ASPath = %v, want %v", got.Route.Attrs.ASPath, tt.asPath)
			}
			if got.Route.Attrs.LearnedIBGP != tt.learnedIBG {
				t.Fatalf("LearnedIBGP = %v, want %v", got.Route.Attrs.LearnedIBGP, tt.learnedIBG)
			}
			if got.Route.ForwardingNextHop.Node != tt.nextHopNode {
				t.Fatalf("ForwardingNextHop.Node = %q, want %q", got.Route.ForwardingNextHop.Node, tt.nextHopNode)
			}
			if got.Route.ForwardingNextHop.Addr != tt.nextHopAddr {
				t.Fatalf("ForwardingNextHop.Addr = %q, want %q", got.Route.ForwardingNextHop.Addr, tt.nextHopAddr)
			}
			if !reflect.DeepEqual(got.Route.Attrs.ASPath, tt.asPath) || got.Route.Attrs.LearnedIBGP != tt.learnedIBG {
				t.Fatalf("structured BGP attrs = %#v, want ASPath %v LearnedIBGP %v", got.Route.Attrs, tt.asPath, tt.learnedIBG)
			}
		})
	}
}

func TestBaseBGPImportRoute(t *testing.T) {
	behavior := NewGenericBehavior("generic")
	to := topology.Node{Name: "r2", ASN: 65002}
	from := topology.Node{Name: "r1", ASN: 65001}

	rejected := behavior.ImportRoute(to, from, topology.BGPNeighbor{}, testRIB("", withASPath(65001, 65002)))
	if rejected.Accept {
		t.Fatalf("route containing receiver ASN was accepted")
	}

	accepted := behavior.ImportRoute(to, from, topology.BGPNeighbor{}, testRIB("", withASPath(65001, 65100), withLocalPref(200)))
	if !accepted.Accept {
		t.Fatalf("route without receiver ASN was rejected: %s", accepted.Reason)
	}
	if !reflect.DeepEqual(accepted.Route.Attrs.ASPath, []uint32{65001, 65100}) {
		t.Fatalf("accepted route mutated: %#v", accepted.Route)
	}
	if accepted.Route.Attrs.LocalPref != 0 {
		t.Fatalf("eBGP import LocalPref = %d, want stripped before receiver default/import policy", accepted.Route.Attrs.LocalPref)
	}
	ibgp := behavior.ImportRoute(topology.Node{Name: "r3", ASN: 65001}, from, topology.BGPNeighbor{}, testRIB("", withASPath(65100), withLocalPref(200)))
	if !ibgp.Accept || ibgp.Route.Attrs.LocalPref != 200 {
		t.Fatalf("iBGP import = %#v, want local-pref preserved", ibgp)
	}
}

func TestDefaultBGPDecisionProcessOrdering(t *testing.T) {
	receiver := topology.Node{Name: "rx", ASN: 65000}
	decision := DefaultBGPDecisionProcess()

	assertLess := func(name string, better, worse RIBEntry) {
		t.Helper()
		if !decision.Less(receiver, better, worse) {
			t.Fatalf("%s: better route was not ordered first", name)
		}
		if decision.Less(receiver, worse, better) {
			t.Fatalf("%s: worse route was ordered first", name)
		}
	}

	assertLess("local-pref", testRIB("", withLocalPref(200)), testRIB("", withLocalPref(100)))
	assertLess("local-origin", testRIB("", withOrigin("rx"), withLocalPref(100)), testRIB("", withOrigin("remote"), withLocalPref(100)))
	assertLess("as-path-length", testRIB("", withASPath(65100)), testRIB("", withASPath(65100, 65200)))
	assertLess("origin-code", testRIB("", withASPath(65100), withOriginCode(BGPOriginIGP), withMED(20)), testRIB("", withASPath(65100), withOriginCode(BGPOriginIncomplete), withMED(10)))
	assertLess("med", testRIB("", withASPath(65100), withMED(10)), testRIB("", withASPath(65100), withMED(20)))
	assertLess("ebgp-over-ibgp", testRIB("", withASPath(65100)), testRIB("", withASPath(65100), withIBGP()))
	assertLess("shorter-link-path", testRIB("", withASPath(65100), withPath([]string{"a"}, []string{"a"})), testRIB("", withASPath(65100), withPath([]string{"a", "b"}, []string{"a", "b"})))
	assertLess("vendor-tie-break", testRIB("", withASPath(65100), withPath([]string{"a"}, nil)), testRIB("", withASPath(65100), withPath([]string{"b"}, nil)))
}

func TestBGPDecisionOptionsControlMEDScope(t *testing.T) {
	receiver := topology.Node{Name: "rx", ASN: 65000}
	always := NewBGPDecisionProcess(BGPDecisionOptions{AlwaysCompareMED: true})
	sameNeighborOnly := NewBGPDecisionProcess(BGPDecisionOptions{})

	lowMEDDifferentNeighbor := testRIB("", withASPath(65100), withMED(10), withPath([]string{"z"}, nil))
	highMEDDifferentNeighbor := testRIB("", withASPath(65200), withMED(20), withPath([]string{"a"}, nil))
	if !always.Less(receiver, lowMEDDifferentNeighbor, highMEDDifferentNeighbor) {
		t.Fatalf("AlwaysCompareMED should compare MED across different neighboring ASNs")
	}
	if sameNeighborOnly.Less(receiver, lowMEDDifferentNeighbor, highMEDDifferentNeighbor) {
		t.Fatalf("MED should be skipped across different neighboring ASNs when AlwaysCompareMED is false")
	}

	lowMEDSameNeighbor := testRIB("", withASPath(65100), withMED(10), withPath([]string{"z"}, nil))
	highMEDSameNeighbor := testRIB("", withASPath(65100), withMED(20), withPath([]string{"a"}, nil))
	if !sameNeighborOnly.Less(receiver, lowMEDSameNeighbor, highMEDSameNeighbor) {
		t.Fatalf("MED should be compared within the same neighboring AS")
	}
}

func TestBGPDecisionOptionsDocumentUnsupportedRouterIDTieBreak(t *testing.T) {
	behavior := NewFRRBehavior()
	options := behavior.DecisionOptions()
	if options.CompareRouterID {
		t.Fatalf("router-id tie-break should remain explicitly unsupported until routes carry router-id attributes")
	}
	if !options.PreferLowerRouterID {
		t.Fatalf("router-id tie-break direction should be documented for future implementation")
	}
}

func TestDefaultBGPDecisionProcessEquivalent(t *testing.T) {
	receiver := topology.Node{Name: "rx", ASN: 65000}
	decision := DefaultBGPDecisionProcess()
	a := testRIB("", withLocalPref(100), withASPath(65100), withMED(10), withPath([]string{"a"}, []string{"a"}))
	b := testRIB("", withLocalPref(100), withASPath(65200), withMED(10), withPath([]string{"b"}, []string{"b"}))
	if !decision.Equivalent(receiver, a, b) {
		t.Fatalf("routes should be equivalent before tie-break")
	}
	c := testRIB("", withLocalPref(100), withASPath(65100), withMED(10), withIBGP())
	if decision.Equivalent(receiver, a, c) {
		t.Fatalf("eBGP and iBGP routes should not be equivalent")
	}
	d := testRIB("", withLocalPref(100), withASPath(65300), withMED(10), withPath(nil, []string{"d", "e"}))
	if !decision.Equivalent(receiver, a, d) {
		t.Fatalf("routes with equal BGP attributes before tie-break should be equivalent")
	}
	e := testRIB("", withLocalPref(100), withASPath(65100), withOriginCode(BGPOriginIncomplete), withMED(10))
	if decision.Equivalent(receiver, a, e) {
		t.Fatalf("routes with different origin-code should not be equivalent")
	}
}

func TestCEOSSelectRoutesKeepsUnreachableNextHopForBgpRIB(t *testing.T) {
	behavior := NewCEOSBehavior()
	device := topology.Node{Name: "ceos", ASN: 65000}
	routes := []RIBEntry{
		testRIB("10.0.0.0/24", withFrom("peer1"), withNextHop("remote"), withLocalPref(300)),
		testRIB("10.0.0.0/24", withFrom("peer2"), withNextHop("peer2"), withLocalPref(200)),
		testRIB("10.0.0.0/24", withFrom("peer3"), withLocalPref(100)),
	}
	selected := behavior.SelectRoutes(device, routes)
	if len(selected) != 3 {
		t.Fatalf("selected routes = %#v, want all BGP RIB routes", selected)
	}
	if selected[0].Provenance.FromNode != "peer1" || selected[1].Provenance.FromNode != "peer2" || selected[2].Provenance.FromNode != "peer3" {
		t.Fatalf("selected routes = %#v", selected)
	}
}

func TestDeviceBehaviorRouteValidityHooks(t *testing.T) {
	prefix := netaddr.MustPrefix("10.0.0.0/24")
	generic := NewGenericBehavior(topology.DeviceKind("generic"))
	genericDevice := topology.Node{Name: "generic", Kind: topology.DeviceKind("generic"), ASN: 65000}
	validRoute := testRIB(prefix.String(), withFrom("peer"), withNextHop("remote"))
	invalidRoute := validRoute
	invalidRoute.Attrs.Invalid = true

	if !generic.RouteValidForRIB(genericDevice, validRoute) {
		t.Fatalf("generic valid route was marked invalid")
	}
	if generic.RouteEligibleForAdvertisement(genericDevice, invalidRoute) {
		t.Fatalf("generic invalid route should not be advertised")
	}
	if generic.RouteInstallableInFIB(genericDevice, nil, invalidRoute) {
		t.Fatalf("generic invalid route should not be installed in FIB")
	}

	ceos := NewCEOSBehavior()
	ceosDevice := topology.Node{Name: "ceos", Kind: topology.KindCEOS, ASN: 65000}
	unresolved := testRIB(prefix.String(), withFrom("peer"), withNextHop("remote"))
	direct := testRIB(prefix.String(), withFrom("peer"), withNextHop("peer"))
	local := testRIB(prefix.String())
	if ceos.RouteValidForRIB(ceosDevice, unresolved) {
		t.Fatalf("cEOS unresolved next-hop route should be invalid")
	}
	if !ceos.RouteValidForRIB(ceosDevice, direct) {
		t.Fatalf("cEOS direct next-hop route should be valid")
	}
	if !ceos.RouteValidForRIB(ceosDevice, local) {
		t.Fatalf("cEOS local route should be valid")
	}

	srl := NewSRLinuxBehavior()
	imported := srl.ImportRoute(topology.Node{Name: "rx", ASN: 65000}, topology.Node{Name: "tx", ASN: 65100}, topology.BGPNeighbor{}, testRIB(prefix.String(), withASPath(65100, 65000)))
	if !imported.Accept || !imported.Route.Attrs.Invalid {
		t.Fatalf("SR Linux AS-loop route should be retained as invalid: %#v", imported)
	}
	if srl.RouteEligibleForAdvertisement(topology.Node{Name: "rx", Kind: topology.KindSRLinux}, imported.Route) {
		t.Fatalf("SR Linux invalid retained route should not be advertised")
	}
}

func TestRegisterBehaviorReturnsRestoreFunction(t *testing.T) {
	restore := RegisterBehavior("test-kind", NewGenericBehavior("registered-kind"))
	if BehaviorFor("test-kind").Kind() != "registered-kind" {
		t.Fatalf("registered behavior was not returned")
	}
	restore()
	if BehaviorFor("test-kind").Kind() != "test-kind" {
		t.Fatalf("fallback generic behavior was not restored")
	}

	old := BehaviorFor("frr")
	restore = RegisterBehavior("frr", NewGenericBehavior("temporary-frr"))
	if BehaviorFor("frr").Kind() != "temporary-frr" {
		t.Fatalf("replacement behavior was not returned")
	}
	restore()
	if BehaviorFor("frr") != old {
		t.Fatalf("original behavior was not restored")
	}
}
