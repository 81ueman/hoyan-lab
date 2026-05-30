package intent

import (
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strconv"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/solver"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/facts"
)

func Verify(doc *Document) (Report, error) {
	return VerifyWithProvider(doc, DefaultSnapshotProvider{})
}

func VerifyWithProvider(doc *Document, provider SnapshotProvider) (Report, error) {
	if provider == nil {
		return Report{}, errors.New("snapshot provider is nil")
	}
	expanded, err := Expand(doc)
	if err != nil {
		return Report{}, err
	}
	snapshots := map[string]facts.Snapshot{}
	report := Report{Version: "hoyan.intent.report/v1"}
	loadSnapshot := func(name string) (facts.Snapshot, error) {
		snapshot, ok := snapshots[name]
		if ok {
			return snapshot, nil
		}
		snapshotDef := expanded.Snapshots[name]
		snapshot, err := provider.LoadSnapshot(name, snapshotDef)
		if err != nil {
			return facts.Snapshot{}, fmt.Errorf("snapshot %q: %w", name, err)
		}
		snapshots[name] = snapshot
		return snapshot, nil
	}
	for _, in := range expanded.Intents {
		if in.Check.Compare != nil {
			results, err := evaluateCompare(in, loadSnapshot)
			if err != nil {
				return Report{}, err
			}
			report.Results = append(report.Results, results...)
			continue
		}
		scenario := expanded.Scenarios[in.Check.Scenario]
		snapshot, err := loadSnapshot(scenario.Snapshot)
		if err != nil {
			return Report{}, err
		}
		report.Results = append(report.Results, evaluateIntent(in, scenario, scenario.Snapshot, snapshot)...)
	}
	report.Summary.Total = len(report.Results)
	for i, result := range report.Results {
		if report.Results[i].Counterexamples == nil {
			report.Results[i].Counterexamples = []any{}
		}
		if result.Status == "pass" {
			report.Summary.Passed++
		} else {
			report.Summary.Failed++
		}
	}
	return report, nil
}

func evaluateIntent(in Intent, scenario Scenario, snapshotName string, snapshot facts.Snapshot) []Result {
	assertion := effectiveAssertion(in)
	if in.Check.Table == "packet" {
		return []Result{evaluatePacketIntent(in, assertion, scenario, snapshotName, snapshot)}
	}
	rows := matchingRows(in.Check.Table, in.Check.Where, snapshot)
	if len(in.Check.GroupBy) == 0 {
		return []Result{evaluateRows(in, assertion, snapshotName, rows, normalizeGroup(in.Group))}
	}
	groups := groupedRows(rows, in.Check.GroupBy)
	results := make([]Result, 0, len(groups))
	for _, group := range groups {
		results = append(results, evaluateRows(in, assertion, snapshotName, group.Rows, group.Values))
	}
	return results
}

func evaluateRows(in Intent, assertion Assertion, snapshotName string, rows []rowView, group map[string]any) Result {
	rowSummaries := summarizeRows(rows)
	actual := Actual{Count: len(rows), Rows: rowSummaries}
	if assertion.DistinctCount != nil || assertion.DistinctValues != nil {
		field := distinctField(assertion)
		actual.Values = distinctValues(rows, field)
		actual.DistinctCount = len(actual.Values)
	}
	result := Result{
		Name:      in.Name,
		Status:    "pass",
		Table:     in.Check.Table,
		Scenario:  in.Check.Scenario,
		Snapshot:  snapshotName,
		Group:     group,
		Assertion: assertion,
		Actual:    actual,
	}
	if !assertionPasses(assertion, actual) {
		result.Status = "fail"
		result.Actual.Reason = failureReason(assertion, len(rows))
		result.Counterexamples = []any{result.Actual.Reason}
	}
	return result
}

