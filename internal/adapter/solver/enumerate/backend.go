package enumerate

import (
	"github.com/81ueman/hoyan-lab/internal/domain/solver"
	"github.com/81ueman/hoyan-lab/internal/domain/symbolic"
)

type Backend struct{}

func (Backend) SolveSymbolic(problem solver.SymbolicFailureProblem) (solver.Answer, error) {
	for k := 0; k <= problem.MaxFailures; k++ {
		var out []solver.FailureElement
		if findCombo(problem.Elements, k, 0, nil, func(combo []solver.FailureElement) bool {
			if evalSymbolicGoal(problem.Goal, combo) {
				out = append([]solver.FailureElement(nil), combo...)
				return true
			}
			return false
		}) {
			return solver.Answer{Sat: true, Failures: out, Backend: "enumerating-symbolic"}, nil
		}
	}
	return solver.Answer{Sat: false, Backend: "enumerating-symbolic"}, nil
}

func findCombo(elements []solver.FailureElement, want, start int, cur []solver.FailureElement, fn func([]solver.FailureElement) bool) bool {
	if len(cur) == want {
		return fn(cur)
	}
	for i := start; i < len(elements); i++ {
		cur = append(cur, elements[i])
		if findCombo(elements, want, i+1, cur, fn) {
			return true
		}
		cur = cur[:len(cur)-1]
	}
	return false
}

func evalSymbolicGoal(expr symbolic.Expr, failures []solver.FailureElement) bool {
	failed := map[string]bool{}
	for _, element := range failures {
		failed[element.String()] = true
	}
	var eval func(symbolic.Expr) bool
	eval = func(e symbolic.Expr) bool {
		switch e.Kind {
		case symbolic.KindTrue:
			return true
		case symbolic.KindFalse:
			return false
		case symbolic.KindVar:
			return !failed[string(e.VarKind)+":"+e.Name]
		case symbolic.KindAnd:
			for _, child := range e.Children {
				if !eval(child) {
					return false
				}
			}
			return true
		case symbolic.KindOr:
			for _, child := range e.Children {
				if eval(child) {
					return true
				}
			}
			return false
		case symbolic.KindNot:
			if len(e.Children) == 0 {
				return false
			}
			return !eval(e.Children[0])
		default:
			return true
		}
	}
	return eval(expr)
}
