package shared

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// AsMap asserts v as map[string]any.
func AsMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// AsSlice asserts v as []any.
func AsSlice(v any) []any {
	if xs, ok := v.([]any); ok {
		return xs
	}
	return nil
}

// StringValue converts v to a trimmed string.
func StringValue(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	case float64:
		if x == float64(int64(x)) {
			return fmt.Sprintf("%.0f", x)
		}
		return fmt.Sprint(x)
	default:
		return ""
	}
}

// IntValue converts v to an int.
func IntValue(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case float64:
		return int(x)
	case json.Number:
		i, _ := x.Int64()
		return int(i)
	case string:
		if x == "" || x == "-" {
			return 0
		}
		i, _ := strconv.Atoi(strings.TrimSpace(x))
		return i
	default:
		return 0
	}
}

// BoolValue converts v to a bool, handling both real booleans and strings.
func BoolValue(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return strings.EqualFold(x, "true") || strings.EqualFold(x, "yes") || strings.EqualFold(x, "active") || strings.EqualFold(x, "valid")
	default:
		return false
	}
}

// FirstPresent returns the first value from m for the given keys, or nil.
func FirstPresent(m map[string]any, keys ...string) any {
	for _, key := range keys {
		if v, ok := m[key]; ok {
			return v
		}
	}
	return nil
}

// FirstString returns the first value from m for the given keys, converted to a string.
func FirstString(m map[string]any, keys ...string) string {
	return StringValue(FirstPresent(m, keys...))
}

// SortedStrings returns a sorted copy of xs.
func SortedStrings(xs []string) []string {
	out := append([]string(nil), xs...)
	sort.Strings(out)
	return out
}

// SplitCommunities splits a comma/space-separated community string into sorted
// tokens, returning nil for empty input.
func SplitCommunities(raw string) []string {
	raw = strings.ReplaceAll(raw, ",", " ")
	if raw == "" || raw == "-" {
		return nil
	}
	return SortedStrings(strings.Fields(raw))
}

// AppendCommunities appends community values to out, recursively flattening
// slices. Returns a sorted, deduplicated slice, or nil if empty.
func AppendCommunities(out []string, values ...any) []string {
	for _, value := range values {
		switch x := value.(type) {
		case nil:
			continue
		case []any:
			for _, item := range x {
				out = AppendCommunities(out, item)
			}
		case []string:
			for _, item := range x {
				out = AppendCommunities(out, item)
			}
		default:
			out = append(out, SplitCommunities(StringValue(x))...)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return SortedStrings(out)
}

// NormalizeLocalNextHop returns an empty string for local/discard next hops.
func NormalizeLocalNextHop(nextHop string) string {
	if nextHop == "0.0.0.0" || nextHop == "::" {
		return ""
	}
	return nextHop
}
