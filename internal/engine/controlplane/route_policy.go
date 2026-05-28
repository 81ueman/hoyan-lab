package controlplane

import (
	"github.com/81ueman/hoyan-lab/internal/core/netaddr"
	"fmt"
	"github.com/81ueman/hoyan-lab/internal/core/predicate"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/config/routing"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

func applyRoutePolicy(idx *topology.TopologyIndex, node topology.Node, peerName string, policyName string, route RIBEntry) BGPRouteDecision {
	return applyRoutePolicyWithRouting(idx, routing.FromTopology(&topology.Topology{Nodes: []topology.Node{node}}).ForNode(node.Name), node.Name, peerName, policyName, route)
}

func (e *Engine) applyRoutePolicy(node topology.Node, peerName string, policyName string, route RIBEntry) BGPRouteDecision {
	return applyRoutePolicyWithRouting(e.idx, e.nodeRouting(node.Name), node.Name, peerName, policyName, route)
}

func applyRoutePolicyWithRouting(idx *topology.TopologyIndex, node routing.NodeRouting, nodeName string, peerName string, policyName string, route RIBEntry) BGPRouteDecision {
	route = route.Normalize()
	if policyName == "" {
		return BGPRouteDecision{Route: route, Accept: true}
	}
	policy, ok := routePolicyByName(node, policyName)
	if !ok {
		return BGPRouteDecision{Route: route, Accept: true}
	}
	for _, rule := range policy.Rules {
		if !routePolicyRuleMatches(idx, node, peerName, rule, route) {
			continue
		}
		if strings.EqualFold(rule.Action, "deny") {
			return BGPRouteDecision{Route: route, Accept: false, Reason: "route-map deny"}
		}
		out := route
		if rule.SetLocalPref != nil {
			out.Attrs.LocalPref = *rule.SetLocalPref
		}
		if rule.SetLocalPrefDelta != nil {
			out.Attrs.LocalPref = defaultLocalPref(out.Attrs.LocalPref) + *rule.SetLocalPrefDelta
		}
		if rule.SetMED != nil {
			out.Attrs.MED = *rule.SetMED
		}
		if rule.SetMEDDelta != nil {
			out.Attrs.MED += *rule.SetMEDDelta
		}
		if len(rule.SetASPathPrepend) > 0 {
			out.Attrs.ASPath = append(append([]uint32(nil), rule.SetASPathPrepend...), out.Attrs.ASPath...)
		}
		if len(rule.SetCommunities) > 0 {
			if rule.SetCommunityAdditive {
				out.Attrs.Communities = appendUniqueStrings(out.Attrs.Communities, rule.SetCommunities...)
			} else {
				out.Attrs.Communities = append([]string(nil), rule.SetCommunities...)
			}
			sort.Strings(out.Attrs.Communities)
		}
		if rule.SetOriginCode != "" {
			out.Attrs.OriginCode = BGPOriginCode(rule.SetOriginCode)
		}
		if rule.SetNextHopSelf {
			out.ForwardingNextHop.Node = nodeName
			out.ForwardingNextHop.Addr = ""
		}
		if rule.SetNextHop != "" {
			out.ForwardingNextHop = routeNextHopForSet(idx, nodeName, rule.SetNextHop)
		}
		return BGPRouteDecision{Route: out.Normalize(), Accept: true}
	}
	return BGPRouteDecision{Route: route, Accept: false, Reason: "route-map implicit deny"}
}

func routePolicyRuleMatches(idx *topology.TopologyIndex, node routing.NodeRouting, peerName string, rule topology.RoutePolicyRule, route RIBEntry) bool {
	route = route.Normalize()
	if rule.MatchPrefixList != "" && !prefixListPermitsPrefix(node, rule.MatchPrefixList, route.NLRI.Prefix.NetIP()) {
		return false
	}
	if rule.MatchNextHopPrefixList != "" && !prefixListPermitsAddress(node, rule.MatchNextHopPrefixList, routeNextHopForPolicy(idx, node.Node, peerName, route)) {
		return false
	}
	if rule.MatchASPathList != "" && !asPathListPermits(node, rule.MatchASPathList, route.Attrs.ASPath) {
		return false
	}
	if rule.MatchCommunityList != "" && !communityListPermits(node, rule.MatchCommunityList, route.Attrs.Communities, rule.MatchCommunityExact) {
		return false
	}
	return true
}

func routePolicyByName(node routing.NodeRouting, name string) (topology.RoutePolicy, bool) {
	for _, policy := range node.RoutePolicies {
		if policy.Name == name {
			return policy, true
		}
	}
	return topology.RoutePolicy{}, false
}

func prefixListPermitsAddress(node routing.NodeRouting, name string, addr string) bool {
	if addr == "" {
		return false
	}
	ip, err := netip.ParseAddr(addr)
	if err != nil {
		return false
	}
	return prefixListPermitsPrefix(node, name, netip.PrefixFrom(ip, ip.BitLen()))
}

func prefixListPermitsPrefix(node routing.NodeRouting, name string, want netip.Prefix) bool {
	prefix := netaddr.PrefixFromNetIP(want)
	for _, prefixList := range node.PrefixLists {
		if prefixList.Name != name {
			continue
		}
		for _, rule := range prefixList.Rules {
			if !prefixListRuleMatches(rule, prefix) {
				continue
			}
			return strings.EqualFold(rule.Action, "permit")
		}
		return false
	}
	return false
}

func prefixListRuleMatches(rule topology.PrefixListRule, want netaddr.Prefix) bool {
	match := rule.Match
	if match == nil {
		var err error
		match, err = predicate.NewPrefixSet(rule.Prefix, rule.Ge, rule.Le)
		if err != nil {
			return false
		}
	}
	return predicate.MatchesNLRI(match, want)
}

func asPathListPermits(node routing.NodeRouting, name string, asPath []uint32) bool {
	path := formatASPath(asPath)
	for _, list := range node.ASPathLists {
		if list.Name != name {
			continue
		}
		for _, rule := range list.Rules {
			matched, err := regexp.MatchString(rule.Pattern, path)
			if err != nil || !matched {
				continue
			}
			return strings.EqualFold(rule.Action, "permit")
		}
		return false
	}
	return false
}

func communityListPermits(node routing.NodeRouting, name string, communities []string, exact bool) bool {
	allowed := map[string]bool{}
	denied := map[string]bool{}
	for _, list := range node.CommunityLists {
		if list.Name != name {
			continue
		}
		for _, rule := range list.Rules {
			if strings.EqualFold(rule.Action, "permit") {
				allowed[rule.Pattern] = true
			} else {
				denied[rule.Pattern] = true
			}
		}
		if exact {
			if len(communities) != len(allowed) {
				return false
			}
			for _, community := range communities {
				if !allowed[community] || denied[community] {
					return false
				}
			}
			return true
		}
		for _, community := range communities {
			if denied[community] {
				return false
			}
			if allowed[community] {
				return true
			}
		}
		return false
	}
	return false
}

func routeNextHopForPolicy(idx *topology.TopologyIndex, node string, peerName string, route RIBEntry) string {
	route = route.Normalize()
	nextHop := route.ForwardingNextHop.Node
	if nextHop == "" {
		nextHop = route.ForwardingNextHop.Addr
	}
	if nextHop == "" {
		return ""
	}
	if nextHop == node && peerName != "" {
		return peerAddress(idx, peerName, node)
	}
	if direct := peerAddress(idx, node, nextHop); direct != nextHop {
		return direct
	}
	for i := 0; i+1 < len(route.Provenance.PathNodes); i++ {
		if route.Provenance.PathNodes[i] != nextHop {
			continue
		}
		if addr := peerAddress(idx, route.Provenance.PathNodes[i+1], nextHop); addr != nextHop {
			return addr
		}
	}
	return nextHop
}

func routeNextHopForSet(idx *topology.TopologyIndex, node, nextHop string) RouteNextHop {
	if idx == nil || nextHop == "" {
		return RouteNextHop{Addr: nextHop}
	}
	for _, adj := range idx.Adj[topology.NodeID(node)] {
		peer := string(adj.To)
		if addr, ok := idx.PeerAddress(node, peer); ok && addr.String() == nextHop {
			return RouteNextHop{Node: peer, Addr: nextHop}
		}
	}
	return RouteNextHop{Addr: nextHop}
}

func peerAddress(idx *topology.TopologyIndex, node, peer string) string {
	if peer == "" {
		return ""
	}
	if addr, ok := idx.PeerAddress(node, peer); ok {
		return addr.String()
	}
	return peer
}

func formatASPath(path []uint32) string {
	parts := make([]string, 0, len(path))
	for _, asn := range path {
		parts = append(parts, fmt.Sprint(asn))
	}
	return strings.Join(parts, " ")
}

func appendUniqueStrings(xs []string, more ...string) []string {
	out := append([]string(nil), xs...)
	seen := map[string]bool{}
	for _, x := range out {
		seen[x] = true
	}
	for _, x := range more {
		if !seen[x] {
			out = append(out, x)
			seen[x] = true
		}
	}
	return out
}

func defaultLocalPref(v int) int {
	if v == 0 {
		return 100
	}
	return v
}
