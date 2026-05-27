package controlplane

import (
	"container/heap"
	"math"
	"net/netip"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/failure"
	"github.com/81ueman/hoyan-lab/internal/model"
)

type ospfInterfaceState struct {
	Node    string
	Name    string
	Prefix  netip.Prefix
	Area    string
	Cost    int
	Passive bool
}

type ospfAdvertisement struct {
	Node         string
	Prefix       model.Prefix
	Cost         int
	External     bool
	ExternalArea string
	DefaultArea  string
}

type ospfPath struct {
	Cost  int
	Nodes []string
	Links []string
	Areas []string
	Cond  failure.Cond
}

type ospfAdjacency struct {
	From string
	To   string
	Link string
	Area string
	Cost int
}

type ospfSPFNode struct {
	Cost         int
	Predecessors []ospfSPFPredecessor
}

type ospfSPFPredecessor struct {
	Node string
	Link string
	Area string
}

type ospfSPFQueueItem struct {
	Node string
	Cost int
}

type ospfSPFQueue []ospfSPFQueueItem

func (e *Engine) installOSPFRoutes() {
	states := e.ospfInterfaceStates()
	advertisements := e.ospfAdvertisements(states)
	if len(advertisements) == 0 {
		return
	}
	for _, src := range e.idx.Topology.Nodes {
		if !src.OSPF.Enabled {
			continue
		}
		paths := e.ospfCandidatePaths(src.Name, states)
		for _, adv := range advertisements {
			if adv.Node == src.Name {
				if adv.External || adv.DefaultArea != "" {
					continue
				}
				e.installLocalOSPFRoute(src, adv, states[src.Name])
				continue
			}
			for _, path := range paths[adv.Node] {
				if len(path.Nodes) < 2 {
					continue
				}
				if !ospfAdvertisementAllowed(src, adv, path, e.idx.Topology) {
					continue
				}
				metric := path.Cost + adv.Cost
				nextHop := path.Nodes[1]
				nextHopAddr := ""
				if addr, ok := e.idx.PeerAddress(src.Name, nextHop); ok {
					nextHopAddr = addr.String()
				}
				cond := path.Cond
				if cond == nil {
					cond = failure.And(pathCondition(path)...)
				}
				route := model.ConfiguredRoute{
					Node:            src.Name,
					NetworkInstance: model.NetworkInstanceDefault,
					AFI:             model.AFIIPv4,
					Prefix:          adv.Prefix,
					Kind:            model.RouteSourceOSPF,
					AdminDistance:   110,
					Metric:          metric,
				}
				entry := RIBEntry{
					NLRI:              RouteNLRI{Prefix: adv.Prefix},
					Attrs:             BGPAttributes{OriginCode: BGPOriginIGP, LocalPref: 100},
					Provenance:        RouteProvenance{OriginNode: adv.Node, FromNode: nextHop, PathNodes: path.Nodes, PathLinks: path.Links},
					ForwardingNextHop: RouteNextHop{Node: nextHop, Addr: nextHopAddr},
					SourceKind:        model.RouteSourceOSPF,
					RouteSource:       route,
					BaseCond:          cond,
					Condition:         cond,
				}.Normalize()
				e.addRIB(src.Name, adv.Prefix, entry)
			}
		}
	}
}

func (e *Engine) installLocalOSPFRoute(node model.Node, adv ospfAdvertisement, states map[string]ospfInterfaceState) {
	route := model.ConfiguredRoute{
		Node:            node.Name,
		NetworkInstance: model.NetworkInstanceDefault,
		AFI:             model.AFIIPv4,
		Prefix:          adv.Prefix,
		Kind:            model.RouteSourceOSPF,
		AdminDistance:   110,
		Metric:          adv.Cost,
		Interface:       ospfInterfaceForPrefix(states, adv.Prefix),
	}
	cond := failure.NodeVar(node.Name)
	entry := RIBEntry{
		NLRI:        RouteNLRI{Prefix: adv.Prefix},
		Attrs:       BGPAttributes{OriginCode: BGPOriginIGP, LocalPref: 100},
		Provenance:  RouteProvenance{OriginNode: node.Name, PathNodes: []string{node.Name}},
		SourceKind:  model.RouteSourceOSPF,
		RouteSource: route,
		BaseCond:    cond,
		Condition:   cond,
	}.Normalize()
	e.addRIB(node.Name, adv.Prefix, entry)
}

