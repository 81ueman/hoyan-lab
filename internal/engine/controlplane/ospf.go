package controlplane

import (
	"container/heap"
	"math"
	"net/netip"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/bgp"
	domainospf "github.com/81ueman/hoyan-lab/internal/domain/routing/ospf"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type ospfSPFQueue []domainospf.SPFQueueItem

const ospfMaxPathsPerDestination = 8

const (
	ospfRouteTypeIntraArea = domainospf.RouteTypeIntraArea
	ospfRouteTypeInterArea = domainospf.RouteTypeInterArea
	ospfRouteTypeExternal1 = domainospf.RouteTypeExternal1
	ospfRouteTypeExternal2 = domainospf.RouteTypeExternal2
	ospfBackboneArea       = domainospf.BackboneArea
)

func (e *Engine) installOSPFRoutes() {
	for _, vrf := range e.ospfVRFs() {
		e.installOSPFRoutesVRF(vrf)
	}
}

func (e *Engine) installOSPFRoutesVRF(vrf model.NetworkInstanceID) {
	processes := e.ospfProcesses(vrf)
	states := e.ospfInterfaceStates(vrf, processes)
	advertisements := e.ospfAdvertisements(states, processes)
	if len(advertisements) == 0 {
		return
	}
	areas := ospfNodeAreas(states)
	abrs := ospfABRs(areas)
	for _, src := range e.idx.Topology.Nodes {
		if _, ok := processes[src.Name]; !ok {
			continue
		}
		anyPaths := e.ospfCandidatePathsAnyArea(src.Name, states)
		areaPaths := map[string]map[string][]domainospf.Path{}
		for area := range areas[src.Name] {
			areaPaths[area] = e.ospfCandidatePaths(src.Name, area, states)
		}
		for _, adv := range advertisements {
			if adv.Node == src.Name {
				if adv.External || adv.DefaultArea != "" {
					continue
				}
				e.installLocalOSPFRoute(src, adv, states[src.Name])
				continue
			}
			if adv.External || adv.DefaultArea != "" {
				for _, path := range anyPaths[adv.Node] {
					if !ospfAdvertisementAllowed(src, adv, path, processes) {
						continue
					}
					e.installRemoteOSPFRoute(src.Name, adv, path, "")
				}
				continue
			}
			for _, path := range areaPaths[adv.Area][adv.Node] {
				if ospfAdvertisementAllowed(src, adv, path, processes) {
					e.installRemoteOSPFRoute(src.Name, adv, path, ospfRouteTypeIntraArea)
				}
			}
			for _, path := range e.ospfInterAreaPaths(src.Name, adv, states, areas, abrs) {
				if ospfAdvertisementAllowed(src, adv, path, processes) {
					e.installRemoteOSPFRoute(src.Name, adv, path, ospfRouteTypeInterArea)
				}
			}
		}
	}
}

func (e *Engine) ospfVRFs() []model.NetworkInstanceID {
	seen := map[model.NetworkInstanceID]bool{}
	for _, node := range e.idx.Topology.Nodes {
		for _, process := range ospfProcessesForNode(node) {
			if !process.Enabled {
				continue
			}
			vrf := model.NormalizeNetworkInstance(string(process.NetworkInstance))
			seen[vrf] = true
		}
	}
	out := make([]model.NetworkInstanceID, 0, len(seen))
	for vrf := range seen {
		out = append(out, vrf)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (e *Engine) ospfProcesses(vrf model.NetworkInstanceID) map[string]model.OSPFProcess {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	out := map[string]model.OSPFProcess{}
	for _, node := range e.idx.Topology.Nodes {
		for _, process := range ospfProcessesForNode(node) {
			if !process.Enabled || model.NormalizeNetworkInstance(string(process.NetworkInstance)) != vrf {
				continue
			}
			process.NetworkInstance = vrf
			out[node.Name] = process
		}
	}
	return out
}

func ospfProcessesForNode(node model.Node) []model.OSPFProcess {
	var out []model.OSPFProcess
	if node.OSPF.Enabled {
		process := node.OSPF
		process.NetworkInstance = model.NormalizeNetworkInstance(string(process.NetworkInstance))
		out = append(out, process)
	}
	for _, process := range node.OSPFProcesses {
		if !process.Enabled {
			continue
		}
		process.NetworkInstance = model.NormalizeNetworkInstance(string(process.NetworkInstance))
		out = append(out, process)
	}
	return out
}

func (e *Engine) installRemoteOSPFRoute(src string, adv domainospf.Advertisement, path domainospf.Path, routeType string) {
	if len(path.Nodes) < 2 {
		return
	}
	metric := path.Cost + adv.Cost
	if adv.External {
		routeType = ospfRouteTypeExternal2
		if adv.MetricType == 1 {
			routeType = ospfRouteTypeExternal1
		} else {
			metric = adv.Cost
		}
	}
	nextHop := path.Nodes[1]
	nextHopAddr := ""
	if addr, ok := e.idx.PeerAddress(src, nextHop); ok {
		nextHopAddr = addr.String()
	}
	cond := path.Cond
	if cond == nil {
		cond = failure.And(pathCondition(path)...)
	}
	if adv.Source.Condition != nil {
		cond = failure.And(cond, adv.Source.Condition)
	}
	route := model.ConfiguredRoute{
		Node:            src,
		NetworkInstance: adv.NetworkInstance,
		AFI:             model.AFIIPv4,
		Prefix:          adv.Prefix,
		Kind:            model.RouteSourceOSPF,
		AdminDistance:   110,
		Metric:          metric,
		OSPFRouteType:   routeType,
	}
	entry := domainroute.RIBEntry{
		NLRI:              domainroute.NLRI{Prefix: adv.Prefix},
		Attrs:             domainroute.BGPAttributes{OriginCode: domainroute.BGPOriginIGP, LocalPref: 100},
		Provenance:        domainroute.Provenance{OriginNode: adv.Node, FromNode: nextHop, PathNodes: path.Nodes, PathLinks: path.Links},
		ForwardingNextHop: domainroute.NextHop{Node: nextHop, Addr: nextHopAddr},
		SourceKind:        model.RouteSourceOSPF,
		RouteSource:       route,
		BaseCond:          cond,
		Condition:         cond,
	}.Normalize()
	e.addRIB(src, adv.Prefix, entry)
}

func (e *Engine) installLocalOSPFRoute(node model.Node, adv domainospf.Advertisement, states map[string]domainospf.InterfaceState) {
	route := model.ConfiguredRoute{
		Node:            node.Name,
		NetworkInstance: adv.NetworkInstance,
		AFI:             model.AFIIPv4,
		Prefix:          adv.Prefix,
		Kind:            model.RouteSourceOSPF,
		AdminDistance:   110,
		Metric:          adv.Cost,
		OSPFRouteType:   ospfRouteTypeIntraArea,
		Interface:       ospfInterfaceForPrefix(states, adv.Prefix),
	}
	cond := failure.NodeVar(node.Name)
	entry := domainroute.RIBEntry{
		NLRI:        domainroute.NLRI{Prefix: adv.Prefix},
		Attrs:       domainroute.BGPAttributes{OriginCode: domainroute.BGPOriginIGP, LocalPref: 100},
		Provenance:  domainroute.Provenance{OriginNode: node.Name, PathNodes: []string{node.Name}},
		SourceKind:  model.RouteSourceOSPF,
		RouteSource: route,
		BaseCond:    cond,
		Condition:   cond,
	}.Normalize()
	e.addRIB(node.Name, adv.Prefix, entry)
}

func ospfInterfaceForPrefix(states map[string]domainospf.InterfaceState, prefix model.Prefix) string {
	for _, state := range states {
		if model.PrefixFromNetIP(state.Prefix).Equal(prefix) {
			return state.Name
		}
	}
	return ""
}

func (e *Engine) ospfInterfaceStates(vrf model.NetworkInstanceID, processes map[string]model.OSPFProcess) map[string]map[string]domainospf.InterfaceState {
	out := map[string]map[string]domainospf.InterfaceState{}
	for _, node := range e.idx.Topology.Nodes {
		process, ok := processes[node.Name]
		if !ok {
			continue
		}
		for _, iface := range node.Interfaces {
			pfx, err := netip.ParsePrefix(iface.Address)
			if err != nil || !pfx.Addr().Is4() {
				continue
			}
			ifState, ok := ospfInterfaceFor(node, process, vrf, iface, pfx)
			if !ok {
				continue
			}
			if out[node.Name] == nil {
				out[node.Name] = map[string]domainospf.InterfaceState{}
			}
			out[node.Name][iface.Name] = ifState
		}
	}
	return out
}

func ospfInterfaceFor(node model.Node, process model.OSPFProcess, vrf model.NetworkInstanceID, iface model.Interface, pfx netip.Prefix) (domainospf.InterfaceState, bool) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	if model.NormalizeNetworkInstance(string(iface.VRF)) != vrf {
		return domainospf.InterfaceState{}, false
	}
	state := domainospf.InterfaceState{Node: node.Name, Name: iface.Name, NetworkInstance: vrf, Prefix: pfx.Masked(), Cost: 1}
	for _, configured := range process.Interfaces {
		if !model.EquivalentInterfaceName(node.Kind, configured.Name, iface.Name) {
			continue
		}
		state.Area = normalizeOSPFArea(configured.Area)
		if configured.Cost > 0 {
			state.Cost = configured.Cost
		}
		state.Passive = configured.Passive
		state.NetworkType = configured.NetworkType
	}
	if state.Area == "" {
		for _, network := range process.Networks {
			if network.Prefix.Contains(pfx.Addr()) {
				state.Area = normalizeOSPFArea(network.Area)
				break
			}
		}
	}
	for _, passive := range process.PassiveInterfaces {
		if model.EquivalentInterfaceName(node.Kind, passive, iface.Name) {
			state.Passive = true
		}
	}
	if state.Area == "" {
		return domainospf.InterfaceState{}, false
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

func (e *Engine) ospfAdvertisements(states map[string]map[string]domainospf.InterfaceState, processes map[string]model.OSPFProcess) []domainospf.Advertisement {
	var out []domainospf.Advertisement
	seen := map[string]bool{}
	for node, byIface := range states {
		for _, state := range byIface {
			prefix := model.PrefixFromNetIP(state.Prefix)
			key := node + "|" + string(state.NetworkInstance) + "|" + prefix.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, domainospf.Advertisement{Node: node, NetworkInstance: state.NetworkInstance, Prefix: prefix, Cost: state.Cost, Area: state.Area})
		}
	}
	for _, node := range e.idx.Topology.Nodes {
		process, ok := processes[node.Name]
		if !ok {
			continue
		}
		for _, route := range e.ospfRedistributedRoutes(node, process) {
			area := ospfExternalArea(process, states[node.Name])
			out = append(out, domainospf.Advertisement{
				Node:            node.Name,
				NetworkInstance: process.NetworkInstance,
				Prefix:          route.RouteSource.Prefix,
				Cost:            route.RouteSource.Metric,
				External:        true,
				MetricType:      route.RouteSource.MetricType,
				ExternalArea:    area,
				Source:          route,
			})
		}
		for _, area := range process.Areas {
			if area.Kind == model.OSPFAreaStub && !ospfNodeAttachedToOtherArea(states[node.Name], area.ID) {
				continue
			}
			if area.Kind != model.OSPFAreaStub && !(area.Kind == model.OSPFAreaNSSA && area.DefaultInformationOriginate) {
				continue
			}
			if !ospfNodeAttachedToArea(states[node.Name], area.ID) {
				continue
			}
			out = append(out, domainospf.Advertisement{Node: node.Name, NetworkInstance: process.NetworkInstance, Prefix: model.MustPrefix("0.0.0.0/0"), Cost: 1, DefaultArea: area.ID})
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

func (e *Engine) ospfRedistributedRoutes(node model.Node, process model.OSPFProcess) []domainroute.RIBEntry {
	var out []domainroute.RIBEntry
	for _, redist := range process.Redistribute {
		for _, route := range e.ospfRedistributionCandidates(node, process.NetworkInstance, redist.Kind) {
			route = route.Normalize()
			if route.SourceKind == model.RouteSourceConnected && route.RouteSource.ConnectedClass == model.ConnectedRouteClassLink {
				continue
			}
			if redist.RouteMap != "" {
				decision := bgp.ApplyRoutePolicy(routePolicyResolver{idx: e.idx}, node, "", redist.RouteMap, route)
				if !decision.Accept {
					continue
				}
				route = decision.Route.Normalize()
			}
			sourceRoute := route.RouteSource
			sourceRoute.Node = node.Name
			sourceRoute.Kind = model.RouteSourceOSPF
			sourceRoute.AdminDistance = 110
			sourceRoute.MetricType = ospfExternalMetricType(redist.MetricType)
			sourceRoute.OSPFRouteType = ospfExternalRouteType(sourceRoute.MetricType)
			sourceRoute.Metric = ospfExternalMetric(redist, route)
			route.SourceKind = model.RouteSourceOSPF
			route.RouteSource = sourceRoute
			out = append(out, route.Normalize())
		}
	}
	return out
}

func (e *Engine) ospfRedistributionCandidates(node model.Node, vrf model.NetworkInstanceID, kind model.RouteSourceKind) []domainroute.RIBEntry {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	var out []domainroute.RIBEntry
	switch kind {
	case model.RouteSourceConnected, model.RouteSourceStatic:
		for _, route := range e.redistributionCandidates(node, kind) {
			if model.NormalizeNetworkInstance(string(route.NetworkInstance)) != vrf {
				continue
			}
			entry := e.bgpRouteFromConfiguredRoute(node, route).Normalize()
			entry.SourceKind = route.Kind
			entry.RouteSource = route
			out = append(out, entry.Normalize())
		}
	case model.RouteSourceBGP:
		byPrefix := e.rib[node.Name][string(vrf)]
		for _, routes := range byPrefix {
			for _, route := range routes {
				route = route.Normalize()
				if route.SourceKind != model.RouteSourceBGP && route.SourceKind != model.RouteSourceAggregate {
					continue
				}
				if route.Provenance.OriginNode == node.Name && len(route.Provenance.PathNodes) == 1 {
					continue
				}
				if route.SelectedCond != nil {
					route.Condition = route.SelectedCond
				}
				out = append(out, route)
			}
		}
	}
	return out
}

func ospfExternalMetric(redist model.OSPFRedistribution, route domainroute.RIBEntry) int {
	route = route.Normalize()
	if redist.Metric > 0 {
		return redist.Metric
	}
	if route.Attrs.MED > 0 {
		return route.Attrs.MED
	}
	if route.RouteSource.Metric > 0 {
		return route.RouteSource.Metric
	}
	return 20
}

func ospfExternalRouteType(metricType int) string {
	if ospfExternalMetricType(metricType) == 1 {
		return ospfRouteTypeExternal1
	}
	return ospfRouteTypeExternal2
}

func ospfExternalMetricType(metricType int) int {
	if metricType == 1 {
		return 1
	}
	return 2
}

func ospfExternalArea(process model.OSPFProcess, states map[string]domainospf.InterfaceState) string {
	for _, state := range states {
		if area := process.Areas[state.Area]; area.Kind == model.OSPFAreaNSSA {
			return state.Area
		}
	}
	return ""
}

func ospfNodeAttachedToArea(states map[string]domainospf.InterfaceState, area string) bool {
	for _, state := range states {
		if state.Area == area {
			return true
		}
	}
	return false
}

func ospfNodeAttachedToOtherArea(states map[string]domainospf.InterfaceState, area string) bool {
	for _, state := range states {
		if state.Area != "" && state.Area != area {
			return true
		}
	}
	return false
}

func ospfAdvertisementAllowed(src model.Node, adv domainospf.Advertisement, path domainospf.Path, processes map[string]model.OSPFProcess) bool {
	if adv.DefaultArea != "" {
		return pathUsesOnlyArea(path, adv.DefaultArea)
	}
	if !adv.External {
		return true
	}
	for _, areaID := range path.Areas {
		area := ospfAreaForPathArea(processes, path, areaID)
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
	return !ospfNodeInStubOrNSSA(processes[src.Name])
}

func pathUsesOnlyArea(path domainospf.Path, area string) bool {
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

func ospfNodeInStubOrNSSA(process model.OSPFProcess) bool {
	for _, area := range process.Areas {
		if area.Kind == model.OSPFAreaStub || area.Kind == model.OSPFAreaNSSA {
			return true
		}
	}
	return false
}

func ospfAreaForPathArea(processes map[string]model.OSPFProcess, path domainospf.Path, areaID string) model.OSPFArea {
	for _, nodeName := range path.Nodes {
		process, ok := processes[nodeName]
		if !ok {
			continue
		}
		area := process.Areas[areaID]
		if area.Kind != "" {
			return area
		}
	}
	return model.OSPFArea{ID: areaID, Kind: model.OSPFAreaNormal}
}

func ospfNodeAreas(states map[string]map[string]domainospf.InterfaceState) map[string]map[string]bool {
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

func (e *Engine) ospfCandidatePaths(src, area string, states map[string]map[string]domainospf.InterfaceState) map[string][]domainospf.Path {
	return e.ospfCandidatePathsWithArea(src, states, func(fromState, toState domainospf.InterfaceState) (string, bool) {
		if fromState.Area != area || toState.Area != area {
			return "", false
		}
		return area, true
	})
}

func (e *Engine) ospfCandidatePathsAnyArea(src string, states map[string]map[string]domainospf.InterfaceState) map[string][]domainospf.Path {
	return e.ospfCandidatePathsWithArea(src, states, func(fromState, toState domainospf.InterfaceState) (string, bool) {
		if fromState.Area != toState.Area {
			return "", false
		}
		return fromState.Area, true
	})
}

func (e *Engine) ospfCandidatePathsWithArea(src string, states map[string]map[string]domainospf.InterfaceState, allowed domainospf.AdjacencyFilter) map[string][]domainospf.Path {
	out := map[string][]domainospf.Path{}
	for _, firstHop := range e.ospfAdjacencies(src, states, allowed) {
		spf := e.ospfShortestPathTree(firstHop.To, src, states, allowed)
		condMemo := map[string]failure.Cond{}
		for dst, state := range spf {
			if dst == firstHop.To {
				path := domainospf.Path{
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
			path := domainospf.Path{
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
		sortOSPFPaths(paths)
		out[node] = paths
	}
	return out
}

func (e *Engine) ospfInterAreaPaths(src string, adv domainospf.Advertisement, states map[string]map[string]domainospf.InterfaceState, areas map[string]map[string]bool, abrs map[string]bool) []domainospf.Path {
	if areas[src][adv.Area] {
		return nil
	}
	srcAreas := sortedAreaKeys(areas[src])
	var out []domainospf.Path
	for _, srcArea := range srcAreas {
		srcPaths := e.ospfCandidatePaths(src, srcArea, states)
		backbonePathsBySrcABR := map[string]map[string][]domainospf.Path{}
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

func ospfZeroPath(src, dst string, paths map[string][]domainospf.Path) domainospf.Path {
	if src == dst {
		return domainospf.Path{Nodes: []string{src}}
	}
	if len(paths[dst]) == 0 {
		return domainospf.Path{}
	}
	return paths[dst][0]
}

func concatOSPFPaths(parts ...domainospf.Path) (domainospf.Path, bool) {
	var out domainospf.Path
	seen := map[string]bool{}
	var conds []failure.Cond
	for i, part := range parts {
		if len(part.Nodes) == 0 {
			return domainospf.Path{}, false
		}
		out.Cost += part.Cost
		if part.Cond != nil {
			conds = append(conds, part.Cond)
		}
		if i == 0 {
			out.Nodes = append(out.Nodes, part.Nodes...)
		} else {
			if out.Nodes[len(out.Nodes)-1] != part.Nodes[0] {
				return domainospf.Path{}, false
			}
			out.Nodes = append(out.Nodes, part.Nodes[1:]...)
		}
		out.Links = append(out.Links, part.Links...)
		out.Areas = append(out.Areas, part.Areas...)
	}
	for _, node := range out.Nodes {
		if seen[node] {
			return domainospf.Path{}, false
		}
		seen[node] = true
	}
	if len(conds) > 0 {
		out.Cond = failure.And(conds...)
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

func sortOSPFPaths(paths []domainospf.Path) {
	sort.Slice(paths, func(i, j int) bool {
		if paths[i].Cost != paths[j].Cost {
			return paths[i].Cost < paths[j].Cost
		}
		return strings.Join(paths[i].Nodes, ",") < strings.Join(paths[j].Nodes, ",")
	})
}

func (e *Engine) ospfShortestPathTree(src, excluded string, states map[string]map[string]domainospf.InterfaceState, allowed domainospf.AdjacencyFilter) map[string]domainospf.SPFNode {
	dist := map[string]domainospf.SPFNode{}
	for _, node := range e.idx.Topology.Nodes {
		if len(states[node.Name]) == 0 || node.Name == excluded {
			continue
		}
		dist[node.Name] = domainospf.SPFNode{Cost: math.MaxInt}
	}
	if _, ok := dist[src]; !ok {
		return dist
	}
	dist[src] = domainospf.SPFNode{Cost: 0}
	q := &ospfSPFQueue{{Node: src}}
	heap.Init(q)
	for q.Len() > 0 {
		item := heap.Pop(q).(domainospf.SPFQueueItem)
		current := dist[item.Node]
		if item.Cost != current.Cost {
			continue
		}
		for _, adj := range e.ospfAdjacencies(item.Node, states, allowed) {
			if adj.To == excluded {
				continue
			}
			next, ok := dist[adj.To]
			if !ok {
				continue
			}
			cost := item.Cost + adj.Cost
			pred := domainospf.SPFPredecessor{Node: item.Node, Link: adj.Link, Area: adj.Area}
			switch {
			case cost < next.Cost:
				next.Cost = cost
				next.Predecessors = []domainospf.SPFPredecessor{pred}
				dist[adj.To] = next
				heap.Push(q, domainospf.SPFQueueItem{Node: adj.To, Cost: cost})
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

func (e *Engine) ospfAdjacencies(from string, states map[string]map[string]domainospf.InterfaceState, allowed domainospf.AdjacencyFilter) []domainospf.Adjacency {
	var out []domainospf.Adjacency
	for _, edge := range e.idx.Adj[model.NodeID(from)] {
		to := string(edge.To)
		cost, area, ok := ospfAdjacencyCost(e.idx, from, to, edge.Link, states, allowed)
		if !ok {
			continue
		}
		out = append(out, domainospf.Adjacency{From: from, To: to, Link: edge.Link.Name, Area: area, Cost: cost})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].To == out[j].To {
			return out[i].Link < out[j].Link
		}
		return out[i].To < out[j].To
	})
	return out
}

func ospfRepresentativePath(src, dst string, spf map[string]domainospf.SPFNode) ([]string, []string, []string, bool) {
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

func ospfSPFCondition(src, dst string, spf map[string]domainospf.SPFNode, memo map[string]failure.Cond) failure.Cond {
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

func ospfAdjacencyCost(idx *model.TopologyIndex, from, to string, link model.Link, states map[string]map[string]domainospf.InterfaceState, allowed domainospf.AdjacencyFilter) (int, string, bool) {
	fromRef, ok := idx.InterfaceOnLink(from, link.Name)
	if !ok {
		return 0, "", false
	}
	toRef, ok := idx.InterfaceOnLink(to, link.Name)
	if !ok {
		return 0, "", false
	}
	fromState, ok := states[from][fromRef.ConfigName]
	if !ok || fromState.Passive {
		return 0, "", false
	}
	toState, ok := states[to][toRef.ConfigName]
	if !ok || toState.Passive {
		return 0, "", false
	}
	area, ok := allowed(fromState, toState)
	if !ok {
		return 0, "", false
	}
	if !ospfNetworkTypesMatchForAdjacency(fromState.NetworkType, toState.NetworkType) {
		return 0, "", false
	}
	return fromState.Cost, area, true
}

func ospfNetworkTypesMatchForAdjacency(a, b string) bool {
	a = normalizeOSPFNetworkType(a)
	b = normalizeOSPFNetworkType(b)
	if a == "" || b == "" {
		return true
	}
	return a == b
}

func normalizeOSPFNetworkType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "p2p":
		return "point-to-point"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func pathCondition(path domainospf.Path) []failure.Cond {
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
	*q = append(*q, x.(domainospf.SPFQueueItem))
}

func (q *ospfSPFQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[:n-1]
	return item
}
