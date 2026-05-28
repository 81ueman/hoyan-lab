package controlplane

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type routePolicyResolver struct {
	idx *model.TopologyIndex
}

func (r routePolicyResolver) NextHopForPolicy(node string, peerName string, route domainroute.RIBEntry) string {
	return routeNextHopForPolicy(r.idx, node, peerName, route)
}

func (r routePolicyResolver) NextHopForSet(node, nextHop string) domainroute.NextHop {
	return routeNextHopForSet(r.idx, node, nextHop)
}

func routeNextHopForPolicy(idx *model.TopologyIndex, node string, peerName string, route domainroute.RIBEntry) string {
	route = route.Normalize()
	nextHop := route.ForwardingNextHop.Node
	if nextHop == "" {
		nextHop = route.ForwardingNextHop.Addr
	}
	if nextHop == "" {
		return ""
	}
	if nextHop == node && peerName != "" {
		return peerAddress(idx, peerName, node)
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

func routeNextHopForSet(idx *model.TopologyIndex, node, nextHop string) domainroute.NextHop {
	if idx == nil || nextHop == "" {
		return domainroute.NextHop{Addr: nextHop}
	}
	for _, adj := range idx.Adj[model.NodeID(node)] {
		peer := string(adj.To)
		if addr, ok := idx.PeerAddress(node, peer); ok && addr.String() == nextHop {
			return domainroute.NextHop{Node: peer, Addr: nextHop}
		}
	}
	return domainroute.NextHop{Addr: nextHop}
}

func peerAddress(idx *model.TopologyIndex, node, peer string) string {
	if peer == "" {
		return ""
	}
	if addr, ok := idx.PeerAddress(node, peer); ok {
		return addr.String()
	}
	return peer
}