func evaluatePacketIntent(in Intent, assertion Assertion, scenario Scenario, snapshotName string, snapshot facts.Snapshot) Result {
	target := sim.PacketTarget{
		To:       in.Check.Packet.To,
		Protocol: in.Check.Packet.Protocol,
		DstPort:  in.Check.Packet.DstPort,
		VRF:      in.Check.Packet.VRF,
	}
	path, reachable, reason := snapshot.Graph.PacketReachableSpecVRF(in.Check.Packet.From, in.Check.Packet.VRF, in.Check.Packet.To, target.Spec(), sim.NoFailures())
	actualReachable := reachable
	result := Result{
		Name:      in.Name,
		Status:    "pass",
		Table:     in.Check.Table,
		Scenario:  in.Check.Scenario,
		Snapshot:  snapshotName,
		Group:     normalizeGroup(in.Group),
		Assertion: assertion,
		Actual: Actual{
			Reachable: &actualReachable,
			Reason:    reason,
			Path:      path.Nodes,
		},
	}
	expected := assertion.Reachable != nil && *assertion.Reachable
	if expected && reachable && scenario.Failures.Max > 0 {
		search, err := snapshot.Graph.FindBreakingFailuresSymbolic(in.Check.Packet.From, target, sim.FailureSearchOptions{
			IncludeLinks: true,
			MaxFailures:  scenario.Failures.Max,
			Domain:       failureDomain(scenario.Failures),
		})
		if err == nil && search.Sat {
			actualReachable = false
			result.Actual.Reachable = &actualReachable
			result.Actual.Reason = "unreachable under failure scenario"
			result.Counterexamples = []any{failureCounterexample(search.Failures, result.Actual.Reason)}
		} else if err != nil {
			result.Status = "fail"
			result.Actual.Reason = err.Error()
			result.Counterexamples = []any{result.Actual.Reason}
			return result
		}
	}
	if assertion.Reachable == nil || actualReachable != *assertion.Reachable {
		result.Status = "fail"
		if result.Actual.Reason == "" {
			result.Actual.Reason = fmt.Sprintf("reachable=%v, want %v", actualReachable, *assertion.Reachable)
		}
		if len(result.Counterexamples) == 0 {
			result.Counterexamples = []any{result.Actual.Reason}
		}
	}
	return result
}

func failureDomain(in FailureConstraints) model.FailureDomain {
	return model.FailureDomain{
		IncludeLinkRoles: in.IncludeLinkRoles,
		ExcludeLinkRoles: in.ExcludeLinkRoles,
		IncludeLinks:     in.IncludeLinks,
		ExcludeLinks:     in.ExcludeLinks,
		IncludeNodeRoles: in.IncludeNodeRoles,
		ExcludeNodeRoles: in.ExcludeNodeRoles,
		IncludeNodes:     in.IncludeNodes,
		ExcludeNodes:     in.ExcludeNodes,
	}
}

func failureCounterexample(elements []solver.FailureElement, reason string) FailureCounterexample {
	out := FailureCounterexample{Reason: reason}
	for _, element := range elements {
		switch element.Kind {
		case solver.FailureLink:
			out.FailedLinks = append(out.FailedLinks, element.Name)
		case solver.FailureNode:
			out.FailedNodes = append(out.FailedNodes, element.Name)
		}
	}
	sort.Strings(out.FailedLinks)
	sort.Strings(out.FailedNodes)
	return out
}

func evaluateCompare(in Intent, loadSnapshot func(string) (facts.Snapshot, error)) ([]Result, error) {
	compare := in.Check.Compare
	left, err := loadSnapshot(compare.Left.Snapshot)
	if err != nil {
		return nil, err
	}
	right, err := loadSnapshot(compare.Right.Snapshot)
	if err != nil {
		return nil, err
	}
	leftRows := facts.CanonicalRIBRows(matchingRIBFacts(left, compare.Left.Where))
	rightRows := facts.CanonicalRIBRows(matchingRIBFacts(right, compare.Right.Where))
	added, removed := ribDiff(leftRows, rightRows)
	assertion := Assertion{Relation: compare.Relation}
	result := Result{
		Name:      in.Name,
		Status:    "pass",
		Table:     compare.Table,
		Assertion: assertion,
		Actual: Actual{
			Count:       len(rightRows),
			AddedRows:   canonicalRowsAsAny(added),
			RemovedRows: canonicalRowsAsAny(removed),
		},
	}
	if len(added) > 0 || len(removed) > 0 {
		result.Status = "fail"
		result.Actual.Reason = "canonical rows differ"
		result.Counterexamples = []any{result.Actual.Reason}
	}
	return []Result{result}, nil
}

type rowView struct {
	Table string
	RIB   *facts.RIBRow
	FIB   *facts.FIBRow
}

func matchingRows(table string, where map[string]any, snapshot facts.Snapshot) []rowView {
	var rows []rowView
	switch table {
	case "rib":
		for _, row := range snapshot.RIB {
			if matchRIB(row, where) {
				cp := row
				rows = append(rows, rowView{Table: table, RIB: &cp})
			}
		}
	case "fib":
		for _, row := range snapshot.FIB {
			if matchFIB(row, where) {
				cp := row
				rows = append(rows, rowView{Table: table, FIB: &cp})
			}
		}
	}
	return rows
}

