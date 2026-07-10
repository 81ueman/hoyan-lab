// Package intent provides the intent DSL expansion engine.
//
// Expand resolves ${var} variable references using document-level Vars
// and forall variable bindings, then expands forall clauses into their
// cartesian product of concrete intents.
package intent

import (
	"regexp"
	"sort"
)

var varRefRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func Expand(doc *Document) (*ExpandedDocument, error) {
	if err := Validate(doc); err != nil {
		return nil, err
	}
	var intents []Intent
	for _, in := range doc.Intents {
		expanded, err := expandIntent(in, doc.Vars)
		if err != nil {
			return nil, err
		}
		intents = append(intents, expanded...)
	}
	return &ExpandedDocument{
		Version:   doc.Version,
		Snapshots: doc.Snapshots,
		Scenarios: doc.Scenarios,
		Intents:   intents,
	}, nil
}

// expandIntent performs variable substitution on an intent and returns the result.
// Doc-level forall expansion is no longer supported; use RCL-level forall instead.
func expandIntent(in Intent, vars map[string]any) ([]Intent, error) {
	out, err := substituteIntent(in, vars, nil)
	if err != nil {
		return nil, err
	}
	return []Intent{out}, nil
}

// substituteIntent creates a copy of the intent with all variable references
// resolved. The group parameter carries forall variable bindings (may be nil).
func substituteIntent(in Intent, vars map[string]any, group map[string]string) (Intent, error) {
	rcl, err := expandRCLExpr(in.RCL, vars, group)
	if err != nil {
		return Intent{}, err
	}
	return Intent{
		Name:     in.Name,
		Scenario: in.Scenario,
		RCL:      rcl,
	}, nil
}

// expandRCLExpr recursively expands variable references in all fields of an RCLExpr.
// It returns a new RCLExpr with substitutions applied; the original is not modified.
func expandRCLExpr(expr *RCLExpr, vars map[string]any, group map[string]string) (*RCLExpr, error) {
	if expr == nil {
		return nil, nil
	}
	out := &RCLExpr{}
	switch {
	case expr.Guard != nil:
		subIntent, err := expandRCLExpr(&expr.Guard.Intent, vars, group)
		if err != nil {
			return nil, err
		}
		out.Guard = &GuardExpr{
			Where:  substituteMap(expr.Guard.Where, vars, group),
			Intent: *subIntent,
		}
	case expr.Forall != nil:
		subIntent, err := expandRCLExpr(&expr.Forall.Intent, vars, group)
		if err != nil {
			return nil, err
		}
		out.Forall = &ForallExpr{
			Var:    substituteString(expr.Forall.Var, vars, group),
			In:     substituteStringSlice(expr.Forall.In, vars, group),
			Intent: *subIntent,
		}
	case len(expr.And) > 0:
		out.And = make([]RCLExpr, len(expr.And))
		for i := range expr.And {
			expanded, err := expandRCLExpr(&expr.And[i], vars, group)
			if err != nil {
				return nil, err
			}
			out.And[i] = *expanded
		}
	case len(expr.Or) > 0:
		out.Or = make([]RCLExpr, len(expr.Or))
		for i := range expr.Or {
			expanded, err := expandRCLExpr(&expr.Or[i], vars, group)
			if err != nil {
				return nil, err
			}
			out.Or[i] = *expanded
		}
	case expr.Not != nil:
		var err error
		out.Not, err = expandRCLExpr(expr.Not, vars, group)
		if err != nil {
			return nil, err
		}
	case expr.Imply[0] != nil || expr.Imply[1] != nil:
		var err error
		out.Imply[0], err = expandRCLExpr(expr.Imply[0], vars, group)
		if err != nil {
			return nil, err
		}
		out.Imply[1], err = expandRCLExpr(expr.Imply[1], vars, group)
		if err != nil {
			return nil, err
		}
	case expr.RIBEq != nil:
		out.RIBEq = &RIBEqExpr{
			Left:  substituteString(expr.RIBEq.Left, vars, group),
			Right: substituteString(expr.RIBEq.Right, vars, group),
			Where: substituteMap(expr.RIBEq.Where, vars, group),
		}
	case expr.RIBEval != nil:
		out.RIBEval = &RIBEvalExpr{
			Snapshot:  substituteString(expr.RIBEval.Snapshot, vars, group),
			Where:     substituteMap(expr.RIBEval.Where, vars, group),
			Aggregate: substituteString(expr.RIBEval.Aggregate, vars, group),
			Eq:        substituteAnySlice(expr.RIBEval.Eq, vars, group),
			Ne:        substituteAnySlice(expr.RIBEval.Ne, vars, group),
			Gt:        expr.RIBEval.Gt,
			Gte:       expr.RIBEval.Gte,
			Lt:        expr.RIBEval.Lt,
			Lte:       expr.RIBEval.Lte,
		}
	case expr.PacketReachable != nil:
		out.PacketReachable = &PacketReachableExpr{
			From:     substituteString(expr.PacketReachable.From, vars, group),
			VRF:      substituteString(expr.PacketReachable.VRF, vars, group),
			To:       substituteString(expr.PacketReachable.To, vars, group),
			Protocol: substituteString(expr.PacketReachable.Protocol, vars, group),
			DstPort:  expr.PacketReachable.DstPort,
			Expect:   expr.PacketReachable.Expect,
		}
	default:
		// No recognized field set – return a shallow copy to avoid pointer aliasing.
		cpy := *expr
		return &cpy, nil
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Variable substitution helpers
// ---------------------------------------------------------------------------

// substituteString replaces all ${var} references in s with the corresponding
// string values from group (forall bindings, checked first) or vars (document-level vars).
// If a variable is not found or is not a string, the original ${var} token is preserved.
func substituteString(s string, vars map[string]any, group map[string]string) string {
	return varRefRE.ReplaceAllStringFunc(s, func(match string) string {
		ref := varRefRE.FindStringSubmatch(match)[1]
		if group != nil {
			if value, ok := group[ref]; ok {
				return value
			}
		}
		if value, ok := vars[ref]; ok {
			if ss, ok := value.(string); ok {
				return ss
			}
		}
		return match
	})
}

// substituteStringSlice applies substituteString to each element of in.
func substituteStringSlice(in []string, vars map[string]any, group map[string]string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = substituteString(s, vars, group)
	}
	return out
}

// substituteAnySlice applies substituteAny to each element of in.
func substituteAnySlice(in []any, vars map[string]any, group map[string]string) []any {
	if len(in) == 0 {
		return nil
	}
	out := make([]any, 0, len(in))
	for _, item := range in {
		out = append(out, substituteAny(item, vars, group))
	}
	return out
}

// substituteMap applies substituteAny to every value in the map, returning a new map.
func substituteMap(in map[string]any, vars map[string]any, group map[string]string) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := map[string]any{}
	for _, key := range sortedKeysAny(in) {
		out[key] = substituteAny(in[key], vars, group)
	}
	return out
}

