package dataplane

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

// ---------------------------------------------------------------------------
// Helpers shared across dataplane test files
// ---------------------------------------------------------------------------

func testFIB(raw map[string][]FIBEntry) FIBTable {
	out := FIBTable{}
	for node, entries := range raw {
		out[model.NodeID(node)] = map[model.NetworkInstanceID][]FIBEntry{model.NetworkInstanceDefault: entries}
	}
	return out
}

func testACLs(name, node string, rules ...model.ACLRule) []model.ACL {
	return []model.ACL{{
		Name:          name,
		Node:          node,
		Vendor:        model.KindFRR,
		DefaultAction: model.ACLDefaultPermit,
		Rules:         rules,
	}}
}

func testACLBindings(name, node, iface, direction string) []model.ACLBinding {
	return []model.ACLBinding{{Node: node, Interface: iface, Direction: direction, ACLName: name}}
}

func firstUnreachableReason(result SymbolicReachabilityResult, kind SymbolicUnreachableReasonKind) (SymbolicUnreachableReason, bool) {
	for _, reason := range result.UnreachableReasons {
		if reason.Kind == kind {
			return reason, true
		}
	}
	return SymbolicUnreachableReason{}, false
}

func firstUnreachableReasonByLink(result SymbolicReachabilityResult, link string) (SymbolicUnreachableReason, bool) {
	for _, reason := range result.UnreachableReasons {
		if reason.Kind == UnreachableLinkFailed && reason.Link == link {
			return reason, true
		}
	}
	return SymbolicUnreachableReason{}, false
}

// ---------------------------------------------------------------------------
// FIB derivation tests
// ---------------------------------------------------------------------------

func TestDeriveFIBUsesVendorInstallEligibility(t *testing.T) {
	prefix := model.MustPrefix("10.0.0.0/24")
	equivalentRoutes := []domainroute.RIBEntry{
		{NLRI: domainroute.NLRI{Prefix: prefix}, Provenance: domainroute.Provenance{OriginNode: "a", PathNodes: []string{"a", "rx"}}, Attrs: domainroute.BGPAttributes{LocalPref: 100, ASPath: []uint32{65100}}, SelectedCond: failure.LinkVar("path-a"), RouteSource: model.ConfiguredRoute{NetworkInstance: model.NetworkInstanceDefault}},
		{NLRI: domainroute.NLRI{Prefix: prefix}, Provenance: domainroute.Provenance{OriginNode: "b", PathNodes: []string{"b", "rx"}}, Attrs: domainroute.BGPAttributes{LocalPref: 100, ASPath: []uint32{65200}}, SelectedCond: failure.LinkVar("path-b"), RouteSource: model.ConfiguredRoute{NetworkInstance: model.NetworkInstanceDefault}},
	}

	frrIdx, err := model.BuildTopologyIndex(&model.Topology{Nodes: []model.Node{{Name: "rx", Kind: model.KindFRR, ASN: 65000}}})
	if err != nil {
		t.Fatal(err)
	}
	frrRIB := domainroute.RIBTable{"rx": {model.NetworkInstanceDefault: {prefix: append([]domainroute.RIBEntry(nil), equivalentRoutes...)}}}
	frrFIB := FIBTable{}
	NewEngine(frrIdx, frrRIB, frrFIB).DeriveFIB()
	if got := len(frrFIB["rx"][model.NetworkInstanceDefault]); got != 1 {
		t.Fatalf("FRR FIB entries = %d, want equivalent route collapsed to 1", got)
	}

	genericKind := model.DeviceKind("generic")
	genericIdx, err := model.BuildTopologyIndex(&model.Topology{Nodes: []model.Node{{Name: "rx", Kind: genericKind, ASN: 65000}}})
	if err != nil {
		t.Fatal(err)
	}
	genericRIB := domainroute.RIBTable{"rx": {model.NetworkInstanceDefault: {prefix: append([]domainroute.RIBEntry(nil), equivalentRoutes...)}}}
	genericFIB := FIBTable{}
	NewEngine(genericIdx, genericRIB, genericFIB).DeriveFIB()
	if got := len(genericFIB["rx"][model.NetworkInstanceDefault]); got != 2 {
		t.Fatalf("generic FIB entries = %d, want equivalent routes kept", got)
	}
	genericEntries := genericFIB["rx"][model.NetworkInstanceDefault]
	if genericEntries[0].Rank != genericEntries[1].Rank || genericEntries[0].GroupID == "" || genericEntries[0].GroupID != genericEntries[1].GroupID {
		t.Fatalf("generic equivalent routes should share rank/group: %#v", genericEntries)
	}
	if !genericEntries[0].Equivalent || !genericEntries[1].Equivalent {
		t.Fatalf("generic equivalent routes should be marked equivalent: %#v", genericEntries)
	}
}

