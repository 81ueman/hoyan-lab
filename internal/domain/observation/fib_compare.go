package observation

import "sort"

func CompareFIBEntries(expected, actual []FIBEntry) Result {
	return compareFIBEntries(expected, actual, fibRouteKey)
}

func CompareFIB(expected, actual FIB) Result {
	result := compareFIBEntries(expected.Entries, actual.Entries, fibTableRouteKey)
	compareFIBTableIdentity(expected, actual, &result)
	sort.Slice(result.Mismatched, func(i, j int) bool {
		return result.Mismatched[i].RouteKey+"|"+result.Mismatched[i].Field < result.Mismatched[j].RouteKey+"|"+result.Mismatched[j].Field
	})
	result.OK = result.OK && len(result.Mismatched) == 0
	return result
}

func CompareFIBs(expected, actual []FIB) Result {
	exp := map[string]FIB{}
	act := map[string]FIB{}
	for _, fib := range expected {
		exp[fib.Key()] = fib
	}
	for _, fib := range actual {
		act[fib.Key()] = fib
	}
	var result Result
	for _, key := range sortedFIBTableUnion(exp, act) {
		e, eok := exp[key]
		a, aok := act[key]
		switch {
		case !eok:
			for _, entry := range a.Entries {
				result.UnexpectedRoutes = append(result.UnexpectedRoutes, fibScopedRouteKey(a, entry))
			}
		case !aok:
			for _, entry := range e.Entries {
				result.MissingRoutes = append(result.MissingRoutes, fibScopedRouteKey(e, entry))
			}
		default:
			table := CompareFIB(e, a)
			result.DuplicateRouteConflicts = append(result.DuplicateRouteConflicts, scopeDuplicateConflicts(e, table.DuplicateRouteConflicts)...)
			result.MissingRoutes = append(result.MissingRoutes, scopeRouteKeys(e, table.MissingRoutes)...)
			result.UnexpectedRoutes = append(result.UnexpectedRoutes, scopeRouteKeys(a, table.UnexpectedRoutes)...)
			result.MissingNextHops = append(result.MissingNextHops, scopeNextHopDiffs(e, table.MissingNextHops)...)
			result.UnexpectedNextHops = append(result.UnexpectedNextHops, scopeNextHopDiffs(a, table.UnexpectedNextHops)...)
			result.Mismatched = append(result.Mismatched, scopeFIBMismatches(e, table.Mismatched)...)
		}
	}
	sort.Strings(result.MissingRoutes)
	sort.Strings(result.UnexpectedRoutes)
	sortNextHopDiffs(result.MissingNextHops)
	sortNextHopDiffs(result.UnexpectedNextHops)
	sort.Slice(result.Mismatched, func(i, j int) bool {
		return result.Mismatched[i].RouteKey+"|"+result.Mismatched[i].Field < result.Mismatched[j].RouteKey+"|"+result.Mismatched[j].Field
	})
	sortDuplicateRouteConflicts(result.DuplicateRouteConflicts)
	result.OK = len(result.MissingRoutes) == 0 &&
		len(result.UnexpectedRoutes) == 0 &&
		len(result.MissingNextHops) == 0 &&
		len(result.UnexpectedNextHops) == 0 &&
		len(result.DuplicateRouteConflicts) == 0 &&
		len(result.Mismatched) == 0 &&
		len(result.UnresolvedRoutes) == 0 &&
		len(result.UnsupportedNodes) == 0
	return result
}

func compareFIBEntries(expected, actual []FIBEntry, keyFunc func(FIBEntry) string) Result {
	expected, expConflicts := normalizeRoutesForSide("expected", expected, keyFunc)
	actual, actConflicts := normalizeRoutesForSide("actual", actual, keyFunc)
	exp := map[string]FIBEntry{}
	act := map[string]FIBEntry{}
	conflictedKeys := map[string]bool{}
	var result Result
	result.DuplicateRouteConflicts = append(result.DuplicateRouteConflicts, expConflicts...)
	result.DuplicateRouteConflicts = append(result.DuplicateRouteConflicts, actConflicts...)
	for _, conflict := range result.DuplicateRouteConflicts {
		conflictedKeys[conflict.RouteKey] = true
	}
	for _, route := range expected {
		exp[keyFunc(route)] = route
	}
	for _, route := range actual {
		act[keyFunc(route)] = route
	}
	keys := sortedUnion(exp, act)
	for _, key := range keys {
		if conflictedKeys[key] {
			continue
		}
		e, eok := exp[key]
		a, aok := act[key]
		switch {
		case !eok:
			result.UnexpectedRoutes = append(result.UnexpectedRoutes, key)
			continue
		case !aok:
			result.MissingRoutes = append(result.MissingRoutes, key)
			continue
		}
		compareNextHops(key, e.NextHops, a.NextHops, &result)
		if e.Preference != 0 && a.Preference != 0 && e.Preference != a.Preference {
			result.Mismatched = append(result.Mismatched, FIBAttributeMismatch{RouteKey: key, Field: "preference", Expected: e.Preference, Actual: a.Preference})
		}
	}
	sort.Strings(result.MissingRoutes)
	sort.Strings(result.UnexpectedRoutes)
	sortNextHopDiffs(result.MissingNextHops)
	sortNextHopDiffs(result.UnexpectedNextHops)
	sort.Slice(result.Mismatched, func(i, j int) bool {
		return result.Mismatched[i].RouteKey+"|"+result.Mismatched[i].Field < result.Mismatched[j].RouteKey+"|"+result.Mismatched[j].Field
	})
	sortDuplicateRouteConflicts(result.DuplicateRouteConflicts)
	result.OK = len(result.MissingRoutes) == 0 &&
		len(result.UnexpectedRoutes) == 0 &&
		len(result.MissingNextHops) == 0 &&
		len(result.UnexpectedNextHops) == 0 &&
		len(result.DuplicateRouteConflicts) == 0 &&
		len(result.Mismatched) == 0 &&
		len(result.UnresolvedRoutes) == 0 &&
		len(result.UnsupportedNodes) == 0
	return result
}

