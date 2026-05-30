package fib

import "sort"

func sortRoutes(routes []NormalizedFIBRoute) {
	sort.SliceStable(routes, func(i, j int) bool {
		return routeKey(routes[i]) < routeKey(routes[j])
	})
	for i := range routes {
		sortNextHops(routes[i].NextHops)
	}
}

func SortRoutes(routes []NormalizedFIBRoute) {
	sortRoutes(routes)
}

func sortNextHops(hops []NormalizedFIBNextHop) {
	sort.SliceStable(hops, func(i, j int) bool {
		return nextHopKey(hops[i]) < nextHopKey(hops[j])
	})
}

func SortNextHops(hops []NormalizedFIBNextHop) {
	sortNextHops(hops)
}

func dedupeNextHops(in []NormalizedFIBNextHop) []NormalizedFIBNextHop {
	seen := map[string]bool{}
	var out []NormalizedFIBNextHop
	for _, hop := range in {
		key := nextHopKey(hop)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, hop)
	}
	sortNextHops(out)
	return out
}

func routeKey(r NormalizedFIBRoute) string {
	protocol := canonicalProtocol(r.Protocol)
	if protocol != "" && protocol != "bgp" {
		return r.Node + "|" + r.VRF + "|" + r.AFI + "|" + protocol + "|" + r.Prefix
	}
	return r.Node + "|" + r.VRF + "|" + r.AFI + "|" + r.Prefix
}

func RouteKey(r NormalizedFIBRoute) string {
	return routeKey(r)
}

func nextHopKey(h NormalizedFIBNextHop) string {
	return h.Address + "|" + h.Interface
}

func sortUnresolvedRoutes(routes []UnresolvedRoute) {
	sort.SliceStable(routes, func(i, j int) bool {
		return unresolvedRouteSortKey(routes[i]) < unresolvedRouteSortKey(routes[j])
	})
}

func unresolvedRouteSortKey(route UnresolvedRoute) string {
	return route.RouteKey + "|" + route.Reason
}

func sortDuplicateRouteConflicts(conflicts []DuplicateRouteConflict) {
	sort.SliceStable(conflicts, func(i, j int) bool {
		return duplicateRouteConflictSortKey(conflicts[i]) < duplicateRouteConflictSortKey(conflicts[j])
	})
}

func duplicateRouteConflictSortKey(conflict DuplicateRouteConflict) string {
	return conflict.RouteKey + "|" + conflict.Side + "|" + conflict.Reason
}
