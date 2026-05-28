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
		Name:   in.Name,
		Check:  substituteCheck(in.Check, vars, group),
		Assert: substituteAssertion(in.Assert, vars, group),
	}
}

func substituteCheck(in Check, vars map[string]any, group map[string]string) Check {
	out := Check{
		Table:    substituteString(in.Table, vars, group),
		Scenario: substituteString(in.Scenario, vars, group),
		Where:    substituteMap(in.Where, vars, group),
		Packet: PacketCheck{
			From:     substituteString(in.Packet.From, vars, group),
			VRF:      substituteString(in.Packet.VRF, vars, group),
			To:       substituteString(in.Packet.To, vars, group),
			Protocol: substituteString(in.Packet.Protocol, vars, group),
			DstPort:  in.Packet.DstPort,
		},
		GroupBy: append([]string(nil), in.GroupBy...),
		Assert:  substituteAssertion(in.Assert, vars, group),
	}
	if in.Compare != nil {
		out.Compare = &CompareCheck{
			Table:    substituteString(in.Compare.Table, vars, group),
			Relation: substituteString(in.Compare.Relation, vars, group),
			Left: CompareSide{
				Snapshot: substituteString(in.Compare.Left.Snapshot, vars, group),
				Where:    substituteMap(in.Compare.Left.Where, vars, group),
			},
			Right: CompareSide{
				Snapshot: substituteString(in.Compare.Right.Snapshot, vars, group),
				Where:    substituteMap(in.Compare.Right.Where, vars, group),
			},
		}
	}
	return out
}

func substituteAssertion(in Assertion, vars map[string]any, group map[string]string) Assertion {
	out := in
	if in.DistinctValues != nil {
		out.DistinctValues = &DistinctValuesCheck{
			Field:  substituteString(in.DistinctValues.Field, vars, group),
			Equals: substituteAnySlice(in.DistinctValues.Equals, vars, group),
		}
	}
	if in.DistinctCount != nil {
		cp := *in.DistinctCount
		cp.Field = substituteString(cp.Field, vars, group)
		out.DistinctCount = &cp
	}
	return out
}

func substituteAnySlice(in []any, vars map[string]any, group map[string]string) []any {
	out := make([]any, 0, len(in))
	for _, item := range in {
		out = append(out, substituteAny(item, vars, group))
	}
	return out
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
