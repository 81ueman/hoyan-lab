package controlplane

import (
	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
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
	areas := NodeAreas(states)
	abrs := ABRs(areas)
	for _, src := range e.idx.Topology.Nodes {
		if _, ok := processes[src.Name]; !ok {
			continue
		}
		anyPaths := e.ospfCandidatePathsAnyArea(src.Name, states)
		areaPaths := map[string]map[string][]Path{}
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
					if !AdvertisementAllowed(src, adv, path, processes) {
						continue
					}
					e.installRemoteOSPFRoute(src.Name, adv, path, "")
				}
				continue
			}
			for _, path := range areaPaths[adv.Area][adv.Node] {
				if AdvertisementAllowed(src, adv, path, processes) {
					e.installRemoteOSPFRoute(src.Name, adv, path, RouteTypeIntraArea)
				}
			}
			for _, path := range e.ospfInterAreaPaths(src.Name, adv, states, areas, abrs) {
				if AdvertisementAllowed(src, adv, path, processes) {
					e.installRemoteOSPFRoute(src.Name, adv, path, RouteTypeInterArea)
				}
			}
		}
	}
}

func (e *Engine) installRemoteOSPFRoute(src string, adv Advertisement, path Path, routeType string) {
	if len(path.Nodes) < 2 {
		return
	}
	metric := path.Cost + adv.Cost
	if adv.External {
		routeType = RouteTypeExternal2
		if adv.MetricType == 1 {
			routeType = RouteTypeExternal1
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
		cond = failure.And(PathCondition(path)...)
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

func (e *Engine) installLocalOSPFRoute(node model.Node, adv Advertisement, states map[string]InterfaceState) {
	route := model.ConfiguredRoute{
		Node:            node.Name,
		NetworkInstance: adv.NetworkInstance,
		AFI:             model.AFIIPv4,
		Prefix:          adv.Prefix,
		Kind:            model.RouteSourceOSPF,
		AdminDistance:   110,
		Metric:          adv.Cost,
		OSPFRouteType:   RouteTypeIntraArea,
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
