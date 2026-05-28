package dataplane

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/solver/enumerate"
	"github.com/81ueman/hoyan-lab/internal/adapter/solver/z3"
	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/solver"
)

func TestPacketReachabilityFailureEnumerationMatchesZ3SymbolicBackend(t *testing.T) {
	engine := redundantPathEngine()
	from, to, protocol := "src", "10.0.0.10", "icmp"
	maxFailures := 2
	elements := []solver.FailureElement{
		{Kind: solver.FailureLink, Name: "src-primary"},
		{Kind: solver.FailureLink, Name: "primary-dst"},
		{Kind: solver.FailureLink, Name: "src-backup"},
		{Kind: solver.FailureLink, Name: "backup-dst"},
		{Kind: solver.FailureNode, Name: "primary"},
		{Kind: solver.FailureNode, Name: "backup"},
	}

	symbolicResult := engine.SymbolicPacketReachability(from, to, protocol)
	enumerated, err := (enumerate.Backend{}).SolveSymbolic(solver.SymbolicFailureProblem{
		Elements:    elements,
		MaxFailures: maxFailures,
		Goal:        failure.BoolExpr(symbolicResult.Unreachable),
	})
	if err != nil {
		t.Fatalf("enumerating symbolic SolveSymbolic() error = %v", err)
	}
	z3Symbolic, err := (z3.Backend{}).SolveSymbolic(solver.SymbolicFailureProblem{
		Elements:    elements,
		MaxFailures: maxFailures,
		Goal:        failure.BoolExpr(symbolicResult.Unreachable),
	})
	if err != nil {
		t.Fatalf("Z3 symbolic SolveSymbolic() error = %v", err)
	}
	assertSolverParityAnswer(t, engine, from, to, protocol, maxFailures, enumerated.Sat, z3Symbolic)
}

func TestRouteReachabilityFailureEnumerationMatchesZ3SymbolicBackend(t *testing.T) {
	engine := routeRedundantPathEngine()
	from, prefix := "src", "10.0.0.0/24"
	maxFailures := 2
	elements := []solver.FailureElement{
		{Kind: solver.FailureLink, Name: "src-primary"},
		{Kind: solver.FailureLink, Name: "primary-dst"},
		{Kind: solver.FailureLink, Name: "src-backup"},
		{Kind: solver.FailureLink, Name: "backup-dst"},
		{Kind: solver.FailureNode, Name: "primary"},
		{Kind: solver.FailureNode, Name: "backup"},
	}

	symbolicResult := engine.SymbolicRouteReachability(from, prefix)
	enumerated, err := (enumerate.Backend{}).SolveSymbolic(solver.SymbolicFailureProblem{
		Elements:    elements,
		MaxFailures: maxFailures,
		Goal:        failure.BoolExpr(symbolicResult.Unreachable),
	})
	if err != nil {
		t.Fatalf("enumerating route SolveSymbolic() error = %v", err)
	}
	z3Symbolic, err := (z3.Backend{}).SolveSymbolic(solver.SymbolicFailureProblem{
		Elements:    elements,
		MaxFailures: maxFailures,
		Goal:        failure.BoolExpr(symbolicResult.Unreachable),
	})
	if err != nil {
		t.Fatalf("Z3 route SolveSymbolic() error = %v", err)
	}
	assertRouteSolverParityAnswer(t, engine, from, prefix, maxFailures, enumerated.Sat, z3Symbolic)
}
