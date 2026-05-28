package enumerate

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/solver"
	"github.com/81ueman/hoyan-lab/internal/domain/symbolic"
)

func TestEnumeratingBackendSymbolicGoalWithNodeElement(t *testing.T) {
	ans, err := (Backend{}).SolveSymbolic(solver.SymbolicFailureProblem{
		Elements: []solver.FailureElement{
			{Kind: solver.FailureLink, Name: "l1"},
			{Kind: solver.FailureNode, Name: "n1"},
		},
		MaxFailures: 1,
		Goal:        symbolic.Not(symbolic.NodeVar("n1")),
	})
	if err != nil {
		t.Fatalf("SolveSymbolic() error = %v", err)
	}
	if !ans.Sat || len(ans.Failures) != 1 || ans.Failures[0].Kind != solver.FailureNode || ans.Failures[0].Name != "n1" {
		t.Fatalf("answer = %#v, want node n1", ans)
	}
}

func TestEnumeratingBackendSymbolicGoal(t *testing.T) {
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
	if !ans.Sat || len(ans.Failures) != 2 {
		t.Fatalf("answer = %#v, want two-failure symbolic cut", ans)
	}
}

func TestAnswerFailureStrings(t *testing.T) {
	got := (solver.Answer{Failures: []solver.FailureElement{{Kind: solver.FailureLink, Name: "l1"}, {Kind: solver.FailureNode, Name: "n1"}}}).FailureStrings()
	want := []string{"link:l1", "node:n1"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("FailureStrings() = %v, want %v", got, want)
	}
}

func linkElements(names ...string) []solver.FailureElement {
	out := make([]solver.FailureElement, 0, len(names))
	for _, name := range names {
		out = append(out, solver.FailureElement{Kind: solver.FailureLink, Name: name})
	}
	return out
}
