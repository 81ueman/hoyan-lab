package bgp

import (
	"net"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type RouteDecision struct {
	Route  route.RIBEntry
	Accept bool
	Reason string
}

type DecisionProcess interface {
	Less(receiver model.Node, a, b route.RIBEntry) bool
	Equivalent(receiver model.Node, a, b route.RIBEntry) bool
}

type DecisionOptions struct {
	AlwaysCompareMED    bool
	DeterministicMED    bool
	CompareRouterID     bool
	PreferLowerRouterID bool
	Multipath           bool
}

func DefaultDecisionOptions() DecisionOptions {
	return DecisionOptions{PreferLowerRouterID: true}
}

type DefaultDecisionProcess struct {
	OptionsValue DecisionOptions
}

func DefaultProcess() DecisionProcess {
	return NewDecisionProcess(DefaultDecisionOptions())
}

func NewDecisionProcess(options DecisionOptions) DecisionProcess {
	return DefaultDecisionProcess{OptionsValue: options}
}

func (d DefaultDecisionProcess) Options() DecisionOptions {
	return d.OptionsValue
}

func (d DefaultDecisionProcess) Less(receiver model.Node, a, b route.RIBEntry) bool {
	return less(receiver, a, b, d.OptionsValue, false)
}

func (d DefaultDecisionProcess) Equivalent(receiver model.Node, a, b route.RIBEntry) bool {
	a = a.Normalize()
	b = b.Normalize()
	if a.SourceKind == model.RouteSourceOSPF || b.SourceKind == model.RouteSourceOSPF {
		return a.SourceKind == b.SourceKind && a.RouteSource.OSPFRouteType == b.RouteSource.OSPFRouteType && a.RouteSource.Metric == b.RouteSource.Metric
	}
	if a.Attrs.Weight != b.Attrs.Weight {
		return false
	}
	if a.Attrs.LocalPref != b.Attrs.LocalPref {
		return false
	}
	if (a.Provenance.OriginNode == receiver.Name) != (b.Provenance.OriginNode == receiver.Name) {
		return false
	}
	if len(a.Attrs.ASPath) != len(b.Attrs.ASPath) {
		return false
	}
	if OriginCodeRank(a.Attrs.OriginCode) != OriginCodeRank(b.Attrs.OriginCode) {
		return false
	}
	if shouldCompareMED(a, b, d.OptionsValue) && a.Attrs.MED != b.Attrs.MED {
		return false
	}
	if nextHopIGPCost(a) != nextHopIGPCost(b) {
		return false
	}
	return a.Attrs.LearnedIBGP == b.Attrs.LearnedIBGP
}

// nextHopIGPCost returns the IGP cost to reach the next-hop of a route.
// Currently returns the stored IGPCost value stored on the route entry;
// in a full implementation this would look up the next-hop in the IGP
// routing table (OSPF/IS-IS).
func nextHopIGPCost(r route.RIBEntry) int {
	return r.IGPCost
}

// ipLess compares two IP address strings and reports whether a < b numerically.
// If either string is not a valid IP address, it falls back to string comparison.
func ipLess(a, b string) bool {
	ipA := net.ParseIP(a)
	ipB := net.ParseIP(b)
	if ipA != nil && ipB != nil {
		return ipLessIP(ipA, ipB)
	}
	return a < b
}

func ipLessIP(a, b net.IP) bool {
	// Compare byte by byte (network byte order).
	// To4() converts IPv4-mapped-IPv6 to 4-byte for shorter comparison.
	a4 := a.To4()
	b4 := b.To4()
	if a4 != nil && b4 != nil {
		for i := 0; i < 4; i++ {
			if a4[i] != b4[i] {
				return a4[i] < b4[i]
			}
		}
		return false
	}
	// Fall back to full 16-byte IPv6 comparison
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return len(a) < len(b)
}

func shouldCompareMED(a, b route.RIBEntry, opts DecisionOptions) bool {
	if opts.DeterministicMED {
		// DeterministicMED groups routes by neighboring AS for MED comparison.
		// MED is only compared within the same neighboring AS, regardless of
		// AlwaysCompareMED.
		return NeighboringAS(a.Attrs.ASPath) == NeighboringAS(b.Attrs.ASPath)
	}
	if opts.AlwaysCompareMED {
		return true
	}
	return NeighboringAS(a.Attrs.ASPath) == NeighboringAS(b.Attrs.ASPath)
}

type FRRDecisionProcess struct {
	DefaultDecisionProcess
}

func NewFRRDecisionProcess(options DecisionOptions) FRRDecisionProcess {
	return FRRDecisionProcess{DefaultDecisionProcess{OptionsValue: options}}
}

func (d FRRDecisionProcess) Less(receiver model.Node, a, b route.RIBEntry) bool {
	return less(receiver, a, b, d.OptionsValue, true)
}

func (d FRRDecisionProcess) Equivalent(receiver model.Node, a, b route.RIBEntry) bool {
	return d.DefaultDecisionProcess.Equivalent(receiver, a, b)
}

type NeverEquivalentDecisionProcess struct {
	DefaultDecisionProcess
}

func NewNeverEquivalentDecisionProcess(options DecisionOptions) NeverEquivalentDecisionProcess {
	return NeverEquivalentDecisionProcess{DefaultDecisionProcess{OptionsValue: options}}
}

func (d NeverEquivalentDecisionProcess) Equivalent(receiver model.Node, a, b route.RIBEntry) bool {
	return false
}

func SelectRoutes(decision DecisionProcess, device model.Node, routes []route.RIBEntry) []route.RIBEntry {
	if decision == nil {
		decision = DefaultProcess()
	}
	out := append([]route.RIBEntry(nil), routes...)
	sort.Slice(out, func(i, j int) bool {
		return decision.Less(device, out[i], out[j])
	})
	return out
}

func EquivalentInstalledRoute(decision DecisionProcess, node model.Node, installed []route.RIBEntry, route route.RIBEntry) bool {
	if decision == nil {
		decision = DefaultProcess()
	}
	for _, existing := range installed {
		if decision.Equivalent(node, existing, route) {
			return true
		}
	}
	return false
}

func ExportRoute(from model.Node, to model.Node, session model.BGPNeighbor, in route.RIBEntry) RouteDecision {
	in = in.Normalize()
	isIBGP := from.ASN == to.ASN
	if isIBGP && in.Attrs.LearnedIBGP {
		return RouteDecision{Route: in, Accept: false, Reason: "ibgp readvertisement"}
	}

	out := in
	out.Attrs.ASPath = append([]uint32(nil), in.Attrs.ASPath...)
	out.Attrs.Communities = append([]string(nil), in.Attrs.Communities...)
	if !isIBGP {
		out.Attrs.ASPath = PrependASN(from.ASN, out.Attrs.ASPath)
	}
	if !isIBGP || session.NextHopSelf || !out.ForwardingNextHop.Valid() {
		out.ForwardingNextHop.Node = from.Name
		out.ForwardingNextHop.Addr = ""
	}
	out.Attrs.LearnedIBGP = isIBGP

	return RouteDecision{Route: out.Normalize(), Accept: true}
}

func ImportRoute(to model.Node, from model.Node, session model.BGPNeighbor, in route.RIBEntry) RouteDecision {
	in = in.Normalize()
	if ContainsASN(in.Attrs.ASPath, to.ASN) {
		return RouteDecision{Route: in, Accept: false, Reason: "as loop"}
	}
	out := in
	if from.ASN != to.ASN {
		out.Attrs.LocalPref = 0
	}
	return RouteDecision{Route: out.Normalize(), Accept: true}
}

func ImportRouteKeepASLoop(to model.Node, from model.Node, session model.BGPNeighbor, in route.RIBEntry) RouteDecision {
	in = in.Normalize()
	if ContainsASN(in.Attrs.ASPath, to.ASN) {
		in.Attrs.Invalid = true
		return RouteDecision{Route: in, Accept: true, Reason: "as loop"}
	}
	return ImportRoute(to, from, session, in)
}

func ContainsASN(xs []uint32, x uint32) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func PrependASN(asn uint32, path []uint32) []uint32 {
	out := make([]uint32, 0, len(path)+1)
	out = append(out, asn)
	out = append(out, path...)
	return out
}

func less(receiver model.Node, a, b route.RIBEntry, opts DecisionOptions, reversePathTie bool) bool {
	a = a.Normalize()
	b = b.Normalize()
	if a.SourceKind == model.RouteSourceOSPF && b.SourceKind == model.RouteSourceOSPF {
		if ospfRouteTypeRank(a.RouteSource.OSPFRouteType) != ospfRouteTypeRank(b.RouteSource.OSPFRouteType) {
			return ospfRouteTypeRank(a.RouteSource.OSPFRouteType) < ospfRouteTypeRank(b.RouteSource.OSPFRouteType)
		}
		if a.RouteSource.Metric != b.RouteSource.Metric {
			return a.RouteSource.Metric < b.RouteSource.Metric
		}
		if len(a.Provenance.PathLinks) != len(b.Provenance.PathLinks) {
			return len(a.Provenance.PathLinks) < len(b.Provenance.PathLinks)
		}
		return pathTie(a, b, reversePathTie)
	}
	if a.Attrs.Weight != b.Attrs.Weight {
		return a.Attrs.Weight > b.Attrs.Weight
	}
	if a.Attrs.LocalPref != b.Attrs.LocalPref {
		return a.Attrs.LocalPref > b.Attrs.LocalPref
	}
	if a.Provenance.OriginNode == receiver.Name || b.Provenance.OriginNode == receiver.Name {
		return a.Provenance.OriginNode == receiver.Name
	}
	if len(a.Attrs.ASPath) != len(b.Attrs.ASPath) {
		return len(a.Attrs.ASPath) < len(b.Attrs.ASPath)
	}
	if OriginCodeRank(a.Attrs.OriginCode) != OriginCodeRank(b.Attrs.OriginCode) {
		return OriginCodeRank(a.Attrs.OriginCode) < OriginCodeRank(b.Attrs.OriginCode)
	}
	if shouldCompareMED(a, b, opts) && a.Attrs.MED != b.Attrs.MED {
		return a.Attrs.MED < b.Attrs.MED
	}
	aExternal := !a.Attrs.LearnedIBGP
	bExternal := !b.Attrs.LearnedIBGP
	if aExternal != bExternal {
		return aExternal
	}
	// IGP cost to next-hop: lower wins
	if nextHopIGPCost(a) != nextHopIGPCost(b) {
		return nextHopIGPCost(a) < nextHopIGPCost(b)
	}
	// Cluster-List: shorter list preferred
	if len(a.Attrs.ClusterList) != len(b.Attrs.ClusterList) {
		return len(a.Attrs.ClusterList) < len(b.Attrs.ClusterList)
	}
	// Originator-ID: IP-address-aware comparison; lower value preferred when both are set.
	// Originator-ID and Router-ID are 32-bit values typically represented as IPv4 addresses.
	if a.Attrs.OriginatorID != "" && b.Attrs.OriginatorID != "" && a.Attrs.OriginatorID != b.Attrs.OriginatorID {
		if opts.PreferLowerRouterID {
			return ipLess(a.Attrs.OriginatorID, b.Attrs.OriginatorID)
		}
		return ipLess(b.Attrs.OriginatorID, a.Attrs.OriginatorID)
	}
	// Router-ID: when CompareRouterID is enabled, compare by the FromNode name.
	// FromNode is the peer that advertised the route; it serves as a proxy for
	// Router-ID since the full Router-ID value is not stored per-route.
	// The Node.RouterID field exists on the topology model for node-level configuration.
	// Note: FromNode names are compared as strings, which may not match real Router-ID
	// numeric ordering (e.g., "r10" < "r2" lexicographically). If precise Router-ID
	// ordering is needed, populate OriginatorID on routes instead.
	if opts.CompareRouterID && a.Provenance.FromNode != b.Provenance.FromNode {
		if opts.PreferLowerRouterID {
			return a.Provenance.FromNode < b.Provenance.FromNode
		}
		return a.Provenance.FromNode > b.Provenance.FromNode
	}
	if len(a.Provenance.PathLinks) != len(b.Provenance.PathLinks) {
		return len(a.Provenance.PathLinks) < len(b.Provenance.PathLinks)
	}
	return pathTie(a, b, reversePathTie)
}

func ospfRouteTypeRank(routeType string) int {
	switch routeType {
	case "", "intra-area":
		return 0
	case "inter-area":
		return 1
	case "external-type-1":
		return 2
	case "external-type-2":
		return 3
	default:
		return 4
	}
}

func pathTie(a, b route.RIBEntry, reverse bool) bool {
	if reverse {
		return strings.Join(a.Provenance.PathNodes, ",") > strings.Join(b.Provenance.PathNodes, ",")
	}
	return strings.Join(a.Provenance.PathNodes, ",") < strings.Join(b.Provenance.PathNodes, ",")
}
