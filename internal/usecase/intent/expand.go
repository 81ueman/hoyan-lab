package intent

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
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

// expandIntent handles forall expansion at the Intent level.
// If the intent has no forall, it performs variable substitution and returns a single intent.
// If forall exists, it computes the cartesian product of all forall values and creates
// one expanded intent per combination, with variable substitution applied.
func expandIntent(in Intent, vars map[string]any) ([]Intent, error) {
	if len(in.Forall) == 0 {
		out := substituteIntent(in, vars, nil)
		out.Forall = nil
		return []Intent{out}, nil
	}
	keys := sortedKeysAny(in.Forall)
	groups := []map[string]string{{}}
	for _, key := range keys {
		ref, _ := singleVarRef(in.Forall[key])
		values, _ := toStringSlice(vars[ref])
		var next []map[string]string
		for _, group := range groups {
			for _, value := range values {
				cp := map[string]string{}
				for k, v := range group {
					cp[k] = v
				}
				cp[key] = value
				next = append(next, cp)
			}
		}
		groups = next
	}
	out := make([]Intent, 0, len(groups))
	for _, group := range groups {
		expanded := substituteIntent(in, vars, group)
		expanded.Forall = nil
		expanded.Group = map[string]any{}
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			expanded.Group[key] = group[key]
			parts = append(parts, fmt.Sprintf("%s=%s", key, group[key]))
		}
		expanded.Name = expanded.Name + "[" + strings.Join(parts, ",") + "]"
		out = append(out, expanded)
	}
	return out, nil
}

// substituteIntent creates a copy of the intent with all variable references
// resolved. The group parameter carries forall variable bindings (may be nil).
func substituteIntent(in Intent, vars map[string]any, group map[string]string) Intent {
	return Intent{
		Name: in.Name,
		RCL:  expandRCLExprPtr(in.RCL, vars, group),
	}
}

// expandRCLExprPtr is a convenience wrapper that calls expandRCLExpr
// and discards errors (inline expansion never fails with the current implementation).
func expandRCLExprPtr(expr *RCLExpr, vars map[string]any, group map[string]string) *RCLExpr {
	if expr == nil {
		return nil
	}
	expanded, _ := expandRCLExpr(expr, vars, group)
	return expanded
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
		out.Guard = &GuardExpr{
			Where:  substituteMap(expr.Guard.Where, vars, group),
			Intent: *expandRCLExprPtr(&expr.Guard.Intent, vars, group),
		}
	case expr.Forall != nil:
		out.Forall = &ForallExpr{
			Var:    substituteString(expr.Forall.Var, vars, group),
			In:     substituteStringSlice(expr.Forall.In, vars, group),
			Intent: *expandRCLExprPtr(&expr.Forall.Intent, vars, group),
		}
	case len(expr.And) > 0:
		out.And = make([]RCLExpr, len(expr.And))
		for i := range expr.And {
			out.And[i] = *expandRCLExprPtr(&expr.And[i], vars, group)
		}
	case len(expr.Or) > 0:
		out.Or = make([]RCLExpr, len(expr.Or))
		for i := range expr.Or {
			out.Or[i] = *expandRCLExprPtr(&expr.Or[i], vars, group)
		}
	case expr.Not != nil:
		out.Not = expandRCLExprPtr(expr.Not, vars, group)
	case expr.Imply[0] != nil || expr.Imply[1] != nil:
		out.Imply = [2]*RCLExpr{
			expandRCLExprPtr(expr.Imply[0], vars, group),
			expandRCLExprPtr(expr.Imply[1], vars, group),
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
		// No recognized field set – return the original expression unchanged.
		return expr, nil
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

// refsInAny collects all variable references found recursively in v.
// It handles string, map[string]any, and []any types.
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
