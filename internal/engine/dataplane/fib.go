package dataplane

import (
	"net/netip"
	"sort"
	"strconv"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/bgp"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
	"github.com/81ueman/hoyan-lab/internal/engine/controlplane"
)

type NextHopEntry struct {
	Node    string
	Address string
	Weight  float64 // distribution ratio (0.0-1.0)
}

type Path struct {
	Nodes []string
	Links []string
	Cost  int
}

type NextHopResolutionStatus string

const (
	NextHopResolutionResolvedAdjacent    NextHopResolutionStatus = "resolved_adjacent"
	NextHopResolutionUnresolvedRecursive NextHopResolutionStatus = "unresolved_recursive_next_hop"
	NextHopResolutionManagementFallback  NextHopResolutionStatus = "next_hop_management_fallback"
)

type FIBEntry struct {
	Prefix netip.Prefix
	VRF    string
	// NextHop is the resolved adjacent topology node used by modeled packet
	// forwarding. It is not a raw BGP next-hop address.
	NextHop          string
	RawNextHop       string
	NextHopAddress   string
	Interface        string
	ResolutionStatus NextHopResolutionStatus
	ResolutionReason string
	SourceKind       model.RouteSourceKind
	Discard          bool
	ConnectedClass   model.ConnectedRouteClass
	Path             Path
	Condition        failure.Cond
	Rank             int
	GroupID          string
	Equivalent       bool
	NextHops         []NextHopEntry // ECMP next-hops with weights
}

type Engine struct {
	idx *model.TopologyIndex
	rib domainroute.RIBTable
	fib FIBTable
}

type FIBTable map[model.NodeID]map[model.NetworkInstanceID][]FIBEntry

func NewEngine(idx *model.TopologyIndex, rib domainroute.RIBTable, fib FIBTable) *Engine {
	if rib == nil {
		rib = domainroute.RIBTable{}
	}
	if fib == nil {
		fib = FIBTable{}
	}
	return &Engine{idx: idx, rib: rib, fib: fib}
}

func (e *Engine) DeriveFIB() {
	for node, byVRF := range e.rib {
		n, _ := e.idx.Node(string(node))
		behavior := controlplane.BehaviorFor(n.Kind)
		if e.fib[node] == nil {
			e.fib[node] = map[model.NetworkInstanceID][]FIBEntry{}
		}
		for vrf, byPrefix := range byVRF {
			var entries []FIBEntry
			for _, routes := range byPrefix {
				routes = append([]domainroute.RIBEntry(nil), routes...)
				sort.SliceStable(routes, func(i, j int) bool {
					ai, aj := fibAdminDistance(routes[i]), fibAdminDistance(routes[j])
					if ai == aj {
						return routes[i].SourceKind < routes[j].SourceKind
					}
					return ai < aj
				})
				seenSelected := map[string]bool{}
				var installed []domainroute.RIBEntry
				var groups []fibRouteGroup
				for _, route := range routes {
					route = route.Normalize()
					selectedKey := ""
					if route.SelectedCond != nil {
						selectedKey = route.SelectedCond.Key()
					}
					if seenSelected[selectedKey] {
						continue
					}
					if !behavior.RouteInstallableInFIB(n, installed, route) {
						continue
					}
					seenSelected[selectedKey] = true
					group, newGroup := routeGroupFor(behavior.DecisionProcess(), n, groups, route)
					installed = append(installed, route)
					if newGroup {
						groups = append(groups, group)
					} else {
						for i := range entries {
							if entries[i].GroupID == group.id {
								entries[i].Equivalent = true
							}
						}
					}
					resolvedNextHop := route.ForwardingNextHop.Node
					nextHopAddress := route.ForwardingNextHop.Addr
					rawNextHop := route.ForwardingNextHop.Node
					if rawNextHop == "" {
						rawNextHop = nextHopAddress
					}
					resolutionStatus, resolutionReason := nextHopResolution(resolvedNextHop, nextHopAddress)
					entries = append(entries, FIBEntry{
						Prefix:           route.NLRI.Prefix.NetIP(),
						VRF:              string(vrf),
						NextHop:          resolvedNextHop,
						RawNextHop:       rawNextHop,
						NextHopAddress:   nextHopAddress,
						Interface:        route.RouteSource.Interface,
						ResolutionStatus: resolutionStatus,
						ResolutionReason: resolutionReason,
						SourceKind:       route.SourceKind,
						Discard:          route.SourceKind == model.RouteSourceBlackhole,
						ConnectedClass:   route.RouteSource.ConnectedClass,
						Path:             Path{Nodes: route.Provenance.PathNodes, Links: route.Provenance.PathLinks, Cost: e.idx.PathCost(route.Provenance.PathLinks)},
						Condition:        route.SelectedCond,
						Rank:             group.rank,
						GroupID:          group.id,
						Equivalent:       group.equivalent,
					})
				}
			}
			sort.SliceStable(entries, func(i, j int) bool {
				if entries[i].Prefix.Bits() == entries[j].Prefix.Bits() {
					if entries[i].Rank == entries[j].Rank {
						return entries[i].Prefix.String() < entries[j].Prefix.String()
					}
					return entries[i].Rank < entries[j].Rank
				}
				return entries[i].Prefix.Bits() > entries[j].Prefix.Bits()
			})
			// Populate NextHops for ECMP groups
			entries = populateECMPNextHops(entries)

			e.fib[node][vrf] = entries
		}
	}
}

