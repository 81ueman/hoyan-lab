package rib

import (
	"github.com/81ueman/hoyan-lab/internal/adapter/live/shared"
)

func asMap(v any) map[string]any {
	return shared.AsMap(v)
}

func asSlice(v any) []any {
	return shared.AsSlice(v)
}

func stringValue(v any) string {
	return shared.StringValue(v)
}

func intValue(v any) int {
	return shared.IntValue(v)
}

func boolValue(v any) bool {
	return shared.BoolValue(v)
}

func firstPresent(m map[string]any, keys ...string) any {
	return shared.FirstPresent(m, keys...)
}

func firstString(m map[string]any, keys ...string) string {
	return shared.FirstString(m, keys...)
}

func sortedStrings(xs []string) []string {
	return shared.SortedStrings(xs)
}

func splitCommunities(raw string) []string {
	return shared.SplitCommunities(raw)
}

func appendCommunities(out []string, values ...any) []string {
	return shared.AppendCommunities(out, values...)
}

func normalizeLocalNextHop(nextHop string) string {
	return shared.NormalizeLocalNextHop(nextHop)
}
