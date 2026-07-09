package intent

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

// ---------------------------------------------------------------------------
// RIBEq: compare two snapshots' RIBs
// ---------------------------------------------------------------------------

func evalRIBEq(e *RIBEqExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	// Load left and right snapshots
	leftSnap, err := loadSnapshot(e.Left, ctx)
	if err != nil {
		return "fail", Actual{Reason: fmt.Sprintf("load left snapshot %q: %v", e.Left, err)}
	}
	rightSnap, err := loadSnapshot(e.Right, ctx)
	if err != nil {
		return "fail", Actual{Reason: fmt.Sprintf("load right snapshot %q: %v", e.Right, err)}
	}

	// Get matching rows from both snapshots
	leftRows, err := matchingRows(leftSnap, e.Where, rowFilter)
	if err != nil {
		return "fail", Actual{Reason: fmt.Sprintf("rib_eq left where: %v", err)}
	}
	rightRows, err := matchingRows(rightSnap, e.Where, rowFilter)
	if err != nil {
		return "fail", Actual{Reason: fmt.Sprintf("rib_eq right where: %v", err)}
	}

	// Build prefix→entry maps for comparison
	leftIndex := buildRIBIndex(leftRows)
	rightIndex := buildRIBIndex(rightRows)

	var added, removed, changed []string

	// Find added and changed in right
	for key, rightRoute := range rightIndex {
		leftRoute, exists := leftIndex[key]
		if !exists {
			added = append(added, key)
			continue
		}
		if !routesEqual(leftRoute, rightRoute) {
			changed = append(changed, key)
		}
	}
	// Find removed
	for key := range leftIndex {
		if _, exists := rightIndex[key]; !exists {
			removed = append(removed, key)
		}
	}

	actual := Actual{
		AddedCount:   len(added),
		RemovedCount: len(removed),
		ChangedCount: len(changed),
	}
	for _, k := range added {
		actual.AddedRows = append(actual.AddedRows, k)
	}
	for _, k := range removed {
		actual.RemovedRows = append(actual.RemovedRows, k)
	}
	for _, k := range changed {
		actual.ChangedRows = append(actual.ChangedRows, k)
	}

	if len(added) == 0 && len(removed) == 0 && len(changed) == 0 {
		return "pass", actual
	}
	return "fail", actual
}

// buildRIBIndex creates a map from route key to route for quick comparison.
func buildRIBIndex(rows []routeRow) map[string]observation.RIBRoute {
	idx := map[string]observation.RIBRoute{}
	for _, r := range rows {
		key := ribRouteKey(r.route, r.rib)
		if _, ok := idx[key]; !ok {
			idx[key] = r.route
		}
	}
	return idx
}

// ribRouteKey builds a unique key for a RIB route across the snapshot.
func ribRouteKey(route observation.RIBRoute, rib observation.RIB) string {
	return string(rib.Node) + "|" + string(rib.VRF) + "|" + string(route.Common.AFI) + "|" + string(route.Common.Protocol) + "|" + route.Common.Prefix
}

// routesEqual compares two RIB routes for equality of key attributes.
func routesEqual(a, b observation.RIBRoute) bool {
	return a.Common.Prefix == b.Common.Prefix &&
		a.Common.Protocol == b.Common.Protocol &&
		a.Common.AFI == b.Common.AFI &&
		a.Common.VRF == b.Common.VRF &&
		a.Common.Metric == b.Common.Metric &&
		a.Common.Preference == b.Common.Preference &&
		a.Common.Eligible == b.Common.Eligible &&
		a.Common.Best == b.Common.Best &&
		reflect.DeepEqual(a.BGP, b.BGP) &&
		reflect.DeepEqual(a.OSPF, b.OSPF) &&
		reflect.DeepEqual(a.Static, b.Static) &&
		reflect.DeepEqual(a.Connected, b.Connected) &&
		reflect.DeepEqual(a.Blackhole, b.Blackhole)
}

// ---------------------------------------------------------------------------
// RIBEval: evaluate aggregate on RIB routes matching where
// ---------------------------------------------------------------------------

func evalRIBEval(e *RIBEvalExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	// Resolve snapshot: use e.Snapshot if specified and different from scenario default
	snap := snapshot
	if e.Snapshot != "" && e.Snapshot != scenario.Snapshot {
		var err error
		snap, err = loadSnapshot(e.Snapshot, ctx)
		if err != nil {
			return "fail", Actual{Reason: fmt.Sprintf("load snapshot %q: %v", e.Snapshot, err)}
		}
	}

	rows, err := matchingRows(snap, e.Where, rowFilter)
	if err != nil {
		return "fail", Actual{Reason: fmt.Sprintf("rib_eval where: %v", err)}
	}
	count := len(rows)

	agg, err := ParseAggregate(e.Aggregate)
	if err != nil {
		return "fail", Actual{Reason: fmt.Sprintf("parse aggregate: %v", err)}
	}

	actual := Actual{Count: count}
	var pass bool

	switch agg.Name {
	case "count":
		pass = compareInt(e, count)

	case "distCnt":
		distinct := distinctFieldValues(rows, agg.Field)
		actual.DistinctCount = len(distinct)
		pass = compareInt(e, len(distinct))

	case "distVals":
		values := distinctFieldValues(rows, agg.Field)
		actual.Values = values
		pass = compareValues(e, values)
	}

	if pass {
		return "pass", actual
	}
	return "fail", actual
}

