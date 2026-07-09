package intent

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

// Verify expands and evaluates all intents in the document, producing a report.
func Verify(doc *Document) (Report, error) {
	return VerifyWithProvider(doc, DefaultSnapshotProvider{})
}

// VerifyWithProvider expands and evaluates all intents in the document using
// the given SnapshotProvider for loading network snapshots.
type verifyContext struct {
	snapshots     map[string]Snapshot
	scenarios     map[string]Scenario
	provider      SnapshotProvider
	snapshotCache map[string]SnapshotContext
}

func VerifyWithProvider(doc *Document, provider SnapshotProvider) (Report, error) {
	if provider == nil {
		return Report{}, fmt.Errorf("snapshot provider is nil")
	}

	expanded, err := Expand(doc)
	if err != nil {
		return Report{}, fmt.Errorf("expand: %w", err)
	}

	ctx := verifyContext{
		snapshots:     expanded.Snapshots,
		scenarios:     expanded.Scenarios,
		provider:      provider,
		snapshotCache: map[string]SnapshotContext{},
	}

	var results []Result

	for _, in := range expanded.Intents {
		scenarioName := in.Scenario
		if scenarioName == "" {
			scenarioName = "normal"
		}
		scenario, ok := ctx.scenarios[scenarioName]
		if !ok {
			return Report{}, fmt.Errorf("intent %q: unknown scenario %q", in.Name, scenarioName)
		}

		snap, err := loadSnapshot(scenario.Snapshot, ctx)
		if err != nil {
			return Report{}, fmt.Errorf("intent %q: load snapshot: %w", in.Name, err)
		}

		result := evaluateTopLevel(in, in.RCL, snap, scenario, ctx)
		results = append(results, result)
	}

	passed := 0
	failed := 0
	for _, r := range results {
		switch r.Status {
		case "pass":
			passed++
		case "fail":
			failed++
		}
	}

	return Report{
		Version: "hoyan.intent.report/v1",
		Summary: ReportSummary{
			Total:  len(results),
			Passed: passed,
			Failed: failed,
		},
		Results: results,
	}, nil
}

// ---------------------------------------------------------------------------
// Snapshot loading with cache
// ---------------------------------------------------------------------------

func loadSnapshot(name string, ctx verifyContext) (SnapshotContext, error) {
	if s, ok := ctx.snapshotCache[name]; ok {
		return s, nil
	}
	def, ok := ctx.snapshots[name]
	if !ok {
		return SnapshotContext{}, fmt.Errorf("unknown snapshot %q", name)
	}
	s, err := ctx.provider.LoadSnapshot(name, def)
	if err != nil {
		return SnapshotContext{}, fmt.Errorf("load %q: %w", name, err)
	}
	ctx.snapshotCache[name] = s
	return s, nil
}

// ---------------------------------------------------------------------------
// Top-level intent evaluation
// ---------------------------------------------------------------------------

func evaluateTopLevel(in Intent, expr *RCLExpr, snapshot SnapshotContext, scenario Scenario, ctx verifyContext) Result {
	status, actual := evalRCLExpr(expr, snapshot, nil, scenario, ctx)
	scenarioName := in.Scenario
	if scenarioName == "" {
		scenarioName = "normal"
	}
	group := in.Group
	// For ForallExpr at the intent level, populate Group from the failing iteration
	if expr.Forall != nil && actual.Reason != "" && strings.Contains(actual.Reason, "=") {
		parts := strings.SplitN(actual.Reason, "=", 2)
		if len(parts) == 2 {
			if group == nil {
				group = map[string]any{}
			}
			group[parts[0]] = parts[1]
		}
	}
	return Result{
		Name:     in.Name,
		Status:   status,
		Scenario: scenarioName,
		Snapshot: scenario.Snapshot,
		Group:    group,
		Actual:   actual,
	}
}

// evalRCLExpr recursively evaluates an RCL expression and returns the status
// ("pass" or "fail") along with the actual measurement data. The rowFilter
// parameter accumulates where predicates from enclosing Guard expressions.
func evalRCLExpr(expr *RCLExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	if expr == nil {
		return "fail", Actual{Reason: "nil expression"}
	}

	switch {
	case expr.Guard != nil:
		return evalGuard(expr.Guard, snapshot, rowFilter, scenario, ctx)
	case expr.Forall != nil:
		return evalForall(expr.Forall, snapshot, rowFilter, scenario, ctx)
	case len(expr.And) > 0:
		return evalAnd(expr.And, snapshot, rowFilter, scenario, ctx)
	case len(expr.Or) > 0:
		return evalOr(expr.Or, snapshot, rowFilter, scenario, ctx)
	case expr.Not != nil:
		innerStatus, innerActual := evalRCLExpr(expr.Not, snapshot, rowFilter, scenario, ctx)
		if innerStatus == "pass" {
			return "fail", innerActual
		}
		return "pass", innerActual
	case expr.Imply != [2]*RCLExpr{}:
		return evalImply(expr.Imply, snapshot, rowFilter, scenario, ctx)
	case expr.RIBEq != nil:
		return evalRIBEq(expr.RIBEq, snapshot, rowFilter, scenario, ctx)
	case expr.RIBEval != nil:
		return evalRIBEval(expr.RIBEval, snapshot, rowFilter, scenario, ctx)
	case expr.PacketReachable != nil:
		return evalPacketReachable(expr.PacketReachable, snapshot)
	default:
		return "fail", Actual{Reason: "empty expression"}
	}
}

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
		return "pass", Actual{Count: 0}
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
	default:
		return nil, fmt.Errorf("unrecognized forall field %q (valid: device, node, vrf, protocol)", field)
	}

	return values, nil
}

