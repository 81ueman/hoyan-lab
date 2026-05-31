package fib

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func canonicalProtocol(protocol string) string {
	return observation.CanonicalProtocol(protocol)
}

func canonicalRouteSource(protocol string) observation.RouteSource {
	return observation.RouteSource{Protocol: model.NormalizeRouteSourceKind(model.RouteSourceKind(canonicalProtocol(protocol)))}
}

func forwardingAction(protocol string, hops []NextHop) observation.ForwardingAction {
	switch model.NormalizeRouteSourceKind(model.RouteSourceKind(canonicalProtocol(protocol))) {
	case model.RouteSourceBlackhole:
		return observation.ActionDrop
	case model.RouteSourceConnected:
		if len(hops) == 0 {
			return observation.ActionReceive
		}
	}
	return observation.ActionForward
}

func sortRoutes(routes []FIBEntry) {
	observation.SortFIBEntriesForCompare(routes)
}

func dedupeNextHops(in []NextHop) []NextHop {
	seen := map[string]bool{}
	var out []NextHop
	for _, hop := range in {
		key := hop.Address + "|" + hop.Interface
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, hop)
	}
	observation.SortNextHops(out)
	return out
}
