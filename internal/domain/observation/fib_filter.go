package observation

import "github.com/81ueman/hoyan-lab/internal/domain/model"

func ComparableRoutes(topo *model.Topology, routes []FIBEntry, opts Options) []FIBEntry {
	return AnalyzeComparableRoutes(topo, routes, opts).Routes
}

func AnalyzeComparableRoutes(topo *model.Topology, routes []FIBEntry, opts Options) FilterResult {
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		panic(err)
	}
	var out []FIBEntry
	var unresolved []UnresolvedRoute
	for _, route := range routes {
		route.Protocol = canonicalProtocol(route.Protocol)
		if route.Protocol == "connected" && route.ConnectedClass == "" {
			route.ConnectedClass = idx.ConnectedClassForRoute(route.Node, route.Prefix, firstNextHopInterface(route.NextHops))
		}
		if !comparableProtocol(route) {
			continue
		}
		if route.Protocol == "connected" && !comparableConnectedClass(route.ConnectedClass) {
			continue
		}
		if route.Protocol == "static" && len(route.NextHops) == 0 {
			continue
		}
		filtered := route
		filtered.NextHops = normalizeRouteNextHops(idx, filtered)
		if route.Protocol == "bgp" {
			var routeUnresolved []UnresolvedNextHop
			filtered.NextHops, routeUnresolved = comparableNextHops(idx, route.Node, filtered.NextHops, opts)
			if len(routeUnresolved) > 0 {
				unresolved = append(unresolved, unresolvedRoute(filtered, routeUnresolved))
			}
			filtered.NextHops = normalizeRouteNextHops(idx, filtered)
			if len(route.NextHops) > 0 && len(filtered.NextHops) == 0 {
				continue
			}
		}
		out = append(out, filtered)
	}
	sortFIBEntriesForCompare(out)
	sortUnresolvedRoutes(unresolved)
	return FilterResult{Routes: out, Unresolved: unresolved}
}

func comparableProtocol(route FIBEntry) bool {
	switch route.Protocol {
	case "bgp", "connected", "static", "blackhole":
		return true
	default:
		return false
	}
}

func comparableConnectedClass(class model.ConnectedRouteClass) bool {
	switch class {
	case model.ConnectedRouteClassLink, model.ConnectedRouteClassLoopback, model.ConnectedRouteClassService:
		return true
	default:
		return false
	}
}

func firstNextHopInterface(hops []NextHop) string {
	for _, hop := range hops {
		if hop.Interface != "" {
			return hop.Interface
		}
	}
	return ""
}

func normalizeRouteNextHops(idx *model.TopologyIndex, route FIBEntry) []NextHop {
	node, ok := idx.Node(route.Node)
	if !ok {
		return dedupeFIBNextHops(route.NextHops)
	}
	out := make([]NextHop, 0, len(route.NextHops))
	for _, hop := range route.NextHops {
		hop.Interface = model.ProfileFor(node.Kind).InterfaceProfile().CanonicalInterfaceName(hop.Interface)
		if canonicalProtocol(route.Protocol) == "connected" {
			hop.Address = ""
		}
		out = append(out, hop)
	}
	return dedupeFIBNextHops(out)
}

func comparableNextHops(idx *model.TopologyIndex, node string, hops []NextHop, opts Options) ([]NextHop, []UnresolvedNextHop) {
	var out []NextHop
	var unresolved []UnresolvedNextHop
	for _, hop := range hops {
		peer, ok := peerForNextHopInterface(idx, node, hop.Interface)
		if !ok {
			if hop.Interface == "" && hop.Address != "" && !isNodeName(idx, hop.Address) {
				out = append(out, hop)
			} else {
				unresolved = append(unresolved, unresolvedNextHop(idx, node, hop))
			}
			continue
		}
		peerNode, ok := idx.Node(peer)
		if !ok {
			unresolved = append(unresolved, unresolvedNextHop(idx, node, hop))
			continue
		}
		if opts.AllowUnsupported && !model.ProfileFor(peerNode.Kind).LiveProfile().SupportsFIBCollection() {
			continue
		}
		out = append(out, hop)
	}
	return dedupeFIBNextHops(out), dedupeUnresolvedNextHops(unresolved)
}

func unresolvedRoute(route FIBEntry, hops []UnresolvedNextHop) UnresolvedRoute {
	reason := "unresolved_or_mgmt_fallback"
	if len(hops) == 1 && hops[0].Reason != "" {
		reason = hops[0].Reason
	}
	return UnresolvedRoute{
		RouteKey: fibRouteKey(route),
		Node:     route.Node,
		VRF:      route.VRF,
		AFI:      string(route.AFI),
		Prefix:   route.Prefix,
		Protocol: route.Protocol,
		NextHops: hops,
		Reason:   reason,
	}
}

func unresolvedNextHop(idx *model.TopologyIndex, node string, hop NextHop) UnresolvedNextHop {
	reason := "topology_interface_missing"
	if hop.Interface == "" {
		reason = "unresolved_recursive_next_hop"
	} else if isManagementInterface(idx, node, hop.Interface) {
		reason = "unresolved_or_mgmt_fallback"
	}
	return UnresolvedNextHop{Address: hop.Address, Interface: hop.Interface, Reason: reason}
}

func isManagementInterface(idx *model.TopologyIndex, node, iface string) bool {
	if iface == "" {
		return false
	}
	if idx == nil {
		return model.ProfileFor(model.KindFRR).InterfaceProfile().IsManagementInterface(nil, iface)
	}
	n, ok := idx.Node(node)
	if !ok {
		return model.ProfileFor(model.KindFRR).InterfaceProfile().IsManagementInterface(nil, iface)
	}
	return model.ProfileFor(n.Kind).InterfaceProfile().IsManagementInterface(n.Interfaces, iface)
}

func dedupeUnresolvedNextHops(in []UnresolvedNextHop) []UnresolvedNextHop {
	seen := map[string]bool{}
	var out []UnresolvedNextHop
	for _, hop := range in {
		key := hop.Address + "|" + hop.Interface + "|" + hop.Reason
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, hop)
	}
	return out
}

func peerForNextHopInterface(idx *model.TopologyIndex, node, iface string) (string, bool) {
	if idx == nil || iface == "" {
		return "", false
	}
	n, ok := idx.Node(node)
	if !ok {
		return "", false
	}
	for _, edge := range idx.Adj[model.NodeID(node)] {
		link := edge.Link
		localIface := link.AIntf
		if link.B == node {
			localIface = link.BIntf
		}
		if model.ProfileFor(n.Kind).InterfaceProfile().EquivalentInterfaceName(localIface, iface) {
			return string(edge.To), true
		}
	}
	return "", false
}

func isNodeName(idx *model.TopologyIndex, name string) bool {
	_, ok := idx.Node(name)
	return ok
}
