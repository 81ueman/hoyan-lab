package sim

import (
	"fmt"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/solver"
	"github.com/81ueman/hoyan-lab/internal/domain/symbolic"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
	"github.com/81ueman/hoyan-lab/internal/engine/controlplane"
)

// RacingRouterResult describes routes and racing status at one router.
type RacingRouterResult struct {
	Node             string   `json:"node"`
	RouteCount       int      `json:"route_count"`
	SatisfiableCount int      `json:"satisfiable_count"`
	RacingFound      bool     `json:"racing_found"`
	FirstModel       []string `json:"first_model,omitempty"`
	SecondModel      []string `json:"second_model,omitempty"`
}

// RacingResult describes the outcome of a racing detection for a single prefix.
type RacingResult struct {
	Prefix  string               `json:"prefix"`
	Routers []RacingRouterResult `json:"routers"`
	Racing  bool                 `json:"racing"`
}

// copyRIB creates a deep copy of the RIB table so that racing propagation
// on one prefix does not contaminate subsequent detection runs.
func copyRIB(original domainroute.RIBTable) domainroute.RIBTable {
	cpy := domainroute.RIBTable{}
	for nodeID, byVRF := range original {
		vrfCopy := map[model.NetworkInstanceID]map[model.Prefix][]domainroute.RIBEntry{}
		for vrf, byPrefix := range byVRF {
			prefixCopy := map[model.Prefix][]domainroute.RIBEntry{}
			for prefix, entries := range byPrefix {
				entriesCopy := make([]domainroute.RIBEntry, len(entries))
				copy(entriesCopy, entries)
				prefixCopy[prefix] = entriesCopy
			}
			vrfCopy[vrf] = prefixCopy
		}
		cpy[nodeID] = vrfCopy
	}
	return cpy
}

// routerSelectedCond returns the SelectedCond that evaluates to true for the given
// router under the given failure context, or nil if none does.
func routerSelectedCond(conds []failure.Cond, ctx failure.Context) failure.Cond {
	for _, cond := range conds {
		if cond == nil || cond.Key() == failure.False().Key() {
			continue
		}
		if cond.Eval(ctx) {
			return cond
		}
	}
	return nil
}

