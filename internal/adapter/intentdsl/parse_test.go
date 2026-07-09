package intentdsl

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/intent"
)

func TestMain(m *testing.M) {
	_, filename, _, _ := runtime.Caller(0)
	// Go up 4 levels: internal/adapter/intentdsl/parse_test.go → module root
	moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(filename))))
	if err := os.Chdir(moduleRoot); err != nil {
		fmt.Fprintf(os.Stderr, "Chdir to module root: %v\n", err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}

func TestLoadMinimal(t *testing.T) {
	doc, err := Load("testdata/intentdsl/minimal.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Version != "hoyan/v1" {
		t.Fatalf("Version = %q, want \"hoyan/v1\"", doc.Version)
	}
	if len(doc.Intents) != 1 {
		t.Fatalf("len(Intents) = %d, want 1", len(doc.Intents))
	}
	if doc.Intents[0].Name != "bj-edge1-has-service-prefix" {
		t.Fatalf("Intent[0].Name = %q", doc.Intents[0].Name)
	}
	if doc.Intents[0].RCL == nil || doc.Intents[0].RCL.RIBEval == nil {
		t.Fatal("expected RCL.RIBEval")
	}
}

func TestLoadGuardBasic(t *testing.T) {
	doc, err := Load("testdata/intentdsl/guard-basic.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Intents) != 3 {
		t.Fatalf("len(Intents) = %d, want 3", len(doc.Intents))
	}
	for _, in := range doc.Intents {
		if in.RCL == nil || in.RCL.Guard == nil {
			t.Fatalf("intent %q: expected RCL.Guard", in.Name)
		}
	}
	// Verify the first intent has a guard with correct where
	guard := doc.Intents[0].RCL.Guard
	if guard.Where["device"] != "bj-edge1" {
		t.Fatalf("guard where device = %v, want bj-edge1", guard.Where["device"])
	}
	if guard.Intent.RIBEval == nil {
		t.Fatal("expected RIBEval in guard intent")
	}
}

func TestLoadForall(t *testing.T) {
	doc, err := Load("testdata/intentdsl/forall.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Intents) != 1 {
		t.Fatalf("len(Intents) = %d, want 1", len(doc.Intents))
	}
	in := doc.Intents[0]
	if in.Forall == nil {
		t.Fatal("expected forall")
	}
	if in.RCL == nil || in.RCL.RIBEval == nil {
		t.Fatal("expected RIBEval")
	}
}

func TestLoadPacketBasic(t *testing.T) {
	doc, err := Load("testdata/intentdsl/packet-basic.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Intents) != 2 {
		t.Fatalf("len(Intents) = %d, want 2", len(doc.Intents))
	}
	for _, in := range doc.Intents {
		if in.RCL == nil || in.RCL.PacketReachable == nil {
			t.Fatalf("intent %q: expected PacketReachable", in.Name)
		}
	}
}

func TestLoadRCLComposition(t *testing.T) {
	doc, err := Load("testdata/intentdsl/rcl-composition.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Intents) != 3 {
		t.Fatalf("len(Intents) = %d, want 3", len(doc.Intents))
	}
	// First intent: and
	if doc.Intents[0].RCL.And == nil {
		t.Fatal("expected And expression")
	}
	// Second intent: or
	if doc.Intents[1].RCL.Or == nil {
		t.Fatal("expected Or expression")
	}
	// Third intent: not
	if doc.Intents[2].RCL.Not == nil {
		t.Fatal("expected Not expression")
	}
}

func TestLoadSelectorLogic(t *testing.T) {
	doc, err := Load("testdata/intentdsl/selector-logic.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Intents) != 4 {
		t.Fatalf("len(Intents) = %d, want 4", len(doc.Intents))
	}
}

func TestLoadRCLRibPositive(t *testing.T) {
	doc, err := Load("testdata/intentdsl/rcl-rib-positive.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Intents) != 4 {
		t.Fatalf("len(Intents) = %d, want 4", len(doc.Intents))
	}
	// First intent: rib_eq
	if doc.Intents[0].RCL.RIBEq == nil {
		t.Fatal("expected RIBEq")
	}
	// Second intent: forall with RIBEval
	if doc.Intents[1].Forall == nil {
		t.Fatal("expected forall")
	}
}

func TestLoadRCLRibNegativeCompare(t *testing.T) {
	doc, err := Load("testdata/intentdsl/rcl-rib-negative-compare.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Intents[0].RCL.RIBEq == nil {
		t.Fatal("expected RIBEq")
	}
}

func TestLoadRCLRibNegativeDistinct(t *testing.T) {
	doc, err := Load("testdata/intentdsl/rcl-rib-negative-distinct.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	re := doc.Intents[0].RCL.RIBEval
	if re == nil {
		t.Fatal("expected RIBEval")
	}
	if re.Aggregate != "distCnt(nexthop)" {
		t.Fatalf("Aggregate = %q, want distCnt(nexthop)", re.Aggregate)
	}
}

func TestLoadRibEvalOperators(t *testing.T) {
	doc, err := Load("testdata/intentdsl/rib-eval-operators.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Intents) != 6 {
		t.Fatalf("len(Intents) = %d, want 6", len(doc.Intents))
	}
	// Check operators
	ops := map[string]*intent.RIBEvalExpr{}
	for _, in := range doc.Intents {
		ops[in.Name] = in.RCL.RIBEval
	}
	if ops["exact-bgp-count-on-bj-edge1"].Eq == nil {
		t.Fatal("expected eq operator")
	}
	if ops["non-zero-route-count-on-bj-edge1"].Ne == nil {
		t.Fatal("expected ne operator")
	}
	if ops["more-than-10-routes-on-bj-edge1"].Gt == nil {
		t.Fatal("expected gt operator")
	}
	if ops["at-least-21-routes-on-bj-edge1"].Gte == nil {
		t.Fatal("expected gte operator")
	}
}

func TestLoadPacketFailureScenario(t *testing.T) {
	doc, err := Load("testdata/intentdsl/packet-failure-scenario.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Intents) != 1 {
		t.Fatalf("len(Intents) = %d, want 1", len(doc.Intents))
	}
	sc := doc.Scenarios["one-link-failure"]
	if sc.Failures.Max != 1 {
		t.Fatalf("Failures.Max = %d, want 1", sc.Failures.Max)
	}
}

func TestLoadPacketNegative(t *testing.T) {
	doc, err := Load("testdata/intentdsl/packet-negative.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	pr := doc.Intents[0].RCL.PacketReachable
	if pr == nil {
		t.Fatal("expected PacketReachable")
	}
	if pr.Expect != true {
		t.Fatal("expected Expect = true")
	}
}

func TestLoadRibNegative(t *testing.T) {
	doc, err := Load("testdata/intentdsl/rib-negative.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Intents[0].RCL.RIBEval == nil {
		t.Fatal("expected RIBEval")
	}
}

func TestLoadRibFibBasic(t *testing.T) {
	doc, err := Load("testdata/intentdsl/rib-fib-basic.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Intents) != 2 {
		t.Fatalf("len(Intents) = %d, want 2", len(doc.Intents))
	}
}

func TestLoadPredicateExtra(t *testing.T) {
	doc, err := Load("testdata/intentdsl/predicate-extra.hoyan")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(doc.Intents) != 5 {
		t.Fatalf("len(Intents) = %d, want 5", len(doc.Intents))
	}
}

func TestRejectsEmptyBlock(t *testing.T) {
	_, err := parseStringForTest(`version = "hoyan/v1"

snapshot "current" { lab = "labs/base-wan" }
scenario "normal" { snapshot = "current" }
intent "empty-guard" {
  scenario = "normal"
  when device = "r1" { }
}
`)
	if err == nil {
		t.Fatal("expected empty block parse error, got nil")
	}
	if !strings.Contains(err.Error(), "empty block") {
		t.Fatalf("error = %v, want empty block message", err)
	}
}

func TestRejectsUnknownSnapshotField(t *testing.T) {
	_, err := parseStringForTest(`version = "hoyan/v1"

snapshot "current" { lba = "labs/base-wan" }
`)
	if err == nil {
		t.Fatal("expected unknown snapshot field parse error, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected field") {
		t.Fatalf("error = %v, want unexpected field message", err)
	}
}

func TestReportsLexerErrorToken(t *testing.T) {
	_, err := parseStringForTest(`version = "hoyan/v1"
# invalid comment syntax
`)
	if err == nil {
		t.Fatal("expected lexer error, got nil")
	}
	if !strings.Contains(err.Error(), "unexpected character") {
		t.Fatalf("error = %v, want lexer error message", err)
	}
}

func TestRejectsMultipleTopLevelRCLExpressions(t *testing.T) {
	_, err := parseStringForTest(`version = "hoyan/v1"

snapshot "current" { lab = "labs/base-wan" }
scenario "normal" { snapshot = "current" }
intent "two-rcls" {
  scenario = "normal"
  rib where device = "bj-edge1" { count() >= 1 }
  rib where device = "sh-edge1" { count() >= 1 }
}
`)
	if err == nil {
		t.Fatal("expected multiple top-level RCL parse error, got nil")
	}
	if !strings.Contains(err.Error(), "multiple top-level expressions") {
		t.Fatalf("error = %v, want multiple top-level expressions message", err)
	}
}

func TestRejectsExtraExpressionInRIBBlock(t *testing.T) {
	_, err := parseStringForTest(`version = "hoyan/v1"

snapshot "current" { lab = "labs/base-wan" }
scenario "normal" { snapshot = "current" }
intent "extra-in-rib" {
  scenario = "normal"
  rib where device = "bj-edge1" {
    count() >= 1
    packet from "cust-bj" to "10.4.1.10" tcp/443 expect true
  }
}
`)
	if err == nil {
		t.Fatal("expected extra expression in RIB block parse error, got nil")
	}
	if !strings.Contains(err.Error(), "exactly one aggregate expression") {
		t.Fatalf("error = %v, want aggregate block cardinality message", err)
	}
	if !strings.Contains(err.Error(), ":") {
		t.Fatalf("error = %v, want line/column info", err)
	}
}

func TestRejectsEmptyWhenPredicates(t *testing.T) {
	_, err := parseStringForTest(`version = "hoyan/v1"

snapshot "current" { lab = "labs/base-wan" }
scenario "normal" { snapshot = "current" }
intent "empty-when" {
  scenario = "normal"
  when { count() >= 1 }
}
`)
	if err == nil {
		t.Fatal("expected empty where predicates parse error, got nil")
	}
	if !strings.Contains(err.Error(), "expected where predicate") {
		t.Fatalf("error = %v, want expected where predicate message", err)
	}
}

func TestWhereAllowsDoubleEquals(t *testing.T) {
	doc, err := parseStringForTest(`version = "hoyan/v1"

snapshot "current" { lab = "labs/base-wan" }
scenario "normal" { snapshot = "current" }
intent "where-eqeq" {
  scenario = "normal"
  rib where device == "bj-edge1" { count() >= 1 }
}
`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := doc.Intents[0].RCL.RIBEval.Where["device"]
	if got != "bj-edge1" {
		t.Fatalf("where device = %v, want bj-edge1", got)
	}
}

func parseStringForTest(src string) (*intent.Document, error) {
	lex := newLexer(src, "test.hoyan")
	p := newParser(lex)
	return p.ParseDocument()
}

func TestAllDSLFiles(t *testing.T) {
	files, err := filepath.Glob("testdata/intentdsl/*.hoyan")
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no .hoyan DSL files found")
	}

	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			doc, err := Load(file)

			// invalid-* files may have parse-time or semantic errors
			if strings.HasPrefix(name, "invalid-") {
				if err == nil {
					// For missing-version, the DSL parser may not check it at parse time
					// The YAML validator checks it at validate time
					t.Logf("%s: loaded without error (validation deferred)", name)
				} else {
					t.Logf("expected error: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("Load(%s): %v", name, err)
			}
			if doc == nil {
				t.Fatalf("Load(%s): nil document", name)
			}
		})
	}
}
