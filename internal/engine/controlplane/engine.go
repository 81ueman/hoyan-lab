package controlplane

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/device"
	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/bgp"
	routingpolicy "github.com/81ueman/hoyan-lab/internal/domain/routing/policy"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type Engine struct {
	idx *model.TopologyIndex
	rib domainroute.RIBTable
}

func NewEngine(idx *model.TopologyIndex, rib domainroute.RIBTable) *Engine {
	if rib == nil {
		rib = domainroute.RIBTable{}
	}
	return &Engine{idx: idx, rib: rib}
}

func (e *Engine) Simulate() error {
	// ── Phase 1: Connected, static, and other non-BGP route installation ──
	for _, origin := range e.idx.Topology.Nodes {
		for _, route := range e.connectedRoutes(origin) {
			e.installConfiguredRoute(origin, route)
		}
		for _, route := range origin.Routes {
			if route.Kind == model.RouteSourceAggregate || route.Kind == model.RouteSourceBGP {
				continue
			}
			e.installConfiguredRoute(origin, route)
		}
	}

	// ── Phase 2: BGP network statements and redistribution ──
	for _, origin := range e.idx.Topology.Nodes {
		if bgpEnabled(origin) {
			for _, prefix := range origin.Prefixes {
				originCond := failure.NodeVar(origin.Name)
				route := domainroute.RIBEntry{
					NLRI:        domainroute.NLRI{Prefix: prefix},
					Attrs:       domainroute.BGPAttributes{OriginCode: model.BGPOriginIGP, LocalPref: model.DefaultLocalPreference},
					Provenance:  domainroute.Provenance{OriginNode: origin.Name, PathNodes: []string{origin.Name}},
					SourceKind:  model.RouteSourceBGP,
					RouteSource: model.ConfiguredRoute{Node: origin.Name, NetworkInstance: model.NetworkInstanceDefault, AFI: model.AFIIPv4, Prefix: prefix, Kind: model.RouteSourceBGP, AdminDistance: model.AdminDistanceBGP},
					BaseCond:    originCond,
					Condition:   originCond,
				}
				e.addRIB(origin.Name, prefix, route)
				e.walkBGP(route)
			}
			for _, network := range origin.Routes {
				if network.Kind != model.RouteSourceBGP || network.Prefix.IsZero() {
					continue
				}
				normalizeConfiguredRoute(&network, origin.Name)
				originCond := failure.NodeVar(origin.Name)
				route := domainroute.RIBEntry{
					NLRI:        domainroute.NLRI{Prefix: network.Prefix},
					Attrs:       domainroute.BGPAttributes{OriginCode: model.BGPOriginIGP, LocalPref: model.DefaultLocalPreference},
					Provenance:  domainroute.Provenance{OriginNode: origin.Name, PathNodes: []string{origin.Name}},
					SourceKind:  model.RouteSourceBGP,
					RouteSource: network,
					BaseCond:    originCond,
					Condition:   originCond,
				}
				e.addRIB(origin.Name, network.Prefix, route)
				e.walkBGP(route)
			}
		}
		for _, route := range e.redistributedRoutes(origin) {
			e.walkBGP(route)
		}
	}

	// ── Phase 3: OSPF route installation ──
	e.installOSPFRoutes()

	// ── Phase 4: Initial route selection and condition convergence ──
	if err := e.selectAndConverge(); err != nil {
		return err
	}

	// ── Phase 5: Aggregate route installation and BGP propagation ──
	for _, origin := range e.idx.Topology.Nodes {
		for _, route := range e.aggregateRoutes(origin) {
			e.addRIB(origin.Name, route.NLRI.Prefix, route)
			e.walkBGP(route)
		}
	}

	// ── Phase 6: Final route selection and convergence ──
	return e.selectAndConverge()
}

func bgpEnabled(node model.Node) bool {
	return node.ASN != 0 || len(node.Neighbors) > 0 || len(node.Redistribute) > 0
}

func (e *Engine) connectedRoutes(node model.Node) []model.ConfiguredRoute {
	var out []model.ConfiguredRoute
	for _, iface := range node.Interfaces {
		pfx, err := netip.ParsePrefix(iface.Address)
		if err != nil {
			continue
		}
		prefix := model.PrefixFromNetIP(pfx.Masked())
		out = append(out, model.ConfiguredRoute{
			Node:            node.Name,
			NetworkInstance: model.NormalizeNetworkInstance(string(iface.VRF)),
			AFI:             model.AFIIPv4,
			Prefix:          prefix,
			Interface:       iface.Name,
			Kind:            model.RouteSourceConnected,
			ConnectedClass:  e.idx.ConnectedClass(node.Name, iface, prefix),
			AdminDistance:   model.AdminDistanceConnected,
		})
	}
	return out
}

