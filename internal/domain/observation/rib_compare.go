package observation

import (
	"sort"
)

func CompareRoutes(expected []RIBRoute, actual []RIBRoute, opts CompareOptions) CompareResult {
	return compareRIBRoutes(expected, actual, opts, routeKey)
}

func CompareRIB(expected RIB, actual RIB, opts CompareOptions) CompareResult {
	result := compareRIBRoutes(comparableRIBRoutes(expected.Routes), comparableRIBRoutes(actual.Routes), opts, ribTableRouteKey)
	compareRIBTableIdentity(expected, actual, &result)
	sort.Slice(result.Mismatched, func(i, j int) bool {
		return mismatchSortKey(result.Mismatched[i]) < mismatchSortKey(result.Mismatched[j])
	})
	result.OK = result.OK && len(result.Mismatched) == 0
	return result
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

func compareRIBTableIdentity(expected, actual RIB, result *CompareResult) {
	if expected.Node != actual.Node {
		result.Mismatched = append(result.Mismatched, AttributeMismatch{
			RouteKey: "table",
			Field:    "node",
			Expected: expected.Node,
			Actual:   actual.Node,
		})
	}
	if expected.VRF != actual.VRF {
		result.Mismatched = append(result.Mismatched, AttributeMismatch{
			RouteKey: "table",
			Field:    "vrf",
			Expected: expected.VRF,
			Actual:   actual.VRF,
		})
	}
}
