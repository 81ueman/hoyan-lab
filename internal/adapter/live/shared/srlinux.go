package shared

import (
	"net/netip"
	"strings"
)

// SRLinuxNextHopAddress normalizes an SRLinux next-hop address string.
// It strips prefixes (e.g., "10.0.0.1/32" → "10.0.0.1"), trims whitespace,
// and returns an empty string for "None" or empty input.
func SRLinuxNextHopAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "None" {
		return ""
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return ""
	}
	addr := fields[0]
	if pfx, err := netip.ParsePrefix(addr); err == nil {
		return pfx.Addr().String()
	}
	if ip, err := netip.ParseAddr(addr); err == nil {
		return ip.String()
	}
	return addr
}