func matchingRIBFacts(snapshot facts.Snapshot, where map[string]any) []facts.RIBRow {
	var rows []facts.RIBRow
	for _, row := range snapshot.RIB {
		if matchRIB(row, where) {
			rows = append(rows, row)
		}
	}
	return rows
}

func matchRIB(row facts.RIBRow, where map[string]any) bool {
	return matchWhere(where, func(field string) (any, bool) {
		switch field {
		case "device":
			return row.Device, true
		case "vrf":
			return row.VRF, true
		case "prefix":
			return row.Prefix, true
		case "protocol":
			return row.Protocol, true
		case "nexthop":
			return row.NextHop, true
		case "local_pref":
			return row.LocalPref, true
		case "med":
			return row.MED, true
		case "selected":
			return row.Selected, true
		case "installed":
			return row.Installed, true
		}
		return nil, false
	})
}

func matchFIB(row facts.FIBRow, where map[string]any) bool {
	return matchWhere(where, func(field string) (any, bool) {
		switch field {
		case "device":
			return row.Device, true
		case "prefix":
			return row.Prefix, true
		case "nexthop":
			return row.NextHop, true
		case "interface":
			return row.Interface, true
		case "installed":
			return row.Installed, true
		}
		return nil, false
	})
}

func matchWhere(where map[string]any, fieldValue func(string) (any, bool)) bool {
	for _, key := range sortedKeysAny(where) {
		value := where[key]
		switch key {
		case "and":
			clauses, ok := value.([]any)
			if !ok {
				return false
			}
			for _, clause := range clauses {
				m, ok := clause.(map[string]any)
				if !ok || !matchWhere(m, fieldValue) {
					return false
				}
			}
		case "or":
			clauses, ok := value.([]any)
			if !ok {
				return false
			}
			matched := false
			for _, clause := range clauses {
				m, ok := clause.(map[string]any)
				if ok && matchWhere(m, fieldValue) {
					matched = true
					break
				}
			}
			if !matched {
				return false
			}
		case "not":
			m, ok := value.(map[string]any)
			if !ok || matchWhere(m, fieldValue) {
				return false
			}
		case "prefix_within":
			raw, ok := fieldValue("prefix")
			if !ok || !prefixWithin(scalar(raw), scalar(value)) {
				return false
			}
		default:
			actual, ok := fieldValue(key)
			if !ok {
				continue
			}
			if key == "device_in" {
				if !stringIn(scalar(actual), value) {
					return false
				}
				continue
			}
			if !valuesEqual(actual, value) {
				return false
			}
		}
	}
	return true
}

func assertionPasses(assert Assertion, actual Actual) bool {
	if assert.Exists != nil {
		if *assert.Exists {
			return actual.Count > 0
		}
		return actual.Count == 0
	}
	if assert.Count != nil {
		if assert.Count.GTE != nil && actual.Count < *assert.Count.GTE {
			return false
		}
		if assert.Count.Equals != nil && actual.Count != *assert.Count.Equals {
			return false
		}
	}
	if assert.DistinctCount != nil {
		if assert.DistinctCount.GTE != nil && actual.DistinctCount < *assert.DistinctCount.GTE {
			return false
		}
		if assert.DistinctCount.Equals != nil && actual.DistinctCount != *assert.DistinctCount.Equals {
			return false
		}
	}
	if assert.DistinctValues != nil {
		want := normalizeDistinctValues(assert.DistinctValues.Equals)
		got := normalizeDistinctValues(actual.Values)
		if len(want) != len(got) {
			return false
		}
		for i := range want {
			if want[i] != got[i] {
				return false
			}
		}
	}
	return true
}

func distinctField(assert Assertion) string {
	if assert.DistinctCount != nil {
		return assert.DistinctCount.Field
	}
	if assert.DistinctValues != nil {
		return assert.DistinctValues.Field
	}
	return ""
}

func groupedRows(rows []rowView, fields []string) []rowGroup {
	byKey := map[string]*rowGroup{}
	var keys []string
	for _, row := range rows {
		values := map[string]any{}
		var parts []string
		for _, field := range fields {
			value := rowField(row, field)
			values[field] = value
			parts = append(parts, scalar(value))
		}
		key := factKey(parts...)
		group, ok := byKey[key]
		if !ok {
			group = &rowGroup{Values: values}
			byKey[key] = group
			keys = append(keys, key)
		}
		group.Rows = append(group.Rows, row)
	}
	sort.Strings(keys)
	out := make([]rowGroup, 0, len(keys))
	for _, key := range keys {
		out = append(out, *byKey[key])
	}
	return out
}

