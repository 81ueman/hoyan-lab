package controlplane

import (
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
	Node   string
	Prefix model.Prefix
	Cost   int
	Area   string
}

type ospfPath struct {
	Cost  int
	Nodes []string
	Links []string
}

const ospfMaxPathsPerDestination = 8
const (
	ospfRouteTypeIntraArea = "intra-area"
	ospfRouteTypeInterArea = "inter-area"
	ospfBackboneArea       = "0"
)

func (e *Engine) installOSPFRoutes() {
	states := e.ospfInterfaceStates()
	advertisements := e.ospfAdvertisements(states)
	if len(advertisements) == 0 {
		return
	}
	areas := ospfNodeAreas(states)
	abrs := ospfABRs(areas)
	for _, src := range e.idx.Topology.Nodes {
		if !src.OSPF.Enabled {
			continue
		}
		areaPaths := map[string]map[string][]ospfPath{}
		for area := range areas[src.Name] {
			areaPaths[area] = e.ospfCandidatePaths(src.Name, area, states)
		}
		for _, adv := range advertisements {
			if adv.Node == src.Name {
				e.installLocalOSPFRoute(src, adv, states[src.Name])
				continue
			}
			for _, path := range areaPaths[adv.Area][adv.Node] {
				e.installRemoteOSPFRoute(src.Name, adv, path, ospfRouteTypeIntraArea)
			}
			for _, path := range e.ospfInterAreaPaths(src.Name, adv, states, areas, abrs) {
				e.installRemoteOSPFRoute(src.Name, adv, path, ospfRouteTypeInterArea)
			}
		}
	}
}

