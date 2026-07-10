package sim

import (
	"fmt"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/solver"
	"github.com/81ueman/hoyan-lab/internal/engine/controlplane"
)

// RacingRouterResult describes routes and racing status at one router.
type RacingRouterResult struct {
	Node            string   `json:"node"`
	RouteCount      int      `json:"route_count"`
	SatisfiableCount int     `json:"satisfiable_count"`
	RacingFound     bool     `json:"racing_found"`
	FirstModel      []string `json:"first_model,omitempty"`
	SecondModel     []string `json:"second_model,omitempty"`
}

// RacingResult describes the outcome of a racing detection for a single prefix.
type RacingResult struct {
	Prefix  string               `json:"prefix"`
	Routers []RacingRouterResult `json:"routers"`
	Racing  bool                 `json:"racing"`
}

// DetectRacing runs the route-update racing detection algorithm
// (SIGCOMM 2020 Appendix B) for the given prefix.
//
// Algorithm:
//  1. Re-propagate all routes with RacingPropagation=true
//  2. Run route selection and convergence
//  3. For each router that has the prefix, collect SelectedCond for all routes
//  4. For each router, check if multiple routes have satisfiable SelectedCond
//  5. If any router has multiple satisfiable routes → racing detected
func (g *Graph) DetectRacing(prefix model.Prefix) (*RacingResult, error) {
	engine := controlplane.NewEngine(g.topoIndex, g.rib)

	// Phase 1: Re-propagate with racing flag.
	engine.RacingPropagate()

	// Phase 2: Collect SelectedCond for all routes at each router.
	candidates := engine.CollectRacingCandidates(prefix)

	if len(candidates) == 0 {
		return &RacingResult{
			Prefix:  prefix.String(),
			Routers: nil,
			Racing:  false,
		}, nil
	}

	if g.solver == nil {
		return nil, fmt.Errorf("solver backend is not configured; use WithSolverBackend option")
	}

	// Collect candidate failure elements from the topology.
	elements := failure.SearchElements(g.topo, failure.SearchOptions{
		IncludeLinks: true,
		IncludeNodes: true,
	})
	if len(elements) == 0 {
		return nil, fmt.Errorf("no failure elements found in topology")
	}

	overallRacing := false
	var routerResults []RacingRouterResult

	// Phase 3: For each router, check if multiple routes have satisfiable SelectedCond.
	for node, conds := range candidates {
		result := RacingRouterResult{
			Node:       node,
			RouteCount: len(conds),
		}

		satisfiableCount := 0
		var firstFailureAssignment []solver.FailureElement

		for _, cond := range conds {
			if cond == nil {
				continue
			}
			if cond.Key() == failure.False().Key() {
				continue
			}
			expr := failure.BoolExpr(cond)
			problem := solver.SymbolicFailureProblem{
				Elements:    elements,
				MaxFailures: -1,
				Goal:        expr,
			}
			answer, err := g.solver.SolveSymbolic(problem)
			if err != nil {
				continue
			}
			if answer.Sat {
				satisfiableCount++
				if firstFailureAssignment == nil {
					firstFailureAssignment = answer.Failures
					result.FirstModel = answer.FailureStrings()
				} else {
					result.SecondModel = answer.FailureStrings()
				}
			}
		}

		result.SatisfiableCount = satisfiableCount
		if satisfiableCount >= 2 {
			result.RacingFound = true
			overallRacing = true
		}
		routerResults = append(routerResults, result)
	}

	// Sort for determinism.
	sort.Slice(routerResults, func(i, j int) bool {
		return routerResults[i].Node < routerResults[j].Node
	})

	return &RacingResult{
		Prefix:  prefix.String(),
		Routers: routerResults,
		Racing:  overallRacing,
	}, nil
}

// DetectAllRacing runs racing detection for all prefixes with multiple origins.
func (g *Graph) DetectAllRacing() []RacingResult {
	engine := controlplane.NewEngine(g.topoIndex, g.rib)
	prefixes := engine.PrefixWithMultipleOrigins()

	var results []RacingResult
	for _, prefix := range prefixes {
		result, err := g.DetectRacing(prefix)
		if err != nil {
			results = append(results, RacingResult{
				Prefix: prefix.String(),
				Racing: false,
			})
			continue
		}
		results = append(results, *result)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Prefix < results[j].Prefix
	})

	return results
}
