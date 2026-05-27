package intent

import (
	"fmt"
	"sort"
	"strings"
)

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

func substituteIntent(in Intent, vars map[string]any, group map[string]string) Intent {
	return Intent{
		Name: in.Name,
		Check: Check{
			Table:    substituteString(in.Check.Table, vars, group),
			Scenario: substituteString(in.Check.Scenario, vars, group),
			Where:    substituteMap(in.Check.Where, vars, group),
		},
		Assert: in.Assert,
	}
}

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

func sortedKeysAny(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