type rowGroup struct {
	Values map[string]any
	Rows   []rowView
}

func distinctValues(rows []rowView, field string) []any {
	seen := map[string]any{}
	var keys []string
	for _, row := range rows {
		value := rowField(row, field)
		key := scalar(value)
		if _, ok := seen[key]; !ok {
			seen[key] = value
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	out := make([]any, 0, len(keys))
	for _, key := range keys {
		out = append(out, seen[key])
	}
	return out
}

func rowField(row rowView, field string) any {
	if row.RIB != nil {
		switch field {
		case "device":
			return row.RIB.Device
		case "vrf":
			return row.RIB.VRF
		case "prefix":
			return row.RIB.Prefix
		case "protocol":
			return row.RIB.Protocol
		case "nexthop":
			return row.RIB.NextHop
		case "local_pref":
			return row.RIB.LocalPref
		case "med":
			return row.RIB.MED
		case "selected":
			return row.RIB.Selected
		case "installed":
			return row.RIB.Installed
		}
	}
	if row.FIB != nil {
		switch field {
		case "device":
			return row.FIB.Device
		case "prefix":
			return row.FIB.Prefix
		case "nexthop":
			return row.FIB.NextHop
		case "interface":
			return row.FIB.Interface
		case "installed":
			return row.FIB.Installed
		}
	}
	return ""
}

func summarizeRows(rows []rowView) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.RIB != nil {
			out = append(out, row.RIB.Device+" "+row.RIB.Prefix+" "+row.RIB.Protocol+" "+row.RIB.NextHop)
		}
		if row.FIB != nil {
			out = append(out, row.FIB.Device+" "+row.FIB.Prefix+" "+row.FIB.NextHop)
		}
	}
	sort.Strings(out)
	return out
}

func ribDiff(left, right []facts.CanonicalRIBRow) ([]facts.CanonicalRIBRow, []facts.CanonicalRIBRow) {
	leftByKey := map[string]facts.CanonicalRIBRow{}
	rightByKey := map[string]facts.CanonicalRIBRow{}
	for _, row := range left {
		leftByKey[row.Key()] = row
	}
	for _, row := range right {
		rightByKey[row.Key()] = row
	}
	var added, removed []facts.CanonicalRIBRow
	for key, row := range rightByKey {
		if _, ok := leftByKey[key]; !ok {
			added = append(added, row)
		}
	}
	for key, row := range leftByKey {
		if _, ok := rightByKey[key]; !ok {
			removed = append(removed, row)
		}
	}
	sort.SliceStable(added, func(i, j int) bool { return added[i].Key() < added[j].Key() })
	sort.SliceStable(removed, func(i, j int) bool { return removed[i].Key() < removed[j].Key() })
	return added, removed
}

func canonicalRowsAsAny(rows []facts.CanonicalRIBRow) []any {
	out := make([]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, row)
	}
	return out
}

func failureReason(assert Assertion, count int) string {
	if assert.Exists != nil && count == 0 {
		return "no matching rows"
	}
	if assert.DistinctCount != nil || assert.DistinctValues != nil {
		return "distinct aggregate did not satisfy assertion"
	}
	return "matching row count did not satisfy assertion"
}

func valuesEqual(actual any, want any) bool {
	switch a := actual.(type) {
	case bool:
		return a == boolValue(want)
	case int:
		return strconv.Itoa(a) == scalar(want)
	default:
		return scalar(a) == scalar(want)
	}
}

func normalizeDistinctValues(values []any) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, scalar(value))
	}
	sort.Strings(out)
	return out
}

func prefixWithin(prefix, parent string) bool {
	p, err := netip.ParsePrefix(prefix)
	if err != nil {
		return false
	}
	container, err := netip.ParsePrefix(parent)
	if err != nil {
		return false
	}
	return container.Contains(p.Addr()) && p.Bits() >= container.Bits()
}

func scalar(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprint(v)
	}
}

func stringIn(needle string, value any) bool {
	values, ok := toStringSlice(value)
	if !ok {
		return false
	}
	for _, candidate := range values {
		if candidate == needle {
			return true
		}
	}
	return false
}

func boolValue(value any) bool {
	v, _ := value.(bool)
	return v
}

func normalizeGroup(in map[string]any) map[string]any {
	if in == nil {
		return map[string]any{}
	}
	return in
}

func factKey(parts ...string) string {
	key := ""
	for _, part := range parts {
		key += "\x00" + part
	}
	return key
}
