package intent

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// GuardExpr: p ⇒ g
// ---------------------------------------------------------------------------

func evalGuard(g *GuardExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	matching, err := matchingRows(snapshot, g.Where, rowFilter)
	if err != nil {
		return "fail", Actual{Reason: fmt.Sprintf("guard where: %v", err)}
	}
	if len(matching) == 0 {
		// Premise false → pass (vacuously true)
		// Warning: no rows matched — user should check their where predicates
		return "pass", Actual{Count: 0, Warning: "WARNING: no rows matched the guard condition — check your where predicates"}
	}
	// Evaluate inner intent with combined filter (guard's where AND inherited rowFilter)
	combined := mergeWhereFilters(rowFilter, g.Where)
	return evalRCLExpr(&g.Intent, snapshot, combined, scenario, ctx)
}

// ---------------------------------------------------------------------------
// ForallExpr
// ---------------------------------------------------------------------------

func evalForall(f *ForallExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	// Determine iteration values
	var values []string
	if len(f.In) > 0 {
		values = f.In
	} else {
		// No explicit list: collect all distinct values of the variable from the snapshot
		var err error
		values, err = collectDistinctValues(snapshot, f.Var)
		if err != nil {
			return "fail", Actual{Reason: fmt.Sprintf("forall: %v", err)}
		}
	}

	if len(values) == 0 {
		// No values to iterate over → vacuous pass (matching SQL/SQL-like forall over empty set)
		return "pass", Actual{Count: 0}
	}

	overall := "pass"
	var actuals []Actual
	var failingGroup string // tracks first failing iteration's group info for Result.Group
	for _, v := range values {
		// Create a forall-binding filter: the forall variable is bound to this value.
		// This is used as a rowFilter so that inner RIBEval/RIBEq expressions only see
		// rows matching the current forall value.
		forallFilter := map[string]any{f.Var: v}
		combined := mergeWhereFilters(rowFilter, forallFilter)
		status, a := evalRCLExpr(&f.Intent, snapshot, combined, scenario, ctx)
		if status == "fail" {
			overall = "fail"
			if failingGroup == "" {
				failingGroup = fmt.Sprintf("%s=%s", f.Var, v)
			}
		}
		a.Reason = fmt.Sprintf("%s=%s: %s", f.Var, v, a.Reason)
		actuals = append(actuals, a)
	}

	actual := Actual{}
	if len(actuals) > 0 {
		actual.Count = len(actuals)
	}
	if failingGroup != "" {
		actual.Reason = failingGroup
	}
	return overall, actual
}

// collectDistinctValues extracts all distinct values for a given field from
// the snapshot's RIB routes. The field can be "device" (node name), "vrf",
// or "protocol". Returns an error if the field name is not recognized.
func collectDistinctValues(snapshot SnapshotContext, field string) ([]string, error) {
	seen := map[string]bool{}
	var values []string

	switch strings.ToLower(field) {
	case "device", "node":
		for _, node := range snapshot.Network.Nodes {
			v := string(node.Node)
			if !seen[v] {
				seen[v] = true
				values = append(values, v)
			}
		}
	case "vrf":
		for _, node := range snapshot.Network.Nodes {
			for _, vrf := range node.VRFs {
				v := string(vrf.VRF)
				if !seen[v] {
					seen[v] = true
					values = append(values, v)
				}
			}
		}
	case "protocol":
		for _, rib := range RIBs(snapshot.Network) {
			for _, route := range rib.Routes {
				v := string(route.Common.Protocol)
				if !seen[v] {
					seen[v] = true
					values = append(values, v)
				}
			}
		}
	case "route_type":
		for _, rib := range RIBs(snapshot.Network) {
			for _, route := range rib.Routes {
				if route.OSPF == nil {
					continue
				}
				v := string(route.OSPF.RouteType)
				if v != "" && !seen[v] {
					seen[v] = true
					values = append(values, v)
				}
			}
		}
	case "area":
		for _, rib := range RIBs(snapshot.Network) {
			for _, route := range rib.Routes {
				if route.OSPF == nil {
					continue
				}
				v := string(route.OSPF.Area)
				if v != "" && !seen[v] {
					seen[v] = true
					values = append(values, v)
				}
			}
		}
	case "origin":
		for _, rib := range RIBs(snapshot.Network) {
			for _, route := range rib.Routes {
				if route.BGP == nil || len(route.BGP.Paths) == 0 {
					continue
				}
				v := string(route.BGP.Paths[0].Origin)
				if v != "" && !seen[v] {
					seen[v] = true
					values = append(values, v)
				}
			}
		}
	case "connected_class":
		for _, rib := range RIBs(snapshot.Network) {
			for _, route := range rib.Routes {
				if route.Connected == nil {
					continue
				}
				v := ""
				if route.Connected.Interface != "" {
					if route.Connected.Interface == "lo" || strings.HasPrefix(route.Connected.Interface, "lo") {
						v = "loopback"
					} else {
						v = "link"
					}
				}
				if v != "" && !seen[v] {
					seen[v] = true
					values = append(values, v)
				}
			}
		}
	default:
		return nil, fmt.Errorf("unrecognized forall field %q (valid: device, node, vrf, protocol, route_type, area, origin, connected_class)", field)
	}

	return values, nil
}
