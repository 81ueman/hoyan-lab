package observation

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type comparablePath struct {
	Best             bool
	Valid            bool
	NextHop          string
	ASPath           []uint32
	Origin           model.BGPOriginCode
	LocalPref        int
	MED              int
	Weight           int
	Communities      []string
	LargeCommunities []string
	OriginatorID     string
	ClusterList      []string
	Peer             string
	PeerAS           uint32
}

type CompareOptions struct {
	CompareBest             bool
	CompareValid            bool
	CompareNextHop          bool
	CompareASPath           bool
	CompareOrigin           bool
	CompareLocalPref        bool
	CompareMED              bool
	CompareWeight           bool
	CompareCommunities      bool
	CompareLargeCommunities bool
	CompareOriginatorID     bool
	CompareClusterList      bool
	ComparePeer             bool
	ComparePeerAS           bool
	AllowExtraPrefixes      bool
	AllowExtraPaths         bool
}

type PathDiff struct {
	RouteKey string
	PathKey  string
}

type AttributeMismatch struct {
	RouteKey string
	PathKey  string
	Field    string
	Expected any
	Actual   any
}

type DuplicatePathConflict struct {
	RouteKey string
	PathKey  string
	Side     string
	Paths    []comparablePath
}

type CompareResult struct {
	OK                     bool
	MissingPrefixes        []string
	UnexpectedPrefixes     []string
	MissingPaths           []PathDiff
	UnexpectedPaths        []PathDiff
	Mismatched             []AttributeMismatch
	DuplicatePathConflicts []DuplicatePathConflict
}

func DefaultCompareOptions() CompareOptions {
	return CompareOptions{
		CompareBest:      true,
		CompareValid:     true,
		CompareNextHop:   true,
		CompareASPath:    true,
		CompareOrigin:    true,
		CompareLocalPref: true,
		CompareMED:       true,
	}
}

func FormatDiffs(result CompareResult) []string {
	var out []string
	for _, k := range result.MissingPrefixes {
		out = append(out, fmt.Sprintf("[DIFF] %s prefix missing", k))
	}
	for _, k := range result.UnexpectedPrefixes {
		out = append(out, fmt.Sprintf("[DIFF] %s prefix unexpected", k))
	}
	for _, d := range result.MissingPaths {
		out = append(out, fmt.Sprintf("[DIFF] %s path %s missing", d.RouteKey, d.PathKey))
	}
	for _, d := range result.UnexpectedPaths {
		out = append(out, fmt.Sprintf("[DIFF] %s path %s unexpected", d.RouteKey, d.PathKey))
	}
	for _, m := range result.Mismatched {
		out = append(out, fmt.Sprintf("[DIFF] %s path %s field=%s expected=%v actual=%v", m.RouteKey, m.PathKey, m.Field, m.Expected, m.Actual))
	}
	for _, c := range result.DuplicatePathConflicts {
		out = append(out, fmt.Sprintf("[DIFF] %s path %s duplicate path conflict side=%s paths=%d", c.RouteKey, c.PathKey, c.Side, len(c.Paths)))
	}
	return out
}

func CompareRoutes(expected []RIBRoute, actual []RIBRoute, opts CompareOptions) CompareResult {
	return compareRIBRoutes(expected, actual, opts, routeKey)
}

func CompareRIB(expected RIB, actual RIB, opts CompareOptions) CompareResult {
	result := compareRIBRoutes(comparableRIBRoutes(expected.Routes), comparableRIBRoutes(actual.Routes), opts, ribTableRouteKey)
	compareRIBTableIdentity(expected, actual, &result)
	sortByKey(result.Mismatched, mismatchSortKey)
	result.OK = result.OK && len(result.Mismatched) == 0
	return result
}

