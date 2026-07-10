package bgp

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
)

func TestCheckPrune(t *testing.T) {
	tests := []struct {
		name        string
		cond        failure.Cond
		maxFailures int
		want        PruneResult
	}{
		{"negative max disables pruning", failure.LinkVar("a-b"), -1, PruneNone},
		{"zero max still allows no-negation", failure.True(), 0, PruneNone},
		{"single negated link with k=0", failure.Not(failure.LinkVar("a-b")), 0, PruneMoreThanKFailures},
		{"single negated link with k=1", failure.Not(failure.LinkVar("a-b")), 1, PruneNone},
		{"two negated links with k=1", failure.And(failure.Not(failure.LinkVar("a")), failure.Not(failure.LinkVar("b"))), 1, PruneMoreThanKFailures},
		{"two negated links with k=2", failure.And(failure.Not(failure.LinkVar("a")), failure.Not(failure.LinkVar("b"))), 2, PruneNone},
		{"impossible condition", failure.And(failure.LinkVar("a"), failure.Not(failure.LinkVar("a"))), 5, PruneImpossibleCondition},
		{"false condition", failure.False(), 5, PruneImpossibleCondition},
		{"nested impossible condition", failure.And(failure.Or(failure.LinkVar("a"), failure.LinkVar("b")), failure.Not(failure.LinkVar("a")), failure.Not(failure.LinkVar("b"))), 5, PruneImpossibleCondition},
		{"negated link count exceeds k on complex", failure.And(failure.Not(failure.LinkVar("a")), failure.Not(failure.LinkVar("b")), failure.Not(failure.LinkVar("c")), failure.LinkVar("d")), 2, PruneMoreThanKFailures},
		{"impossible takes priority over more-than-k", failure.And(failure.LinkVar("a"), failure.Not(failure.LinkVar("a")), failure.Not(failure.LinkVar("b")), failure.Not(failure.LinkVar("c"))), 0, PruneImpossibleCondition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CheckPrune(tt.cond, tt.maxFailures)
			if got != tt.want {
				t.Errorf("CheckPrune() = %v, want %v\n  cond: %s\n  maxFailures: %d", got, tt.want, tt.cond, tt.maxFailures)
			}
		})
	}
}