func compareNextHops(routeKey string, expected, actual []NextHop, result *Result) {
	exp := map[string]bool{}
	act := map[string]bool{}
	for _, hop := range expected {
		exp[fibNextHopKey(hop)] = true
	}
	for _, hop := range actual {
		act[fibNextHopKey(hop)] = true
	}
	for _, key := range sortedBoolUnion(exp, act) {
		switch {
		case !exp[key]:
			result.UnexpectedNextHops = append(result.UnexpectedNextHops, NextHopDiff{RouteKey: routeKey, NextHopKey: key})
		case !act[key]:
			result.MissingNextHops = append(result.MissingNextHops, NextHopDiff{RouteKey: routeKey, NextHopKey: key})
		}
	}
}

func sortedUnion(a, b map[string]FIBEntry) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedBoolUnion(a, b map[string]bool) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func sortedFIBTableUnion(a, b map[string]FIB) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func scopeRouteKeys(fib FIB, keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, fib.Key()+"|"+key)
	}
	return out
}

func scopeNextHopDiffs(fib FIB, diffs []NextHopDiff) []NextHopDiff {
	out := make([]NextHopDiff, 0, len(diffs))
	for _, diff := range diffs {
		diff.RouteKey = fib.Key() + "|" + diff.RouteKey
		out = append(out, diff)
	}
	return out
}

func scopeFIBMismatches(fib FIB, diffs []FIBAttributeMismatch) []FIBAttributeMismatch {
	out := make([]FIBAttributeMismatch, 0, len(diffs))
	for _, diff := range diffs {
		if diff.RouteKey != "table" {
			diff.RouteKey = fib.Key() + "|" + diff.RouteKey
		}
		out = append(out, diff)
	}
	return out
}

func scopeDuplicateConflicts(fib FIB, conflicts []DuplicateRouteConflict) []DuplicateRouteConflict {
	out := make([]DuplicateRouteConflict, 0, len(conflicts))
	for _, conflict := range conflicts {
		conflict.RouteKey = fib.Key() + "|" + conflict.RouteKey
		out = append(out, conflict)
	}
	return out
}

func sortNextHopDiffs(diffs []NextHopDiff) {
	sort.Slice(diffs, func(i, j int) bool {
		return diffs[i].RouteKey+"|"+diffs[i].NextHopKey < diffs[j].RouteKey+"|"+diffs[j].NextHopKey
	})
}

func compareFIBTableIdentity(expected, actual FIB, result *Result) {
	if expected.Node != actual.Node {
		result.Mismatched = append(result.Mismatched, FIBAttributeMismatch{
			RouteKey: "table",
			Field:    "node",
			Expected: expected.Node,
			Actual:   actual.Node,
		})
	}
	if expected.VRF != actual.VRF {
		result.Mismatched = append(result.Mismatched, FIBAttributeMismatch{
			RouteKey: "table",
			Field:    "vrf",
			Expected: expected.VRF,
			Actual:   actual.VRF,
		})
	}
}

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
	protocol := string(r.Source.Protocol)
	if protocol != "" && protocol != "bgp" {
		return string(r.AFI) + "|" + protocol + "|" + string(r.Action) + "|" + r.Prefix
	}
	return string(r.AFI) + "|" + string(r.Action) + "|" + r.Prefix
}

func fibTableRouteKey(r FIBEntry) string {
	r = normalizeFIBEntryForCompare(r)
	return string(r.AFI) + "|" + string(r.Source.Protocol) + "|" + string(r.Action) + "|" + r.Prefix
}

func RouteKey(r FIBEntry) string {
	return fibRouteKey(r)
}

func fibScopedRouteKey(fib FIB, entry FIBEntry) string {
	return fib.Key() + "|" + fibTableRouteKey(entry)
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
