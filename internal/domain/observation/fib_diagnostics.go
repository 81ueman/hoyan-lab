package observation

func CompareFilterResults(expected, actual FilterResult, opts Options) Result {
	policy := opts.UnresolvedPolicy.normalized()
	expectedFIBs := expected.FIBs
	actualFIBs := actual.FIBs
	if policy == UnresolvedPolicyWarn || policy == UnresolvedPolicyIgnore {
		expectedFIBs = removeFIBRoutesByKey(expectedFIBs, unresolvedRouteKeys(actual.Unresolved))
		actualFIBs = removeFIBRoutesByKey(actualFIBs, unresolvedRouteKeys(actual.Unresolved))
	}
	result := CompareFIBs(expectedFIBs, actualFIBs)
	if policy == UnresolvedPolicyFail {
		result.UnresolvedRoutes = append(result.UnresolvedRoutes, actual.Unresolved...)
		sortUnresolvedRoutes(result.UnresolvedRoutes)
		result.OK = false
	}
	return result
}

func WarningDiagnostics(result FilterResult, opts Options) []UnresolvedRoute {
	if opts.UnresolvedPolicy.normalized() != UnresolvedPolicyWarn {
		return nil
	}
	return result.Unresolved
}

func removeFIBRoutesByKey(fibs []FIB, keys map[string]bool) []FIB {
	if len(keys) == 0 {
		return fibs
	}
	out := make([]FIB, 0, len(fibs))
	for _, fib := range fibs {
		filtered := FIB{Node: fib.Node, VRF: fib.VRF}
		for _, route := range fib.Entries {
			if keys[fibScopedRouteKey(fib, route)] {
				continue
			}
			filtered.Entries = append(filtered.Entries, route)
		}
		out = append(out, filtered)
	}
	return out
}

func unresolvedRouteKeys(routes []UnresolvedRoute) map[string]bool {
	out := map[string]bool{}
	for _, route := range routes {
		out[route.RouteKey] = true
	}
	return out
}
