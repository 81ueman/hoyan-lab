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
