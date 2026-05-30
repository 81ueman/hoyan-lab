package fib

import observationfib "github.com/81ueman/hoyan-lab/internal/domain/observation/fib"

func canonicalProtocol(protocol string) string {
	return observationfib.CanonicalProtocol(protocol)
}

func sortRoutes(routes []NormalizedFIBRoute) {
	observationfib.SortRoutes(routes)
}

func dedupeNextHops(in []NormalizedFIBNextHop) []NormalizedFIBNextHop {
	seen := map[string]bool{}
	var out []NormalizedFIBNextHop
	for _, hop := range in {
		key := hop.Address + "|" + hop.Interface
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, hop)
	}
	observationfib.SortNextHops(out)
	return out
}
