package sim

import (
	"fmt"
	"github.com/81ueman/hoyan-lab/internal/core/netaddr"
	"github.com/81ueman/hoyan-lab/internal/core/predicate"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/engine/space"
)

func CollectRIBPrefixPredicates(g *Graph) []space.PrefixPredicate {
	if g == nil {
		return nil
	}
	var nodes []string
	for node := range g.rib {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	var out []space.PrefixPredicate
	seen := map[string]bool{}
	for _, node := range nodes {
		var vrfs []string
		for vrf := range g.rib[node] {
			vrfs = append(vrfs, vrf)
		}
		sort.Strings(vrfs)
		for _, vrf := range vrfs {
			var prefixes []string
			for prefix := range g.rib[node][vrf] {
				prefixes = append(prefixes, prefix)
			}
			sort.Strings(prefixes)
			for _, rawPrefix := range prefixes {
				routes := append([]RIBEntry(nil), g.rib[node][vrf][rawPrefix]...)
				sort.SliceStable(routes, func(i, j int) bool {
					left := routes[i].Normalize()
					right := routes[j].Normalize()
					if left.Provenance.OriginNode == right.Provenance.OriginNode {
						return left.Provenance.FromNode < right.Provenance.FromNode
					}
					return left.Provenance.OriginNode < right.Provenance.OriginNode
				})
				for _, route := range routes {
					route = route.Normalize()
					if route.NLRI.Prefix.IsZero() {
						continue
					}
					source := fmt.Sprintf("rib:%s:%s:%s", node, vrf, route.NLRI.Prefix.String())
					if route.Provenance.OriginNode != "" {
						source += ":origin=" + route.Provenance.OriginNode
					}
					key := source + "\x00" + route.NLRI.Prefix.String()
					if seen[key] {
						continue
					}
					seen[key] = true
					out = append(out, space.PrefixPredicate{
						Source: source,
						Set:    predicate.ExactPrefixSet{Prefix: route.NLRI.Prefix},
					})
				}
			}
		}
	}
	return out
}

func CollectFIBPrefixPredicates(g *Graph) []space.PrefixPredicate {
	if g == nil {
		return nil
	}
	var nodes []string
	for node := range g.fib {
		nodes = append(nodes, node)
	}
	sort.Strings(nodes)

	var out []space.PrefixPredicate
	seen := map[string]bool{}
	for _, node := range nodes {
		var vrfs []string
		for vrf := range g.fib[node] {
			vrfs = append(vrfs, vrf)
		}
		sort.Strings(vrfs)
		for _, vrf := range vrfs {
			entries := append([]FIBEntry(nil), g.fib[node][vrf]...)
			sort.SliceStable(entries, func(i, j int) bool {
				if entries[i].Prefix.String() == entries[j].Prefix.String() {
					return entries[i].GroupID < entries[j].GroupID
				}
				return entries[i].Prefix.String() < entries[j].Prefix.String()
			})
			for _, entry := range entries {
				if !entry.Prefix.IsValid() {
					continue
				}
				prefix := netaddr.PrefixFromNetIP(entry.Prefix)
				if prefix.IsZero() {
					continue
				}
				source := fmt.Sprintf("fib:%s:%s:%s", node, vrf, prefix.String())
				if entry.GroupID != "" {
					source += ":group=" + entry.GroupID
				}
				key := source + "\x00" + prefix.String()
				if seen[key] {
					continue
				}
				seen[key] = true
				out = append(out, space.PrefixPredicate{
					Source: source,
					Set:    predicate.ExactPrefixSet{Prefix: prefix},
				})
			}
		}
	}
	return out
}
