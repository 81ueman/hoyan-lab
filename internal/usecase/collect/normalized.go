package collect

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	observationfib "github.com/81ueman/hoyan-lab/internal/domain/observation/fib"
	observationrib "github.com/81ueman/hoyan-lab/internal/domain/observation/rib"
)

func filterNormalizedRIBRoutes(routes []observationrib.NormalizedRoute, node, vrf string) []observationrib.NormalizedRoute {
	vrf = string(model.NormalizeNetworkInstance(vrf))
	out := make([]observationrib.NormalizedRoute, 0, len(routes))
	for _, route := range routes {
		route = observationrib.NormalizeRoute(route)
		if route.Node == node && route.NetworkInstance == vrf {
			out = append(out, route)
		}
	}
	observationrib.SortRoutes(out)
	return out
}

func filterNormalizedFIBRoutes(routes []observationfib.NormalizedFIBRoute, node, vrf string) []observationfib.NormalizedFIBRoute {
	vrf = string(model.NormalizeNetworkInstance(vrf))
	out := make([]observationfib.NormalizedFIBRoute, 0, len(routes))
	for _, route := range routes {
		if route.VRF == "" {
			route.VRF = string(model.NetworkInstanceDefault)
		}
		if route.Node == node && route.VRF == vrf {
			out = append(out, route)
		}
	}
	observationfib.SortRoutes(out)
	return out
}
