package rib

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	observationrib "github.com/81ueman/hoyan-lab/internal/domain/observation/rib"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

// ExpectedBuilder builds modeled RIB observations. Its zero value is ready to use.
type ExpectedBuilder struct{}

func (ExpectedBuilder) Build(topo *model.Topology) []observationrib.NormalizedRoute {
	return ExpectedBuilder{}.BuildWithFailureSet(topo, sim.NoFailures())
}

func (ExpectedBuilder) BuildForNodes(topo *model.Topology, nodes []model.Node) []observationrib.NormalizedRoute {
	return ExpectedBuilder{}.BuildForNodesWithFailureSet(topo, nodes, sim.NoFailures())
}

func (ExpectedBuilder) BuildForNodesWithFailureSet(topo *model.Topology, nodes []model.Node, failures sim.FailureSet) []observationrib.NormalizedRoute {
	allowed := map[string]bool{}
	for _, n := range nodes {
		allowed[n.Name] = true
	}
	return expected(topo, allowed, failures)
}

func (ExpectedBuilder) BuildWithFailureSet(topo *model.Topology, failures sim.FailureSet) []observationrib.NormalizedRoute {
	return expected(topo, nil, failures)
}

func expected(topo *model.Topology, allowed map[string]bool, failures sim.FailureSet) []observationrib.NormalizedRoute {
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		panic(err)
	}
	g := sim.NewGraph(topo)
	ctx := g.FailureContext(failures)
	var out []observationrib.NormalizedRoute
	for _, n := range topo.Nodes {
		if allowed != nil && !allowed[n.Name] {
			continue
		}
		if ctx.NodeFailed(model.NodeID(n.Name)) {
			continue
		}
		for vrf, table := range g.RIBTables(n.Name) {
			for prefix, rib := range table {
				pathsByProtocol := map[string][]observationrib.NormalizedPath{}
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
					pathsByProtocol[protocol] = append(pathsByProtocol[protocol], expectedPath(idx, n, route, ctx))
				}
				for _, protocol := range sortedProtocolKeys(pathsByProtocol) {
					paths := pathsByProtocol[protocol]
					if len(paths) == 0 {
						continue
					}
					observationrib.SortPaths(paths, observationrib.DefaultCompareOptions())
					out = append(out, observationrib.NormalizedRoute{
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
	observationrib.SortRoutes(out)
	return out
}

func sortedProtocolKeys(m map[string][]observationrib.NormalizedPath) []string {
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

func connectedClassForProtocol(protocol string, routes []sim.RIBEntry) model.ConnectedRouteClass {
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

func expectedPath(idx *model.TopologyIndex, node model.Node, route sim.RIBEntry, ctx sim.FailureContext) observationrib.NormalizedPath {
	route = route.Normalize()
	if expectedRouteProtocol(route) != "bgp" {
		return observationrib.NormalizedPath{
			Best:      route.SelectedCond != nil && route.SelectedCond.Eval(ctx),
			Valid:     expectedRouteValid(node, route),
			NextHop:   routeNextHopAddress(idx, node.Name, route),
			Origin:    "igp",
			LocalPref: 100,
		}
	}
	return observationrib.NormalizedPath{
		Best:      route.SelectedCond != nil && route.SelectedCond.Eval(ctx),
		Valid:     expectedRouteValid(node, route),
		NextHop:   routeNextHopAddress(idx, node.Name, route),
		ASPath:    append([]uint32(nil), route.Attrs.ASPath...),
		Origin:    expectedRouteOrigin(route),
		LocalPref: observationrib.DefaultLocalPref(route.Attrs.LocalPref),
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