// ---------------------------------------------------------------------------
// And / Or
// ---------------------------------------------------------------------------

func evalAnd(exprs []RCLExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	var actuals []Actual
	for i := range exprs {
		status, a := evalRCLExpr(&exprs[i], snapshot, rowFilter, scenario, ctx)
		actuals = append(actuals, a)
		if status == "fail" {
			return "fail", combineActuals(actuals)
		}
	}
	if len(actuals) > 0 {
		return "pass", combineActuals(actuals)
	}
	return "pass", Actual{Count: 0}
}

func evalOr(exprs []RCLExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	var actuals []Actual
	for i := range exprs {
		status, a := evalRCLExpr(&exprs[i], snapshot, rowFilter, scenario, ctx)
		actuals = append(actuals, a)
		if status == "pass" {
			return "pass", combineActuals(actuals)
		}
	}
	return "fail", combineActuals(actuals)
}

// ---------------------------------------------------------------------------
// Imply
// ---------------------------------------------------------------------------

func evalImply(pair [2]*RCLExpr, snapshot SnapshotContext, rowFilter map[string]any, scenario Scenario, ctx verifyContext) (string, Actual) {
	if pair[0] == nil || pair[1] == nil {
		return "fail", Actual{Reason: "imply requires exactly 2 sub-expressions"}
	}
	// Evaluate antecedent
	antStatus, _ := evalRCLExpr(pair[0], snapshot, rowFilter, scenario, ctx)
	if antStatus == "fail" {
		// Antecedent false → implication is vacuously true
		return "pass", Actual{Count: 0, Reason: "antecedent is false"}
	}
	// Antecedent true → evaluate consequent; propagate full consequent Actual
	conStatus, conActual := evalRCLExpr(pair[1], snapshot, rowFilter, scenario, ctx)
	conActual.Reason = fmt.Sprintf("antecedent passed, consequent: %s", conStatus)
	return conStatus, conActual
}

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
// metric, preference, eligible, best, device, node, vrf.
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
	case "metric":
		return route.Common.Metric
	case "preference":
		return route.Common.Preference
	case "eligible":
		return route.Common.Eligible
	case "best":
		return route.Common.Best
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

// ---------------------------------------------------------------------------
// PacketReachableExpr
// ---------------------------------------------------------------------------

func evalPacketReachable(e *PacketReachableExpr, snapshot SnapshotContext) (string, Actual) {
	spec := model.PacketSpec{
		Protocol: e.Protocol,
		DstPort:  model.ExactPort(e.DstPort),
	}

	var reachable bool
	var reason string

	if e.VRF != "" {
		_, reachable, reason = snapshot.Graph.PacketReachableSpecVRF(e.From, e.VRF, e.To, spec, sim.NoFailures())
	} else {
		_, reachable, reason = snapshot.Graph.PacketReachableSpec(e.From, e.To, spec, sim.NoFailures())
	}

	actual := Actual{
		Reachable: &reachable,
		Reason:    reason,
	}

	if reachable == e.Expect {
		return "pass", actual
	}
	return "fail", actual
}

// ---------------------------------------------------------------------------
// Row matching and filtering
// ---------------------------------------------------------------------------

// routeRow represents a matching RIB route with its parent RIB metadata.
type routeRow struct {
	rib   observation.RIB
	route observation.RIBRoute
}

// matchingRows returns all RIB routes from the snapshot that satisfy all
// given where predicates (AND semantics). Pass nil for no filter.
// Returns an error if any where predicate contains unrecognized keys.
func matchingRows(snapshot SnapshotContext, filters ...map[string]any) ([]routeRow, error) {
	var out []routeRow
	for _, rib := range RIBs(snapshot.Network) {
		for _, route := range rib.Routes {
			ok, err := matchAllWhere(route, rib, filters...)
			if err != nil {
				return nil, err
			}
			if ok {
				out = append(out, routeRow{rib: rib, route: route})
			}
		}
	}
	return out, nil
}

