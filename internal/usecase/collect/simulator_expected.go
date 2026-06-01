package collect

import (
	"net/netip"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/engine/controlplane"
	"github.com/81ueman/hoyan-lab/internal/engine/dataplane"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

func (s Simulator) expectedRIB(node model.Node, vrf model.NetworkInstanceID, failures sim.FailureSet) observation.RIB {
	rib := observation.RIB{Node: model.NodeID(node.Name), VRF: vrf}
	if s.graph == nil || s.idx == nil {
		return rib
	}
	ctx := s.graph.FailureContext(failures)
	if ctx.NodeFailed(model.NodeID(node.Name)) {
		return rib
	}
	for prefix, entries := range s.graph.RIBTableVRF(model.NodeID(node.Name), vrf) {
		routesByProtocol := map[routeGroupKey][]sim.RIBEntry{}
		for _, route := range entries {
			route = route.Normalize()
			if route.Condition == nil || !route.Condition.Eval(ctx) {
				continue
			}
			if route.SourceKind == model.RouteSourceOSPF && route.Provenance.OriginNode != node.Name && (route.SelectedCond == nil || !route.SelectedCond.Eval(ctx)) {
				continue
			}
			if !s.routeComparableInLiveRIB(node.Name, route) {
				continue
			}
			group := expectedRouteGroup(route)
			routesByProtocol[group] = append(routesByProtocol[group], route)
		}
		for _, group := range sortedRouteGroupKeys(routesByProtocol) {
			entries := routesByProtocol[group]
			if len(entries) == 0 {
				continue
			}
			rib.Routes = append(rib.Routes, s.expectedRIBRoute(node, prefix.String(), group, entries, ctx))
		}
	}
	observation.SortRIBRoutes(rib.Routes)
	return rib
}

func (s Simulator) expectedFIB(node model.Node, vrf model.NetworkInstanceID, failures sim.FailureSet) observation.FIB {
	fib := observation.FIB{Node: model.NodeID(node.Name), VRF: vrf}
	if s.graph == nil || s.idx == nil {
		return fib
	}
	ctx := s.graph.FailureContext(failures)
	if ctx.NodeFailed(model.NodeID(node.Name)) {
		return fib
	}
	behavior := controlplane.BehaviorFor(node.Kind)
	modeledFIB := s.graph.FIBVRF(model.NodeID(node.Name), vrf)
	suppressedBGP := bgpSuppressedByNonBGPFIB(modeledFIB, ctx)
	byRoute := map[string]observation.FIBEntry{}
	for _, entries := range s.graph.RIBTableVRF(model.NodeID(node.Name), vrf) {
		for _, entry := range entries {
			entry = entry.Normalize()
			if entry.SourceKind != model.RouteSourceBGP && entry.SourceKind != model.RouteSourceAggregate && entry.SourceKind != model.RouteSourceOSPF {
				continue
			}
			if suppressedBGP[entry.NLRI.Prefix.String()] {
				continue
			}
			if entry.SelectedCond == nil || !entry.SelectedCond.Eval(ctx) || !behavior.RouteValidForRIB(node, entry) {
				continue
			}
			metric := s.idx.PathCost(entry.Provenance.PathLinks)
			if entry.SourceKind == model.RouteSourceOSPF {
				metric = entry.RouteSource.Metric
			}
			addExpectedFIBRoute(byRoute, s.idx, modeledFIB, ctx, node.Name, vrf, entry.NLRI.Prefix.String(), forwardingNextHop(entry), entry.RouteSource.Interface, entry.SourceKind, entry.RouteSource.ConnectedClass, metric)
		}
	}
	for _, entry := range modeledFIB {
		if entry.SourceKind == model.RouteSourceBGP || entry.SourceKind == model.RouteSourceAggregate || entry.SourceKind == model.RouteSourceOSPF {
			continue
		}
		if !model.ProfileFor(node.Kind).FIBProfile().ExpectedFIBRouteVisible(entry.SourceKind, entry.ConnectedClass) {
			continue
		}
		if entry.Condition == nil || !entry.Condition.Eval(ctx) {
			continue
		}
		addExpectedFIBRoute(byRoute, s.idx, nil, ctx, node.Name, vrf, entry.Prefix.String(), entry.NextHop, entry.Interface, entry.SourceKind, entry.ConnectedClass, entry.Path.Cost)
	}
	for _, entry := range byRoute {
		entry.NextHops = dedupeNextHops(entry.NextHops)
		fib.Entries = append(fib.Entries, entry)
	}
	observation.SortFIBEntriesForCompare(fib.Entries)
	return fib
}

type routeGroupKey struct {
	Protocol      model.RouteSourceKind
	OSPFRouteType observation.OSPFRouteType
}

func sortedRouteGroupKeys(m map[routeGroupKey][]sim.RIBEntry) []routeGroupKey {
	order := []model.RouteSourceKind{
		model.RouteSourceBGP,
		model.RouteSourceOSPF,
		model.RouteSourceConnected,
		model.RouteSourceStatic,
		model.RouteSourceBlackhole,
	}
	var out []routeGroupKey
	seen := map[routeGroupKey]bool{}
	for _, protocol := range order {
		for key := range m {
			if key.Protocol != protocol {
				continue
			}
			out = append(out, key)
			seen[key] = true
		}
	}
	for key := range m {
		if !seen[key] {
			out = append(out, key)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return routeGroupSortKey(out[i]) < routeGroupSortKey(out[j])
	})
	return out
}

func routeGroupSortKey(key routeGroupKey) string {
	return routeGroupRank(key.Protocol) + "|" + string(key.OSPFRouteType)
}

func routeGroupRank(protocol model.RouteSourceKind) string {
	switch protocol {
	case model.RouteSourceBGP:
		return "0"
	case model.RouteSourceOSPF:
		return "1"
	case model.RouteSourceConnected:
		return "2"
	case model.RouteSourceStatic:
		return "3"
	case model.RouteSourceBlackhole:
		return "4"
	default:
		return "9" + string(protocol)
	}
}

func expectedRouteGroup(route sim.RIBEntry) routeGroupKey {
	route = route.Normalize()
	switch route.SourceKind {
	case model.RouteSourceConnected:
		return routeGroupKey{Protocol: model.RouteSourceConnected}
	case model.RouteSourceStatic:
		return routeGroupKey{Protocol: model.RouteSourceStatic}
	case model.RouteSourceOSPF:
		return routeGroupKey{Protocol: model.RouteSourceOSPF, OSPFRouteType: expectedOSPFRouteType(route)}
	case model.RouteSourceBlackhole:
		return routeGroupKey{Protocol: model.RouteSourceBlackhole}
	default:
		return routeGroupKey{Protocol: model.RouteSourceBGP}
	}
}

func (s Simulator) routeComparableInLiveRIB(node string, route sim.RIBEntry) bool {
	route = route.Normalize()
	switch route.SourceKind {
	case model.RouteSourceBGP, model.RouteSourceAggregate:
		return true
	case model.RouteSourceConnected:
		return comparableConnectedClass(route.RouteSource.ConnectedClass)
	case model.RouteSourceStatic:
		return route.RouteSource.NextHop != ""
	case model.RouteSourceOSPF:
		if route.Provenance.OriginNode == node {
			n, ok := s.idx.Node(node)
			return ok && n.Kind == model.KindFRR
		}
		return route.ForwardingNextHop.Node != ""
	case model.RouteSourceBlackhole:
		return true
	default:
		return false
	}
}

func comparableConnectedClass(class model.ConnectedRouteClass) bool {
	switch class {
	case model.ConnectedRouteClassLink, model.ConnectedRouteClassLoopback, model.ConnectedRouteClassService:
		return true
	default:
		return false
	}
}

func (s Simulator) expectedRIBRoute(node model.Node, prefix string, group routeGroupKey, entries []sim.RIBEntry, ctx sim.FailureContext) observation.RIBRoute {
	routeProtocol := model.NormalizeRouteSourceKind(group.Protocol)
	common := observation.RIBRouteCommon{
		AFI:      model.AFIIPv4,
		Prefix:   prefix,
		Protocol: routeProtocol,
		Eligible: s.expectedEntriesHaveEligiblePath(node, entries),
		Best:     expectedEntriesHaveBestPath(entries, ctx),
	}
	route := observation.RIBRoute{
		Common:    common,
		ModelInfo: &observation.ModelRouteInfo{Provenance: observation.RouteProvenance{FromNode: model.NodeID(node.Name)}},
	}
	switch routeProtocol {
	case model.RouteSourceBGP:
		paths := make([]observation.BGPPath, 0, len(entries))
		for _, entry := range entries {
			paths = append(paths, s.expectedBGPPath(node, entry, ctx))
		}
		observation.SortBGPPaths(paths, observation.DefaultCompareOptions())
		route.BGP = &observation.BGPRIBRoute{Paths: paths}
	case model.RouteSourceOSPF:
		paths := make([]observation.OSPFPath, 0, len(entries))
		for _, entry := range entries {
			paths = append(paths, s.expectedOSPFPath(node, entry))
		}
		observation.SortOSPFPaths(paths, observation.DefaultCompareOptions())
		route.OSPF = &observation.OSPFRIBRoute{RouteType: group.OSPFRouteType, Paths: paths}
	case model.RouteSourceStatic:
		route.Static = &observation.StaticRIBRoute{NextHops: s.expectedRIBNextHops(node, entries)}
	case model.RouteSourceConnected:
		route.Connected = &observation.ConnectedRIBRoute{}
	case model.RouteSourceBlackhole:
		route.Blackhole = &observation.BlackholeRIBRoute{}
	}
	return route
}

func (s Simulator) expectedBGPPath(node model.Node, route sim.RIBEntry, ctx sim.FailureContext) observation.BGPPath {
	route = route.Normalize()
	return observation.BGPPath{
		Best:      route.SelectedCond != nil && route.SelectedCond.Eval(ctx),
		Eligible:  expectedRouteValid(node, route),
		NextHop:   observation.NextHop{Address: s.routeNextHopAddress(node.Name, route)},
		ASPath:    append([]uint32(nil), route.Attrs.ASPath...),
		Origin:    expectedRouteOrigin(route),
		LocalPref: observation.DefaultLocalPref(route.Attrs.LocalPref),
		MED:       route.Attrs.MED,
	}
}

func (s Simulator) expectedOSPFPath(node model.Node, route sim.RIBEntry) observation.OSPFPath {
	route = route.Normalize()
	return observation.OSPFPath{NextHop: observation.NextHop{Address: s.routeNextHopAddress(node.Name, route)}, Cost: route.Attrs.MED}
}

func (s Simulator) expectedRIBNextHops(node model.Node, entries []sim.RIBEntry) []observation.NextHop {
	out := make([]observation.NextHop, 0, len(entries))
	for _, entry := range entries {
		if nh := s.routeNextHopAddress(node.Name, entry); nh != "" {
			out = append(out, observation.NextHop{Address: nh})
		}
	}
	return out
}

func (s Simulator) expectedEntriesHaveEligiblePath(node model.Node, entries []sim.RIBEntry) bool {
	for _, entry := range entries {
		if expectedRouteValid(node, entry) {
			return true
		}
	}
	return len(entries) == 0
}

func expectedEntriesHaveBestPath(entries []sim.RIBEntry, ctx sim.FailureContext) bool {
	for _, entry := range entries {
		if entry.SelectedCond != nil && entry.SelectedCond.Eval(ctx) {
			return true
		}
	}
	return false
}

func expectedOSPFRouteType(route sim.RIBEntry) observation.OSPFRouteType {
	route = route.Normalize()
	switch route.RouteSource.OSPFRouteType {
	case "inter-area":
		return observation.OSPFRouteTypeInterArea
	case "external-type-1":
		return observation.OSPFRouteTypeExternal1
	case "external-type-2":
		return observation.OSPFRouteTypeExternal2
	case "intra-area", "":
		return observation.OSPFRouteTypeIntraArea
	default:
		return observation.OSPFRouteTypeUnknown
	}
}

func expectedRouteOrigin(route sim.RIBEntry) model.BGPOriginCode {
	route = route.Normalize()
	if route.Attrs.OriginCode != "" {
		return route.Attrs.OriginCode
	}
	return model.BGPOriginIGP
}

func expectedRouteValid(node model.Node, route sim.RIBEntry) bool {
	return controlplane.BehaviorFor(node.Kind).RouteValidForRIB(node, route)
}

func (s Simulator) peerAddress(node, peer string) string {
	if peer == "" {
		return ""
	}
	if addr, ok := s.idx.PeerAddress(node, peer); ok {
		return addr.String()
	}
	return peer
}

func (s Simulator) routeNextHopAddress(node string, route sim.RIBEntry) string {
	route = route.Normalize()
	if route.ForwardingNextHop.Addr != "" {
		return route.ForwardingNextHop.Addr
	}
	nextHop := route.ForwardingNextHop.Node
	if nextHop == "" {
		return ""
	}
	if direct := s.peerAddress(node, nextHop); direct != nextHop {
		return direct
	}
	for i := 0; i+1 < len(route.Provenance.PathNodes); i++ {
		if route.Provenance.PathNodes[i] != nextHop {
			continue
		}
		if addr := s.peerAddress(route.Provenance.PathNodes[i+1], nextHop); addr != nextHop {
			return addr
		}
	}
	return nextHop
}

func bgpSuppressedByNonBGPFIB(entries []dataplane.FIBEntry, ctx sim.FailureContext) map[string]bool {
	out := map[string]bool{}
	for _, entry := range entries {
		if entry.SourceKind == model.RouteSourceBGP || entry.SourceKind == model.RouteSourceAggregate || entry.SourceKind == model.RouteSourceOSPF {
			continue
		}
		if entry.Condition == nil || !entry.Condition.Eval(ctx) {
			continue
		}
		out[entry.Prefix.String()] = true
	}
	return out
}

func addExpectedFIBRoute(byRoute map[string]observation.FIBEntry, idx *model.TopologyIndex, fib []dataplane.FIBEntry, ctx sim.FailureContext, node string, vrf model.NetworkInstanceID, prefix, nextHop, iface string, source model.RouteSourceKind, class model.ConnectedRouteClass, metric int) {
	_ = class
	route := observation.FIBEntry{
		AFI:    model.AFIIPv4,
		Prefix: prefix,
		Source: observation.RouteSource{
			Protocol: expectedFIBProtocol(source, nextHop),
		},
		Action: expectedFIBAction(source, nextHop),
		Metric: metric,
	}
	if nextHop != "" {
		route.NextHops = []observation.NextHop{expectedFIBNextHop(idx, fib, ctx, node, prefix, nextHop)}
	} else if iface != "" && source != model.RouteSourceBlackhole {
		route.NextHops = []observation.NextHop{{Interface: iface}}
	}
	key := node + "|" + string(model.NormalizeNetworkInstance(string(vrf))) + "|" + observation.RouteKey(route)
	existing := byRoute[key]
	if existing.Prefix == "" {
		byRoute[key] = route
		return
	}
	existing.NextHops = append(existing.NextHops, route.NextHops...)
	if route.Metric < existing.Metric || existing.Metric == 0 {
		existing.Metric = route.Metric
	}
	byRoute[key] = existing
}

func expectedFIBProtocol(source model.RouteSourceKind, nextHop string) model.RouteSourceKind {
	switch source {
	case model.RouteSourceConnected:
		return model.RouteSourceConnected
	case model.RouteSourceStatic:
		return model.RouteSourceStatic
	case model.RouteSourceBlackhole:
		return model.RouteSourceBlackhole
	case model.RouteSourceOSPF:
		return model.RouteSourceOSPF
	}
	return model.RouteSourceBGP
}

func expectedFIBAction(source model.RouteSourceKind, nextHop string) observation.ForwardingAction {
	switch source {
	case model.RouteSourceBlackhole:
		return observation.ActionDrop
	case model.RouteSourceConnected:
		if nextHop == "" {
			return observation.ActionReceive
		}
	}
	return observation.ActionForward
}

func forwardingNextHop(entry sim.RIBEntry) string {
	entry = entry.Normalize()
	if entry.ForwardingNextHop.Node != "" {
		return entry.ForwardingNextHop.Node
	}
	return entry.ForwardingNextHop.Addr
}

func expectedFIBNextHop(idx *model.TopologyIndex, fib []dataplane.FIBEntry, ctx sim.FailureContext, node, routePrefix, nextHop string) observation.NextHop {
	if resolved, ok := resolveRecursiveNextHop(idx, fib, ctx, node, routePrefix, nextHop); ok {
		return resolved
	}
	out := observation.NextHop{}
	if ref, ok := idx.InterfaceToPeer(node, nextHop); ok {
		out.Interface = ref.ConfigName
	}
	if addr, ok := idx.PeerAddress(node, nextHop); ok {
		out.Address = addr.String()
		return out
	}
	out.Address = nextHop
	return out
}

func resolveRecursiveNextHop(idx *model.TopologyIndex, fib []dataplane.FIBEntry, ctx sim.FailureContext, node, routePrefix, nextHop string) (observation.NextHop, bool) {
	addr, err := netip.ParseAddr(nextHop)
	if err != nil {
		return observation.NextHop{}, false
	}
	for _, entry := range fib {
		if entry.Prefix.String() == routePrefix || !entry.Prefix.Contains(addr) {
			continue
		}
		if entry.Condition == nil || !entry.Condition.Eval(ctx) || entry.NextHop == "" {
			continue
		}
		return expectedFIBNextHop(idx, nil, ctx, node, entry.Prefix.String(), entry.NextHop), true
	}
	return observation.NextHop{}, false
}

func dedupeNextHops(in []observation.NextHop) []observation.NextHop {
	seen := map[string]bool{}
	var out []observation.NextHop
	for _, hop := range in {
		key := hop.Address + "|" + hop.Interface
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, hop)
	}
	observation.SortNextHops(out)
	return out
}
