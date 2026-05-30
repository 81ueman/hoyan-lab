package rib

import (
	"sort"
)

func sortRoutes(routes []NormalizedRoute) {
	sort.Slice(routes, func(i, j int) bool {
		return routeKey(routes[i]) < routeKey(routes[j])
	})
}

func SortRoutes(routes []NormalizedRoute) {
	sortRoutes(routes)
}

func sortPaths(paths []NormalizedPath, opts CompareOptions) {
	sort.Slice(paths, func(i, j int) bool {
		return pathKey(paths[i], opts) < pathKey(paths[j], opts)
	})
}

func SortPaths(paths []NormalizedPath, opts CompareOptions) {
	sortPaths(paths, opts)
}

func sortPathDiffs(diffs []PathDiff) {
	sort.Slice(diffs, func(i, j int) bool {
		if diffs[i].RouteKey == diffs[j].RouteKey {
			return diffs[i].PathKey < diffs[j].PathKey
		}
		return diffs[i].RouteKey < diffs[j].RouteKey
	})
}

func sortedUnionKeys[A any, B any](a map[string]A, b map[string]B) []string {
	seen := map[string]bool{}
	for k := range a {
		seen[k] = true
	}
	for k := range b {
		seen[k] = true
	}
	var out []string
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func mismatchSortKey(m AttributeMismatch) string {
	return m.RouteKey + "|" + m.PathKey + "|" + m.Field
}

func duplicateConflictSortKey(c DuplicatePathConflict) string {
	return c.RouteKey + "|" + c.PathKey + "|" + c.Side
}
