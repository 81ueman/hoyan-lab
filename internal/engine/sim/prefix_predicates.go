package sim

import (
	"fmt"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func CollectRIBPrefixPredicates(g *Graph) []model.PrefixPredicate {
	if g == nil {
		return nil
	}
	var nodes []model.NodeID
	for node := range g.rib {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })

	var out []model.PrefixPredicate
	seen := map[string]bool{}
	for _, node := range nodes {
		var vrfs []model.NetworkInstanceID
		for vrf := range g.rib[node] {
			vrfs = append(vrfs, vrf)
		}
		sort.Slice(vrfs, func(i, j int) bool { return vrfs[i] < vrfs[j] })
		for _, vrf := range vrfs {
			var prefixes []model.Prefix
			for prefix := range g.rib[node][vrf] {
				prefixes = append(prefixes, prefix)
			}
			sort.Slice(prefixes, func(i, j int) bool { return prefixes[i].String() < prefixes[j].String() })
			for _, prefix := range prefixes {
				routes := append([]RIBEntry(nil), g.rib[node][vrf][prefix]...)
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
					out = append(out, model.PrefixPredicate{
						Source: source,
						Set:    model.ExactPrefixSet{Prefix: route.NLRI.Prefix},
					})
				}
			}
		}
	}
	return out
}

func CollectFIBPrefixPredicates(g *Graph) []model.PrefixPredicate {
	if g == nil {
		return nil
	}
	var nodes []model.NodeID
	for node := range g.fib {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i] < nodes[j] })

	var out []model.PrefixPredicate
	seen := map[string]bool{}
	for _, node := range nodes {
		var vrfs []model.NetworkInstanceID
		for vrf := range g.fib[node] {
			vrfs = append(vrfs, vrf)
		}
		sort.Slice(vrfs, func(i, j int) bool { return vrfs[i] < vrfs[j] })
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
				prefix := model.PrefixFromNetIP(entry.Prefix)
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
				out = append(out, model.PrefixPredicate{
					Source: source,
					Set:    model.ExactPrefixSet{Prefix: prefix},
				})
			}
		}
	}
	return out
}
