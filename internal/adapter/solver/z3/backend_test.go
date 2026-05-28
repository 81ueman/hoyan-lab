package z3

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/solver/enumerate"
	"github.com/81ueman/hoyan-lab/internal/domain/solver"
	"github.com/81ueman/hoyan-lab/internal/domain/symbolic"
)

func TestZ3BackendSymbolicGoal(t *testing.T) {
	reachable := symbolic.Or(
		symbolic.And(symbolic.LinkVar("a"), symbolic.LinkVar("b")),
		symbolic.And(symbolic.LinkVar("c"), symbolic.LinkVar("d")),
	)
	ans, err := (Backend{}).SolveSymbolic(solver.SymbolicFailureProblem{
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
	ans, err = (Backend{}).SolveSymbolic(solver.SymbolicFailureProblem{
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
	enumerated, err := (enumerate.Backend{}).SolveSymbolic(solver.SymbolicFailureProblem{
		Elements:    elements,
		MaxFailures: 2,
		Goal:        symbolic.Not(reachable),
	})
	if err != nil {
		t.Fatalf("enumerating SolveSymbolic() error = %v", err)
	}
	symbolicAns, err := (Backend{}).SolveSymbolic(solver.SymbolicFailureProblem{
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

func linkElements(names ...string) []solver.FailureElement {
	out := make([]solver.FailureElement, 0, len(names))
	for _, name := range names {
		out = append(out, solver.FailureElement{Kind: solver.FailureLink, Name: name})
	}
	return out
}
