package controlplane

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	routingpolicy "github.com/81ueman/hoyan-lab/internal/domain/routing/policy"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

func (e *Engine) ospfRedistributedRoutes(node model.Node, process model.OSPFProcess) []domainroute.RIBEntry {
	var out []domainroute.RIBEntry
	for _, redist := range process.Redistribute {
		for _, route := range e.ospfRedistributionCandidates(node, process.NetworkInstance, redist.Kind) {
			route = route.Normalize()
			if route.SourceKind == model.RouteSourceConnected && route.RouteSource.ConnectedClass == model.ConnectedRouteClassLink {
				continue
			}
			if redist.RouteMap != "" {
				decision := routingpolicy.ApplyRoutePolicy(routePolicyResolver{idx: e.idx}, node, "", redist.RouteMap, route)
				if !decision.Accept {
					continue
				}
				route = decision.Route.Normalize()
			}
			out = append(out, RedistributedExternalRoute(node.Name, redist, route))
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
