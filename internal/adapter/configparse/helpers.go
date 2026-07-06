package configparse

import (
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func unsupportedStatement(vendor, file string, line int, text, reason string) UnsupportedStatement {
	return UnsupportedStatement{
		Vendor: vendor,
		File:   file,
		Line:   line,
		Text:   text,
		Reason: reason,
	}
}

func intPtr(v int) *int {
	return &v
}

func upsertInterface(xs []model.Interface, iface model.Interface) []model.Interface {
	for i := range xs {
		if xs[i].Name == iface.Name {
			if iface.Address != "" {
				xs[i].Address = iface.Address
			}
			if iface.VRF != "" {
				xs[i].VRF = iface.VRF
			}
			return xs
		}
	}
	return append(xs, iface)
}

func appendUnique(xs []string, x string) []string {
	for _, existing := range xs {
		if existing == x {
			return xs
		}
	}
	return append(xs, x)
}

func containsSeq(fields []string, seq ...string) bool {
	pos := 0
	for _, f := range fields {
		if f == seq[pos] {
			pos++
			if pos == len(seq) {
				return true
			}
		}
	}
	return false
}

func containsAnyField(fields []string, matches ...string) bool {
	for _, field := range fields {
		for _, match := range matches {
			if field == match {
				return true
			}
		}
	}
	return false
}

func fieldAfter(fields []string, marker string) string {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == marker {
			return strings.Trim(fields[i+1], "[]")
		}
	}
	return ""
}
