package fib

import "strings"

func canonicalProtocol(protocol string) string {
	normalized := strings.ToLower(strings.TrimSpace(protocol))
	if strings.Contains(normalized, "ospf") {
		return "ospf"
	}
	switch normalized {
	case "ebgp", "ibgp", "bgp":
		return "bgp"
	case "ospf", "188":
		return "ospf"
	case "kernel", "connected", "direct", "local", "host":
		return "connected"
	case "static", "196":
		return "static"
	case "blackhole", "discard", "drop", "null0", "null":
		return "blackhole"
	default:
		return strings.ToLower(strings.TrimSpace(protocol))
	}
}

func CanonicalProtocol(protocol string) string {
	return canonicalProtocol(protocol)
}