func compareRIBRoutes(expected []RIBRoute, actual []RIBRoute, opts CompareOptions, keyFunc func(RIBRoute) string) CompareResult {
	opts = fillCompareDefaults(opts)
	exp := indexByValue(expected, keyFunc, normalizeRoute)
	act := indexByValue(actual, keyFunc, normalizeRoute)
	var result CompareResult
	walkSortedUnion(exp, act, func(key string, e RIBRoute, eok bool, a RIBRoute, aok bool) {
		switch {
		case !eok:
			if normalizeRoute(a).Common.Protocol != model.RouteSourceBGP {
				return
			}
			if !opts.AllowExtraPrefixes {
				result.UnexpectedPrefixes = append(result.UnexpectedPrefixes, key)
			}
			return
		case !aok:
			result.MissingPrefixes = append(result.MissingPrefixes, key)
			return
		}
		comparePaths(key, comparablePaths(e), comparablePaths(a), opts, &result)
	})
	sortPathDiffs(result.MissingPaths)
	sortPathDiffs(result.UnexpectedPaths)
	sortByKey(result.Mismatched, mismatchSortKey)
	sortByKey(result.DuplicatePathConflicts, duplicateConflictSortKey)
	result.OK = okIfNoDiffs(
		len(result.MissingPrefixes),
		len(result.UnexpectedPrefixes),
		len(result.MissingPaths),
		len(result.UnexpectedPaths),
		len(result.Mismatched),
		len(result.DuplicatePathConflicts),
	)
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

func fillCompareDefaults(opts CompareOptions) CompareOptions {
	if !opts.CompareBest && !opts.CompareValid && !opts.CompareNextHop && !opts.CompareASPath &&
		!opts.CompareOrigin && !opts.CompareLocalPref && !opts.CompareMED && !opts.CompareWeight &&
		!opts.CompareCommunities && !opts.CompareLargeCommunities && !opts.CompareOriginatorID &&
		!opts.CompareClusterList && !opts.ComparePeer && !opts.ComparePeerAS {
		return DefaultCompareOptions()
	}
	return opts
}

func comparePaths(routeKey string, expected, actual []comparablePath, opts CompareOptions, result *CompareResult) {
	exp, expConflicts := buildPathIndex(routeKey, "expected", expected, opts)
	act, actConflicts := buildPathIndex(routeKey, "actual", actual, opts)
	result.DuplicatePathConflicts = append(result.DuplicatePathConflicts, expConflicts...)
	result.DuplicatePathConflicts = append(result.DuplicatePathConflicts, actConflicts...)
	walkSortedUnion(exp, act, func(key string, e comparablePath, eok bool, a comparablePath, aok bool) {
		switch {
		case !eok:
			if !opts.AllowExtraPaths {
				result.UnexpectedPaths = append(result.UnexpectedPaths, PathDiff{RouteKey: routeKey, PathKey: key})
			}
		case !aok:
			result.MissingPaths = append(result.MissingPaths, PathDiff{RouteKey: routeKey, PathKey: key})
		default:
			appendMismatches(routeKey, key, e, a, opts, result)
		}
	})
}

type pathIndexEntry struct {
	path       comparablePath
	paths      []comparablePath
	conflicted bool
}

func buildPathIndex(routeKey, side string, paths []comparablePath, opts CompareOptions) (map[string]comparablePath, []DuplicatePathConflict) {
	entries := map[string]pathIndexEntry{}
	for _, p := range paths {
		p = normalizePath(p)
		key := pathKey(p, opts)
		entry, ok := entries[key]
		if !ok {
			entries[key] = pathIndexEntry{path: p, paths: []comparablePath{p}}
			continue
		}
		if !samePathAttributes(entry.path, p) {
			entry.conflicted = true
		}
		// The simulator can produce the same visible BGP path under multiple
		// failure conditions with different selected/valid states. Keep that as
		// a single visible path, but only when all non-state attributes match.
		entry.path.Best = entry.path.Best || p.Best
		entry.path.Valid = entry.path.Valid || p.Valid
		entry.paths = append(entry.paths, p)
		entries[key] = entry
	}

	index := map[string]comparablePath{}
	var conflicts []DuplicatePathConflict
	for key, entry := range entries {
		if entry.conflicted {
			conflicts = append(conflicts, DuplicatePathConflict{
				RouteKey: routeKey,
				PathKey:  key,
				Side:     side,
				Paths:    entry.paths,
			})
			continue
		}
		index[key] = entry.path
	}
	return index, conflicts
}

func samePathAttributes(a, b comparablePath) bool {
	a.Best = false
	a.Valid = false
	b.Best = false
	b.Valid = false
	return reflect.DeepEqual(a, b)
}

func appendMismatches(routeKey, pathKey string, e, a comparablePath, opts CompareOptions, result *CompareResult) {
	check := func(enabled bool, field string, expected, actual any) {
		if enabled && !reflect.DeepEqual(expected, actual) {
			result.Mismatched = append(result.Mismatched, AttributeMismatch{RouteKey: routeKey, PathKey: pathKey, Field: field, Expected: expected, Actual: actual})
		}
	}
	check(opts.CompareBest, "best", e.Best, a.Best)
	check(opts.CompareValid, "valid", e.Valid, a.Valid)
	check(opts.CompareNextHop, "next_hop", e.NextHop, a.NextHop)
	check(opts.CompareASPath, "as_path", e.ASPath, a.ASPath)
	check(opts.CompareOrigin, "origin", e.Origin, a.Origin)
	check(opts.CompareLocalPref, "local_pref", e.LocalPref, a.LocalPref)
	check(opts.CompareMED, "med", e.MED, a.MED)
	check(opts.CompareWeight, "weight", e.Weight, a.Weight)
	check(opts.CompareCommunities, "communities", e.Communities, a.Communities)
	check(opts.CompareLargeCommunities, "large_communities", e.LargeCommunities, a.LargeCommunities)
	check(opts.CompareOriginatorID, "originator_id", e.OriginatorID, a.OriginatorID)
	check(opts.CompareClusterList, "cluster_list", e.ClusterList, a.ClusterList)
	check(opts.ComparePeer, "peer", e.Peer, a.Peer)
	check(opts.ComparePeerAS, "peer_as", e.PeerAS, a.PeerAS)
}

func normalizeRoute(r RIBRoute) RIBRoute {
	r.Common.AFI = model.NormalizeAFI(r.Common.AFI)
	r.Common.Protocol = model.NormalizeRouteSourceKind(r.Common.Protocol)
	// Explicit empty→IPv4 check removed (was redundant with NormalizeAFI).
	// NormalizeAFI still normalizes empty→IPv4; old snapshots with missing AFI
	// are migrated at file-load boundary via migrateSnapshotForCompare
	// (snapshot_collector.go), not silently hidden during comparison.
	return r
}

func comparableRIBRoutes(routes []RIBRoute) []RIBRoute {
	out := make([]RIBRoute, 0, len(routes))
	for _, route := range routes {
		out = append(out, comparableRIBRoute(route))
	}
	return out
}

func comparableRIBRoute(route RIBRoute) RIBRoute {
	return normalizeRoute(route)
}

func comparablePaths(route RIBRoute) []comparablePath {
	switch {
	case route.BGP != nil:
		return comparablePathsFromBGPPaths(route.BGP.Paths)
	case route.OSPF != nil:
		return comparablePathsFromOSPFPaths(route.OSPF.Paths)
	case route.Static != nil:
		return comparablePathsFromNextHops(route.Static.NextHops)
	case route.Connected != nil, route.Blackhole != nil:
		return []comparablePath{{Best: route.Common.Best, Valid: route.Common.Eligible, Origin: model.BGPOriginIGP, LocalPref: model.DefaultLocalPreference}}
	default:
		return nil
	}
}

func comparablePathsFromBGPPaths(paths []BGPPath) []comparablePath {
	out := make([]comparablePath, 0, len(paths))
	for _, path := range paths {
		out = append(out, comparablePath{
			NextHop:          path.NextHop.Address,
			ASPath:           append([]uint32(nil), path.ASPath...),
			Origin:           path.Origin,
			LocalPref:        path.LocalPref,
			MED:              path.MED,
			Weight:           path.Weight,
			Communities:      append([]string(nil), path.Communities...),
			LargeCommunities: append([]string(nil), path.LargeCommunities...),
			OriginatorID:     path.OriginatorID,
			ClusterList:      append([]string(nil), path.ClusterList...),
			Peer:             path.Peer,
			PeerAS:           path.PeerAS,
			Valid:            path.Eligible,
			Best:             path.Best,
		})
	}
	return out
}

func comparablePathsFromOSPFPaths(paths []OSPFPath) []comparablePath {
	out := make([]comparablePath, 0, len(paths))
	for _, path := range paths {
		out = append(out, comparablePath{
			Best:      true,
			Valid:     true,
			NextHop:   path.NextHop.Address,
			Origin:    model.BGPOriginIGP,
			LocalPref: model.DefaultLocalPreference,
			MED:       path.Cost,
		})
	}
	return out
}

func comparablePathsFromNextHops(hops []NextHop) []comparablePath {
	if len(hops) == 0 {
		return []comparablePath{{Best: true, Valid: true, Origin: model.BGPOriginIGP, LocalPref: model.DefaultLocalPreference}}
	}
	out := make([]comparablePath, 0, len(hops))
	for _, hop := range hops {
		out = append(out, comparablePath{
			Best:      true,
			Valid:     true,
			NextHop:   hop.Address,
			Origin:    model.BGPOriginIGP,
			LocalPref: model.DefaultLocalPreference,
			Weight:    hop.Weight,
		})
	}
	return out
}

func normalizePath(p comparablePath) comparablePath {
	p.Origin = normalizeOrigin(p.Origin)
	p.Communities = sortedStrings(p.Communities)
	p.LargeCommunities = sortedStrings(p.LargeCommunities)
	p.ClusterList = sortedStrings(p.ClusterList)
	return p
}

func sortedStrings(xs []string) []string {
	out := append([]string(nil), xs...)
	sort.Strings(out)
	return out
}

func routeKey(r RIBRoute) string {
	r = normalizeRoute(r)
	protocol := string(r.Common.Protocol)
	if protocol != "" && protocol != "bgp" {
		return string(r.Common.AFI) + "|" + protocol + "|" + r.Common.Prefix
	}
	return string(r.Common.AFI) + "|" + r.Common.Prefix
}

func ribTableRouteKey(r RIBRoute) string {
	r = normalizeRoute(r)
	return string(r.Common.AFI) + "|" + string(r.Common.Protocol) + "|" + r.Common.Prefix
}

func pathKey(p comparablePath, opts CompareOptions) string {
	// Path identity is deliberately narrower than full path equality. The
	// default identity is next-hop plus AS path; attributes such as best, valid,
	// origin, local-pref, MED, weight, communities, originator ID, and cluster
	// list are compared after identity matching so attribute mismatches stay
	// distinct from missing/unexpected paths. ComparePeer and ComparePeerAS are
	// the only options that extend identity, letting callers distinguish
	// otherwise identical multipath entries learned from different peers.
	parts := []string{"nh=" + p.NextHop, "as=" + formatASPath(p.ASPath)}
	if opts.ComparePeer && p.Peer != "" {
		parts = append(parts, "peer="+p.Peer)
	}
	if opts.ComparePeerAS && p.PeerAS != 0 {
		parts = append(parts, fmt.Sprintf("peer_as=%d", p.PeerAS))
	}
	return strings.Join(parts, "|")
}

func formatASPath(path []uint32) string {
	parts := make([]string, 0, len(path))
	for _, asn := range path {
		parts = append(parts, fmt.Sprint(asn))
	}
	return strings.Join(parts, " ")
}

func normalizeOrigin(origin model.BGPOriginCode) model.BGPOriginCode {
	switch strings.ToLower(strings.TrimSpace(string(origin))) {
	case "", "i", "igp":
		return model.BGPOriginIGP
	case "e", "egp":
		return model.BGPOriginEGP
	case "?", "incomplete":
		return model.BGPOriginIncomplete
	default:
		return model.NormalizeBGPOriginCode(origin)
	}
}

func DefaultLocalPref(v int) int {
	if v == 0 {
		return model.DefaultLocalPreference
	}
	return v
}

func sortRoutes(routes []RIBRoute) {
	sort.Slice(routes, func(i, j int) bool {
		return routeKey(routes[i]) < routeKey(routes[j])
	})
}

func SortRoutes(routes []RIBRoute) {
	sortRoutes(routes)
}

func SortBGPPaths(paths []BGPPath, opts CompareOptions) {
	sort.Slice(paths, func(i, j int) bool {
		return pathKey(comparablePathsFromBGPPaths([]BGPPath{paths[i]})[0], opts) <
			pathKey(comparablePathsFromBGPPaths([]BGPPath{paths[j]})[0], opts)
	})
}

func SortOSPFPaths(paths []OSPFPath, opts CompareOptions) {
	sort.Slice(paths, func(i, j int) bool {
		return pathKey(comparablePathsFromOSPFPaths([]OSPFPath{paths[i]})[0], opts) <
			pathKey(comparablePathsFromOSPFPaths([]OSPFPath{paths[j]})[0], opts)
	})
}

func sortPathDiffs(diffs []PathDiff) {
	sortByKey(diffs, func(diff PathDiff) string {
		return diff.RouteKey + "|" + diff.PathKey
	})
}

func mismatchSortKey(m AttributeMismatch) string {
	return m.RouteKey + "|" + m.PathKey + "|" + m.Field
}

func duplicateConflictSortKey(c DuplicatePathConflict) string {
	return c.RouteKey + "|" + c.PathKey + "|" + c.Side
}
