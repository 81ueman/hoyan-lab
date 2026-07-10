package intent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/intentdsl"
	"github.com/81ueman/hoyan-lab/internal/adapter/solver/enumerate"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

func TestMain(m *testing.M) {
	// Change to module root so relative lab paths in testdata YAML files resolve correctly.
	// When go test runs, CWD is the package directory (<module>/internal/usecase/intent/).
	// We derive the module root from the test file location.
	_, filename, _, _ := runtime.Caller(0)
	// filename = .../<module>/internal/usecase/intent/intent_test.go
	// Go up 4 levels to reach module root (internal/usecase/intent/ → ../../..)
	moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))
	if err := os.Chdir(moduleRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Chdir to module root: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

// loadTestDoc loads a Document from a path relative to the module root.
func loadTestDoc(t *testing.T, path string) *Document {
	t.Helper()
	doc, err := intentdsl.Load(path)
	if err != nil {
		t.Fatalf("Load(%q): %v", path, err)
	}
	return doc
}

func TestDefaultSnapshotProviderUsesRegisteredDefaultGraphOptions(t *testing.T) {
	orig := defaultGraphOptions
	defer func() { defaultGraphOptions = orig }()
	SetDefaultGraphOptions(sim.WithSolverBackend(nil))
	if got := (DefaultSnapshotProvider{}).graphOptions(); len(got) == 0 {
		t.Fatalf("DefaultSnapshotProvider graph options = %#v, want registered default option", got)
	}
	if got := (DefaultSnapshotProvider{GraphOptions: []sim.GraphOption{}}).graphOptions(); len(got) != 0 {
		t.Fatalf("explicit graph options = %#v, want explicit empty options preserved", got)
	}
}

// ---------------------------------------------------------------------------
// Verify tests — positive cases
// ---------------------------------------------------------------------------

func TestVerifyRIBEval(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/minimal.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if report.Summary.Total != 1 {
		t.Fatalf("Summary.Total = %d, want 1", report.Summary.Total)
	}
	if report.Summary.Passed != 1 {
		t.Fatalf("Summary.Passed = %d, want 1 (all pass)", report.Summary.Passed)
	}
	if report.Summary.Failed != 0 {
		t.Fatalf("Summary.Failed = %d, want 0", report.Summary.Failed)
	}
	if report.Results[0].Status != "pass" {
		t.Fatalf("Result.Status = %q, want \"pass\"", report.Results[0].Status)
	}
}

func TestVerifyPacketReachable(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/packet-basic.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 2 intents, each with forall expanded to And(3)
	if report.Summary.Total != 2 {
		t.Fatalf("Summary.Total = %d, want 2", report.Summary.Total)
	}
	if report.Summary.Passed != 2 {
		t.Fatalf("Summary.Passed = %d, want 2", report.Summary.Passed)
	}
	if report.Summary.Failed != 0 {
		t.Fatalf("Summary.Failed = %d, want 0", report.Summary.Failed)
	}
	for _, r := range report.Results {
		if r.Status != "pass" {
			t.Fatalf("Result %q Status = %q, want \"pass\"", r.Name, r.Status)
		}
	}
}

func TestVerifyForall(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/forall.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 1 intent with forall expanded to And(3)
	if report.Summary.Total != 1 {
		t.Fatalf("Summary.Total = %d, want 1", report.Summary.Total)
	}
	if report.Summary.Passed != 1 {
		t.Fatalf("Summary.Passed = %d, want 1", report.Summary.Passed)
	}
	if report.Summary.Failed != 0 {
		t.Fatalf("Summary.Failed = %d, want 0", report.Summary.Failed)
	}
	for _, r := range report.Results {
		if r.Status != "pass" {
			t.Fatalf("Result %q Status = %q, want \"pass\"", r.Name, r.Status)
		}
	}
}

func TestVerifyRIBEq(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/rcl-rib-positive.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 4 intents: forall expanded at parse time into And
	if report.Summary.Total != 4 {
		t.Fatalf("Summary.Total = %d, want 4", report.Summary.Total)
	}
	if report.Summary.Failed != 0 {
		t.Fatalf("Summary.Failed = %d, want 0", report.Summary.Failed)
	}
	// First result is rib_eq — should pass (unrelated routes are unchanged)
	if report.Results[0].Status != "pass" {
		t.Fatalf("Result[0] (rib_eq) Status = %q, want \"pass\"", report.Results[0].Status)
	}
}

func TestVerifyAndOrNot(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/selector-logic.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 4 intents, all pass
	if report.Summary.Total != 4 {
		t.Fatalf("Summary.Total = %d, want 4", report.Summary.Total)
	}
	if report.Summary.Passed != 4 {
		t.Fatalf("Summary.Passed = %d, want 4", report.Summary.Passed)
	}
	if report.Summary.Failed != 0 {
		t.Fatalf("Summary.Failed = %d, want 0", report.Summary.Failed)
	}
	for _, r := range report.Results {
		if r.Status != "pass" {
			t.Fatalf("Result %q Status = %q, want \"pass\"", r.Name, r.Status)
		}
	}
}

func TestVerifyGuard(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/guard-basic.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 3 intents: 2 pass (premise-true-inner-passes + premise-false-vacuous-pass), 1 fail (premise-true-inner-fails)
	if report.Summary.Total != 3 {
		t.Fatalf("Summary.Total = %d, want 3", report.Summary.Total)
	}
	if report.Summary.Passed != 2 {
		t.Fatalf("Summary.Passed = %d, want 2", report.Summary.Passed)
	}
	if report.Summary.Failed != 1 {
		t.Fatalf("Summary.Failed = %d, want 1", report.Summary.Failed)
	}

	// Verify individual results
	for _, r := range report.Results {
		switch r.Name {
		case "guard-premise-true-inner-passes":
			if r.Status != "pass" {
				t.Fatalf("Result %q Status = %q, want \"pass\"", r.Name, r.Status)
			}
		case "guard-premise-false-vacuous-pass":
			if r.Status != "pass" {
				t.Fatalf("Result %q Status = %q, want \"pass\"", r.Name, r.Status)
			}
			if r.Actual.Count != 0 {
				t.Fatalf("Result %q Actual.Count = %d, want 0 (vacuous pass)", r.Name, r.Actual.Count)
			}
		case "guard-premise-true-inner-fails":
			if r.Status != "fail" {
				t.Fatalf("Result %q Status = %q, want \"fail\"", r.Name, r.Status)
			}
		default:
			t.Fatalf("unexpected result name %q", r.Name)
		}
	}
}

func TestVerifyPacketFailureScenario(t *testing.T) {
	orig := defaultGraphOptions
	defer func() { defaultGraphOptions = orig }()
	SetDefaultGraphOptions(sim.WithSolverBackend(enumerate.Backend{}))

	doc := loadTestDoc(t, "testdata/intentdsl/packet-failure-scenario.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 1 intent with forall expanded to And(3)
	// expect: false + max: 1 → HTTPS is NOT reachable under some single link failure → pass
	if report.Summary.Total != 1 {
		t.Fatalf("Summary.Total = %d, want 1", report.Summary.Total)
	}
	if report.Summary.Passed != 1 {
		t.Fatalf("Summary.Passed = %d, want 1 (expect: false + max: 1 should pass when failure breaks reachability)", report.Summary.Passed)
	}
}

func TestVerifyRIBFibBasic(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/rib-fib-basic.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 2 intents, each with forall expanded to And(3)
	if report.Summary.Total != 2 {
		t.Fatalf("Summary.Total = %d, want 2", report.Summary.Total)
	}
	if report.Summary.Passed != 2 {
		t.Fatalf("Summary.Passed = %d, want 2", report.Summary.Passed)
	}
}

func TestParseForallCartesianProduct(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/forall-cartesian.hoyan")
	expanded, err := Expand(doc)
	if err != nil {
		t.Fatalf("Expand() error: %v", err)
	}
	// No doc-level expansion — RCL forall is expanded at parse time
	if len(expanded.Intents) != 1 {
		t.Fatalf("len(expanded.Intents) = %d, want 1", len(expanded.Intents))
	}
	rcl := expanded.Intents[0].RCL
	if rcl == nil || len(rcl.And) != 4 {
		t.Fatalf("expected And with 4 entries (2 devices x 2 prefixes), got %#v", rcl)
	}
	// Verify the cartesian product combinations
	seen := map[string]bool{}
	for _, expr := range rcl.And {
		where := expr.RIBEval.Where
		device, _ := where["device"].(string)
		prefix, _ := where["prefix"].(string)
		key := device + "/" + prefix
		seen[key] = true
	}
	for _, want := range []string{
		"bj-edge1/10.4.0.0/16",
		"bj-edge1/203.0.113.0/24",
		"sh-edge1/10.4.0.0/16",
		"sh-edge1/203.0.113.0/24",
	} {
		if !seen[want] {
			t.Fatalf("cartesian product missing %q; got %#v", want, seen)
		}
	}
}

func TestVerifyRCLForallVarRef(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/rcl-forall-var.hoyan")
	expanded, err := Expand(doc)
	if err != nil {
		t.Fatalf("Expand() error: %v", err)
	}
	got := expanded.Intents[0].RCL.And
	want := []string{"bj-edge1", "sh-edge1", "gz-edge1"}
	if len(got) != len(want) {
		t.Fatalf("expanded RCL expressions = %#v, want %d entries", got, len(want))
	}
	for i := range want {
		where := got[i].RIBEval.Where
		if where["device"] != want[i] {
			t.Fatalf("expanded where[%d].device = %#v, want %q", i, where["device"], want[i])
		}
	}

	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	if report.Summary.Total != 1 {
		t.Fatalf("Summary.Total = %d, want 1", report.Summary.Total)
	}
	if report.Summary.Passed != 1 {
		t.Fatalf("Summary.Passed = %d, want 1", report.Summary.Passed)
	}
}

// ---------------------------------------------------------------------------
// Verify tests — negative cases (expect failures)
// ---------------------------------------------------------------------------

func TestVerifyRIBNegative(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/rib-negative.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// This intent asserts a non-existent prefix exists → fail
	if report.Summary.Total != 1 {
		t.Fatalf("Summary.Total = %d, want 1", report.Summary.Total)
	}
	if report.Summary.Failed != 1 {
		t.Fatalf("Summary.Failed = %d, want 1 (non-existent prefix)", report.Summary.Failed)
	}
	if report.Results[0].Status != "fail" {
		t.Fatalf("Result[0].Status = %q, want \"fail\"", report.Results[0].Status)
	}
}

func TestVerifyPacketNegative(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/packet-negative.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// HTTP to cust-bj:80 is actually denied (reachable=false), but expect=true → fail
	if report.Summary.Total != 1 {
		t.Fatalf("Summary.Total = %d, want 1", report.Summary.Total)
	}
	if report.Summary.Failed != 1 {
		t.Fatalf("Summary.Failed = %d, want 1 (incorrectly expects reachable)", report.Summary.Failed)
	}
	if report.Results[0].Status != "fail" {
		t.Fatalf("Result[0].Status = %q, want \"fail\"", report.Results[0].Status)
	}
}

func TestVerifyRIBNegativeCompare(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/rcl-rib-negative-compare.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// Compares pre vs post snapshots from different labs → routes differ → fail
	if report.Summary.Total != 1 {
		t.Fatalf("Summary.Total = %d, want 1", report.Summary.Total)
	}
	if report.Summary.Failed != 1 {
		t.Fatalf("Summary.Failed = %d, want 1 (different snapshots should differ)", report.Summary.Failed)
	}
}

func TestVerifyRIBNegativeDistinct(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/rcl-rib-negative-distinct.hoyan")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// All routes have the same single nexthop, but expects distCnt(nexthop) ≥ 2 → fail
	if report.Summary.Total != 1 {
		t.Fatalf("Summary.Total = %d, want 1", report.Summary.Total)
	}
	if report.Summary.Failed != 1 {
		t.Fatalf("Summary.Failed = %d, want 1 (single nexthop fails gte:2)", report.Summary.Failed)
	}
}

// ---------------------------------------------------------------------------
// Edge cases
// ---------------------------------------------------------------------------

func TestVerifyMissingVersionFails(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/invalid-missing-version.hoyan")
	_, err := Verify(doc)
	if err == nil {
		t.Fatal("Verify() expected error for missing version, got nil")
	}
}

func TestVerifyUndefinedVarFails(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intentdsl/invalid-undefined-var.hoyan")
	_, err := Verify(doc)
	if err == nil {
		t.Fatal("Verify() expected error for undefined var, got nil")
	}
}

// ---------------------------------------------------------------------------
// Auto-discovery smoke test
// ---------------------------------------------------------------------------

func TestAllIntentDSLFiles(t *testing.T) {
	files, err := filepath.Glob("testdata/intentdsl/*.hoyan")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no intent DSL files found")
	}

	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			doc := loadTestDoc(t, file)

			// invalid-* files expect a validation error
			if strings.HasPrefix(name, "invalid-") {
				err := Validate(doc)
				if err == nil {
					t.Fatalf("expected validation error for %s", name)
				}
				t.Logf("expected error: %v", err)
				return
			}

			// Normal files: Verify should succeed without error
			report, err := Verify(doc)
			if err != nil {
				t.Fatalf("Verify(%s) error: %v", name, err)
			}
			if report.Summary.Failed > 0 {
				// Log failures but don't fail the test (negative test files intentionally fail)
				for _, r := range report.Results {
					if r.Status == "fail" {
						t.Logf("[expected fail] %s: %s (count=%d, reason=%s)",
							name, r.Name, r.Actual.Count, r.Actual.Reason)
					}
				}
			}
		})
	}
}