func ospfInterfaceForPrefix(states map[string]ospfInterfaceState, prefix model.Prefix) string {
	for _, state := range states {
		if model.PrefixFromNetIP(state.Prefix).Equal(prefix) {
			return state.Name
		}
	}
	return ""
}

func (e *Engine) ospfInterfaceStates() map[string]map[string]ospfInterfaceState {
	out := map[string]map[string]ospfInterfaceState{}
	for _, node := range e.idx.Topology.Nodes {
		if !node.OSPF.Enabled {
			continue
		}
		for _, iface := range node.Interfaces {
			pfx, err := netip.ParsePrefix(iface.Address)
			if err != nil || !pfx.Addr().Is4() {
				continue
			}
			ifState, ok := ospfInterfaceFor(node, iface, pfx)
			if !ok {
				continue
			}
			if out[node.Name] == nil {
				out[node.Name] = map[string]ospfInterfaceState{}
			}
			out[node.Name][iface.Name] = ifState
		}
	}
	return out
}

func ospfInterfaceFor(node model.Node, iface model.Interface, pfx netip.Prefix) (ospfInterfaceState, bool) {
	var state ospfInterfaceState
	state = ospfInterfaceState{Node: node.Name, Name: iface.Name, Prefix: pfx.Masked(), Cost: 1}
	for _, configured := range node.OSPF.Interfaces {
		if !model.EquivalentInterfaceName(node.Kind, configured.Name, iface.Name) {
			continue
		}
		state.Area = configured.Area
		if configured.Cost > 0 {
			state.Cost = configured.Cost
		}
		state.Passive = configured.Passive
	}
	if state.Area == "" {
		for _, network := range node.OSPF.Networks {
			if network.Prefix.Contains(pfx.Addr()) {
				state.Area = network.Area
				break
			}
		}
	}
	for _, passive := range node.OSPF.PassiveInterfaces {
		if model.EquivalentInterfaceName(node.Kind, passive, iface.Name) {
			state.Passive = true
		}
	}
	if state.Area == "" {
		return ospfInterfaceState{}, false
	}
	if isOSPFLoopbackInterface(iface.Name) {
		state.Cost = 0
	}
	return state, true
}

func isOSPFLoopbackInterface(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return name == "lo" || strings.HasPrefix(name, "lo") || strings.HasPrefix(name, "loopback")
}

func (e *Engine) ospfAdvertisements(states map[string]map[string]ospfInterfaceState) []ospfAdvertisement {
	var out []ospfAdvertisement
	seen := map[string]bool{}
	for node, byIface := range states {
		for _, state := range byIface {
			prefix := model.PrefixFromNetIP(state.Prefix)
			key := node + "|" + prefix.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, ospfAdvertisement{Node: node, Prefix: prefix, Cost: state.Cost})
		}
	}
	for _, node := range e.idx.Topology.Nodes {
		if !node.OSPF.Enabled {
			continue
		}
		for _, route := range ospfRedistributedRoutes(node, e.connectedRoutes(node)) {
			area := ospfExternalArea(node, states[node.Name])
			out = append(out, ospfAdvertisement{Node: node.Name, Prefix: route.Prefix, Cost: route.Metric, External: true, ExternalArea: area})
		}
		for _, area := range node.OSPF.Areas {
			if area.Kind == model.OSPFAreaStub && !ospfNodeAttachedToOtherArea(states[node.Name], area.ID) {
				continue
			}
			if area.Kind != model.OSPFAreaStub && !(area.Kind == model.OSPFAreaNSSA && area.DefaultInformationOriginate) {
				continue
			}
			if !ospfNodeAttachedToArea(states[node.Name], area.ID) {
				continue
			}
			out = append(out, ospfAdvertisement{Node: node.Name, Prefix: model.MustPrefix("0.0.0.0/0"), Cost: 1, DefaultArea: area.ID})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Node == out[j].Node {
			return out[i].Prefix.String() < out[j].Prefix.String()
		}
		return out[i].Node < out[j].Node
	})
	return out
}

