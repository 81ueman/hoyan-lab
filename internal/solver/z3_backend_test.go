package solver

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/symbolic"
)

func TestZ3BackendSymbolicGoal(t *testing.T) {
	reachable := symbolic.Or(
		symbolic.And(symbolic.LinkVar("a"), symbolic.LinkVar("b")),
		symbolic.And(symbolic.LinkVar("c"), symbolic.LinkVar("d")),
	)
	ans, err := (Z3Backend{}).SolveSymbolic(SymbolicFailureProblem{
		Elements:    linkElements("a", "b", "c", "d"),
		MaxFailures: 1,
		Goal:        symbolic.Not(reachable),
	})
	if err != nil {
		t.Fatalf("SolveSymbolic() error = %v", err)
	}
	if ans.Sat {
		t.Fatalf("answer = %#v, want unsat with one failure against redundant paths", ans)
	}
	ans, err = (Z3Backend{}).SolveSymbolic(SymbolicFailureProblem{
		Elements:    linkElements("a", "b", "c", "d"),
		MaxFailures: 2,
		Goal:        symbolic.Not(reachable),
	})
	if err != nil {
		t.Fatalf("SolveSymbolic() error = %v", err)
	}
	if !ans.Sat || ans.Backend != "z3-symbolic" || len(ans.Failures) != 2 {
		t.Fatalf("answer = %#v, want two-failure symbolic cut", ans)
	}
}

func TestZ3BackendSymbolicMatchesEnumeratingBackend(t *testing.T) {
	elements := linkElements("a", "b", "c", "d")
	reachable := symbolic.Or(
		symbolic.And(symbolic.LinkVar("a"), symbolic.LinkVar("b")),
		symbolic.And(symbolic.LinkVar("c"), symbolic.LinkVar("d")),
	)
	enumerated, err := (EnumeratingBackend{}).SolveSymbolic(SymbolicFailureProblem{
		Elements:    elements,
		MaxFailures: 2,
		Goal:        symbolic.Not(reachable),
	})
	if err != nil {
		t.Fatalf("enumerating SolveSymbolic() error = %v", err)
	}
	symbolicAns, err := (Z3Backend{}).SolveSymbolic(SymbolicFailureProblem{
		Elements:    elements,
		MaxFailures: 2,
		Goal:        symbolic.Not(reachable),
	})
	if err != nil {
		t.Fatalf("SolveSymbolic() error = %v", err)
	}
	if !enumerated.Sat || !symbolicAns.Sat || len(enumerated.Failures) != len(symbolicAns.Failures) {
		t.Fatalf("enumerated=%#v symbolic=%#v, want matching SAT answers", enumerated, symbolicAns)
	}
}
