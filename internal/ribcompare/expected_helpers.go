package ribcompare

import (
	"github.com/81ueman/hoyan-lab/internal/model"
	"github.com/81ueman/hoyan-lab/internal/sim"
)

func peerAddress(idx *model.TopologyIndex, node, peer string) string {
	if peer == "" {
		return ""
	}
	if addr, ok := idx.PeerAddress(node, peer); ok {
		return addr.String()
	}
	return peer
}

func routeNextHopAddress(idx *model.TopologyIndex, node string, route sim.RIBEntry) string {
	route = route.Normalize()
	if route.ForwardingNextHop.Addr != "" {
		return route.ForwardingNextHop.Addr
	}
	nextHop := route.ForwardingNextHop.Node
	if nextHop == "" {
		return ""
	}
	if direct := peerAddress(idx, node, nextHop); direct != nextHop {
		return direct
	}
	for i := 0; i+1 < len(route.Provenance.PathNodes); i++ {
		if route.Provenance.PathNodes[i] != nextHop {
			continue
		}
		if addr := peerAddress(idx, route.Provenance.PathNodes[i+1], nextHop); addr != nextHop {
			return addr
		}
	}
	return nextHop
}

func expectedRouteValid(node model.Node, route sim.RIBEntry) bool {
	return sim.BehaviorFor(node.Kind).RouteValidForRIB(node, route)
}