func ospfRedistributedRoutes(node model.Node, connected []model.ConfiguredRoute) []model.ConfiguredRoute {
	enabled := map[model.RouteSourceKind]bool{}
	for _, redist := range node.OSPF.Redistribute {
		enabled[redist.Kind] = true
	}
	if len(enabled) == 0 {
		return nil
	}
	var out []model.ConfiguredRoute
	if enabled[model.RouteSourceConnected] {
		for _, route := range connected {
			if route.ConnectedClass == model.ConnectedRouteClassLink {
				continue
			}
			route.Metric = 20
			out = append(out, route)
		}
	}
	if enabled[model.RouteSourceStatic] {
		for _, route := range node.Routes {
			if route.Kind != model.RouteSourceStatic && route.Kind != model.RouteSourceBlackhole {
				continue
			}
			if route.Metric == 0 {
				route.Metric = 20
			}
			out = append(out, route)
		}
	}
	return out
}

func ospfExternalArea(node model.Node, states map[string]ospfInterfaceState) string {
	for _, state := range states {
		if area := node.OSPF.Areas[state.Area]; area.Kind == model.OSPFAreaNSSA {
			return state.Area
		}
	}
	return ""
}

func ospfNodeAttachedToArea(states map[string]ospfInterfaceState, area string) bool {
	for _, state := range states {
		if state.Area == area {
			return true
		}
	}
	return false
}

func ospfNodeAttachedToOtherArea(states map[string]ospfInterfaceState, area string) bool {
	for _, state := range states {
		if state.Area != "" && state.Area != area {
			return true
		}
	}
	return false
}

func ospfAdvertisementAllowed(src model.Node, adv ospfAdvertisement, path ospfPath, topo *model.Topology) bool {
	if adv.DefaultArea != "" {
		return pathUsesOnlyArea(path, adv.DefaultArea)
	}
	if !adv.External {
		return true
	}
	for _, areaID := range path.Areas {
		area := ospfAreaForPathArea(topo, path, areaID)
		switch area.Kind {
		case model.OSPFAreaStub:
			return false
		case model.OSPFAreaNSSA:
			if adv.ExternalArea != areaID {
				return false
			}
		}
	}
	if adv.ExternalArea != "" {
		return true
	}
	return !ospfNodeInStubOrNSSA(src)
}

func pathUsesOnlyArea(path ospfPath, area string) bool {
	if len(path.Areas) == 0 {
		return false
	}
	for _, pathArea := range path.Areas {
		if pathArea != area {
			return false
		}
	}
	return true
}

func ospfNodeInStubOrNSSA(node model.Node) bool {
	for _, area := range node.OSPF.Areas {
		if area.Kind == model.OSPFAreaStub || area.Kind == model.OSPFAreaNSSA {
			return true
		}
	}
	return false
}

func ospfAreaForPathArea(topo *model.Topology, path ospfPath, areaID string) model.OSPFArea {
	for _, nodeName := range path.Nodes {
		node, ok := topo.Node(nodeName)
		if !ok {
			continue
		}
		area := node.OSPF.Areas[areaID]
		if area.Kind != "" {
			return area
		}
	}
	return model.OSPFArea{ID: areaID, Kind: model.OSPFAreaNormal}
}

