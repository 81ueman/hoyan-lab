package solver

import "github.com/81ueman/hoyan-lab/internal/domain/symbolic"

type FailureElementKind string

const (
	FailureLink FailureElementKind = "link"
	FailureNode FailureElementKind = "node"
)

type FailureElement struct {
	Kind FailureElementKind
	Name string
}

func (e FailureElement) String() string {
	return string(e.Kind) + ":" + e.Name
}

type SymbolicFailureProblem struct {
	Elements    []FailureElement
	MaxFailures int
	Goal        symbolic.Expr
}

type Answer struct {
	Sat      bool
	Failures []FailureElement
	Backend  string
}

func (a Answer) FailureStrings() []string {
	out := make([]string, 0, len(a.Failures))
	for _, f := range a.Failures {
		out = append(out, f.String())
	}
	return out
}

type Backend interface {
	SolveSymbolic(problem SymbolicFailureProblem) (Answer, error)
}
