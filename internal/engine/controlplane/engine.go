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
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type Engine struct {
	idx *model.TopologyIndex
	rib map[string]map[string]map[string][]domainroute.RIBEntry
}

func NewEngine(idx *model.TopologyIndex, rib map[string]map[string]map[string][]domainroute.RIBEntry) *Engine {
	if rib == nil {
		rib = map[string]map[string]map[string][]domainroute.RIBEntry{}
	}
	return &Engine{idx: idx, rib: rib}
}

func (e *Engine) Simulate() {
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
		if bgpEnabled(origin) {
			for _, prefix := range origin.Prefixes {
				originCond := failure.NodeVar(origin.Name)
				route := domainroute.RIBEntry{
					NLRI:        domainroute.NLRI{Prefix: prefix},
					Attrs:       domainroute.BGPAttributes{OriginCode: domainroute.BGPOriginIGP, LocalPref: 100},
					Provenance:  domainroute.Provenance{OriginNode: origin.Name, PathNodes: []string{origin.Name}},
					SourceKind:  model.RouteSourceBGP,
					RouteSource: model.ConfiguredRoute{Node: origin.Name, NetworkInstance: model.NetworkInstanceDefault, AFI: model.AFIIPv4, Prefix: prefix, Kind: model.RouteSourceBGP, AdminDistance: 200},
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
				network.Node = origin.Name
				if network.NetworkInstance == "" {
					network.NetworkInstance = model.NetworkInstanceDefault
				}
				if network.AFI == "" {
					network.AFI = model.AFIIPv4
				}
				originCond := failure.NodeVar(origin.Name)
				route := domainroute.RIBEntry{
					NLRI:        domainroute.NLRI{Prefix: network.Prefix},
					Attrs:       domainroute.BGPAttributes{OriginCode: domainroute.BGPOriginIGP, LocalPref: 100},
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
	e.installOSPFRoutes()
	e.SelectRoutes()
	e.ConvergeAdvertisementConditions()
	for _, origin := range e.idx.Topology.Nodes {
		for _, route := range e.aggregateRoutes(origin) {
			e.addRIB(origin.Name, route.NLRI.Prefix, route)
			e.walkBGP(route)
		}
	}
	e.SelectRoutes()
	e.ConvergeAdvertisementConditions()
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
			AdminDistance:   0,
		})
	}
	return out
}

func (e *Engine) installConfiguredRoute(node model.Node, route model.ConfiguredRoute) {
	if route.Prefix.IsZero() {
		return
	}
	route.Node = node.Name
	if route.NetworkInstance == "" {
		route.NetworkInstance = model.NetworkInstanceDefault
	}
	if route.AFI == "" {
		route.AFI = model.AFIIPv4
	}
	if route.AdminDistance == 0 && route.Kind != model.RouteSourceConnected {
		route.AdminDistance = 1
	}
	cond := failure.NodeVar(node.Name)
	entry := domainroute.RIBEntry{
		NLRI:              domainroute.NLRI{Prefix: route.Prefix},
		Attrs:             domainroute.BGPAttributes{OriginCode: domainroute.BGPOriginIncomplete},
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
				decision := bgp.ApplyRoutePolicy(routePolicyResolver{idx: e.idx}, node, "", redist.RouteMap, entry)
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
		Attrs:      domainroute.BGPAttributes{OriginCode: domainroute.BGPOriginIncomplete, LocalPref: 100},
		Provenance: domainroute.Provenance{OriginNode: node.Name, PathNodes: []string{node.Name}},
		SourceKind: model.RouteSourceBGP,
		RouteSource: model.ConfiguredRoute{
			Node:            node.Name,
			NetworkInstance: model.NormalizeNetworkInstance(string(route.NetworkInstance)),
			AFI:             model.AFIIPv4,
			Prefix:          route.Prefix,
			Kind:            model.RouteSourceBGP,
			Source:          route.Source,
			AdminDistance:   200,
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
		route.Node = node.Name
		if route.NetworkInstance == "" {
			route.NetworkInstance = model.NetworkInstanceDefault
		}
		if route.AFI == "" {
			route.AFI = model.AFIIPv4
		}
		if route.AdminDistance == 0 {
			route.AdminDistance = 200
		}
		cond, contributors, ok := e.aggregateContributorCondVRF(node.Name, route.NetworkInstance, route.Prefix)
		if !ok {
			continue
		}
		entry := domainroute.RIBEntry{
			NLRI:                  domainroute.NLRI{Prefix: route.Prefix},
			Attrs:                 domainroute.BGPAttributes{OriginCode: domainroute.BGPOriginIGP, LocalPref: 100},
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
	for prefix, routes := range e.rib[node][string(model.NormalizeNetworkInstance(string(vrf)))] {
		candidate, err := model.ParsePrefix(prefix)
		if err != nil || !isMoreSpecificWithin(candidate, aggregate) {
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
				n, _ := e.idx.Node(node)
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

func (e *Engine) ConvergeAdvertisementConditions() {
	maxIterations := e.MaxRouteDepth() + 1
	if maxIterations < 1 {
		maxIterations = 1
	}
	for i := 0; i < maxIterations; i++ {
		if !e.ApplyAdvertisementConditions() {
			return
		}
		e.SelectRoutes()
	}
	panic(fmt.Sprintf("advertisement conditions did not converge within %d iterations", maxIterations))
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
	for _, candidate := range e.rib[route.Provenance.FromNode][string(route.RouteSource.NetworkInstance)][route.NLRI.Prefix.String()] {
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

func (e *Engine) walkBGP(route domainroute.RIBEntry) {
	route = route.Normalize()
	current := route.Provenance.PathNodes[len(route.Provenance.PathNodes)-1]
	curNode, _ := e.idx.Node(current)
	curBehavior := BehaviorFor(curNode.Kind)
	for _, adj := range e.idx.Adj[model.NodeID(current)] {
		next := string(adj.To)
		session, ok := e.bgpSession(current, next, route.RouteSource.NetworkInstance)
		if !ok {
			continue
		}
		nextNode, _ := e.idx.Node(next)
		nextBehavior := BehaviorFor(nextNode.Kind)
		exportMsg := device.ControlMessage{From: current, To: next, Prefix: route.NLRI.Prefix.String(), Route: route}
		if !curBehavior.CheckControlEgress(curNode, exportMsg) {
			continue
		}
		routeForExport := e.applyAggregateSuppression(curNode, route)
		exported := curBehavior.ExportRoute(curNode, nextNode, session, routeForExport)
		if !exported.Accept {
			continue
		}
		exportPolicy := bgp.ApplyRoutePolicy(routePolicyResolver{idx: e.idx}, curNode, next, session.ExportPolicy, exported.Route)
		if !exportPolicy.Accept {
			continue
		}
		exported.Route = exportPolicy.Route
		importMsg := device.ControlMessage{From: current, To: next, Prefix: exported.Route.NLRI.Prefix.String(), Route: exported.Route}
		if !nextBehavior.CheckControlIngress(nextNode, importMsg) {
			continue
		}
		receiverSession, _ := e.bgpSession(next, current, route.RouteSource.NetworkInstance)
		imported := nextBehavior.ImportRoute(nextNode, curNode, receiverSession, exported.Route)
		if !imported.Accept {
			continue
		}
		importPolicy := bgp.ApplyRoutePolicy(routePolicyResolver{idx: e.idx}, nextNode, current, receiverSession.ImportPolicy, imported.Route)
		if !importPolicy.Accept {
			continue
		}
		imported.Route = importPolicy.Route
		revisitsNode := containsString(route.Provenance.PathNodes, next)
		if revisitsNode && !imported.Route.Attrs.Invalid {
			continue
		}
		nextLinks := append(append([]string(nil), imported.Route.Provenance.PathLinks...), adj.Link.Name)
		nextNodes := append(append([]string(nil), imported.Route.Provenance.PathNodes...), next)
		nextCond := failure.And(imported.Route.Condition, failure.LinkVar(adj.Link.Name), failure.NodeVar(next))

		entry := imported.Route
		entry.Provenance.FromNode = current
		entry.Provenance.PathNodes = append([]string(nil), nextNodes...)
		entry.Provenance.PathLinks = append([]string(nil), nextLinks...)
		entry.BaseCond = nextCond
		entry.Condition = nextCond
		entry.Attrs.LocalPref = bgp.DefaultLocalPref(entry.Attrs.LocalPref)
		entry = entry.Normalize()

		e.addRIB(next, entry.NLRI.Prefix, entry)
		if !nextBehavior.RouteEligibleForAdvertisement(nextNode, entry) {
			continue
		}
		e.walkBGP(entry)
	}
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
	if e.rib[node] == nil {
		e.rib[node] = map[string]map[string][]domainroute.RIBEntry{}
	}
	if e.rib[node][string(vrf)] == nil {
		e.rib[node][string(vrf)] = map[string][]domainroute.RIBEntry{}
	}
	key := prefix.String()
	for _, existing := range e.rib[node][string(vrf)][key] {
		if routeKey(existing) == routeKey(entry) {
			return
		}
	}
	e.rib[node][string(vrf)][key] = append(e.rib[node][string(vrf)][key], entry)
}

func routeKey(r domainroute.RIBEntry) string {
	r = r.Normalize()
	valid := "valid"
	if r.Attrs.Invalid {
		valid = "invalid"
	}
	return string(r.RouteSource.NetworkInstance) + "|" + r.NLRI.Prefix.String() + "|" + string(r.SourceKind) + "|" + r.RouteSource.OSPFRouteType + "|" + r.Provenance.OriginNode + "|" + r.ForwardingNextHop.Node + "|" + r.RouteSource.Interface + "|" + strings.Join(r.Provenance.PathNodes, ">") + "|" + valid
}

func containsString(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