func (e *Engine) ospfCandidatePaths(src string, states map[string]map[string]ospfInterfaceState) map[string][]ospfPath {
	out := map[string][]ospfPath{}
	for _, firstHop := range e.ospfAdjacencies(src, states) {
		spf := e.ospfShortestPathTree(firstHop.To, src, states)
		condMemo := map[string]failure.Cond{}
		for dst, state := range spf {
			if dst == firstHop.To {
				path := ospfPath{
					Cost:  firstHop.Cost,
					Nodes: []string{src, firstHop.To},
					Links: []string{firstHop.Link},
					Areas: []string{firstHop.Area},
					Cond:  failure.And(failure.NodeVar(src), failure.LinkVar(firstHop.Link), failure.NodeVar(firstHop.To)),
				}
				out[dst] = append(out[dst], path)
				continue
			}
			if state.Cost == math.MaxInt {
				continue
			}
			nodes, links, areas, ok := ospfRepresentativePath(firstHop.To, dst, spf)
			if !ok {
				continue
			}
			path := ospfPath{
				Cost:  firstHop.Cost + state.Cost,
				Nodes: append([]string{src}, nodes...),
				Links: append([]string{firstHop.Link}, links...),
				Areas: append([]string{firstHop.Area}, areas...),
				Cond:  failure.And(failure.NodeVar(src), failure.LinkVar(firstHop.Link), ospfSPFCondition(firstHop.To, dst, spf, condMemo)),
			}
			out[dst] = append(out[dst], path)
		}
	}
	for node, paths := range out {
		sort.Slice(paths, func(i, j int) bool {
			if paths[i].Cost != paths[j].Cost {
				return paths[i].Cost < paths[j].Cost
			}
			return strings.Join(paths[i].Nodes, ",") < strings.Join(paths[j].Nodes, ",")
		})
		out[node] = paths
	}
	return out
}

func ospfAdjacencyArea(idx *model.TopologyIndex, from, to string, link model.Link, states map[string]map[string]ospfInterfaceState) string {
	fromRef, ok := idx.InterfaceOnLink(from, link.Name)
	if !ok {
		return ""
	}
	toRef, ok := idx.InterfaceOnLink(to, link.Name)
	if !ok {
		return ""
	}
	fromState, ok := states[from][fromRef.ConfigName]
	if !ok {
		return ""
	}
	toState, ok := states[to][toRef.ConfigName]
	if !ok || fromState.Area != toState.Area {
		return ""
	}
	return fromState.Area
}

func (e *Engine) ospfShortestPathTree(src, excluded string, states map[string]map[string]ospfInterfaceState) map[string]ospfSPFNode {
	dist := map[string]ospfSPFNode{}
	for _, node := range e.idx.Topology.Nodes {
		if !node.OSPF.Enabled || node.Name == excluded {
			continue
		}
		dist[node.Name] = ospfSPFNode{Cost: math.MaxInt}
	}
	if _, ok := dist[src]; !ok {
		return dist
	}
	dist[src] = ospfSPFNode{Cost: 0}
	q := &ospfSPFQueue{{Node: src}}
	heap.Init(q)
	for q.Len() > 0 {
		item := heap.Pop(q).(ospfSPFQueueItem)
		current := dist[item.Node]
		if item.Cost != current.Cost {
			continue
		}
		for _, adj := range e.ospfAdjacencies(item.Node, states) {
			if adj.To == excluded {
				continue
			}
			next, ok := dist[adj.To]
			if !ok {
				continue
			}
			cost := item.Cost + adj.Cost
			pred := ospfSPFPredecessor{Node: item.Node, Link: adj.Link, Area: adj.Area}
			switch {
			case cost < next.Cost:
				next.Cost = cost
				next.Predecessors = []ospfSPFPredecessor{pred}
				dist[adj.To] = next
				heap.Push(q, ospfSPFQueueItem{Node: adj.To, Cost: cost})
			case cost == next.Cost:
				next.Predecessors = append(next.Predecessors, pred)
				sort.Slice(next.Predecessors, func(i, j int) bool {
					if next.Predecessors[i].Node == next.Predecessors[j].Node {
						return next.Predecessors[i].Link < next.Predecessors[j].Link
					}
					return next.Predecessors[i].Node < next.Predecessors[j].Node
				})
				dist[adj.To] = next
			}
		}
	}
	return dist
}