func populateECMPNextHops(entries []FIBEntry) []FIBEntry {
	// Group entries by GroupID
	groups := map[string][]int{}
	for i, entry := range entries {
		if entry.GroupID == "" {
			continue
		}
		groups[entry.GroupID] = append(groups[entry.GroupID], i)
	}
	// For groups with multiple entries, populate NextHops with equal weights
	for _, indices := range groups {
		if len(indices) < 2 {
			continue
		}
		// Collect all distinct next-hops in the group
		seen := map[string]bool{}
		var nhs []NextHopEntry
		for _, idx := range indices {
			key := entries[idx].NextHop + "@" + entries[idx].NextHopAddress
			if seen[key] {
				continue
			}
			seen[key] = true
			nhs = append(nhs, NextHopEntry{
				Node:    entries[idx].NextHop,
				Address: entries[idx].NextHopAddress,
				Weight:  0,
			})
		}
		if len(nhs) == 0 {
			continue
		}
		weight := 1.0 / float64(len(nhs))
		for i := range nhs {
			nhs[i].Weight = weight
		}
		// Assign the same NextHops slice to all entries in the group
		for _, idx := range indices {
			entries[idx].NextHops = nhs
		}
	}
	return entries
}

func fibAdminDistance(route domainroute.RIBEntry) int {
	route = route.Normalize()
	if route.RouteSource.AdminDistance != model.AdminDistanceConnected || route.SourceKind == model.RouteSourceConnected {
		return route.RouteSource.AdminDistance
	}
	switch route.SourceKind {
	case model.RouteSourceConnected:
		return model.AdminDistanceConnected
	case model.RouteSourceStatic, model.RouteSourceBlackhole:
		return model.AdminDistanceStatic
	case model.RouteSourceOSPF:
		return model.AdminDistanceOSPF
	default:
		return model.AdminDistanceBGP
	}
}

type fibRouteGroup struct {
	route      domainroute.RIBEntry
	rank       int
	id         string
	equivalent bool
}

func routeGroupFor(decision bgp.DecisionProcess, node model.Node, groups []fibRouteGroup, route domainroute.RIBEntry) (fibRouteGroup, bool) {
	prefix := route.NLRI.Prefix.String()
	for _, group := range groups {
		if decision.Equivalent(node, group.route, route) {
			return fibRouteGroup{
				route:      route,
				rank:       group.rank,
				id:         group.id,
				equivalent: true,
			}, false
		}
	}
	rank := len(groups)
	return fibRouteGroup{
		route: route,
		rank:  rank,
		id:    prefix + "#rank-" + strconv.Itoa(rank),
	}, true
}

func (e *Engine) LookupFIB(node, dst string, ctx failure.Context) (FIBEntry, bool) {
	return e.LookupFIBVRF(node, string(model.NetworkInstanceDefault), dst, ctx)
}

func (e *Engine) LookupFIBVRF(node, vrf, dst string, ctx failure.Context) (FIBEntry, bool) {
	ip, err := netip.ParseAddr(dst)
	if err != nil {
		return FIBEntry{}, false
	}
	for _, rule := range e.fib[model.NodeID(node)][model.NormalizeNetworkInstance(vrf)] {
		if rule.Prefix.Contains(ip) && rule.Condition.Eval(ctx) {
			return rule, true
		}
	}
	return FIBEntry{}, false
}

func nextHopResolution(node, addr string) (NextHopResolutionStatus, string) {
	if node != "" {
		return NextHopResolutionResolvedAdjacent, ""
	}
	if addr != "" {
		return NextHopResolutionUnresolvedRecursive, "recursive next-hop unresolved"
	}
	return "", ""
}

func (entry FIBEntry) effectiveResolutionStatus() NextHopResolutionStatus {
	if entry.ResolutionStatus != "" {
		return entry.ResolutionStatus
	}
	if entry.NextHop == "" && (entry.RawNextHop != "" || entry.NextHopAddress != "") {
		return NextHopResolutionUnresolvedRecursive
	}
	if _, err := netip.ParseAddr(entry.NextHop); err == nil {
		return NextHopResolutionUnresolvedRecursive
	}
	return ""
}
