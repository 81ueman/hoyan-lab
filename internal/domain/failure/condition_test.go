package failure

import (
	"testing"
)

func TestNegatedLinkCount(t *testing.T) {
	tests := []struct {
		name string
		cond Cond
		want int
	}{
		{"true has none", True(), 0},
		{"false has none", False(), 0},
		{"link var alone", LinkVar("a-b"), 0},
		{"node var alone", NodeVar("a"), 0},
		{"negated link var", Not(LinkVar("a-b")), 1},
		{"double negated link resolves to identity", Not(Not(LinkVar("a-b"))), 0},
		{"negated node var is not counted", Not(NodeVar("a")), 0},
		{"and with one negated link", And(LinkVar("a-b"), Not(LinkVar("b-c"))), 1},
		{"and with two negated links", And(Not(LinkVar("a")), Not(LinkVar("b"))), 2},
		{"or with one negated link", Or(Not(LinkVar("a")), LinkVar("b")), 1},
		{"and inside not is not direct negation", Not(And(LinkVar("a"), LinkVar("b"))), 0},
		{"or inside not is not direct negation", Not(Or(LinkVar("a"), LinkVar("b"))), 0},
		{"nested not link in and", And(Not(Not(LinkVar("a"))), LinkVar("b")), 0},
		{"mixed negated and non-negated", And(Not(LinkVar("a")), Not(LinkVar("b")), LinkVar("c"), Not(LinkVar("d"))), 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NegatedLinkCount(tt.cond)
			if got != tt.want {
				t.Errorf("NegatedLinkCount() = %d, want %d\n  cond: %s", got, tt.want, tt.cond)
			}
		})
	}
}

func TestSimplifyCond(t *testing.T) {
	tests := []struct {
		name string
		cond Cond
		want Cond
	}{
		{"true simplifies to true", True(), True()},
		{"false simplifies to false", False(), False()},
		{"link var unchanged", LinkVar("a-b"), LinkVar("a-b")},
		{"node var unchanged", NodeVar("a"), NodeVar("a")},
		{"negated link var unchanged", Not(LinkVar("a-b")), Not(LinkVar("a-b"))},
		{"and with true child", And(True(), LinkVar("a")), LinkVar("a")},
		{"and with false child", And(False(), LinkVar("a")), False()},
		{"or with true child", Or(True(), LinkVar("a")), True()},
		{"or with false child", Or(False(), LinkVar("a")), LinkVar("a")},
		{"and of var and its negation", And(LinkVar("a"), Not(LinkVar("a"))), False()},
		{"or of var and its negation", Or(LinkVar("a"), Not(LinkVar("a"))), True()},
		{"and of node var and its negation", And(NodeVar("a"), Not(NodeVar("a"))), False()},
		{"or of node var and its negation", Or(NodeVar("a"), Not(NodeVar("a"))), True()},
		{"and with contradiction among children", And(LinkVar("a"), LinkVar("b"), Not(LinkVar("a"))), False()},
		{"nested and/or with contradiction", And(Or(LinkVar("a"), LinkVar("b")), Not(LinkVar("a")), Not(LinkVar("b"))), False()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SimplifyCond(tt.cond)
			if got.Key() != tt.want.Key() {
				t.Errorf("SimplifyCond() key = %s, want %s\n  got: %s\n  want: %s", got.Key(), tt.want.Key(), got, tt.want)
			}
		})
	}
}
