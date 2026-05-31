package policy

import (
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/bgp"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type PolicyNextHopResolver interface {
	NextHopForPolicy(node string, peerName string, in route.RIBEntry) string
	NextHopForSet(node string, nextHop string) route.NextHop
}

func ApplyRoutePolicy(resolver PolicyNextHopResolver, node model.Node, peerName string, policyName string, in route.RIBEntry) bgp.RouteDecision {
	in = in.Normalize()
	if policyName == "" {
		return bgp.RouteDecision{Route: in, Accept: true}
	}
	policy, ok := RoutePolicyByName(node, policyName)
	if !ok {
		return bgp.RouteDecision{Route: in, Accept: true}
	}
	for _, rule := range policy.Rules {
		if !RoutePolicyRuleMatches(resolver, node, peerName, rule, in) {
			continue
		}
		if strings.EqualFold(rule.Action, "deny") {
			return bgp.RouteDecision{Route: in, Accept: false, Reason: "route-map deny"}
		}
		out := in
		if rule.SetLocalPref != nil {
			out.Attrs.LocalPref = *rule.SetLocalPref
		}
		if rule.SetLocalPrefDelta != nil {
			out.Attrs.LocalPref = bgp.DefaultLocalPref(out.Attrs.LocalPref) + *rule.SetLocalPrefDelta
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
			out.Attrs.OriginCode = model.NormalizeBGPOriginCode(rule.SetOriginCode)
		}
		if rule.SetNextHopSelf {
			out.ForwardingNextHop.Node = node.Name
			out.ForwardingNextHop.Addr = ""
		}
		if rule.SetNextHop != "" {
			out.ForwardingNextHop = resolver.NextHopForSet(node.Name, rule.SetNextHop)
		}
		return bgp.RouteDecision{Route: out.Normalize(), Accept: true}
	}
	return bgp.RouteDecision{Route: in, Accept: false, Reason: "route-map implicit deny"}
}

func RoutePolicyRuleMatches(resolver PolicyNextHopResolver, node model.Node, peerName string, rule model.RoutePolicyRule, in route.RIBEntry) bool {
	in = in.Normalize()
	if rule.MatchPrefixList != "" && !PrefixListPermitsPrefix(node, rule.MatchPrefixList, in.NLRI.Prefix.NetIP()) {
		return false
	}
	if rule.MatchNextHopPrefixList != "" && !PrefixListPermitsAddress(node, rule.MatchNextHopPrefixList, resolver.NextHopForPolicy(node.Name, peerName, in)) {
		return false
	}
	if rule.MatchASPathList != "" && !ASPathListPermits(node, rule.MatchASPathList, in.Attrs.ASPath) {
		return false
	}
	if rule.MatchCommunityList != "" && !CommunityListPermits(node, rule.MatchCommunityList, in.Attrs.Communities, rule.MatchCommunityExact) {
		return false
	}
	return true
}

func RoutePolicyByName(node model.Node, name string) (model.RoutePolicy, bool) {
	for _, policy := range node.RoutePolicies {
		if policy.Name == name {
			return policy, true
		}
	}
	return model.RoutePolicy{}, false
}

func PrefixListPermitsAddress(node model.Node, name string, addr string) bool {
	if addr == "" {
		return false
	}
	ip, err := netip.ParseAddr(addr)
	if err != nil {
		return false
	}
	return PrefixListPermitsPrefix(node, name, netip.PrefixFrom(ip, ip.BitLen()))
}

func PrefixListPermitsPrefix(node model.Node, name string, want netip.Prefix) bool {
	prefix := model.PrefixFromNetIP(want)
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

func ASPathListPermits(node model.Node, name string, asPath []uint32) bool {
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

func CommunityListPermits(node model.Node, name string, communities []string, exact bool) bool {
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

func prefixListRuleMatches(rule model.PrefixListRule, want model.Prefix) bool {
	match := rule.Match
	if match == nil {
		var err error
		match, err = model.NewPrefixSet(rule.Prefix, rule.Ge, rule.Le)
		if err != nil {
			return false
		}
	}
	return model.MatchesNLRI(match, want)
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
