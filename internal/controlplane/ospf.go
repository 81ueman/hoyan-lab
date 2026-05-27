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
}

const ospfMaxPathsPerDestination = 8

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
				cond := failure.And(pathCondition(path)...)
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
			cost, ok := ospfAdjacencyCost(e.idx, current, next, edge.Link, states)
			if !ok {
				continue
			}
			visited[next] = true
			walk(next, ospfPath{
				Cost:  path.Cost + cost,
				Nodes: append(append([]string(nil), path.Nodes...), next),
				Links: append(append([]string(nil), path.Links...), edge.Link.Name),
				Areas: append(append([]string(nil), path.Areas...), ospfAdjacencyArea(e.idx, current, next, edge.Link, states)),
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
