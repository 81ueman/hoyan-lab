package collect

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func filterObservationRIBRoutes(routes []observation.RIBRoute, node, vrf string) []observation.RIBRoute {
	vrf = string(model.NormalizeNetworkInstance(vrf))
	out := make([]observation.RIBRoute, 0, len(routes))
	for _, route := range routes {
		route = observation.NormalizeRIBRouteRecord(route)
		if route.Node == node && route.NetworkInstance == vrf {
			out = append(out, route)
		}
	}
	observation.SortRoutes(out)
	return out
}

func filterFIBEntrys(routes []observation.FIBEntry, node, vrf string) []observation.FIBEntry {
	vrf = string(model.NormalizeNetworkInstance(vrf))
	out := make([]observation.FIBEntry, 0, len(routes))
	for _, route := range routes {
		if route.VRF == "" {
			route.VRF = string(model.NetworkInstanceDefault)
		}
		if route.Node == node && route.VRF == vrf {
			out = append(out, route)
		}
	}
	observation.SortFIBEntriesForCompare(out)
	return out
}
