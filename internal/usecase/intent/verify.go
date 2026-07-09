package intent

import (
	"fmt"
	"strings"
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