func (e *Engine) installRemoteOSPFRoute(src string, adv ospfAdvertisement, path ospfPath, routeType string) {
	if len(path.Nodes) < 2 {
		return
	}
	metric := path.Cost + adv.Cost
	nextHop := path.Nodes[1]
	nextHopAddr := ""
	if addr, ok := e.idx.PeerAddress(src, nextHop); ok {
		nextHopAddr = addr.String()
	}
	cond := failure.And(pathCondition(path)...)
	route := model.ConfiguredRoute{
		Node:            src,
		NetworkInstance: model.NetworkInstanceDefault,
		AFI:             model.AFIIPv4,
		Prefix:          adv.Prefix,
		Kind:            model.RouteSourceOSPF,
		AdminDistance:   110,
		Metric:          metric,
		OSPFRouteType:   routeType,
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
	e.addRIB(src, adv.Prefix, entry)
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
		OSPFRouteType:   ospfRouteTypeIntraArea,
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
		state.Area = normalizeOSPFArea(configured.Area)
		if configured.Cost > 0 {
			state.Cost = configured.Cost
		}
		state.Passive = configured.Passive
	}
	if state.Area == "" {
		for _, network := range node.OSPF.Networks {
			if network.Prefix.Contains(pfx.Addr()) {
				state.Area = normalizeOSPFArea(network.Area)
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

func normalizeOSPFArea(area string) string {
	area = strings.TrimSpace(area)
	if area == "0.0.0.0" {
		return ospfBackboneArea
	}
	return area
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
			out = append(out, ospfAdvertisement{Node: node, Prefix: prefix, Cost: state.Cost, Area: state.Area})
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

func ospfNodeAreas(states map[string]map[string]ospfInterfaceState) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for node, byIface := range states {
		for _, state := range byIface {
			if state.Area == "" {
				continue
			}
			if out[node] == nil {
				out[node] = map[string]bool{}
			}
			out[node][state.Area] = true
		}
	}
	return out
}

func ospfABRs(areas map[string]map[string]bool) map[string]bool {
	out := map[string]bool{}
	for node, nodeAreas := range areas {
		if len(nodeAreas) > 1 && nodeAreas[ospfBackboneArea] {
			out[node] = true
		}
	}
	return out
}

func (e *Engine) ospfCandidatePaths(src, area string, states map[string]map[string]ospfInterfaceState) map[string][]ospfPath {
	out := map[string][]ospfPath{}
	visited := map[string]bool{src: true}
	var walk func(current string, path ospfPath)
	walk = func(current string, path ospfPath) {
		if current != src {
			out[current] = append(out[current], path)
		}
		for _, edge := range e.idx.Adj[model.NodeID(current)] {
			next := string(edge.To)
			if visited[next] {
				continue
			}
			cost, ok := ospfAdjacencyCost(e.idx, current, next, edge.Link, area, states)
			if !ok {
				continue
			}
			visited[next] = true
			walk(next, ospfPath{
				Cost:  path.Cost + cost,
				Nodes: append(append([]string(nil), path.Nodes...), next),
				Links: append(append([]string(nil), path.Links...), edge.Link.Name),
			})
			delete(visited, next)
		}
	}
	walk(src, ospfPath{Nodes: []string{src}})
	for node, paths := range out {
		sort.Slice(paths, func(i, j int) bool {
			if paths[i].Cost != paths[j].Cost {
				return paths[i].Cost < paths[j].Cost
			}
			return strings.Join(paths[i].Nodes, ",") < strings.Join(paths[j].Nodes, ",")
		})
		if len(paths) > ospfMaxPathsPerDestination {
			paths = paths[:ospfMaxPathsPerDestination]
		}
		out[node] = paths
	}
	return out
}

func (e *Engine) ospfInterAreaPaths(src string, adv ospfAdvertisement, states map[string]map[string]ospfInterfaceState, areas map[string]map[string]bool, abrs map[string]bool) []ospfPath {
	if areas[src][adv.Area] {
		return nil
	}
	srcAreas := sortedAreaKeys(areas[src])
	var out []ospfPath
	for _, srcArea := range srcAreas {
		srcPaths := e.ospfCandidatePaths(src, srcArea, states)
		backbonePathsBySrcABR := map[string]map[string][]ospfPath{}
		for _, srcABR := range ospfAreaBoundaries(src, srcArea, areas, abrs) {
			toSrcABR := ospfZeroPath(src, srcABR, srcPaths)
			if len(toSrcABR.Nodes) == 0 {
				continue
			}
			if _, ok := backbonePathsBySrcABR[srcABR]; !ok {
				backbonePathsBySrcABR[srcABR] = e.ospfCandidatePaths(srcABR, ospfBackboneArea, states)
			}
			for _, dstABR := range ospfAreaBoundaries(adv.Node, adv.Area, areas, abrs) {
				toDstABR := ospfZeroPath(srcABR, dstABR, backbonePathsBySrcABR[srcABR])
				if len(toDstABR.Nodes) == 0 {
					continue
				}
				dstPaths := e.ospfCandidatePaths(dstABR, adv.Area, states)
				toAdv := ospfZeroPath(dstABR, adv.Node, dstPaths)
				if len(toAdv.Nodes) == 0 {
					continue
				}
				combined, ok := concatOSPFPaths(toSrcABR, toDstABR, toAdv)
				if ok {
					out = append(out, combined)
				}
			}
		}
	}
	sortOSPFPaths(out)
	if len(out) > ospfMaxPathsPerDestination {
		out = out[:ospfMaxPathsPerDestination]
	}
	return out
}

func ospfAreaBoundaries(node, area string, areas map[string]map[string]bool, abrs map[string]bool) []string {
	if area == ospfBackboneArea {
		if areas[node][ospfBackboneArea] {
			return []string{node}
		}
	}
	var out []string
	for n := range abrs {
		if areas[n][area] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func ospfZeroPath(src, dst string, paths map[string][]ospfPath) ospfPath {
	if src == dst {
		return ospfPath{Nodes: []string{src}}
	}
	if len(paths[dst]) == 0 {
		return ospfPath{}
	}
	return paths[dst][0]
}

func concatOSPFPaths(parts ...ospfPath) (ospfPath, bool) {
	var out ospfPath
	seen := map[string]bool{}
	for i, part := range parts {
		if len(part.Nodes) == 0 {
			return ospfPath{}, false
		}
		out.Cost += part.Cost
		if i == 0 {
			out.Nodes = append(out.Nodes, part.Nodes...)
		} else {
			if out.Nodes[len(out.Nodes)-1] != part.Nodes[0] {
				return ospfPath{}, false
			}
			out.Nodes = append(out.Nodes, part.Nodes[1:]...)
		}
		out.Links = append(out.Links, part.Links...)
	}
	for _, node := range out.Nodes {
		if seen[node] {
			return ospfPath{}, false
		}
		seen[node] = true
	}
	return out, true
}

func sortedAreaKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for area := range m {
		out = append(out, area)
	}
	sort.Strings(out)
	return out
}

func sortOSPFPaths(paths []ospfPath) {
	sort.Slice(paths, func(i, j int) bool {
		if paths[i].Cost != paths[j].Cost {
			return paths[i].Cost < paths[j].Cost
		}
		return strings.Join(paths[i].Nodes, ",") < strings.Join(paths[j].Nodes, ",")
	})
}

func ospfAdjacencyCost(idx *model.TopologyIndex, from, to string, link model.Link, area string, states map[string]map[string]ospfInterfaceState) (int, bool) {
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
	if fromState.Area != area || toState.Area != area {
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
