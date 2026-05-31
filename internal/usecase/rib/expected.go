package rib

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

// ExpectedBuilder builds modeled RIB observations. Its zero value is ready to use.
type ExpectedBuilder struct{}

func (ExpectedBuilder) Build(topo *model.Topology) []observation.RIBRoute {
	return ExpectedBuilder{}.BuildWithFailureSet(topo, sim.NoFailures())
}

func (ExpectedBuilder) BuildForNodes(topo *model.Topology, nodes []model.Node) []observation.RIBRoute {
	return ExpectedBuilder{}.BuildForNodesWithFailureSet(topo, nodes, sim.NoFailures())
}

func (ExpectedBuilder) BuildForNodesWithFailureSet(topo *model.Topology, nodes []model.Node, failures sim.FailureSet) []observation.RIBRoute {
	allowed := map[string]bool{}
	for _, n := range nodes {
		allowed[n.Name] = true
	}
	return expected(topo, allowed, failures)
}

func (ExpectedBuilder) BuildWithFailureSet(topo *model.Topology, failures sim.FailureSet) []observation.RIBRoute {
	return expected(topo, nil, failures)
}

func expected(topo *model.Topology, allowed map[string]bool, failures sim.FailureSet) []observation.RIBRoute {
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		panic(err)
	}
	g := sim.NewGraph(topo)
	ctx := g.FailureContext(failures)
	var out []observation.RIBRoute
	for _, n := range topo.Nodes {
		if allowed != nil && !allowed[n.Name] {
			continue
		}
		if ctx.NodeFailed(model.NodeID(n.Name)) {
			continue
		}
		for vrf, table := range g.RIBTables(n.Name) {
			for prefix, rib := range table {
				routesByProtocol := map[string][]sim.RIBEntry{}
				for _, route := range rib {
					route = route.Normalize()
					if route.Condition == nil || !route.Condition.Eval(ctx) {
						continue
					}
					// OSPF keeps alternate remote paths so failure checks can prove
					// fallback reachability. FRR's route table is a point-in-time SPF
					// result, so only selected remote OSPF routes are expected live.
					if route.SourceKind == model.RouteSourceOSPF && route.Provenance.OriginNode != n.Name && (route.SelectedCond == nil || !route.SelectedCond.Eval(ctx)) {
						continue
					}
					if !routeComparableInLiveRIB(idx, n.Name, route) {
						continue
					}
					protocol := expectedRouteProtocol(route)
					routesByProtocol[protocol] = append(routesByProtocol[protocol], route)
				}
				for _, protocol := range sortedProtocolKeys(routesByProtocol) {
					entries := routesByProtocol[protocol]
					if len(entries) == 0 {
						continue
					}
					_ = vrf
					out = append(out, expectedRoute(idx, n, prefix, protocol, entries, ctx))
				}
			}
		}
	}
	observation.SortRoutes(out)
	return out
}

func sortedProtocolKeys(m map[string][]sim.RIBEntry) []string {
	order := []string{"bgp", "ospf", "connected", "static", "blackhole"}
	var out []string
	seen := map[string]bool{}
	for _, protocol := range order {
		if _, ok := m[protocol]; ok {
			out = append(out, protocol)
			seen[protocol] = true
		}
	}
	for protocol := range m {
		if !seen[protocol] {
			out = append(out, protocol)
		}
	}
	return out
}

func expectedRouteProtocol(route sim.RIBEntry) string {
	route = route.Normalize()
	switch route.SourceKind {
	case model.RouteSourceConnected:
		return "connected"
	case model.RouteSourceStatic:
		return "static"
	case model.RouteSourceOSPF:
		if route.RouteSource.OSPFRouteType == "inter-area" {
			return "ospf-ia"
		}
		return "ospf"
	case model.RouteSourceBlackhole:
		return "blackhole"
	default:
		return "bgp"
	}
}

func routeComparableInLiveRIB(idx *model.TopologyIndex, node string, route sim.RIBEntry) bool {
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
			n, ok := idx.Node(node)
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

func expectedRoute(idx *model.TopologyIndex, node model.Node, prefix, protocol string, entries []sim.RIBEntry, ctx sim.FailureContext) observation.RIBRoute {
	routeProtocol := model.NormalizeRouteSourceKind(model.RouteSourceKind(protocol))
	common := observation.RIBRouteCommon{
		AFI:      model.AFIIPv4,
		Prefix:   prefix,
		Protocol: routeProtocol,
		Eligible: expectedEntriesHaveEligiblePath(node, entries),
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
			paths = append(paths, expectedBGPPath(idx, node, entry, ctx))
		}
		observation.SortBGPPaths(paths, observation.DefaultCompareOptions())
		route.BGP = &observation.BGPRIBRoute{Paths: paths}
	case model.RouteSourceOSPF:
		paths := make([]observation.OSPFPath, 0, len(entries))
		for _, entry := range entries {
			paths = append(paths, expectedOSPFPath(idx, node, entry))
		}
		observation.SortOSPFPaths(paths, observation.DefaultCompareOptions())
		route.OSPF = &observation.OSPFRIBRoute{RouteType: expectedOSPFRouteType(protocol), Paths: paths}
	case model.RouteSourceStatic:
		route.Static = &observation.StaticRIBRoute{NextHops: expectedNextHops(idx, node, entries)}
	case model.RouteSourceConnected:
		route.Connected = &observation.ConnectedRIBRoute{}
	case model.RouteSourceBlackhole:
		route.Blackhole = &observation.BlackholeRIBRoute{}
	}
	return route
}

func expectedBGPPath(idx *model.TopologyIndex, node model.Node, route sim.RIBEntry, ctx sim.FailureContext) observation.BGPPath {
	route = route.Normalize()
	return observation.BGPPath{
		Best:      route.SelectedCond != nil && route.SelectedCond.Eval(ctx),
		Eligible:  expectedRouteValid(node, route),
		NextHop:   observation.NextHop{Address: routeNextHopAddress(idx, node.Name, route)},
		ASPath:    append([]uint32(nil), route.Attrs.ASPath...),
		Origin:    expectedRouteOrigin(route),
		LocalPref: observation.DefaultLocalPref(route.Attrs.LocalPref),
		MED:       route.Attrs.MED,
	}
}

func expectedOSPFPath(idx *model.TopologyIndex, node model.Node, route sim.RIBEntry) observation.OSPFPath {
	route = route.Normalize()
	return observation.OSPFPath{NextHop: observation.NextHop{Address: routeNextHopAddress(idx, node.Name, route)}, Cost: route.Attrs.MED}
}

func expectedNextHops(idx *model.TopologyIndex, node model.Node, entries []sim.RIBEntry) []observation.NextHop {
	out := make([]observation.NextHop, 0, len(entries))
	for _, entry := range entries {
		if nh := routeNextHopAddress(idx, node.Name, entry); nh != "" {
			out = append(out, observation.NextHop{Address: nh})
		}
	}
	return out
}

func expectedEntriesHaveEligiblePath(node model.Node, entries []sim.RIBEntry) bool {
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

func expectedOSPFRouteType(protocol string) observation.OSPFRouteType {
	if protocol == "ospf-ia" {
		return observation.OSPFRouteTypeInterArea
	}
	return observation.OSPFRouteTypeIntraArea
}

func expectedRouteOrigin(route sim.RIBEntry) string {
	route = route.Normalize()
	if route.Attrs.OriginCode != "" {
		return string(route.Attrs.OriginCode)
	}
	return "igp"
}
