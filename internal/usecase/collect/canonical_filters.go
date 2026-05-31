package collect

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func filterObservationRIBRoutes(routes []observation.RIBRoute, node, vrf string) []observation.RIBRoute {
	_, _ = node, vrf
	out := make([]observation.RIBRoute, 0, len(routes))
	out = append(out, routes...)
	observation.SortRoutes(out)
	return out
}

func filterFIBs(fibs []observation.FIB, node model.NodeID, vrf model.NetworkInstanceID) observation.FIB {
	vrf = model.NetworkInstanceID(model.NormalizeNetworkInstance(string(vrf)))
	for _, fib := range fibs {
		if fib.Node == node && fib.VRF == vrf {
			observation.SortFIBEntriesForCompare(fib.Entries)
			return fib
		}
	}
	return observation.FIB{Node: node, VRF: vrf}
}
