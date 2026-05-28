package controlplane

import (
	"github.com/81ueman/hoyan-lab/internal/core/netaddr"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/config/routing"
	"github.com/81ueman/hoyan-lab/internal/core/failure"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

type RIBEntry struct {
	NLRI                  RouteNLRI
	Attrs                 BGPAttributes
	Provenance            RouteProvenance
	ForwardingNextHop     RouteNextHop
	SourceKind            topology.RouteSourceKind
	RouteSource           topology.ConfiguredRoute
	AggregateContributors []string
	BaseCond              failure.Cond
	Condition             failure.Cond
	SelectedCond          failure.Cond
}

type RouteNLRI struct {
	Prefix netaddr.Prefix
}

type BGPOriginCode string

const (
	BGPOriginIGP        BGPOriginCode = "igp"
	BGPOriginEGP        BGPOriginCode = "egp"
	BGPOriginIncomplete BGPOriginCode = "incomplete"
)

type BGPAttributes struct {
	ASPath      []uint32
	Communities []string
	OriginCode  BGPOriginCode
	LocalPref   int
	MED         int
	LearnedIBGP bool
	Invalid     bool
}

type RouteProvenance struct {
	OriginNode string
	FromNode   string
	PathNodes  []string
	PathLinks  []string
}

// RouteNextHop keeps simulated forwarding next-hop node identity separate from
// the resolved address used when comparing against live device RIB output.
type RouteNextHop struct {
	Node string
	Addr string
}

func (h RouteNextHop) Valid() bool {
	return h.Node != "" || h.Addr != ""
}

func (r RIBEntry) Normalize() RIBEntry {
	if r.Attrs.OriginCode == "" {
		r.Attrs.OriginCode = BGPOriginIGP
	}
	if r.SourceKind == "" {
		r.SourceKind = topology.RouteSourceBGP
	}
	if r.RouteSource.Kind == "" {
		r.RouteSource.Kind = r.SourceKind
	}
	if r.RouteSource.NetworkInstance == "" {
		r.RouteSource.NetworkInstance = topology.NetworkInstanceDefault
	}
	if r.RouteSource.Prefix.IsZero() {
		r.RouteSource.Prefix = r.NLRI.Prefix
	}
	return r
}

type Engine struct {
	idx     *topology.TopologyIndex
	routing routing.TopologyRouting
	rib     map[string]map[string]map[string][]RIBEntry
}

func NewEngine(idx *topology.TopologyIndex, rib map[string]map[string]map[string][]RIBEntry) *Engine {
	var topo *topology.Topology
	if idx != nil {
		topo = idx.Topology
	}
	return NewEngineWithRouting(idx, routing.FromTopology(topo), rib)
}

func NewEngineWithRouting(idx *topology.TopologyIndex, routes routing.TopologyRouting, rib map[string]map[string]map[string][]RIBEntry) *Engine {
	if rib == nil {
		rib = map[string]map[string]map[string][]RIBEntry{}
	}
	if routes.Nodes == nil {
		routes.Nodes = map[string]routing.NodeRouting{}
	}
	return &Engine{idx: idx, routing: routes, rib: rib}
}

func (e *Engine) nodeRouting(node string) routing.NodeRouting {
	return e.routing.ForNode(node)
}

func (e *Engine) nodeWithRouting(node topology.Node) topology.Node {
	nodeRouting := e.nodeRouting(node.Name)
	node.ASN = nodeRouting.ASN
	node.ConfigPath = nodeRouting.ConfigPath
	node.Routes = append([]topology.ConfiguredRoute(nil), nodeRouting.Routes...)
	node.Neighbors = append([]topology.BGPNeighbor(nil), nodeRouting.Neighbors...)
	node.Redistribute = append([]topology.BGPRedistribution(nil), nodeRouting.Redistribute...)
	node.OSPF = nodeRouting.OSPF
	node.OSPFProcesses = append([]topology.OSPFProcess(nil), nodeRouting.OSPFProcesses...)
	node.PrefixLists = append([]topology.PrefixList(nil), nodeRouting.PrefixLists...)
	node.ASPathLists = append([]topology.ASPathList(nil), nodeRouting.ASPathLists...)
	node.CommunityLists = append([]topology.CommunityList(nil), nodeRouting.CommunityLists...)
	node.RoutePolicies = append([]topology.RoutePolicy(nil), nodeRouting.RoutePolicies...)
	return node
}

func (e *Engine) Simulate() {
	for _, origin := range e.idx.Topology.Nodes {
		originRouting := e.nodeRouting(origin.Name)
		for _, route := range e.connectedRoutes(origin) {
			e.installConfiguredRoute(origin, route)
		}
		for _, route := range originRouting.Routes {
			if route.Kind == topology.RouteSourceAggregate || route.Kind == topology.RouteSourceBGP {
				continue
			}
			e.installConfiguredRoute(origin, route)
		}
		if bgpEnabled(originRouting) {
			for _, prefix := range origin.Prefixes {
				originCond := failure.NodeVar(origin.Name)
				route := RIBEntry{
					NLRI:        RouteNLRI{Prefix: prefix},
					Attrs:       BGPAttributes{OriginCode: BGPOriginIGP, LocalPref: 100},
					Provenance:  RouteProvenance{OriginNode: origin.Name, PathNodes: []string{origin.Name}},
					SourceKind:  topology.RouteSourceBGP,
					RouteSource: topology.ConfiguredRoute{Node: origin.Name, NetworkInstance: topology.NetworkInstanceDefault, AFI: topology.AFIIPv4, Prefix: prefix, Kind: topology.RouteSourceBGP, AdminDistance: 200},
					BaseCond:    originCond,
					Condition:   originCond,
				}
				e.addRIB(origin.Name, prefix, route)
				e.walkBGP(route)
			}
			for _, network := range originRouting.Routes {
				if network.Kind != topology.RouteSourceBGP || network.Prefix.IsZero() {
					continue
				}
				network.Node = origin.Name
				if network.NetworkInstance == "" {
					network.NetworkInstance = topology.NetworkInstanceDefault
				}
				if network.AFI == "" {
					network.AFI = topology.AFIIPv4
				}
				originCond := failure.NodeVar(origin.Name)
				route := RIBEntry{
					NLRI:        RouteNLRI{Prefix: network.Prefix},
					Attrs:       BGPAttributes{OriginCode: BGPOriginIGP, LocalPref: 100},
					Provenance:  RouteProvenance{OriginNode: origin.Name, PathNodes: []string{origin.Name}},
					SourceKind:  topology.RouteSourceBGP,
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

func bgpEnabled(r routing.NodeRouting) bool {
	return r.ASN != 0 || len(r.Neighbors) > 0 || len(r.Redistribute) > 0
}

func (e *Engine) connectedRoutes(node topology.Node) []topology.ConfiguredRoute {
	var out []topology.ConfiguredRoute
	for _, iface := range node.Interfaces {
		pfx, err := netip.ParsePrefix(iface.Address)
		if err != nil {
			continue
		}
		prefix := netaddr.PrefixFromNetIP(pfx.Masked())
		out = append(out, topology.ConfiguredRoute{
			Node:            node.Name,
			NetworkInstance: topology.NormalizeNetworkInstance(string(iface.VRF)),
			AFI:             topology.AFIIPv4,
			Prefix:          prefix,
			Interface:       iface.Name,
			Kind:            topology.RouteSourceConnected,
			ConnectedClass:  e.idx.ConnectedClass(node.Name, iface, prefix),
			AdminDistance:   0,
		})
	}
	return out
}

func (e *Engine) installConfiguredRoute(node topology.Node, route topology.ConfiguredRoute) {
	if route.Prefix.IsZero() {
		return
	}
	route.Node = node.Name
	if route.NetworkInstance == "" {
		route.NetworkInstance = topology.NetworkInstanceDefault
	}
	if route.AFI == "" {
		route.AFI = topology.AFIIPv4
	}
	if route.AdminDistance == 0 && route.Kind != topology.RouteSourceConnected {
		route.AdminDistance = 1
	}
	cond := failure.NodeVar(node.Name)
	entry := RIBEntry{
		NLRI:              RouteNLRI{Prefix: route.Prefix},
		Attrs:             BGPAttributes{OriginCode: BGPOriginIncomplete},
		Provenance:        RouteProvenance{OriginNode: node.Name, PathNodes: []string{node.Name}},
		ForwardingNextHop: e.configuredRouteNextHop(node.Name, route),
		SourceKind:        route.Kind,
		RouteSource:       route,
		BaseCond:          cond,
		Condition:         cond,
	}
	e.addRIB(node.Name, route.Prefix, entry)
}

func (e *Engine) redistributedRoutes(node topology.Node) []RIBEntry {
	var out []RIBEntry
	nodeRouting := e.nodeRouting(node.Name)
	for _, redist := range nodeRouting.Redistribute {
		for _, route := range e.redistributionCandidates(node, redist.Kind) {
			redistVRF := topology.NormalizeNetworkInstance(string(redist.NetworkInstance))
			routeVRF := topology.NormalizeNetworkInstance(string(route.NetworkInstance))
			if routeVRF != redistVRF {
				continue
			}
			entry := e.bgpRouteFromConfiguredRoute(node, route)
			if redist.RouteMap != "" {
				decision := e.applyRoutePolicy(node, "", redist.RouteMap, entry)
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

func (e *Engine) redistributionCandidates(node topology.Node, kind topology.RouteSourceKind) []topology.ConfiguredRoute {
	var out []topology.ConfiguredRoute
	nodeRouting := e.nodeRouting(node.Name)
	if kind == topology.RouteSourceConnected {
		out = append(out, e.connectedRoutes(node)...)
	}
	if kind == topology.RouteSourceStatic {
		for _, route := range nodeRouting.Routes {
			if route.Kind == topology.RouteSourceStatic || route.Kind == topology.RouteSourceBlackhole {
				out = append(out, route)
			}
		}
	}
	return out
}

func (e *Engine) bgpRouteFromConfiguredRoute(node topology.Node, route topology.ConfiguredRoute) RIBEntry {
	cond := failure.NodeVar(node.Name)
	entry := RIBEntry{
		NLRI:       RouteNLRI{Prefix: route.Prefix},
		Attrs:      BGPAttributes{OriginCode: BGPOriginIncomplete, LocalPref: 100},
		Provenance: RouteProvenance{OriginNode: node.Name, PathNodes: []string{node.Name}},
		SourceKind: topology.RouteSourceBGP,
		RouteSource: topology.ConfiguredRoute{
			Node:            node.Name,
			NetworkInstance: topology.NormalizeNetworkInstance(string(route.NetworkInstance)),
			AFI:             topology.AFIIPv4,
			Prefix:          route.Prefix,
			Kind:            topology.RouteSourceBGP,
			Source:          route.Source,
			AdminDistance:   200,
		},
		BaseCond:  cond,
		Condition: cond,
	}
	return entry.Normalize()
}

func (e *Engine) aggregateRoutes(node topology.Node) []RIBEntry {
	var out []RIBEntry
	for _, route := range e.nodeRouting(node.Name).Routes {
		if route.Kind != topology.RouteSourceAggregate || route.Prefix.IsZero() {
			continue
		}
		route.Node = node.Name
		if route.NetworkInstance == "" {
			route.NetworkInstance = topology.NetworkInstanceDefault
		}
		if route.AFI == "" {
			route.AFI = topology.AFIIPv4
		}
		if route.AdminDistance == 0 {
			route.AdminDistance = 200
		}
		cond, contributors, ok := e.aggregateContributorCondVRF(node.Name, route.NetworkInstance, route.Prefix)
		if !ok {
			continue
		}
		entry := RIBEntry{
			NLRI:                  RouteNLRI{Prefix: route.Prefix},
			Attrs:                 BGPAttributes{OriginCode: BGPOriginIGP, LocalPref: 100},
			Provenance:            RouteProvenance{OriginNode: node.Name, PathNodes: []string{node.Name}},
			SourceKind:            topology.RouteSourceAggregate,
			RouteSource:           route,
			AggregateContributors: contributors,
			BaseCond:              cond,
			Condition:             cond,
		}
		out = append(out, entry.Normalize())
	}
	return out
}

func (e *Engine) aggregateContributorCond(node string, aggregate netaddr.Prefix) (failure.Cond, []string, bool) {
	return e.aggregateContributorCondVRF(node, topology.NetworkInstanceDefault, aggregate)
}

func (e *Engine) aggregateContributorCondVRF(node string, vrf topology.NetworkInstanceID, aggregate netaddr.Prefix) (failure.Cond, []string, bool) {
	var contributors []failure.Cond
	contributorPrefixes := map[string]bool{}
	for prefix, routes := range e.rib[node][string(topology.NormalizeNetworkInstance(string(vrf)))] {
		candidate, err := netaddr.ParsePrefix(prefix)
		if err != nil || !isMoreSpecificWithin(candidate, aggregate) {
			continue
		}
		for _, route := range routes {
			route = route.Normalize()
			if route.SourceKind == topology.RouteSourceAggregate {
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

func isMoreSpecificWithin(candidate, aggregate netaddr.Prefix) bool {
	if candidate.IsZero() || aggregate.IsZero() {
		return false
	}
	return candidate.Bits() > aggregate.Bits() && aggregate.Contains(candidate.Addr())
}

func (e *Engine) configuredRouteNextHop(node string, route topology.ConfiguredRoute) RouteNextHop {
	if route.NextHop == "" {
		return RouteNextHop{}
	}
	wantVRF := topology.NormalizeNetworkInstance(string(route.NetworkInstance))
	localNode, _ := e.idx.Node(node)
	for _, adj := range e.idx.Adj[topology.NodeID(node)] {
		iface, ok := e.idx.InterfaceToPeer(node, string(adj.To))
		if !ok || interfaceVRF(localNode, iface.ConfigName) != wantVRF {
			continue
		}
		if addr, ok := e.idx.PeerAddress(node, string(adj.To)); ok && addr.String() == route.NextHop {
			return RouteNextHop{Node: string(adj.To), Addr: route.NextHop}
		}
	}
	return RouteNextHop{Addr: route.NextHop}
}

func interfaceVRF(node topology.Node, name string) topology.NetworkInstanceID {
	for _, iface := range node.Interfaces {
		if topology.EquivalentInterfaceName(node.Kind, iface.Name, name) {
			return topology.NormalizeNetworkInstance(string(iface.VRF))
		}
	}
	return topology.NetworkInstanceDefault
}

func (e *Engine) SelectRoutes() {
	for node, byVRF := range e.rib {
		for vrf, byPrefix := range byVRF {
			for prefix, routes := range byPrefix {
				n, _ := e.idx.Node(node)
				n = e.nodeWithRouting(n)
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

func routeSelectionFamily(route RIBEntry) topology.RouteSourceKind {
	route = route.Normalize()
	if route.SourceKind == topology.RouteSourceBGP || route.SourceKind == topology.RouteSourceAggregate {
		return topology.RouteSourceBGP
	}
	if route.SourceKind == topology.RouteSourceOSPF {
		return topology.RouteSourceOSPF
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
					if routes[i].Normalize().SourceKind == topology.RouteSourceOSPF {
						routes[i].Condition = nextCond
						continue
					}
					if len(routes[i].Provenance.PathNodes) > 1 {
						if parent, ok := e.ParentRoute(routes[i]); ok {
							parentSelected := parent.SelectedCond
							if len(parent.Normalize().Provenance.PathNodes) == 1 && (parent.SourceKind == topology.RouteSourceBGP || parent.SourceKind == topology.RouteSourceAggregate) {
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

func (e *Engine) ParentRoute(route RIBEntry) (RIBEntry, bool) {
	route = route.Normalize()
	if route.Provenance.FromNode == "" || len(route.Provenance.PathNodes) < 2 {
		return RIBEntry{}, false
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
	return RIBEntry{}, false
}

func (e *Engine) walkBGP(route RIBEntry) {
	route = route.Normalize()
	current := route.Provenance.PathNodes[len(route.Provenance.PathNodes)-1]
	curNode, _ := e.idx.Node(current)
	curNode = e.nodeWithRouting(curNode)
	curBehavior := BehaviorFor(curNode.Kind)
	for _, adj := range e.idx.Adj[topology.NodeID(current)] {
		next := string(adj.To)
		session, ok := e.bgpSession(current, next, route.RouteSource.NetworkInstance)
		if !ok {
			continue
		}
		nextNode, _ := e.idx.Node(next)
		nextNode = e.nodeWithRouting(nextNode)
		nextBehavior := BehaviorFor(nextNode.Kind)
		exportMsg := ControlMessage{From: current, To: next, Prefix: route.NLRI.Prefix.String(), Route: route}
		if !curBehavior.CheckControlEgress(curNode, exportMsg) {
			continue
		}
		routeForExport := e.applyAggregateSuppression(curNode, route)
		exported := curBehavior.ExportRoute(curNode, nextNode, session, routeForExport)
		if !exported.Accept {
			continue
		}
		exportPolicy := e.applyRoutePolicy(curNode, next, session.ExportPolicy, exported.Route)
		if !exportPolicy.Accept {
			continue
		}
		exported.Route = exportPolicy.Route
		importMsg := ControlMessage{From: current, To: next, Prefix: exported.Route.NLRI.Prefix.String(), Route: exported.Route}
		if !nextBehavior.CheckControlIngress(nextNode, importMsg) {
			continue
		}
		receiverSession, _ := e.bgpSession(next, current, route.RouteSource.NetworkInstance)
		imported := nextBehavior.ImportRoute(nextNode, curNode, receiverSession, exported.Route)
		if !imported.Accept {
			continue
		}
		importPolicy := e.applyRoutePolicy(nextNode, current, receiverSession.ImportPolicy, imported.Route)
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
		entry.Attrs.LocalPref = defaultLocalPref(entry.Attrs.LocalPref)
		entry = entry.Normalize()

		e.addRIB(next, entry.NLRI.Prefix, entry)
		if !nextBehavior.RouteEligibleForAdvertisement(nextNode, entry) {
			continue
		}
		e.walkBGP(entry)
	}
}

func (e *Engine) applyAggregateSuppression(node topology.Node, route RIBEntry) RIBEntry {
	route = route.Normalize()
	for _, aggregate := range e.nodeRouting(node.Name).Routes {
		if aggregate.Kind != topology.RouteSourceAggregate || !aggregate.SummaryOnly {
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

func (e *Engine) bgpSession(a, b string, vrf topology.NetworkInstanceID) (topology.BGPNeighbor, bool) {
	vrf = topology.NormalizeNetworkInstance(string(vrf))
	for _, peer := range e.nodeRouting(a).Neighbors {
		if topology.NormalizeNetworkInstance(string(peer.NetworkInstance)) != vrf {
			continue
		}
		if peer.PeerNode == b && (peer.Activated || peer.RemoteAS != 0) {
			return peer, true
		}
	}
	return topology.BGPNeighbor{}, false
}

func (e *Engine) addRIB(node string, prefix netaddr.Prefix, entry RIBEntry) {
	entry = entry.Normalize()
	if prefix.IsZero() {
		prefix = entry.NLRI.Prefix
	}
	vrf := topology.NormalizeNetworkInstance(string(entry.RouteSource.NetworkInstance))
	entry.RouteSource.NetworkInstance = vrf
	if e.rib[node] == nil {
		e.rib[node] = map[string]map[string][]RIBEntry{}
	}
	if e.rib[node][string(vrf)] == nil {
		e.rib[node][string(vrf)] = map[string][]RIBEntry{}
	}
	key := prefix.String()
	for _, existing := range e.rib[node][string(vrf)][key] {
		if routeKey(existing) == routeKey(entry) {
			return
		}
	}
	e.rib[node][string(vrf)][key] = append(e.rib[node][string(vrf)][key], entry)
}

func EquivalentInstalledRoute(decision BGPDecisionProcess, node topology.Node, installed []RIBEntry, route RIBEntry) bool {
	for _, existing := range installed {
		if decision.Equivalent(node, existing, route) {
			return true
		}
	}
	return false
}

func routeKey(r RIBEntry) string {
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

func containsASN(xs []uint32, x uint32) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func prependASN(asn uint32, path []uint32) []uint32 {
	out := make([]uint32, 0, len(path)+1)
	out = append(out, asn)
	out = append(out, path...)
	return out
}