// matchAllWhere checks that a route satisfies all given where predicates (AND).
// Returns an error if any where predicate contains unrecognized keys.
func matchAllWhere(route observation.RIBRoute, rib observation.RIB, filters ...map[string]any) (bool, error) {
	for _, f := range filters {
		if len(f) > 0 {
			ok, err := matchWhere(route, rib, f)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
	}
	return true, nil
}

// validWhereKeys contains all recognized keys for RCL where predicates.
var validWhereKeys = map[string]bool{
	"prefix": true, "device": true, "node": true, "vrf": true,
	"protocol": true, "not": true, "and": true, "or": true,
}

// matchWhere checks if a RIB route matches a simple where predicate map.
// Supported keys: prefix, device, node, vrf, protocol, not, and, or.
// Multiple keys are ANDed together. The "not" key contains a nested predicate
// that is negated. Returns an error for unrecognized keys.
func matchWhere(route observation.RIBRoute, rib observation.RIB, where map[string]any) (bool, error) {
	if len(where) == 0 {
		return true, nil
	}

	for key, raw := range where {
		lkey := strings.ToLower(key)
		if !validWhereKeys[lkey] {
			return false, fmt.Errorf("unrecognized where key %q", key)
		}

		switch lkey {
		case "prefix":
			val, ok := raw.(string)
			if !ok {
				continue
			}
			if !matchesPrefix(route.Common.Prefix, val) {
				return false, nil
			}

		case "device", "node":
			val, ok := raw.(string)
			if !ok {
				continue
			}
			if string(rib.Node) != val {
				return false, nil
			}

		case "vrf":
			val, ok := raw.(string)
			if !ok {
				continue
			}
			if string(rib.VRF) != val {
				return false, nil
			}

		case "protocol":
			val, ok := raw.(string)
			if !ok {
				continue
			}
			if string(route.Common.Protocol) != val {
				return false, nil
			}

		case "not":
			inner, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			innerOk, err := matchWhere(route, rib, inner)
			if err != nil {
				return false, err
			}
			if innerOk {
				return false, nil // negate
			}

		case "and":
			conds, ok := raw.([]any)
			if !ok {
				continue
			}
			for _, c := range conds {
				inner, ok := c.(map[string]any)
				if !ok {
					return false, nil
				}
				innerOk, err := matchWhere(route, rib, inner)
				if err != nil {
					return false, err
				}
				if !innerOk {
					return false, nil
				}
			}

		case "or":
			conds, ok := raw.([]any)
			if !ok {
				continue
			}
			anyMatch := false
			for _, c := range conds {
				inner, ok := c.(map[string]any)
				if !ok {
					continue
				}
				innerOk, err := matchWhere(route, rib, inner)
				if err != nil {
					return false, err
				}
				if innerOk {
					anyMatch = true
					break
				}
			}
			if !anyMatch {
				return false, nil
			}
		}
	}
	return true, nil
}

// matchesPrefix checks if the route prefix is within or equal to the given
// CIDR prefix string.
func matchesPrefix(routePrefix, wherePrefix string) bool {
	if routePrefix == "" || wherePrefix == "" {
		return routePrefix == wherePrefix
	}
	rp, err := model.ParsePrefix(routePrefix)
	if err != nil {
		return routePrefix == wherePrefix
	}
	wp, err := model.ParsePrefix(wherePrefix)
	if err != nil {
		return routePrefix == wherePrefix
	}
	// Check if routePrefix is within wherePrefix (subnet containment)
	if wp.Equal(rp) {
		return true
	}
	// Check if the route prefix address is within the where prefix range
	return wp.Contains(rp.Addr())
}

// ---------------------------------------------------------------------------
// Where filter merging
// ---------------------------------------------------------------------------

// mergeWhereFilters merges two where predicates by overlaying inner over outer.
// For simple key-value pairs the inner value wins on conflict. This is
// sufficient when outer and inner come from nested Guard/Forall scopes
// that constrain disjoint fields. A full AND-combination that properly
// handles "not" clauses is not required for current use cases.
func mergeWhereFilters(outer, inner map[string]any) map[string]any {
	if len(outer) == 0 {
		return copyMap(inner)
	}
	if len(inner) == 0 {
		return copyMap(outer)
	}
	// For simple cases, we prefer the inner filter since it's more specific.
	// A proper AND merging would need to handle "not" correctly, but for
	// practical use cases this works.
	result := copyMap(outer)
	for k, v := range inner {
		result[k] = v
	}
	return result
}

func copyMap(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// Utility helpers
// ---------------------------------------------------------------------------

// toInt converts a value to int, supporting various numeric types.
func toInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	case string:
		var i int
		if _, err := fmt.Sscanf(n, "%d", &i); err != nil {
			return 0
		}
		return i
	default:
		return 0
	}
}

// combineActuals merges multiple Actuals from sub-expressions into one.
func combineActuals(actuals []Actual) Actual {
	if len(actuals) == 0 {
		return Actual{Count: 0}
	}
	total := 0
	for _, a := range actuals {
		total += a.Count
	}
	reasons := make([]string, 0, len(actuals))
	for _, a := range actuals {
		if a.Reason != "" {
			reasons = append(reasons, a.Reason)
		}
	}
	return Actual{
		Count:  total,
		Reason: strings.Join(reasons, "; "),
	}
}


