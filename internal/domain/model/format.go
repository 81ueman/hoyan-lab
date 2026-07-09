package model

import (
	"fmt"
	"sort"
	"strings"
)

// FormatASPath formats an AS path slice as a space-separated string.
func FormatASPath(path []uint32) string {
	if len(path) == 0 {
		return ""
	}
	parts := make([]string, 0, len(path))
	for _, asn := range path {
		parts = append(parts, fmt.Sprint(asn))
	}
	return strings.Join(parts, " ")
}

// SortedStrings returns a sorted copy of xs.
func SortedStrings(xs []string) []string {
	out := append([]string(nil), xs...)
	sort.Strings(out)
	return out
}

// DefaultLocalPref returns v, or DefaultLocalPreference if v is zero.
func DefaultLocalPref(v int) int {
	if v == 0 {
		return DefaultLocalPreference
	}
	return v
}
