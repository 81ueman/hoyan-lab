package fib

import "github.com/81ueman/hoyan-lab/internal/domain/observation"

func canonicalProtocol(protocol string) string {
	return observation.CanonicalProtocol(protocol)
}

func sortRoutes(routes []FIBEntry) {
	observation.SortFIBEntriesForCompare(routes)
}

func dedupeNextHops(in []NextHop) []NextHop {
	seen := map[string]bool{}
	var out []NextHop
	for _, hop := range in {
		key := hop.Address + "|" + hop.Interface
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, hop)
	}
	observation.SortNextHops(out)
	return out
}
