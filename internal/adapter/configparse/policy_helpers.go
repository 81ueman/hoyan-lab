package configparse

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func routeMapVendorName(kind model.DeviceKind) string {
	return model.ProfileFor(kind).ConfigProfile().RouteMapVendorName()
}

func getNeighbor(neighbors map[string]*model.BGPNeighbor, vrf model.NetworkInstanceID, addr string) *model.BGPNeighbor {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	key := string(vrf) + "|" + addr
	if neighbors[key] == nil {
		neighbors[key] = &model.BGPNeighbor{NetworkInstance: vrf, Address: addr}
	}
	return neighbors[key]
}

func parsePrefixListRule(seq int, action, prefix string, fields []string) (model.PrefixListRule, error) {
	rule := model.PrefixListRule{Seq: seq, Action: action, Prefix: prefix}
	for i := 0; i < len(fields); i += 2 {
		if i+1 >= len(fields) {
			return model.PrefixListRule{}, fmt.Errorf("invalid prefix-list range")
		}
		v, err := strconv.Atoi(fields[i+1])
		if err != nil {
			return model.PrefixListRule{}, err
		}
		switch fields[i] {
		case "ge":
			rule.Ge = v
		case "le":
			rule.Le = v
		default:
			return model.PrefixListRule{}, fmt.Errorf("unsupported prefix-list option %q", fields[i])
		}
	}
	match, err := model.NewPrefixSet(rule.Prefix, rule.Ge, rule.Le)
	if err != nil {
		return model.PrefixListRule{}, err
	}
	rule.Match = match
	return rule, nil
}

func parseRouteMapInt(raw string) (int, bool, error) {
	delta := strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "-")
	v, err := strconv.Atoi(raw)
	return v, delta, err
}

func parseASPathFields(fields []string) ([]uint32, error) {
	var out []uint32
	for _, field := range fields {
		asn, err := strconv.ParseUint(field, 10, 32)
		if err != nil {
			return nil, err
		}
		out = append(out, uint32(asn))
	}
	return out, nil
}

func addPrefixListRule(prefixLists map[string]*model.PrefixList, name string, rule model.PrefixListRule) {
	if prefixLists[name] == nil {
		prefixLists[name] = &model.PrefixList{Name: name}
	}
	prefixLists[name].Rules = append(prefixLists[name].Rules, rule)
}

func addStringListRule(asPathLists map[string]*model.ASPathList, name string, rule model.StringListRule) {
	if asPathLists[name] == nil {
		asPathLists[name] = &model.ASPathList{Name: name}
	}
	asPathLists[name].Rules = append(asPathLists[name].Rules, rule)
}

func addCommunityListRule(communityLists map[string]*model.CommunityList, name string, rule model.StringListRule) {
	if communityLists[name] == nil {
		communityLists[name] = &model.CommunityList{Name: name}
	}
	communityLists[name].Rules = append(communityLists[name].Rules, rule)
}

func addRoutePolicyRule(routePolicies map[string]*model.RoutePolicy, name string, action string, seq int) (*model.RoutePolicy, *model.RoutePolicyRule) {
	if routePolicies[name] == nil {
		routePolicies[name] = &model.RoutePolicy{Name: name}
	}
	routePolicies[name].Rules = append(routePolicies[name].Rules, model.RoutePolicyRule{Seq: seq, Action: action})
	policy := routePolicies[name]
	return policy, &policy.Rules[len(policy.Rules)-1]
}

func sortedPrefixLists(prefixLists map[string]*model.PrefixList) []model.PrefixList {
	var out []model.PrefixList
	for _, prefixList := range prefixLists {
		cp := *prefixList
		cp.Rules = append([]model.PrefixListRule(nil), prefixList.Rules...)
		sort.Slice(cp.Rules, func(i, j int) bool {
			return cp.Rules[i].Seq < cp.Rules[j].Seq
		})
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func sortedASPathLists(asPathLists map[string]*model.ASPathList) []model.ASPathList {
	var out []model.ASPathList
	for _, list := range asPathLists {
		cp := *list
		cp.Rules = append([]model.StringListRule(nil), list.Rules...)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func sortedCommunityLists(communityLists map[string]*model.CommunityList) []model.CommunityList {
	var out []model.CommunityList
	for _, list := range communityLists {
		cp := *list
		cp.Rules = append([]model.StringListRule(nil), list.Rules...)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func sortedRoutePolicies(routePolicies map[string]*model.RoutePolicy) []model.RoutePolicy {
	var out []model.RoutePolicy
	for _, routePolicy := range routePolicies {
		cp := *routePolicy
		cp.Rules = append([]model.RoutePolicyRule(nil), routePolicy.Rules...)
		sort.Slice(cp.Rules, func(i, j int) bool {
			return cp.Rules[i].Seq < cp.Rules[j].Seq
		})
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}
