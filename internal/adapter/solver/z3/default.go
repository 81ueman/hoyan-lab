package z3

import "github.com/81ueman/hoyan-lab/internal/domain/solver"

func DefaultBackend() solver.Backend {
	return Backend{}
}
