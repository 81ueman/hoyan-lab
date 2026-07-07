package solveradapter

import (
	"github.com/81ueman/hoyan-lab/internal/adapter/solver/enumerate"
	"github.com/81ueman/hoyan-lab/internal/adapter/solver/z3"
	"github.com/81ueman/hoyan-lab/internal/domain/solver"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/intent"
)

func init() {
	intent.SetDefaultGraphOptions(sim.WithSolverBackend(DefaultBackend()))
}

type fallbackBackend struct {
	primary  solver.Backend
	fallback solver.Backend
}

func DefaultBackend() solver.Backend {
	return fallbackBackend{
		primary:  z3.DefaultBackend(),
		fallback: enumerate.Backend{},
	}
}

func (b fallbackBackend) SolveSymbolic(problem solver.SymbolicFailureProblem) (solver.Answer, error) {
	ans, err := b.primary.SolveSymbolic(problem)
	if err == nil {
		return ans, nil
	}
	return b.fallback.SolveSymbolic(problem)
}
