package bgp

import (
	"github.com/81ueman/hoyan-lab/internal/domain/failure"
)

// PruneResult indicates the outcome of a pruning check during BGP route
// propagation.
type PruneResult int

const (
	// PruneNone indicates the route should not be pruned.
	PruneNone PruneResult = iota
	// PruneMoreThanKFailures indicates the route condition contains more
	// negated link variables than the maximum allowed failures.
	PruneMoreThanKFailures
	// PruneImpossibleCondition indicates the route condition is a
	// contradiction (always false).
	PruneImpossibleCondition
)

// CheckPrune evaluates whether a route condition should be pruned during
// BGP propagation based on two criteria:
//
//  1. More-than-k-failure: if the number of negated link variables in the
//     condition exceeds maxFailures, the route is pruned.
//  2. Impossible condition: if the condition simplifies to False, the
//     route is pruned.
//
// If maxFailures is negative, pruning is disabled and PruneNone is always
// returned.
//
// The impossible-condition check takes priority: if the condition is both
// impossible and has too many negated links, PruneImpossibleCondition is
// returned.
func CheckPrune(cond failure.Cond, maxFailures int) PruneResult {
	if maxFailures < 0 {
		return PruneNone
	}
	simplified := failure.SimplifyCond(cond)
	if simplified.Key() == failure.False().Key() {
		return PruneImpossibleCondition
	}
	if failure.NegatedLinkCount(simplified) > maxFailures {
		return PruneMoreThanKFailures
	}
	return PruneNone
}