func TestDeriveFIBPopulatesECMPNextHops(t *testing.T) {
	prefix := model.MustPrefix("10.0.0.0/24")
	equivalentRoutes := []domainroute.RIBEntry{
		{
			NLRI:              domainroute.NLRI{Prefix: prefix},
			Provenance:        domainroute.Provenance{OriginNode: "a", PathNodes: []string{"a", "rx"}},
			Attrs:             domainroute.BGPAttributes{LocalPref: 100, ASPath: []uint32{65100}},
			ForwardingNextHop: domainroute.NextHop{Node: "a", Addr: "10.0.0.1"},
			SelectedCond:      failure.LinkVar("path-a"),
			RouteSource:       model.ConfiguredRoute{NetworkInstance: model.NetworkInstanceDefault},
		},
		{
			NLRI:              domainroute.NLRI{Prefix: prefix},
			Provenance:        domainroute.Provenance{OriginNode: "b", PathNodes: []string{"b", "rx"}},
			Attrs:             domainroute.BGPAttributes{LocalPref: 100, ASPath: []uint32{65200}},
			ForwardingNextHop: domainroute.NextHop{Node: "b", Addr: "10.0.0.2"},
			SelectedCond:      failure.LinkVar("path-b"),
			RouteSource:       model.ConfiguredRoute{NetworkInstance: model.NetworkInstanceDefault},
		},
	}
	idx, err := model.BuildTopologyIndex(&model.Topology{
		Nodes: []model.Node{{Name: "rx", Kind: model.DeviceKind("generic"), ASN: 65000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	rib := domainroute.RIBTable{"rx": {model.NetworkInstanceDefault: {prefix: equivalentRoutes}}}
	fib := FIBTable{}
	NewEngine(idx, rib, fib).DeriveFIB()
	entries := fib["rx"][model.NetworkInstanceDefault]
	if len(entries) != 2 {
		t.Fatalf("FIB entries = %d, want 2 ECMP routes", len(entries))
	}
	if !entries[0].Equivalent || !entries[1].Equivalent {
		t.Fatalf("ECMP routes should be marked Equivalent: %#v", entries)
	}
	if len(entries[0].NextHops) != 2 {
		t.Fatalf("entry[0] NextHops = %d, want 2 (all group next-hops)", len(entries[0].NextHops))
	}
	if len(entries[1].NextHops) != 2 {
		t.Fatalf("entry[1] NextHops = %d, want 2 (all group next-hops)", len(entries[1].NextHops))
	}
	// Verify weights sum to 1.0
	var totalWeight float64
	for _, nh := range entries[0].NextHops {
		totalWeight += nh.Weight
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Fatalf("entry[0] NextHops weights sum to %f, want ~1.0", totalWeight)
	}
	totalWeight = 0
	for _, nh := range entries[1].NextHops {
		totalWeight += nh.Weight
	}
	if totalWeight < 0.99 || totalWeight > 1.01 {
		t.Fatalf("entry[1] NextHops weights sum to %f, want ~1.0", totalWeight)
	}
}

func TestDeriveFIBMarksAddressOnlyNextHopUnresolved(t *testing.T) {
	prefix := model.MustPrefix("10.0.0.0/24")
	idx, err := model.BuildTopologyIndex(&model.Topology{
		Nodes: []model.Node{{Name: "rx", Kind: model.KindFRR}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fib := FIBTable{}
	NewEngine(idx, domainroute.RIBTable{
		"rx": {model.NetworkInstanceDefault: {prefix: {{
			NLRI:              domainroute.NLRI{Prefix: prefix},
			ForwardingNextHop: domainroute.NextHop{Addr: "192.0.2.1"},
			SelectedCond:      failure.True(),
		}}}},
	}, fib).DeriveFIB()
	if got := len(fib["rx"][model.NetworkInstanceDefault]); got != 1 {
		t.Fatalf("FIB entries = %d, want 1", got)
	}
	entry := fib["rx"][model.NetworkInstanceDefault][0]
	if entry.NextHop != "" || entry.NextHopAddress != "192.0.2.1" || entry.ResolutionStatus != NextHopResolutionUnresolvedRecursive {
		t.Fatalf("FIB next-hop resolution = %#v, want unresolved address-only next-hop", entry)
	}
}

func TestDeriveFIBMarksBlackholeRouteAsDiscard(t *testing.T) {
	prefix := model.MustPrefix("10.0.0.0/24")
	idx := mustTopologyIndex(&model.Topology{
		Nodes: []model.Node{{Name: "src", Kind: model.KindFRR}},
	})
	rib := domainroute.RIBTable{
		"src": {
			model.NetworkInstanceDefault: {prefix: {
				{
					NLRI:         domainroute.NLRI{Prefix: prefix},
					SourceKind:   model.RouteSourceBlackhole,
					RouteSource:  model.ConfiguredRoute{Prefix: prefix, Kind: model.RouteSourceBlackhole, Interface: "Null0"},
					SelectedCond: failure.True(),
				},
				{
					NLRI:              domainroute.NLRI{Prefix: prefix},
					SourceKind:        model.RouteSourceBGP,
					ForwardingNextHop: domainroute.NextHop{Node: "remote"},
					SelectedCond:      failure.True(),
				},
			}},
		},
	}
	fib := FIBTable{}
	NewEngine(idx, rib, fib).DeriveFIB()
	if len(fib["src"][model.NetworkInstanceDefault]) != 1 {
		t.Fatalf("FIB entries = %#v, want local blackhole selected over same-prefix BGP", fib["src"])
	}
	entry := fib["src"][model.NetworkInstanceDefault][0]
	if !entry.Discard || entry.SourceKind != model.RouteSourceBlackhole || entry.Interface != "Null0" {
		t.Fatalf("blackhole FIB entry = %#v, want discard blackhole via Null0", entry)
	}
}
