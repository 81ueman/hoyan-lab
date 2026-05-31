package observation

import (
	"sort"
)

func CompareRoutes(expected []RIBRoute, actual []RIBRoute, opts CompareOptions) CompareResult {
	return compareRIBRoutes(expected, actual, opts, routeKey)
}

func CompareRIB(expected RIB, actual RIB, opts CompareOptions) CompareResult {
	return compareRIBRoutes(comparableRIBRoutes(expected.Routes), comparableRIBRoutes(actual.Routes), opts, ribTableRouteKey)
}

func compareRIBRoutes(expected []RIBRoute, actual []RIBRoute, opts CompareOptions, keyFunc func(RIBRoute) string) CompareResult {
	opts = fillCompareDefaults(opts)
	exp := map[string]RIBRoute{}
	act := map[string]RIBRoute{}
	for _, r := range expected {
		exp[keyFunc(r)] = normalizeRoute(r)
	}
	for _, r := range actual {
		act[keyFunc(r)] = normalizeRoute(r)
	}
	keys := sortedUnionKeys(exp, act)
	var result CompareResult
	for _, key := range keys {
		e, eok := exp[key]
		a, aok := act[key]
		switch {
		case !eok:
			if normalizeRoute(a).Protocol != "bgp" {
				continue
			}
			if !opts.AllowExtraPrefixes {
				result.UnexpectedPrefixes = append(result.UnexpectedPrefixes, key)
			}
			continue
		case !aok:
			result.MissingPrefixes = append(result.MissingPrefixes, key)
			continue
		}
		comparePaths(key, e.Paths, a.Paths, opts, &result)
	}
	sortPathDiffs(result.MissingPaths)
	sortPathDiffs(result.UnexpectedPaths)
	sort.Slice(result.Mismatched, func(i, j int) bool {
		return mismatchSortKey(result.Mismatched[i]) < mismatchSortKey(result.Mismatched[j])
	})
	sort.Slice(result.DuplicatePathConflicts, func(i, j int) bool {
		return duplicateConflictSortKey(result.DuplicatePathConflicts[i]) < duplicateConflictSortKey(result.DuplicatePathConflicts[j])
	})
	result.OK = len(result.MissingPrefixes) == 0 &&
		len(result.UnexpectedPrefixes) == 0 &&
		len(result.MissingPaths) == 0 &&
		len(result.UnexpectedPaths) == 0 &&
		len(result.Mismatched) == 0 &&
		len(result.DuplicatePathConflicts) == 0
	return result
}

func Compare(expected RIB, actual RIB) CompareResult {
	return CompareRIB(expected, actual, DefaultCompareOptions())
}