// DetectRacing runs the route-update racing detection algorithm
// (SIGCOMM 2020 Appendix B) for the given prefix.
//
// Algorithm (per spec §5.4, §7.1, Appendix B):
//  1. Re-propagate all routes with RacingPropagation=true
//  2. Run route selection and convergence
//  3. Collect SelectedCond for all routes at each router
//  4. Build F = AND over routers of (OR over their routes' SelectedCond)
//  5. Find a satisfying assignment (model A) for F via Z3
//  6. From model A, determine which route is selected at each router
//  7. Block those specific route selections: OR(¬S_selected_at_R) for each R
//  8. Solve F ∧ blocking — if SAT → different routing outcome exists → racing
func (g *Graph) DetectRacing(prefix model.Prefix) (*RacingResult, error) {
	if g.solver == nil {
		return nil, fmt.Errorf("solver backend is not configured; use WithSolverBackend option")
	}

	// Work on a copy of the RIB to avoid cross-prefix contamination.
	ribCopy := copyRIB(g.rib)

	// Phase 1: Re-propagate with racing flag.
	engine := controlplane.NewEngine(g.topoIndex, ribCopy)
	engine.RacingPropagate()

	// Phase 2: Collect SelectedCond for all BGP routes at each router.
	candidates := engine.CollectRacingCandidates(prefix)

	if len(candidates) == 0 {
		return &RacingResult{
			Prefix:  prefix.String(),
			Routers: nil,
			Racing:  false,
		}, nil
	}

	// Collect candidate failure elements from the topology.
	elements := failure.SearchElements(g.topo, failure.SearchOptions{
		IncludeLinks: true,
		IncludeNodes: true,
	})
	if len(elements) == 0 {
		return nil, fmt.Errorf("no failure elements found in topology")
	}

	// Phase 3: Build F = AND over R of (OR over r of S_R_r)
	var routerOrs []failure.Cond
	for _, conds := range candidates {
		routerOrs = append(routerOrs, failure.Or(conds...))
	}
	F := failure.And(routerOrs...)

	// Phase 4: Convert to symbolic and solve.
	expr := failure.BoolExpr(F)

	firstProblem := solver.SymbolicFailureProblem{
		Elements:    elements,
		MaxFailures: -1,
		Goal:        expr,
	}

	answer1, err := g.solver.SolveSymbolic(firstProblem)
	if err != nil {
		return nil, fmt.Errorf("solver error (first pass): %w", err)
	}
	if !answer1.Sat {
		// No assignment satisfies the combined formula — no routing outcome possible.
		return &RacingResult{
			Prefix:  prefix.String(),
			Routers: nil,
			Racing:  false,
		}, nil
	}

	// Phase 5: Determine which route is selected at each router in model A.
	ctx := failure.Context{
		Failures:    failure.SetFromElements(answer1.Failures),
		LinksByName: g.topoIndex.LinksByName,
	}

	selectedInModel := map[string]failure.Cond{}
	for node, conds := range candidates {
		for _, cond := range conds {
			if cond == nil || cond.Key() == failure.False().Key() {
				continue
			}
			if cond.Eval(ctx) {
				selectedInModel[node] = cond
				break
			}
		}
	}

	// Phase 6: Build blocking formula.
	// We require at least one router to NOT select its model-A route.
	var blockingParts []symbolic.Expr
	for _, cond := range selectedInModel {
		blockExpr := failure.BoolExpr(failure.Not(cond))
		blockingParts = append(blockingParts, blockExpr)
	}
	if len(blockingParts) == 0 {
		return &RacingResult{
			Prefix:  prefix.String(),
			Routers: nil,
			Racing:  false,
		}, nil
	}
	blockExpr := symbolic.Or(blockingParts...)

	// Phase 7: Solve F ∧ blocking to find a different routing outcome.
	secondProblem := solver.SymbolicFailureProblem{
		Elements:    elements,
		MaxFailures: -1,
		Goal:        symbolic.And(expr, blockExpr),
	}

	answer2, err := g.solver.SolveSymbolic(secondProblem)
	if err != nil {
		return nil, fmt.Errorf("solver error (second pass): %w", err)
	}

	racing := answer2.Sat

	// Phase 8: Build detailed per-router diagnostics.
	var routerResults []RacingRouterResult
	for node, conds := range candidates {
		result := RacingRouterResult{
			Node:       node,
			RouteCount: len(conds),
		}

		satisfiableCount := 0
		for _, cond := range conds {
			if cond == nil || cond.Key() == failure.False().Key() {
				continue
			}
			evalCtx := failure.Context{
				Failures:    failure.SetFromElements(answer1.Failures),
				LinksByName: g.topoIndex.LinksByName,
			}
			if cond.Eval(evalCtx) {
				satisfiableCount++
				if result.FirstModel == nil {
					result.FirstModel = answer1.FailureStrings()
				}
			}
		}
		// For the second model, re-evaluate under answer2 if racing found.
		if racing {
			satisfiableCount2 := 0
			evalCtx2 := failure.Context{
				Failures:    failure.SetFromElements(answer2.Failures),
				LinksByName: g.topoIndex.LinksByName,
			}
			for _, cond := range conds {
				if cond == nil || cond.Key() == failure.False().Key() {
					continue
				}
				if cond.Eval(evalCtx2) {
					satisfiableCount2++
					if satisfiableCount2 > 0 && result.SecondModel == nil {
						result.SecondModel = answer2.FailureStrings()
					}
				}
			}
			// If second model selects a different route at this router, flag it.
			if satisfiableCount2 > 0 {
				// Check if the selected route differs from model A.
				selectedInA := ""
				selectedInB := ""
				for _, cond := range conds {
					if cond == nil || cond.Key() == failure.False().Key() {
						continue
					}
					if cond.Eval(failure.Context{Failures: failure.SetFromElements(answer1.Failures), LinksByName: g.topoIndex.LinksByName}) {
						selectedInA = cond.Key()
					}
					if cond.Eval(failure.Context{Failures: failure.SetFromElements(answer2.Failures), LinksByName: g.topoIndex.LinksByName}) {
						selectedInB = cond.Key()
					}
				}
				if selectedInA != "" && selectedInB != "" && selectedInA != selectedInB {
					result.RacingFound = true
				}
			}
		}

		result.SatisfiableCount = satisfiableCount
		if !racing {
			// Even if no global racing, still report per-router satisfiability.
			count := 0
			for _, cond := range conds {
				if cond == nil || cond.Key() == failure.False().Key() {
					continue
				}
				// Check satisfiability independently
				evalCtx := failure.Context{
					Failures:    failure.SetFromElements(answer1.Failures),
					LinksByName: g.topoIndex.LinksByName,
				}
				if cond.Eval(evalCtx) {
					count++
				}
			}
			result.SatisfiableCount = count
		}
		routerResults = append(routerResults, result)
	}

	sort.Slice(routerResults, func(i, j int) bool {
		return routerResults[i].Node < routerResults[j].Node
	})

	return &RacingResult{
		Prefix:  prefix.String(),
		Routers: routerResults,
		Racing:  racing,
	}, nil
}

// DetectAllRacing runs racing detection for all prefixes with multiple BGP origins.
func (g *Graph) DetectAllRacing() []RacingResult {
	// Collect prefixes first, before any racing propagation.
	initialEngine := controlplane.NewEngine(g.topoIndex, g.rib)
	prefixes := initialEngine.PrefixWithMultipleOrigins()

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