func (e *Engine) installConfiguredRoute(node model.Node, route model.ConfiguredRoute) {
	if route.Prefix.IsZero() {
		return
	}
	normalizeConfiguredRoute(&route, node.Name)
	if route.AdminDistance == model.AdminDistanceConnected && route.Kind != model.RouteSourceConnected {
		route.AdminDistance = model.AdminDistanceStatic
	}
	cond := failure.NodeVar(node.Name)
	entry := domainroute.RIBEntry{
		NLRI:              domainroute.NLRI{Prefix: route.Prefix},
		Attrs:             domainroute.BGPAttributes{OriginCode: model.BGPOriginIncomplete},
		Provenance:        domainroute.Provenance{OriginNode: node.Name, PathNodes: []string{node.Name}},
		ForwardingNextHop: e.configuredRouteNextHop(node.Name, route),
		SourceKind:        route.Kind,
		RouteSource:       route,
		BaseCond:          cond,
		Condition:         cond,
	}
	e.addRIB(node.Name, route.Prefix, entry)
}

func (e *Engine) redistributedRoutes(node model.Node) []domainroute.RIBEntry {
	var out []domainroute.RIBEntry
	for _, redist := range node.Redistribute {
		for _, route := range e.redistributionCandidates(node, redist.Kind) {
			redistVRF := model.NormalizeNetworkInstance(string(redist.NetworkInstance))
			routeVRF := model.NormalizeNetworkInstance(string(route.NetworkInstance))
			if routeVRF != redistVRF {
				continue
			}
			entry := e.bgpRouteFromConfiguredRoute(node, route)
			if redist.RouteMap != "" {
				decision := routingpolicy.ApplyRoutePolicy(routePolicyResolver{idx: e.idx}, node, "", redist.RouteMap, entry)
				if !decision.Accept {
					continue
				}
				entry = decision.Route
			}
			e.addRIB(node.Name, entry.NLRI.Prefix, entry)
			out = append(out, entry)
		}
	}
	return out
}

func (e *Engine) redistributionCandidates(node model.Node, kind model.RouteSourceKind) []model.ConfiguredRoute {
	var out []model.ConfiguredRoute
	if kind == model.RouteSourceConnected {
		out = append(out, e.connectedRoutes(node)...)
	}
	if kind == model.RouteSourceStatic {
		for _, route := range node.Routes {
			if route.Kind == model.RouteSourceStatic || route.Kind == model.RouteSourceBlackhole {
				out = append(out, route)
			}
		}
	}
	return out
}

func (e *Engine) bgpRouteFromConfiguredRoute(node model.Node, route model.ConfiguredRoute) domainroute.RIBEntry {
	cond := failure.NodeVar(node.Name)
	entry := domainroute.RIBEntry{
		NLRI:       domainroute.NLRI{Prefix: route.Prefix},
		Attrs:      domainroute.BGPAttributes{OriginCode: model.BGPOriginIncomplete, LocalPref: model.DefaultLocalPreference},
		Provenance: domainroute.Provenance{OriginNode: node.Name, PathNodes: []string{node.Name}},
		SourceKind: model.RouteSourceBGP,
		RouteSource: model.ConfiguredRoute{
			Node:            node.Name,
			NetworkInstance: model.NormalizeNetworkInstance(string(route.NetworkInstance)),
			AFI:             model.AFIIPv4,
			Prefix:          route.Prefix,
			Kind:            model.RouteSourceBGP,
			Source:          route.Source,
			AdminDistance:   model.AdminDistanceBGP,
		},
		BaseCond:  cond,
		Condition: cond,
	}
	return entry.Normalize()
}

func (e *Engine) aggregateRoutes(node model.Node) []domainroute.RIBEntry {
	var out []domainroute.RIBEntry
	for _, route := range node.Routes {
		if route.Kind != model.RouteSourceAggregate || route.Prefix.IsZero() {
			continue
		}
		normalizeConfiguredRoute(&route, node.Name)
		if route.AdminDistance == model.AdminDistanceConnected {
			route.AdminDistance = model.AdminDistanceAggregate
		}
		cond, contributors, ok := e.aggregateContributorCondVRF(node.Name, route.NetworkInstance, route.Prefix)
		if !ok {
			continue
		}
		entry := domainroute.RIBEntry{
			NLRI:                  domainroute.NLRI{Prefix: route.Prefix},
			Attrs:                 domainroute.BGPAttributes{OriginCode: model.BGPOriginIGP, LocalPref: model.DefaultLocalPreference},
			Provenance:            domainroute.Provenance{OriginNode: node.Name, PathNodes: []string{node.Name}},
			SourceKind:            model.RouteSourceAggregate,
			RouteSource:           route,
			AggregateContributors: contributors,
			BaseCond:              cond,
			Condition:             cond,
		}
		out = append(out, entry.Normalize())
	}
	return out
}

