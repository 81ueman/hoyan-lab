package ribcompare

import (
	"github.com/81ueman/hoyan-lab/internal/config/routing"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

func Expected(topo *topology.Topology, routes routing.TopologyRouting) []NormalizedBgpRoute {
	return ExpectedWithFailureSet(topo, routes, sim.NoFailures())
}

func ExpectedForNodes(topo *topology.Topology, routes routing.TopologyRouting, nodes []topology.Node) []NormalizedBgpRoute {
	return ExpectedForNodesWithFailureSet(topo, routes, nodes, sim.NoFailures())
}

func ExpectedForNodesWithFailureSet(topo *topology.Topology, routes routing.TopologyRouting, nodes []topology.Node, failures sim.FailureSet) []NormalizedBgpRoute {
	allowed := map[string]bool{}
	for _, n := range nodes {
		allowed[n.Name] = true
	}
	return expected(topo, routes, allowed, failures)
}

func ExpectedWithFailureSet(topo *topology.Topology, routes routing.TopologyRouting, failures sim.FailureSet) []NormalizedBgpRoute {
	return expected(topo, routes, nil, failures)
}

func expected(topo *topology.Topology, routes routing.TopologyRouting, allowed map[string]bool, failures sim.FailureSet) []NormalizedBgpRoute {
	idx, err := topology.BuildTopologyIndex(topo)
	if err != nil {
		panic(err)
	}
	g := sim.NewGraphWithRouting(topo, routes)
	ctx := g.FailureContext(failures)
	var out []NormalizedBgpRoute
	for _, n := range topo.Nodes {
		if allowed != nil && !allowed[n.Name] {
			continue
		}
		if ctx.NodeFailed(topology.NodeID(n.Name)) {
			continue
		}
		for vrf, table := range g.RIBTables(n.Name) {
			for prefix, rib := range table {
				pathsByProtocol := map[string][]NormalizedBgpPath{}
				for _, route := range rib {
					route = route.Normalize()
					if route.Condition == nil || !route.Condition.Eval(ctx) {
						continue
					}
					// OSPF keeps alternate remote paths so failure checks can prove
					// fallback reachability. FRR's route table is a point-in-time SPF
					// result, so only selected remote OSPF routes are expected live.
					if route.SourceKind == topology.RouteSourceOSPF && route.Provenance.OriginNode != n.Name && (route.SelectedCond == nil || !route.SelectedCond.Eval(ctx)) {
						continue
					}
					if !routeComparableInLiveRIB(idx, n.Name, route) {
						continue
					}
					protocol := expectedRouteProtocol(route)
					pathsByProtocol[protocol] = append(pathsByProtocol[protocol], expectedPath(idx, n, route, ctx))
				}
				for _, protocol := range sortedProtocolKeys(pathsByProtocol) {
					paths := pathsByProtocol[protocol]
					if len(paths) == 0 {
						continue
					}
					sortPaths(paths, DefaultBgpRibCompareOptions())
					out = append(out, NormalizedBgpRoute{
						Node:            n.Name,
						NetworkInstance: vrf,
						AFI:             "ipv4",
						Prefix:          prefix,
						Protocol:        protocol,
						ConnectedClass:  connectedClassForProtocol(protocol, rib),
						Paths:           paths,
					})
				}
			}
		}
	}
	sortRoutes(out)
	return out
}

func sortedProtocolKeys(m map[string][]NormalizedBgpPath) []string {
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
	case topology.RouteSourceConnected:
		return "connected"
	case topology.RouteSourceStatic:
		return "static"
	case topology.RouteSourceOSPF:
		if route.RouteSource.OSPFRouteType == "inter-area" {
			return "ospf-ia"
		}
		return "ospf"
	case topology.RouteSourceBlackhole:
		return "blackhole"
	default:
		return "bgp"
	}
}

func connectedClassForProtocol(protocol string, routes []sim.RIBEntry) topology.ConnectedRouteClass {
	if protocol != "connected" {
		return ""
	}
	for _, route := range routes {
		route = route.Normalize()
		if expectedRouteProtocol(route) == "connected" && route.RouteSource.ConnectedClass != "" {
			return route.RouteSource.ConnectedClass
		}
	}
	return ""
}

func routeComparableInLiveRIB(idx *topology.TopologyIndex, node string, route sim.RIBEntry) bool {
	route = route.Normalize()
	switch route.SourceKind {
	case topology.RouteSourceBGP, topology.RouteSourceAggregate:
		return true
	case topology.RouteSourceConnected:
		return comparableConnectedClass(route.RouteSource.ConnectedClass)
	case topology.RouteSourceStatic:
		return route.RouteSource.NextHop != ""
	case topology.RouteSourceOSPF:
		if route.Provenance.OriginNode == node {
			n, ok := idx.Node(node)
			return ok && n.Kind == topology.KindFRR
		}
		return route.ForwardingNextHop.Node != ""
	case topology.RouteSourceBlackhole:
		return true
	default:
		return false
	}
}

func comparableConnectedClass(class topology.ConnectedRouteClass) bool {
	switch class {
	case topology.ConnectedRouteClassLink, topology.ConnectedRouteClassLoopback, topology.ConnectedRouteClassService:
		return true
	default:
		return false
	}
}

func expectedPath(idx *topology.TopologyIndex, node topology.Node, route sim.RIBEntry, ctx sim.FailureContext) NormalizedBgpPath {
	route = route.Normalize()
	if expectedRouteProtocol(route) != "bgp" {
		return NormalizedBgpPath{
			Best:      route.SelectedCond != nil && route.SelectedCond.Eval(ctx),
			Valid:     expectedRouteValid(node, route),
			NextHop:   routeNextHopAddress(idx, node.Name, route),
			Origin:    "igp",
			LocalPref: 100,
		}
	}
	return NormalizedBgpPath{
		Best:      route.SelectedCond != nil && route.SelectedCond.Eval(ctx),
		Valid:     expectedRouteValid(node, route),
		NextHop:   routeNextHopAddress(idx, node.Name, route),
		ASPath:    append([]uint32(nil), route.Attrs.ASPath...),
		Origin:    expectedRouteOrigin(route),
		LocalPref: defaultLocalPref(route.Attrs.LocalPref),
		MED:       route.Attrs.MED,
	}
}

func expectedRouteOrigin(route sim.RIBEntry) string {
	route = route.Normalize()
	if route.Attrs.OriginCode != "" {
		return string(route.Attrs.OriginCode)
	}
	return "igp"
}