func (e *Engine) ospfAdjacencies(from string, states map[string]map[string]ospfInterfaceState) []ospfAdjacency {
	var out []ospfAdjacency
	for _, edge := range e.idx.Adj[model.NodeID(from)] {
		to := string(edge.To)
		cost, ok := ospfAdjacencyCost(e.idx, from, to, edge.Link, states)
		if !ok {
			continue
		}
		out = append(out, ospfAdjacency{From: from, To: to, Link: edge.Link.Name, Area: ospfAdjacencyArea(e.idx, from, to, edge.Link, states), Cost: cost})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].To == out[j].To {
			return out[i].Link < out[j].Link
		}
		return out[i].To < out[j].To
	})
	return out
}

func ospfRepresentativePath(src, dst string, spf map[string]ospfSPFNode) ([]string, []string, []string, bool) {
	if src == dst {
		return []string{src}, nil, nil, true
	}
	state, ok := spf[dst]
	if !ok || state.Cost == math.MaxInt || len(state.Predecessors) == 0 {
		return nil, nil, nil, false
	}
	pred := state.Predecessors[0]
	nodes, links, areas, ok := ospfRepresentativePath(src, pred.Node, spf)
	if !ok {
		return nil, nil, nil, false
	}
	return append(nodes, dst), append(links, pred.Link), append(areas, pred.Area), true
}

func ospfSPFCondition(src, dst string, spf map[string]ospfSPFNode, memo map[string]failure.Cond) failure.Cond {
	if cond, ok := memo[dst]; ok {
		return cond
	}
	if src == dst {
		cond := failure.NodeVar(src)
		memo[dst] = cond
		return cond
	}
	state, ok := spf[dst]
	if !ok || state.Cost == math.MaxInt || len(state.Predecessors) == 0 {
		return failure.False()
	}
	branches := make([]failure.Cond, 0, len(state.Predecessors))
	for _, pred := range state.Predecessors {
		branches = append(branches, failure.And(ospfSPFCondition(src, pred.Node, spf, memo), failure.LinkVar(pred.Link), failure.NodeVar(dst)))
	}
	cond := failure.Or(branches...)
	memo[dst] = cond
	return cond
}

func ospfAdjacencyCost(idx *model.TopologyIndex, from, to string, link model.Link, states map[string]map[string]ospfInterfaceState) (int, bool) {
	fromRef, ok := idx.InterfaceOnLink(from, link.Name)
	if !ok {
		return 0, false
	}
	toRef, ok := idx.InterfaceOnLink(to, link.Name)
	if !ok {
		return 0, false
	}
	fromState, ok := states[from][fromRef.ConfigName]
	if !ok || fromState.Passive {
		return 0, false
	}
	toState, ok := states[to][toRef.ConfigName]
	if !ok || toState.Passive {
		return 0, false
	}
	if fromState.Area != toState.Area {
		return 0, false
	}
	return fromState.Cost, true
}

func pathCondition(path ospfPath) []failure.Cond {
	conds := make([]failure.Cond, 0, len(path.Nodes)+len(path.Links))
	for _, node := range path.Nodes {
		conds = append(conds, failure.NodeVar(node))
	}
	for _, link := range path.Links {
		conds = append(conds, failure.LinkVar(link))
	}
	return conds
}

func (q ospfSPFQueue) Len() int { return len(q) }

func (q ospfSPFQueue) Less(i, j int) bool {
	if q[i].Cost == q[j].Cost {
		return q[i].Node < q[j].Node
	}
	return q[i].Cost < q[j].Cost
}

func (q ospfSPFQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

func (q *ospfSPFQueue) Push(x any) {
	*q = append(*q, x.(ospfSPFQueueItem))
}

func (q *ospfSPFQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[:n-1]
	return item
}
