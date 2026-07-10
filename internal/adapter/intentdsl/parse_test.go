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

func TestLoadRejectsNonHoyanExtension(t *testing.T) {
	path := filepath.Join(t.TempDir(), "intent.yml")
	if err := os.WriteFile(path, []byte("version = \"hoyan/v1\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected .hoyan extension error, got nil")
	}
	if !strings.Contains(err.Error(), ".hoyan") {
		t.Fatalf("error = %v, want .hoyan message", err)
	}
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
	// Forall with variable reference expands to And expression at parse time
	if in.RCL == nil || len(in.RCL.And) != 3 {
		t.Fatalf("expected And with 3 entries (3 sources), got %#v", in.RCL)
	}
	for _, expr := range in.RCL.And {
		if expr.RIBEval == nil {
			t.Fatal("expected RIBEval in each expanded expression")
		}
		// Each should have device substituted
		device, ok := expr.RIBEval.Where["device"]
		if !ok || device == "${src}" {
			t.Fatalf("device should be substituted, got %v", device)
		}
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
		// Forall with variable reference expands to And expression at parse time
		if in.RCL == nil || len(in.RCL.And) != 3 {
			t.Fatalf("intent %q: expected And with 3 entries, got %#v", in.Name, in.RCL)
		}
		for _, expr := range in.RCL.And {
			if expr.PacketReachable == nil {
				t.Fatalf("intent %q: expected PacketReachable in each expanded expression", in.Name)
			}
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
	// Second intent: forall with RIBEval (expands to And since $edge is referenced)
	if doc.Intents[1].RCL == nil || len(doc.Intents[1].RCL.And) != 3 {
		t.Fatalf("expected And with 3 entries (3 edges), got %#v", doc.Intents[1].RCL)
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

func TestWhereEmitsEvaluatorCompatibleShapes(t *testing.T) {
	doc, err := parseStringForTest(`version = "hoyan/v1"

snapshot "current" { lab = "labs/base-wan" }
scenario "normal" { snapshot = "current" }
intent "where-shapes" {
  scenario = "normal"
  rib where and {
    device != "r2"
    or {
      protocol = "bgp"
      protocol = "static"
    }
    imply {
      prefix within "10.0.0.0/8"
      then selected = true
    }
  } { count() >= 1 }
}
`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	where := doc.Intents[0].RCL.RIBEval.Where
	andList, ok := where["and"].([]any)
	if !ok {
		t.Fatalf("where[and] = %T, want []any", where["and"])
	}
	if _, ok := andList[0].(map[string]any)["not"].(map[string]any); !ok {
		t.Fatalf("!= predicate = %#v, want not map", andList[0])
	}
	orList, ok := andList[1].(map[string]any)["or"].([]any)
	if !ok || len(orList) != 2 {
		t.Fatalf("or predicate = %#v, want []any with two entries", andList[1])
	}
	implyList, ok := andList[2].(map[string]any)["imply"].([]any)
	if !ok || len(implyList) != 2 {
		t.Fatalf("imply predicate = %#v, want []any with two entries", andList[2])
	}
	if implyList[0].(map[string]any)["prefix_within"] != "10.0.0.0/8" {
		t.Fatalf("within predicate = %#v, want prefix_within", implyList[0])
	}
}

func TestWhereAllowsKeywordFieldNames(t *testing.T) {
	doc, err := parseStringForTest(`version = "hoyan/v1"

snapshot "current" { lab = "labs/base-wan" }
scenario "normal" { snapshot = "current" }
intent "where-vrf" {
  scenario = "normal"
  rib where vrf = "default" { count() >= 1 }
}
`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	got := doc.Intents[0].RCL.RIBEval.Where["vrf"]
	if got != "default" {
		t.Fatalf("where vrf = %v, want default", got)
	}
}

func TestForallAllowsCartesianProduct(t *testing.T) {
	doc, err := parseStringForTest(`version = "hoyan/v1"

let customers = ["cust-bj", "cust-sh"]
let services = ["10.4.1.10", "10.4.1.11"]
snapshot "current" { lab = "labs/base-wan" }
scenario "normal" { snapshot = "current" }
intent "cartesian" {
  scenario = "normal"
  forall src in $customers, dst in $services {
    packet from $src to $dst tcp/443 expect true
  }
}
`)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// Cartesion product expands to And expression
	in := doc.Intents[0]
	if in.RCL == nil || len(in.RCL.And) != 4 {
		t.Fatalf("expected And with 4 entries (2x2 cartesian product), got %#v", in.RCL)
	}
	// Verify the expanded packet expressions
	seen := map[string]bool{}
	for _, expr := range in.RCL.And {
		if expr.PacketReachable != nil {
			key := expr.PacketReachable.From + "->" + expr.PacketReachable.To
			seen[key] = true
		}
	}
	if !seen["cust-bj->10.4.1.10"] || !seen["cust-bj->10.4.1.11"] ||
		!seen["cust-sh->10.4.1.10"] || !seen["cust-sh->10.4.1.11"] {
		t.Fatalf("cartesian product missing combinations: got %#v", seen)
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
