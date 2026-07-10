package failure

import (
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/symbolic"
)

type Cond interface {
	Eval(ctx Context) bool
	Key() string
	String() string
}

type condVarKind string

const (
	condVarLink condVarKind = "link"
	condVarNode condVarKind = "node"
)

type trueCond struct{}
type falseCond struct{}
type varCond struct {
	kind condVarKind
	name string
}
type andCond []Cond
type orCond []Cond
type notCond struct{ c Cond }

func True() Cond  { return trueCond{} }
func False() Cond { return falseCond{} }
func LinkVar(name string) Cond {
	return varCond{kind: condVarLink, name: name}
}
func NodeVar(name string) Cond {
	return varCond{kind: condVarNode, name: name}
}
func And(cs ...Cond) Cond { return flattenAnd(cs) }
func Or(cs ...Cond) Cond  { return flattenOr(cs) }
func Not(c Cond) Cond     { return simplifyNot(c) }

func (trueCond) Eval(Context) bool { return true }
func (trueCond) Key() string       { return "true" }
func (trueCond) String() string    { return "true" }

func (falseCond) Eval(Context) bool { return false }
func (falseCond) Key() string       { return "false" }
func (falseCond) String() string    { return "false" }

func (c varCond) Eval(ctx Context) bool {
	switch c.kind {
	case condVarNode:
		return !ctx.NodeFailed(model.NodeID(c.name))
	case condVarLink:
		return !ctx.LinkFailed(model.LinkID(c.name))
	default:
		return true
	}
}
func (c varCond) Key() string    { return "var:" + string(c.kind) + ":" + c.name }
func (c varCond) String() string { return string(c.kind) + ":" + c.name }

func (c andCond) Eval(ctx Context) bool {
	for _, x := range c {
		if !x.Eval(ctx) {
			return false
		}
	}
	return true
}
func (c andCond) Key() string    { return joinCondKey("and", c) }
func (c andCond) String() string { return joinCond(" && ", c) }

func (c orCond) Eval(ctx Context) bool {
	for _, x := range c {
		if x.Eval(ctx) {
			return true
		}
	}
	return false
}
func (c orCond) Key() string    { return joinCondKey("or", c) }
func (c orCond) String() string { return joinCond(" || ", c) }

func (c notCond) Eval(ctx Context) bool {
	return !c.c.Eval(ctx)
}
func (c notCond) Key() string    { return "not(" + c.c.Key() + ")" }
func (c notCond) String() string { return "!(" + c.c.String() + ")" }

func flattenAnd(cs []Cond) Cond {
	var out []Cond
	seen := map[string]bool{}
	for _, c := range cs {
		c = normalizeCond(c)
		switch x := c.(type) {
		case trueCond:
			continue
		case falseCond:
			return falseCond{}
		case andCond:
			for _, child := range x {
				if seen[child.Key()] {
					continue
				}
				seen[child.Key()] = true
				out = append(out, child)
			}
			continue
		}
		if seen[c.Key()] {
			continue
		}
		seen[c.Key()] = true
		out = append(out, c)
	}
	if len(out) == 0 {
		return trueCond{}
	}
	if len(out) == 1 {
		return out[0]
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key() < out[j].Key()
	})
	return andCond(out)
}

func flattenOr(cs []Cond) Cond {
	var out []Cond
	seen := map[string]bool{}
	for _, c := range cs {
		c = normalizeCond(c)
		switch x := c.(type) {
		case trueCond:
			return trueCond{}
		case falseCond:
			continue
		case orCond:
			for _, child := range x {
				if seen[child.Key()] {
					continue
				}
				seen[child.Key()] = true
				out = append(out, child)
			}
			continue
		}
		if seen[c.Key()] {
			continue
		}
		seen[c.Key()] = true
		out = append(out, c)
	}
	if len(out) == 0 {
		return falseCond{}
	}
	if len(out) == 1 {
		return out[0]
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Key() < out[j].Key()
	})
	return orCond(out)
}

func simplifyNot(c Cond) Cond {
	c = normalizeCond(c)
	switch x := c.(type) {
	case trueCond:
		return falseCond{}
	case falseCond:
		return trueCond{}
	case notCond:
		return x.c
	default:
		return notCond{c: c}
	}
}

func normalizeCond(c Cond) Cond {
	switch x := c.(type) {
	case andCond:
		return flattenAnd(x)
	case orCond:
		return flattenOr(x)
	case notCond:
		return simplifyNot(x.c)
	default:
		return c
	}
}

// NegatedLinkCount counts the number of directly negated link variables
// in a condition. A directly negated link variable is a notCond whose
// child is a varCond with kind condVarLink. This is used for the
// more-than-k-failure pruning optimization.
func NegatedLinkCount(c Cond) int {
	c = normalizeCond(c)
	switch x := c.(type) {
	case notCond:
		if v, ok := x.c.(varCond); ok && v.kind == condVarLink {
			return 1
		}
		return 0
	case andCond:
		total := 0
		for _, child := range x {
			total += NegatedLinkCount(child)
		}
		return total
	case orCond:
		total := 0
		for _, child := range x {
			total += NegatedLinkCount(child)
		}
		return total
	default:
		return 0
	}
}

