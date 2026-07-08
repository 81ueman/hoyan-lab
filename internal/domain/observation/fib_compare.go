package observation

import "sort"

func CompareFIBEntries(expected, actual []FIBEntry) Result {
	return compareFIBEntries(expected, actual, fibRouteKey)
}

func CompareFIB(expected, actual FIB) Result {
	result := compareFIBEntries(expected.Entries, actual.Entries, fibTableRouteKey)
	compareFIBTableIdentity(expected, actual, &result)
	sortByKey(result.Mismatched, fibMismatchSortKey)
	result.OK = result.OK && len(result.Mismatched) == 0
	return result
}

func CompareFIBs(expected, actual []FIB) Result {
	exp := indexBy(expected, FIB.Key)
	act := indexBy(actual, FIB.Key)
	var result Result
	walkSortedUnion(exp, act, func(key string, e FIB, eok bool, a FIB, aok bool) {
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
	})
	sort.Strings(result.MissingRoutes)
	sort.Strings(result.UnexpectedRoutes)
	sortNextHopDiffs(result.MissingNextHops)
	sortNextHopDiffs(result.UnexpectedNextHops)
	sortByKey(result.Mismatched, fibMismatchSortKey)
	sortDuplicateRouteConflicts(result.DuplicateRouteConflicts)
	result.OK = fibResultOK(result)
	return result
}

func compareFIBEntries(expected, actual []FIBEntry, keyFunc func(FIBEntry) string) Result {
	expected, expConflicts := normalizeRoutesForSide("expected", expected, keyFunc)
	actual, actConflicts := normalizeRoutesForSide("actual", actual, keyFunc)
	conflictedKeys := map[string]bool{}
	var result Result
	result.DuplicateRouteConflicts = append(result.DuplicateRouteConflicts, expConflicts...)
	result.DuplicateRouteConflicts = append(result.DuplicateRouteConflicts, actConflicts...)
	for _, conflict := range result.DuplicateRouteConflicts {
		conflictedKeys[conflict.RouteKey] = true
	}
	exp := indexBy(expected, keyFunc)
	act := indexBy(actual, keyFunc)
	walkSortedUnion(exp, act, func(key string, e FIBEntry, eok bool, a FIBEntry, aok bool) {
		if conflictedKeys[key] {
			return
		}
		switch {
		case !eok:
			result.UnexpectedRoutes = append(result.UnexpectedRoutes, key)
			return
		case !aok:
			result.MissingRoutes = append(result.MissingRoutes, key)
			return
		}
		compareNextHops(key, e.NextHops, a.NextHops, &result)
		// Only preference is compared here. New optional metadata fields (SAFI,
		// TableID, TableName, ProtocolInstance, Age, AgeSeconds, Tag,
		// InstalledReason, Raw) are intentionally excluded from FIB entry
		// comparison to avoid noisy diffs when one source populates them
		// and another does not. They remain available for callers that
		// want to inspect them via the raw structs.
		if e.Preference != 0 && a.Preference != 0 && e.Preference != a.Preference {
			result.Mismatched = append(result.Mismatched, FIBAttributeMismatch{RouteKey: key, Field: "preference", Expected: e.Preference, Actual: a.Preference})
		}
	})
	sort.Strings(result.MissingRoutes)
	sort.Strings(result.UnexpectedRoutes)
	sortNextHopDiffs(result.MissingNextHops)
	sortNextHopDiffs(result.UnexpectedNextHops)
	sortByKey(result.Mismatched, fibMismatchSortKey)
	sortDuplicateRouteConflicts(result.DuplicateRouteConflicts)
	result.OK = fibResultOK(result)
	return result
}

func compareNextHops(routeKey string, expected, actual []NextHop, result *Result) {
	exp := indexByValue(expected, fibNextHopKey, func(NextHop) bool { return true })
	act := indexByValue(actual, fibNextHopKey, func(NextHop) bool { return true })
	walkSortedUnion(exp, act, func(key string, _ bool, eok bool, _ bool, aok bool) {
		switch {
		case !eok:
			result.UnexpectedNextHops = append(result.UnexpectedNextHops, NextHopDiff{RouteKey: routeKey, NextHopKey: key})
		case !aok:
			result.MissingNextHops = append(result.MissingNextHops, NextHopDiff{RouteKey: routeKey, NextHopKey: key})
		}
	})
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
	sortByKey(diffs, func(diff NextHopDiff) string {
		return diff.RouteKey + "|" + diff.NextHopKey
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
	sortStableByKey(routes, fibRouteKey)
	for i := range routes {
		sortNextHops(routes[i].NextHops)
	}
}

func SortFIBEntriesForCompare(routes []FIBEntry) {
	sortFIBEntriesForCompare(routes)
}

func sortNextHops(hops []NextHop) {
	sortStableByKey(hops, fibNextHopKey)
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
	sortStableByKey(routes, unresolvedRouteSortKey)
}

func unresolvedRouteSortKey(route UnresolvedRoute) string {
	return route.RouteKey + "|" + route.Reason
}

func sortDuplicateRouteConflicts(conflicts []DuplicateRouteConflict) {
	sortStableByKey(conflicts, duplicateRouteConflictSortKey)
}

func duplicateRouteConflictSortKey(conflict DuplicateRouteConflict) string {
	return conflict.RouteKey + "|" + conflict.Side + "|" + conflict.Reason
}

func fibMismatchSortKey(diff FIBAttributeMismatch) string {
	return diff.RouteKey + "|" + diff.Field
}

func fibResultOK(result Result) bool {
	return okIfNoDiffs(
		len(result.MissingRoutes),
		len(result.UnexpectedRoutes),
		len(result.MissingNextHops),
		len(result.UnexpectedNextHops),
		len(result.DuplicateRouteConflicts),
		len(result.Mismatched),
		len(result.UnresolvedRoutes),
		len(result.UnsupportedNodes),
	)
}
