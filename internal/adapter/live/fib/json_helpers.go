package fib

import (
	"strings"

	"github.com/81ueman/hoyan-lab/internal/adapter/live/shared"
)

func mapValue(v any) map[string]any {
	return shared.AsMap(v)
}

func sliceValue(v any) []any {
	return shared.AsSlice(v)
}

func boolValue(v any) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return boolString(v)
}

func boolString(v any) bool {
	s := strings.ToLower(strings.TrimSpace(stringValue(v)))
	return s == "true" || s == "yes" || s == "up"
}