// SimplifyCond applies additional simplification rules beyond normalizeCond
// to reduce the variable count in a condition. Specifically:
//   - And(x, Not(x), ...) → False (contradiction)
//   - Or(x, Not(x), ...) → True (tautology)
//   - And(Or(a, b), Not(a), Not(b)) → False (De Morgan)
//   - Or(And(a, b), Not(a), Not(b)) → True (De Morgan)
// It recursively simplifies children and then applies these deeper checks.
func SimplifyCond(c Cond) Cond {
	c = normalizeCond(c)
	switch x := c.(type) {
	case andCond:
		// Recursively simplify children first
		children := make(andCond, len(x))
		for i, child := range x {
			children[i] = SimplifyCond(child)
		}
		norm := normalizeCond(children)
		if _, ok := norm.(falseCond); ok {
			return norm
		}
		if ac, ok := norm.(andCond); ok {
			return simplifyAnd(ac)
		}
		return norm
	case orCond:
		children := make(orCond, len(x))
		for i, child := range x {
			children[i] = SimplifyCond(child)
		}
		norm := normalizeCond(children)
		if _, ok := norm.(trueCond); ok {
			return norm
		}
		if oc, ok := norm.(orCond); ok {
			return simplifyOr(oc)
		}
		return norm
	default:
		return c
	}
}

func simplifyAnd(c andCond) Cond {
	// Direct contradiction: x and Not(x)
	for i, ci := range c {
		ciNorm := normalizeCond(ci)
		for j := i + 1; j < len(c); j++ {
			cjNorm := normalizeCond(c[j])
			if isNegation(ciNorm, cjNorm) {
				return falseCond{}
			}
		}
	}
	// Contradiction via or-child: And(Or(a, b), Not(a), Not(b)) → False
	for _, ci := range c {
		if or, ok := ci.(orCond); ok {
			allNegated := true
			for _, orChild := range or {
				found := false
				for _, cj := range c {
					if isNegation(orChild, cj) {
						found = true
						break
					}
				}
				if !found {
					allNegated = false
					break
				}
			}
			if allNegated {
				return falseCond{}
			}
		}
	}
	return c
}

func simplifyOr(c orCond) Cond {
	// Direct tautology: x or Not(x)
	for i, ci := range c {
		ciNorm := normalizeCond(ci)
		for j := i + 1; j < len(c); j++ {
			cjNorm := normalizeCond(c[j])
			if isNegation(ciNorm, cjNorm) {
				return trueCond{}
			}
		}
	}
	// Tautology via and-child: Or(And(a, b), Not(a), Not(b)) → True
	for _, ci := range c {
		if and, ok := ci.(andCond); ok {
			allNegated := true
			for _, andChild := range and {
				found := false
				for _, cj := range c {
					if isNegation(andChild, cj) {
						found = true
						break
					}
				}
				if !found {
					allNegated = false
					break
				}
			}
			if allNegated {
				return trueCond{}
			}
		}
	}
	return c
}

func isNegation(a, b Cond) bool {
	a = normalizeCond(a)
	b = normalizeCond(b)
	// Check if a is Not(b)
	if n, ok := a.(notCond); ok {
		if equalCond(n.c, b) {
			return true
		}
	}
	// Check if b is Not(a)
	if n, ok := b.(notCond); ok {
		if equalCond(n.c, a) {
			return true
		}
	}
	return false
}

func equalCond(a, b Cond) bool {
	return a.Key() == b.Key()
}

func BoolExpr(c Cond) symbolic.Expr {
	c = normalizeCond(c)
	switch x := c.(type) {
	case trueCond:
		return symbolic.True()
	case falseCond:
		return symbolic.False()
	case varCond:
		switch x.kind {
		case condVarNode:
			return symbolic.NodeVar(x.name)
		case condVarLink:
			return symbolic.LinkVar(x.name)
		default:
			return symbolic.True()
		}
	case andCond:
		children := make([]symbolic.Expr, 0, len(x))
		for _, child := range x {
			children = append(children, BoolExpr(child))
		}
		return symbolic.And(children...)
	case orCond:
		children := make([]symbolic.Expr, 0, len(x))
		for _, child := range x {
			children = append(children, BoolExpr(child))
		}
		return symbolic.Or(children...)
	case notCond:
		return symbolic.Not(BoolExpr(x.c))
	default:
		return symbolic.True()
	}
}

func ExpandLinkVars(c Cond, linksByName map[model.LinkID]model.Link) Cond {
	c = normalizeCond(c)
	switch x := c.(type) {
	case varCond:
		if x.kind != condVarLink {
			return x
		}
		link, ok := linksByName[model.LinkID(x.name)]
		if !ok {
			return x
		}
		return And(x, NodeVar(link.A), NodeVar(link.B))
	case andCond:
		children := make([]Cond, 0, len(x))
		for _, child := range x {
			children = append(children, ExpandLinkVars(child, linksByName))
		}
		return And(children...)
	case orCond:
		children := make([]Cond, 0, len(x))
		for _, child := range x {
			children = append(children, ExpandLinkVars(child, linksByName))
		}
		return Or(children...)
	case notCond:
		return Not(ExpandLinkVars(x.c, linksByName))
	default:
		return c
	}
}

func joinCondKey(op string, cs []Cond) string {
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		parts = append(parts, c.Key())
	}
	return op + "(" + strings.Join(parts, ",") + ")"
}

func joinCond(sep string, cs []Cond) string {
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		parts = append(parts, c.String())
	}
	return "(" + strings.Join(parts, sep) + ")"
}
