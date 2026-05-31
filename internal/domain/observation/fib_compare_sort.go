package observation

import "sort"

func sortFIBEntriesForCompare(routes []FIBEntry) {
	sort.SliceStable(routes, func(i, j int) bool {
		return fibRouteKey(routes[i]) < fibRouteKey(routes[j])
	})
	for i := range routes {
		sortNextHops(routes[i].NextHops)
	}
}

func SortFIBEntriesForCompare(routes []FIBEntry) {
	sortFIBEntriesForCompare(routes)
}

func sortNextHops(hops []NextHop) {
	sort.SliceStable(hops, func(i, j int) bool {
		return fibNextHopKey(hops[i]) < fibNextHopKey(hops[j])
	})
}

func SortNextHops(hops []NextHop) {
	sortNextHops(hops)
}

func dedupeFIBNextHops(in []NextHop) []NextHop {
	seen := map[string]bool{}
	var out []NextHop
	for _, hop := range in {
		key := fibNextHopKey(hop)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, hop)
	}
	sortNextHops(out)
	return out
}

func fibRouteKey(r FIBEntry) string {
	r = normalizeFIBEntryForCompare(r)
	protocol := canonicalProtocol(r.Protocol)
	if protocol != "" && protocol != "bgp" {
		return r.Node + "|" + r.VRF + "|" + string(r.AFI) + "|" + protocol + "|" + r.Prefix
	}
	return r.Node + "|" + r.VRF + "|" + string(r.AFI) + "|" + r.Prefix
}

func fibTableRouteKey(r FIBEntry) string {
	r = normalizeFIBEntryForCompare(r)
	return string(r.AFI) + "|" + canonicalProtocol(r.Protocol) + "|" + string(r.Action) + "|" + r.Prefix
}

func RouteKey(r FIBEntry) string {
	return fibRouteKey(r)
}

func fibNextHopKey(h NextHop) string {
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
