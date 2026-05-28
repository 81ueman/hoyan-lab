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
}

type Engine struct {
	idx *model.TopologyIndex
	rib map[string]map[string]map[string][]domainroute.RIBEntry
	fib map[string]map[string][]FIBEntry
}

func NewEngine(idx *model.TopologyIndex, rib map[string]map[string]map[string][]domainroute.RIBEntry, fib map[string]map[string][]FIBEntry) *Engine {
	if rib == nil {
		rib = map[string]map[string]map[string][]domainroute.RIBEntry{}
	}
	if fib == nil {
		fib = map[string]map[string][]FIBEntry{}
	}
	return &Engine{idx: idx, rib: rib, fib: fib}
}

func (e *Engine) DeriveFIB() {
	for node, byVRF := range e.rib {
		n, _ := e.idx.Node(node)
		behavior := controlplane.BehaviorFor(n.Kind)
		if e.fib[node] == nil {
			e.fib[node] = map[string][]FIBEntry{}
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
						VRF:              vrf,
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
			e.fib[node][vrf] = entries
		}
	}
}

func fibAdminDistance(route domainroute.RIBEntry) int {
	route = route.Normalize()
	if route.RouteSource.AdminDistance != 0 || route.SourceKind == model.RouteSourceConnected {
		return route.RouteSource.AdminDistance
	}
	switch route.SourceKind {
	case model.RouteSourceConnected:
		return 0
	case model.RouteSourceStatic, model.RouteSourceBlackhole:
		return 1
	case model.RouteSourceOSPF:
		return 110
	default:
		return 200
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
	for _, rule := range e.fib[node][string(model.NormalizeNetworkInstance(vrf))] {
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