// substituteAny recursively resolves ${var} references in any value.
// - For strings: single-var-ref values are replaced directly; others go through substituteString.
// - For slices/maps: recursion into elements/values.
// - Other types are returned unchanged.
func substituteAny(raw any, vars map[string]any, group map[string]string) any {
	switch v := raw.(type) {
	case string:
		if ref, ok := singleVarRef(v); ok {
			if group != nil {
				if value, ok := group[ref]; ok {
					return value
				}
			}
			if value, ok := vars[ref]; ok {
				return value
			}
		}
		return substituteString(v, vars, group)
	case []any:
		out := make([]any, 0, len(v))
		for _, item := range v {
			out = append(out, substituteAny(item, vars, group))
		}
		return out
	case map[string]any:
		return substituteMap(v, vars, group)
	default:
		return raw
	}
}

// ---------------------------------------------------------------------------
// Variable reference helpers
// ---------------------------------------------------------------------------

// singleVarRef returns the variable name if raw is a bare "${var}" reference
// (the whole string is one reference). Otherwise it returns false.
func singleVarRef(raw any) (string, bool) {
	s, ok := raw.(string)
	if !ok {
		return "", false
	}
	m := varRefRE.FindStringSubmatch(s)
	if len(m) != 2 || m[0] != s {
		return "", false
	}
	return m[1], true
}

// refsInAny collects all ${var} references found recursively in v.
// It handles string, map[string]any, and []any types.
//
// This function is a building block for validation and analysis passes
// that need to discover all variable dependencies in a subtree.
// It is ported from the legacy intent engine per the spec requirement and
// will be wired into RCLExpr-level validation in a follow-up task.
func refsInAny(v any) []string {
	var refs []string
	switch x := v.(type) {
	case string:
		for _, m := range varRefRE.FindAllStringSubmatch(x, -1) {
			refs = append(refs, m[1])
		}
	case map[string]any:
		for _, value := range x {
			refs = append(refs, refsInAny(value)...)
		}
	case []any:
		for _, value := range x {
			refs = append(refs, refsInAny(value)...)
		}
	}
	return refs
}

// ---------------------------------------------------------------------------
// Generic helpers
// ---------------------------------------------------------------------------

// toStringSlice coerces a value to []string.
// It supports []any (each element must be a string) and []string directly.
func toStringSlice(raw any) ([]string, bool) {
	switch v := raw.(type) {
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			s, ok := item.(string)
			if !ok {
				return nil, false
			}
			out = append(out, s)
		}
		return out, true
	case []string:
		return append([]string(nil), v...), true
	default:
		return nil, false
	}
}

// sortedKeysAny returns the sorted keys of a map[string]any, ensuring
// deterministic iteration order.
func sortedKeysAny(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