// compareInt checks an integer aggregate value against comparison operators.
func compareInt(e *RIBEvalExpr, val int) bool {
	if e.Eq != nil {
		for _, v := range e.Eq {
			if toInt(v) == val {
				return true
			}
		}
		return false
	}
	if e.Ne != nil {
		for _, v := range e.Ne {
			if toInt(v) == val {
				return false
			}
		}
		return true
	}
	if e.Gt != nil && val > *e.Gt {
		return true
	}
	if e.Gte != nil && val >= *e.Gte {
		return true
	}
	if e.Lt != nil && val < *e.Lt {
		return true
	}
	if e.Lte != nil && val <= *e.Lte {
		return true
	}
	return false
}

// compareValues checks a distinct-values aggregate against comparison operators.
func compareValues(e *RIBEvalExpr, values []any) bool {
	if e.Eq != nil {
		for _, expected := range e.Eq {
			if sliceEqualAny(values, expected) {
				return true
			}
		}
		return false
	}
	if e.Ne != nil {
		for _, forbidden := range e.Ne {
			if sliceEqualAny(values, forbidden) {
				return false
			}
		}
		return true
	}
	return false
}

// sliceEqualAny checks if a slice of values is equal to an expected value,
// supporting both []any and string comparisons.
func sliceEqualAny(values []any, expected any) bool {
	switch e := expected.(type) {
	case []any:
		if len(values) != len(e) {
			return false
		}
		for i := range values {
			if fmt.Sprintf("%v", values[i]) != fmt.Sprintf("%v", e[i]) {
				return false
			}
		}
		return true
	default:
		if len(values) == 1 {
			return fmt.Sprintf("%v", values[0]) == fmt.Sprintf("%v", expected)
		}
		return false
	}
}

// distinctFieldValues returns distinct values of a field from matching route rows.
func distinctFieldValues(rows []routeRow, field string) []any {
	seen := map[string]bool{}
	var out []any
	for _, r := range rows {
		v := routeFieldValue(r.route, field)
		key := fmt.Sprintf("%v", v)
		if !seen[key] {
			seen[key] = true
			out = append(out, v)
		}
	}
	return out
}

// routeFieldValue extracts a field value from a RIB route for aggregation.
// Supported fields: nexthop, local_pref, localPref, protocol, as_path, asPath,
// metric, preference, eligible, best, device, node, vrf,
// route_type, area, cost.
func routeFieldValue(route observation.RIBRoute, field string) any {
	switch strings.ToLower(field) {
	case "nexthop", "next_hop":
		return extractNextHops(route)
	case "local_pref", "localpref":
		if route.BGP != nil && len(route.BGP.Paths) > 0 {
			return route.BGP.Paths[0].LocalPref
		}
		return 0
	case "protocol":
		return string(route.Common.Protocol)
	case "as_path", "aspath":
		if route.BGP != nil && len(route.BGP.Paths) > 0 {
			return fmt.Sprintf("%v", route.BGP.Paths[0].ASPath)
		}
		return ""
	case "as_path_len", "aspath_len":
		return routeASPathLen(route)
	case "metric":
		return route.Common.Metric
	case "preference":
		return route.Common.Preference
	case "eligible":
		return route.Common.Eligible
	case "best":
		return route.Common.Best
	case "communities":
		if route.BGP != nil && len(route.BGP.Paths) > 0 {
			return route.BGP.Paths[0].Communities
		}
		return []string{}
	case "weight":
		if route.BGP != nil && len(route.BGP.Paths) > 0 {
			return route.BGP.Paths[0].Weight
		}
		return 0
	case "route_type":
		if route.OSPF != nil {
			return string(route.OSPF.RouteType)
		}
		return ""
	case "area":
		if route.OSPF != nil {
			return route.OSPF.Area
		}
		return ""
	case "cost":
		if route.OSPF != nil && len(route.OSPF.Paths) > 0 {
			return route.OSPF.Paths[0].Cost
		}
		return 0
	case "origin":
		if route.BGP != nil && len(route.BGP.Paths) > 0 {
			return string(route.BGP.Paths[0].Origin)
		}
		return ""
	case "med":
		if route.BGP != nil && len(route.BGP.Paths) > 0 {
			return route.BGP.Paths[0].MED
		}
		return 0
	case "large_communities", "largecommunities":
		if route.BGP != nil && len(route.BGP.Paths) > 0 {
			return route.BGP.Paths[0].LargeCommunities
		}
		return []string{}
	default:
		return fmt.Sprintf("%v", field)
	}
}

func extractNextHops(route observation.RIBRoute) string {
	var hops []string
	switch {
	case route.BGP != nil:
		for _, p := range route.BGP.Paths {
			if p.NextHop.Address != "" {
				hops = append(hops, p.NextHop.Address)
			}
		}
	case route.OSPF != nil:
		for _, p := range route.OSPF.Paths {
			if p.NextHop.Address != "" {
				hops = append(hops, p.NextHop.Address)
			}
		}
	case route.Static != nil:
		for _, nh := range route.Static.NextHops {
			if nh.Address != "" {
				hops = append(hops, nh.Address)
			}
		}
	}
	return strings.Join(hops, ",")
}
