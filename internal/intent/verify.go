package intent

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/81ueman/hoyan-lab/internal/facts"
)

func Verify(doc *Document) (Report, error) {
	expanded, err := Expand(doc)
	if err != nil {
		return Report{}, err
	}
	snapshots := map[string]facts.Snapshot{}
	report := Report{Version: "hoyan.intent.report/v1"}
	for _, in := range expanded.Intents {
		scenario := expanded.Scenarios[in.Check.Scenario]
		snapshotDef := expanded.Snapshots[scenario.Snapshot]
		snapshot, ok := snapshots[scenario.Snapshot]
		if !ok {
			snapshot, err = facts.Build(snapshotDef.Lab, scenario.Snapshot)
			if err != nil {
				return Report{}, fmt.Errorf("snapshot %q: %w", scenario.Snapshot, err)
			}
			snapshots[scenario.Snapshot] = snapshot
		}
		result := evaluateIntent(in, scenario.Snapshot, snapshot)
		report.Results = append(report.Results, result)
	}
	report.Summary.Total = len(report.Results)
	for _, result := range report.Results {
		if result.Status == "pass" {
			report.Summary.Passed++
		} else {
			report.Summary.Failed++
		}
	}
	return report, nil
}

func evaluateIntent(in Intent, snapshotName string, snapshot facts.Snapshot) Result {
	count, rows := matchingRows(in, snapshot)
	result := Result{
		Name:      in.Name,
		Status:    "pass",
		Table:     in.Check.Table,
		Scenario:  in.Check.Scenario,
		Snapshot:  snapshotName,
		Group:     normalizeGroup(in.Group),
		Assertion: in.Assert,
		Actual:    Actual{Count: count, Rows: rows},
	}
	if !assertionPasses(in.Assert, count) {
		result.Status = "fail"
		result.Actual.Reason = "no matching rows"
		if count > 0 {
			result.Actual.Reason = "matching row count did not satisfy assertion"
		}
		result.Counterexamples = []string{result.Actual.Reason}
	}
	return result
}

func matchingRows(in Intent, snapshot facts.Snapshot) (int, []string) {
	var rows []string
	switch in.Check.Table {
	case "rib":
		for _, row := range snapshot.RIB {
			if matchRIB(row, in.Check.Where) {
				rows = append(rows, row.Device+" "+row.Prefix+" "+row.Protocol)
			}
		}
	case "fib":
		for _, row := range snapshot.FIB {
			if matchFIB(row, in.Check.Where) {
				rows = append(rows, row.Device+" "+row.Prefix)
			}
		}
	}
	sort.Strings(rows)
	return len(rows), rows
}

func matchRIB(row facts.RIBRow, where map[string]any) bool {
	for key, value := range where {
		switch key {
		case "device":
			if row.Device != scalar(value) {
				return false
			}
		case "device_in":
			if !stringIn(row.Device, value) {
				return false
			}
		case "prefix":
			if row.Prefix != scalar(value) {
				return false
			}
		case "selected":
			if row.Selected != boolValue(value) {
				return false
			}
		case "installed":
			if row.Installed != boolValue(value) {
				return false
			}
		}
	}
	return true
}

func matchFIB(row facts.FIBRow, where map[string]any) bool {
	for key, value := range where {
		switch key {
		case "device":
			if row.Device != scalar(value) {
				return false
			}
		case "device_in":
			if !stringIn(row.Device, value) {
				return false
			}
		case "prefix":
			if row.Prefix != scalar(value) {
				return false
			}
		case "installed":
			if row.Installed != boolValue(value) {
				return false
			}
		}
	}
	return true
}

func assertionPasses(assert Assertion, count int) bool {
	if assert.Exists != nil {
		if *assert.Exists {
			return count > 0
		}
		return count == 0
	}
	if assert.Count != nil {
		if assert.Count.GTE != nil && count < *assert.Count.GTE {
			return false
		}
		if assert.Count.Equals != nil && count != *assert.Count.Equals {
			return false
		}
	}
	return true
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
