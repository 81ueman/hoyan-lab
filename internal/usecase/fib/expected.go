package fib

import (
	"net/netip"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/engine/controlplane"
	"github.com/81ueman/hoyan-lab/internal/engine/dataplane"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

// ExpectedBuilder builds modeled FIB routes without depending on a live collector.
type ExpectedBuilder struct{}

// NewExpectedBuilder returns the preferred entry point for expected FIB generation.
func NewExpectedBuilder() ExpectedBuilder {
	return ExpectedBuilder{}
}

// Expected builds modeled FIB routes for all topology nodes.
func (b ExpectedBuilder) Expected(topo *model.Topology) []observation.FIBEntry {
	return flattenFIBs(b.ExpectedFIBs(topo))
}

// ExpectedForNodes builds modeled FIB routes for the selected topology nodes.
func (ExpectedBuilder) ExpectedForNodes(topo *model.Topology, nodes []model.Node) []observation.FIBEntry {
	return flattenFIBs(ExpectedBuilder{}.ExpectedFIBsForNodesWithFailureSet(topo, nodes, sim.NoFailures()))
}

func (ExpectedBuilder) ExpectedForNodesWithFailureSet(topo *model.Topology, nodes []model.Node, failures sim.FailureSet) []observation.FIBEntry {
	return flattenFIBs(ExpectedBuilder{}.ExpectedFIBsForNodesWithFailureSet(topo, nodes, failures))
}

func (b ExpectedBuilder) ExpectedFIBs(topo *model.Topology) []observation.FIB {
	return b.ExpectedFIBsForNodes(topo, topo.Nodes)
}

func (ExpectedBuilder) ExpectedFIBsForNodes(topo *model.Topology, nodes []model.Node) []observation.FIB {
	return ExpectedBuilder{}.ExpectedFIBsForNodesWithFailureSet(topo, nodes, sim.NoFailures())
}

func (ExpectedBuilder) ExpectedFIBsForNodesWithFailureSet(topo *model.Topology, nodes []model.Node, failures sim.FailureSet) []observation.FIB {
	allowed := map[string]bool{}
	for _, n := range nodes {
		allowed[n.Name] = true
	}
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		panic(err)
	}
	graph := sim.NewGraph(topo)
	ctx := graph.FailureContext(failures)
	byRoute := map[string]observation.FIBEntry{}
	for _, n := range topo.Nodes {
		if !allowed[n.Name] || ctx.NodeFailed(model.NodeID(n.Name)) {
			continue
		}
		behavior := controlplane.BehaviorFor(n.Kind)
		for vrf, table := range graph.RIBTables(n.Name) {
			fib := graph.FIBVRF(n.Name, vrf)
			suppressedBGP := bgpSuppressedByNonBGPFIB(fib, ctx)
			for _, rib := range table {
				for _, entry := range rib {
					entry = entry.Normalize()
					if entry.SourceKind != model.RouteSourceBGP && entry.SourceKind != model.RouteSourceAggregate && entry.SourceKind != model.RouteSourceOSPF {
						continue
					}
					if suppressedBGP[entry.NLRI.Prefix.String()] {
						continue
					}
					if entry.SelectedCond == nil || !entry.SelectedCond.Eval(ctx) || !behavior.RouteValidForRIB(n, entry) {
						continue
					}
					metric := idx.PathCost(entry.Provenance.PathLinks)
					if entry.SourceKind == model.RouteSourceOSPF {
						metric = entry.RouteSource.Metric
					}
					addExpectedRoute(byRoute, idx, fib, ctx, n.Name, vrf, entry.NLRI.Prefix.String(), forwardingNextHop(entry), entry.RouteSource.Interface, entry.SourceKind, entry.RouteSource.ConnectedClass, metric)
				}
			}
		}
		for vrf, fib := range graph.FIBTables(n.Name) {
			for _, entry := range fib {
				if entry.SourceKind == model.RouteSourceBGP || entry.SourceKind == model.RouteSourceAggregate || entry.SourceKind == model.RouteSourceOSPF {
					continue
				}
				if !model.ProfileFor(n.Kind).FIBProfile().ExpectedFIBRouteVisible(entry.SourceKind, entry.ConnectedClass) {
					continue
				}
				if entry.Condition == nil || !entry.Condition.Eval(ctx) {
					continue
				}
				addExpectedRoute(byRoute, idx, nil, ctx, n.Name, vrf, entry.Prefix.String(), entry.NextHop, entry.Interface, entry.SourceKind, entry.ConnectedClass, entry.Path.Cost)
			}
		}
	}
	byFIB := map[string]observation.FIB{}
	for key, route := range byRoute {
		route.NextHops = dedupeNextHops(route.NextHops)
		parts := strings.SplitN(key, "|", 3)
		fibKey := parts[0] + "|" + parts[1]
		fib := byFIB[fibKey]
		if fib.Node == "" {
			fib.Node = model.NodeID(parts[0])
			fib.VRF = model.NetworkInstanceID(parts[1])
		}
		fib.Entries = append(fib.Entries, route)
		byFIB[fibKey] = fib
	}
	out := make([]observation.FIB, 0, len(byFIB))
	for _, fib := range byFIB {
		observation.SortFIBEntriesForCompare(fib.Entries)
		out = append(out, fib)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}

func bgpSuppressedByNonBGPFIB(entries []dataplane.FIBEntry, ctx sim.FailureContext) map[string]bool {
	out := map[string]bool{}
	for _, entry := range entries {
		if entry.SourceKind == model.RouteSourceBGP || entry.SourceKind == model.RouteSourceAggregate || entry.SourceKind == model.RouteSourceOSPF {
			continue
		}
		if entry.Condition == nil || !entry.Condition.Eval(ctx) {
			continue
		}
		out[entry.Prefix.String()] = true
	}
	return out
}

func addExpectedRoute(byRoute map[string]observation.FIBEntry, idx *model.TopologyIndex, fib []dataplane.FIBEntry, ctx sim.FailureContext, node, vrf, prefix, nextHop, iface string, source model.RouteSourceKind, class model.ConnectedRouteClass, metric int) {
	_ = class
	route := observation.FIBEntry{
		AFI:    model.AFIIPv4,
		Prefix: prefix,
		Source: observation.RouteSource{
			Protocol: expectedProtocol(source, nextHop),
		},
		Action: actionForExpectedRoute(source, nextHop),
		Metric: metric,
	}
	if nextHop != "" {
		route.NextHops = []observation.NextHop{expectedNextHop(idx, fib, ctx, node, prefix, nextHop)}
	} else if iface != "" && source != model.RouteSourceBlackhole {
		route.NextHops = []observation.NextHop{{Interface: iface}}
	}
	key := node + "|" + string(model.NormalizeNetworkInstance(vrf)) + "|" + observation.RouteKey(route)
	existing := byRoute[key]
	if existing.Prefix == "" {
		byRoute[key] = route
		return
	}
	existing.NextHops = append(existing.NextHops, route.NextHops...)
	if route.Metric < existing.Metric || existing.Metric == 0 {
		existing.Metric = route.Metric
	}
	byRoute[key] = existing
}

func expectedProtocol(source model.RouteSourceKind, nextHop string) model.RouteSourceKind {
	switch source {
	case model.RouteSourceConnected:
		return model.RouteSourceConnected
	case model.RouteSourceStatic:
		return model.RouteSourceStatic
	case model.RouteSourceBlackhole:
		return model.RouteSourceBlackhole
	case model.RouteSourceOSPF:
		return model.RouteSourceOSPF
	}
	return model.RouteSourceBGP
}

func actionForExpectedRoute(source model.RouteSourceKind, nextHop string) observation.ForwardingAction {
	switch source {
	case model.RouteSourceBlackhole:
		return observation.ActionDrop
	case model.RouteSourceConnected:
		if nextHop == "" {
			return observation.ActionReceive
		}
	}
	return observation.ActionForward
}

func flattenFIBs(fibs []observation.FIB) []observation.FIBEntry {
	var out []observation.FIBEntry
	for _, fib := range fibs {
		out = append(out, fib.Entries...)
	}
	observation.SortFIBEntriesForCompare(out)
	return out
}

func forwardingNextHop(entry sim.RIBEntry) string {
	entry = entry.Normalize()
	if entry.ForwardingNextHop.Node != "" {
		return entry.ForwardingNextHop.Node
	}
	return entry.ForwardingNextHop.Addr
}

func expectedNextHop(idx *model.TopologyIndex, fib []dataplane.FIBEntry, ctx sim.FailureContext, node, routePrefix, nextHop string) observation.NextHop {
	if resolved, ok := resolveRecursiveNextHop(idx, fib, ctx, node, routePrefix, nextHop); ok {
		return resolved
	}
	out := observation.NextHop{}
	if ref, ok := idx.InterfaceToPeer(node, nextHop); ok {
		out.Interface = ref.ConfigName
	}
	if addr, ok := idx.PeerAddress(node, nextHop); ok {
		out.Address = addr.String()
		return out
	}
	out.Address = nextHop
	return out
}

func resolveRecursiveNextHop(idx *model.TopologyIndex, fib []dataplane.FIBEntry, ctx sim.FailureContext, node, routePrefix, nextHop string) (observation.NextHop, bool) {
	addr, err := netip.ParseAddr(nextHop)
	if err != nil {
		return observation.NextHop{}, false
	}
	for _, entry := range fib {
		if entry.Prefix.String() == routePrefix || !entry.Prefix.Contains(addr) {
			continue
		}
		if entry.Condition == nil || !entry.Condition.Eval(ctx) || entry.NextHop == "" {
			continue
		}
		return expectedNextHop(idx, nil, ctx, node, entry.Prefix.String(), entry.NextHop), true
	}
	return observation.NextHop{}, false
}

func dedupeNextHops(in []observation.NextHop) []observation.NextHop {
	seen := map[string]bool{}
	var out []observation.NextHop
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
