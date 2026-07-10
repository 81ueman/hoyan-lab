package controlplane

import (
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

// isBGPOrAggregate returns true if the route's SourceKind is BGP or Aggregate.
func isBGPOrAggregate(entry domainroute.RIBEntry) bool {
	entry = entry.Normalize()
	return entry.SourceKind == model.RouteSourceBGP || entry.SourceKind == model.RouteSourceAggregate
}

// RacingPropagate re-propagates all BGP RIB entries with RacingPropagation=true
// so that routes normally filtered by EligibleForAdvertisement are also propagated.
// After propagation, it re-runs route selection and convergence. Use this to detect
// BGP route update racing (SIGCOMM 2020 §5.4, §7.1, Appendix B).
func (e *Engine) RacingPropagate() {
	// Collect all BGP entries that should be re-propagated with RacingPropagation.
	type entryKey struct {
		node   string
		vrf    model.NetworkInstanceID
		prefix model.Prefix
		idx    int
	}
	var bgpEntries []entryKey

	for nodeID, byVRF := range e.rib {
		for vrf, byPrefix := range byVRF {
			for prefix, routes := range byPrefix {
				for i, entry := range routes {
					if !isBGPOrAggregate(entry) {
						continue
					}
					if len(entry.Provenance.PathNodes) < 1 {
						continue
					}
					bgpEntries = append(bgpEntries, entryKey{
						node:   string(nodeID),
						vrf:    vrf,
						prefix: prefix,
						idx:    i,
					})
				}
			}
		}
	}

	// Sort for determinism.
	sort.Slice(bgpEntries, func(i, j int) bool {
		if bgpEntries[i].node != bgpEntries[j].node {
			return bgpEntries[i].node < bgpEntries[j].node
		}
		if bgpEntries[i].vrf != bgpEntries[j].vrf {
			return bgpEntries[i].vrf < bgpEntries[j].vrf
		}
		return bgpEntries[i].prefix.String() < bgpEntries[j].prefix.String()
	})

	// Re-propagate each entry with RacingPropagation=true.
	for _, ek := range bgpEntries {
		nodeID := model.NodeID(ek.node)
		routes := e.rib[nodeID][ek.vrf][ek.prefix]
		if ek.idx >= len(routes) {
			continue
		}
		entry := routes[ek.idx]
		entry.RacingPropagation = true
		e.walkBGP(entry)
	}

	// Re-run route selection and convergence after racing propagation.
	e.SelectRoutes()
	e.ConvergeAdvertisementConditions()
}

// CollectRacingCandidates returns, for each router that has the given prefix,
// the SelectedCond of every BGP or Aggregate route. The returned map is
// router → list of SelectedCond (one per route, in route-preference order).
// Only routes with SourceKind BGP or Aggregate are included.
func (e *Engine) CollectRacingCandidates(prefix model.Prefix) map[string][]failure.Cond {
	conds := map[string][]failure.Cond{}
	for nodeID, byVRF := range e.rib {
		for _, byPrefix := range byVRF {
			routes := byPrefix[prefix]
			if len(routes) == 0 {
				continue
			}
			var nodeConds []failure.Cond
			for _, r := range routes {
				if !isBGPOrAggregate(r) {
					continue
				}
				if r.SelectedCond == nil {
					continue
				}
				nodeConds = append(nodeConds, r.SelectedCond)
			}
			if len(nodeConds) > 0 {
				conds[string(nodeID)] = nodeConds
			}
		}
	}
	return conds
}

// PrefixWithMultipleOrigins returns BGP prefixes that have RIB entries
// originating from more than one distinct origin node. Only routes with
// BGP or Aggregate SourceKind are considered.
func (e *Engine) PrefixWithMultipleOrigins() []model.Prefix {
	originCount := map[model.Prefix]map[string]bool{}
	for _, byVRF := range e.rib {
		for _, byPrefix := range byVRF {
			for prefix, routes := range byPrefix {
				for _, entry := range routes {
					if !isBGPOrAggregate(entry) {
						continue
					}
					if entry.Provenance.OriginNode == "" {
						continue
					}
					if originCount[prefix] == nil {
						originCount[prefix] = map[string]bool{}
					}
					originCount[prefix][entry.Provenance.OriginNode] = true
				}
			}
		}
	}
	var out []model.Prefix
	for prefix, origins := range originCount {
		if len(origins) >= 2 {
			out = append(out, prefix)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].String() < out[j].String()
	})
	return out
}
