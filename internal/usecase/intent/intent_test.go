package intent

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/intentfile"
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
	doc, err := intentfile.Load(path)
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
	doc := loadTestDoc(t, "testdata/intent/minimal.yml")
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
	doc := loadTestDoc(t, "testdata/intent/packet-basic.yml")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 2 forall intents × 3 customers = 6 expanded intents
	if report.Summary.Total != 6 {
		t.Fatalf("Summary.Total = %d, want 6", report.Summary.Total)
	}
	if report.Summary.Passed != 6 {
		t.Fatalf("Summary.Passed = %d, want 6", report.Summary.Passed)
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
	doc := loadTestDoc(t, "testdata/intent/forall.yml")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 1 forall intent × 3 sources = 3 expanded intents
	if report.Summary.Total != 3 {
		t.Fatalf("Summary.Total = %d, want 3", report.Summary.Total)
	}
	if report.Summary.Passed != 3 {
		t.Fatalf("Summary.Passed = %d, want 3", report.Summary.Passed)
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
	doc := loadTestDoc(t, "testdata/intent/rcl-rib-positive.yml")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 4 intents: 1 rib_eq (no forall) + 1 forall(edge:3) + 1 rib_eval + 1 rib_eval = 6
	if report.Summary.Total != 6 {
		t.Fatalf("Summary.Total = %d, want 6", report.Summary.Total)
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
	doc := loadTestDoc(t, "testdata/intent/selector-logic.yml")
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
	doc := loadTestDoc(t, "testdata/intent/guard-basic.yml")
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
	doc := loadTestDoc(t, "testdata/intent/packet-failure-scenario.yml")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 1 forall intent × 3 customers = 3 expanded intents
	if report.Summary.Total != 3 {
		t.Fatalf("Summary.Total = %d, want 3", report.Summary.Total)
	}
	if report.Summary.Passed != 3 {
		t.Fatalf("Summary.Passed = %d, want 3 (HTTPS survives failure)", report.Summary.Passed)
	}
}

func TestVerifyRIBFibBasic(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intent/rib-fib-basic.yml")
	report, err := Verify(doc)
	if err != nil {
		t.Fatalf("Verify() error: %v", err)
	}
	// 2 forall intents × 3 edges = 6 expanded intents
	if report.Summary.Total != 6 {
		t.Fatalf("Summary.Total = %d, want 6", report.Summary.Total)
	}
	if report.Summary.Passed != 6 {
		t.Fatalf("Summary.Passed = %d, want 6", report.Summary.Passed)
	}
}

// ---------------------------------------------------------------------------
// Verify tests — negative cases (expect failures)
// ---------------------------------------------------------------------------

func TestVerifyRIBNegative(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intent/rib-negative.yml")
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
	doc := loadTestDoc(t, "testdata/intent/packet-negative.yml")
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
	doc := loadTestDoc(t, "testdata/intent/rcl-rib-negative-compare.yml")
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
	doc := loadTestDoc(t, "testdata/intent/rcl-rib-negative-distinct.yml")
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
	doc := loadTestDoc(t, "testdata/intent/invalid-missing-version.yml")
	_, err := Verify(doc)
	if err == nil {
		t.Fatal("Verify() expected error for missing version, got nil")
	}
}

func TestVerifyUndefinedVarFails(t *testing.T) {
	doc := loadTestDoc(t, "testdata/intent/invalid-undefined-var.yml")
	_, err := Verify(doc)
	if err == nil {
		t.Fatal("Verify() expected error for undefined var, got nil")
	}
}