func (e *Engine) aggregateContributorCond(node string, aggregate model.Prefix) (failure.Cond, []string, bool) {
	return e.aggregateContributorCondVRF(node, model.NetworkInstanceDefault, aggregate)
}

func (e *Engine) aggregateContributorCondVRF(node string, vrf model.NetworkInstanceID, aggregate model.Prefix) (failure.Cond, []string, bool) {
	var contributors []failure.Cond
	contributorPrefixes := map[string]bool{}
	for candidate, routes := range e.rib[model.NodeID(node)][model.NormalizeNetworkInstance(string(vrf))] {
		if !isMoreSpecificWithin(candidate, aggregate) {
			continue
		}
		for _, route := range routes {
			route = route.Normalize()
			if route.SourceKind == model.RouteSourceAggregate {
				continue
			}
			cond := route.SelectedCond
			if cond == nil {
				cond = route.Condition
			}
			if cond != nil {
				contributors = append(contributors, cond)
				contributorPrefixes[candidate.String()] = true
			}
		}
	}
	if len(contributors) == 0 {
		return failure.False(), nil, false
	}
	prefixes := make([]string, 0, len(contributorPrefixes))
	for prefix := range contributorPrefixes {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	return failure.Or(contributors...), prefixes, true
}

func isMoreSpecificWithin(candidate, aggregate model.Prefix) bool {
	if candidate.IsZero() || aggregate.IsZero() {
		return false
	}
	return candidate.Bits() > aggregate.Bits() && aggregate.Contains(candidate.Addr())
}

func (e *Engine) configuredRouteNextHop(node string, route model.ConfiguredRoute) domainroute.NextHop {
	if route.NextHop == "" {
		return domainroute.NextHop{}
	}
	wantVRF := model.NormalizeNetworkInstance(string(route.NetworkInstance))
	localNode, _ := e.idx.Node(node)
	for _, adj := range e.idx.Adj[model.NodeID(node)] {
		iface, ok := e.idx.InterfaceToPeer(node, string(adj.To))
		if !ok || interfaceVRF(localNode, iface.ConfigName) != wantVRF {
			continue
		}
		if addr, ok := e.idx.PeerAddress(node, string(adj.To)); ok && addr.String() == route.NextHop {
			return domainroute.NextHop{Node: string(adj.To), Addr: route.NextHop}
		}
	}
	return domainroute.NextHop{Addr: route.NextHop}
}

func interfaceVRF(node model.Node, name string) model.NetworkInstanceID {
	for _, iface := range node.Interfaces {
		if model.EquivalentInterfaceName(node.Kind, iface.Name, name) {
			return model.NormalizeNetworkInstance(string(iface.VRF))
		}
	}
	return model.NetworkInstanceDefault
}

func (e *Engine) SelectRoutes() {
	for node, byVRF := range e.rib {
		for vrf, byPrefix := range byVRF {
			for prefix, routes := range byPrefix {
				n, _ := e.idx.Node(string(node))
				behavior := BehaviorFor(n.Kind)
				routes = behavior.SelectRoutes(n, routes)
				for i := range routes {
					if !behavior.RouteValidForRIB(n, routes[i]) {
						routes[i].SelectedCond = failure.False()
						continue
					}
					selected := routes[i].Condition
					var higherDistinct []failure.Cond
					for j := 0; j < i; j++ {
						if !behavior.RouteValidForRIB(n, routes[j]) {
							continue
						}
						if routeSelectionFamily(routes[j]) != routeSelectionFamily(routes[i]) {
							continue
						}
						if behavior.DecisionProcess().Equivalent(n, routes[j], routes[i]) {
							continue
						}
						higherDistinct = append(higherDistinct, routes[j].Condition)
					}
					if len(higherDistinct) > 0 {
						selected = failure.And(selected, failure.Not(failure.Or(higherDistinct...)))
					}
					routes[i].SelectedCond = selected
				}
				e.rib[node][vrf][prefix] = routes
			}
		}
	}
}

func routeSelectionFamily(route domainroute.RIBEntry) model.RouteSourceKind {
	route = route.Normalize()
	if route.SourceKind == model.RouteSourceBGP || route.SourceKind == model.RouteSourceAggregate {
		return model.RouteSourceBGP
	}
	if route.SourceKind == model.RouteSourceOSPF {
		return model.RouteSourceOSPF
	}
	return ""
}

func (e *Engine) ApplyAdvertisementConditions() bool {
	changed := false
	for node, byVRF := range e.rib {
		for vrf, byPrefix := range byVRF {
			for prefix, routes := range byPrefix {
				for i := range routes {
					base := routes[i].BaseCond
					if base == nil {
						base = routes[i].Condition
					}
					nextCond := base
					if routes[i].Normalize().SourceKind == model.RouteSourceOSPF {
						routes[i].Condition = nextCond
						continue
					}
					if len(routes[i].Provenance.PathNodes) > 1 {
						if parent, ok := e.ParentRoute(routes[i]); ok {
							parentSelected := parent.SelectedCond
							if len(parent.Normalize().Provenance.PathNodes) == 1 && (parent.SourceKind == model.RouteSourceBGP || parent.SourceKind == model.RouteSourceAggregate) {
								parentSelected = parent.Condition
							}
							if parentSelected == nil {
								parentSelected = parent.Condition
							}
							nextCond = failure.And(base, parentSelected)
						} else {
							nextCond = failure.False()
						}
					}
					if routes[i].Condition == nil || routes[i].Condition.Key() != nextCond.Key() {
						routes[i].Condition = nextCond
						changed = true
					}
				}
				e.rib[node][vrf][prefix] = routes
			}
		}
	}
	return changed
}

func (e *Engine) ConvergeAdvertisementConditions() error {
	maxIterations := e.MaxRouteDepth() + 1
	if maxIterations < 1 {
		maxIterations = 1
	}
	for i := 0; i < maxIterations; i++ {
		if !e.ApplyAdvertisementConditions() {
			return nil
		}
		e.SelectRoutes()
	}
	return fmt.Errorf("advertisement conditions did not converge within %d iterations", maxIterations)
}

func (e *Engine) MaxRouteDepth() int {
	maxDepth := 0
	for _, byVRF := range e.rib {
		for _, byPrefix := range byVRF {
			for _, routes := range byPrefix {
				for _, route := range routes {
					if len(route.Provenance.PathNodes) > maxDepth {
						maxDepth = len(route.Provenance.PathNodes)
					}
				}
			}
		}
	}
	return maxDepth
}

func (e *Engine) ParentRoute(route domainroute.RIBEntry) (domainroute.RIBEntry, bool) {
	route = route.Normalize()
	if route.Provenance.FromNode == "" || len(route.Provenance.PathNodes) < 2 {
		return domainroute.RIBEntry{}, false
	}
	parentNodes := strings.Join(route.Provenance.PathNodes[:len(route.Provenance.PathNodes)-1], ">")
	for _, candidate := range e.rib[model.NodeID(route.Provenance.FromNode)][route.RouteSource.NetworkInstance][route.NLRI.Prefix] {
		candidate = candidate.Normalize()
		if candidate.SourceKind != route.SourceKind {
			continue
		}
		if strings.Join(candidate.Provenance.PathNodes, ">") == parentNodes {
			return candidate, true
		}
	}
	return domainroute.RIBEntry{}, false
}

func (e *Engine) bgpPropagationContext() bgp.PropagationContext {
	return bgp.PropagationContext{
		Adjacencies: func(nodeID model.NodeID) []model.AdjEdge {
			return e.idx.Adj[nodeID]
		},
		Node: e.idx.Node,
		BGPSession: func(a, b string, vrf model.NetworkInstanceID) (model.BGPNeighbor, bool) {
			return e.bgpSession(a, b, vrf)
		},
		ExportRoute: func(from, to model.Node, session model.BGPNeighbor, route domainroute.RIBEntry) bgp.RouteDecision {
			return BehaviorFor(from.Kind).ExportRoute(from, to, session, route)
		},
		ImportRoute: func(to, from model.Node, session model.BGPNeighbor, route domainroute.RIBEntry) bgp.RouteDecision {
			return BehaviorFor(to.Kind).ImportRoute(to, from, session, route)
		},
		ApplyRoutePolicy: func(node model.Node, peer, policyName string, route domainroute.RIBEntry) bgp.RouteDecision {
			return routingpolicy.ApplyRoutePolicy(routePolicyResolver{idx: e.idx}, node, peer, policyName, route)
		},
		AddRIB: e.addRIB,
		ControlEgress: func(from, to string, route domainroute.RIBEntry) bool {
			curNode, _ := e.idx.Node(from)
			return BehaviorFor(curNode.Kind).CheckControlEgress(curNode, device.ControlMessage{
				From: from, To: to, Prefix: route.NLRI.Prefix.String(), Route: route,
			})
		},
		ControlIngress: func(from, to string, route domainroute.RIBEntry) bool {
			nextNode, _ := e.idx.Node(to)
			return BehaviorFor(nextNode.Kind).CheckControlIngress(nextNode, device.ControlMessage{
				From: from, To: to, Prefix: route.NLRI.Prefix.String(), Route: route,
			})
		},
		EligibleForAdvertisement: func(node model.Node, route domainroute.RIBEntry) bool {
			return BehaviorFor(node.Kind).RouteEligibleForAdvertisement(node, route)
		},
		ApplyAggregateSuppression: e.applyAggregateSuppression,
	}
}

func (e *Engine) walkBGP(route domainroute.RIBEntry) {
	bgp.WalkRoute(e.bgpPropagationContext(), route)
}

func (e *Engine) applyAggregateSuppression(node model.Node, route domainroute.RIBEntry) domainroute.RIBEntry {
	route = route.Normalize()
	for _, aggregate := range node.Routes {
		if aggregate.Kind != model.RouteSourceAggregate || !aggregate.SummaryOnly {
			continue
		}
		if !isMoreSpecificWithin(route.NLRI.Prefix, aggregate.Prefix) {
			continue
		}
		cond, _, ok := e.aggregateContributorCond(node.Name, aggregate.Prefix)
		if !ok {
			continue
		}
		route.BaseCond = failure.And(route.BaseCond, failure.Not(cond))
		route.Condition = failure.And(route.Condition, failure.Not(cond))
	}
	return route.Normalize()
}

func (e *Engine) bgpSession(a, b string, vrf model.NetworkInstanceID) (model.BGPNeighbor, bool) {
	an, ok := e.idx.Node(a)
	if !ok {
		return model.BGPNeighbor{}, false
	}
	vrf = model.NormalizeNetworkInstance(string(vrf))
	for _, peer := range an.Neighbors {
		if model.NormalizeNetworkInstance(string(peer.NetworkInstance)) != vrf {
			continue
		}
		if peer.PeerNode == b && (peer.Activated || peer.RemoteAS != 0) {
			return peer, true
		}
	}
	return model.BGPNeighbor{}, false
}

func (e *Engine) addRIB(node string, prefix model.Prefix, entry domainroute.RIBEntry) {
	entry = entry.Normalize()
	if prefix.IsZero() {
		prefix = entry.NLRI.Prefix
	}
	vrf := model.NormalizeNetworkInstance(string(entry.RouteSource.NetworkInstance))
	entry.RouteSource.NetworkInstance = vrf
	nodeID := model.NodeID(node)
	if e.rib[nodeID] == nil {
		e.rib[nodeID] = map[model.NetworkInstanceID]map[model.Prefix][]domainroute.RIBEntry{}
	}
	if e.rib[nodeID][vrf] == nil {
		e.rib[nodeID][vrf] = map[model.Prefix][]domainroute.RIBEntry{}
	}
	for _, existing := range e.rib[nodeID][vrf][prefix] {
		if routeKey(existing) == routeKey(entry) {
			return
		}
	}
	e.rib[nodeID][vrf][prefix] = append(e.rib[nodeID][vrf][prefix], entry)
}

func routeKey(r domainroute.RIBEntry) string {
	r = r.Normalize()
	valid := "valid"
	if r.Attrs.Invalid {
		valid = "invalid"
	}
	return string(r.RouteSource.NetworkInstance) + "|" + r.NLRI.Prefix.String() + "|" + string(r.SourceKind) + "|" + r.RouteSource.OSPFRouteType + "|" + r.Provenance.OriginNode + "|" + r.ForwardingNextHop.Node + "|" + r.RouteSource.Interface + "|" + strings.Join(r.Provenance.PathNodes, ">") + "|" + valid
}

func normalizeConfiguredRoute(route *model.ConfiguredRoute, nodeName string) {
	route.Node = nodeName
	if route.NetworkInstance == "" {
		route.NetworkInstance = model.NetworkInstanceDefault
	}
	if route.AFI == "" {
		route.AFI = model.AFIIPv4
	}
}

func (e *Engine) selectAndConverge() error {
	e.SelectRoutes()
	return e.ConvergeAdvertisementConditions()
}
