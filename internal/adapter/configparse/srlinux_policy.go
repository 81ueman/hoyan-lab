package configparse

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func parseSRLinuxPrefixSet(prefixLists map[string]*model.PrefixList, fields []string) error {
	name := fieldAfter(fields, "prefix-set")
	prefix := fieldAfter(fields, "prefix")
	if name == "" || prefix == "" {
		return fmt.Errorf("unsupported SR Linux prefix-set statement")
	}
	ge, le, err := parseSRLinuxMaskLengthRange(prefix, fieldAfter(fields, "mask-length-range"))
	if err != nil {
		return err
	}
	rule, err := parsePrefixListRule(0, "permit", prefix, prefixRangeFields(ge, le))
	if err != nil {
		return err
	}
	addPrefixListRule(prefixLists, name, rule)
	return nil
}

func parseSRLinuxMaskLengthRange(prefix, raw string) (int, int, error) {
	if raw == "" || raw == "exact" {
		return 0, 0, nil
	}
	parts := strings.Split(raw, "..")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unsupported SR Linux mask-length-range %q", raw)
	}
	ge, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	le, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	parsed, err := netip.ParsePrefix(prefix)
	if err != nil {
		return 0, 0, err
	}
	if ge == parsed.Bits() {
		ge = 0
	}
	if le == parsed.Bits() {
		le = 0
	}
	return ge, le, nil
}

func prefixRangeFields(ge, le int) []string {
	var fields []string
	if ge > 0 {
		fields = append(fields, "ge", strconv.Itoa(ge))
	}
	if le > 0 {
		fields = append(fields, "le", strconv.Itoa(le))
	}
	return fields
}

const unsupportedSRLinuxPolicyPrefixList = "__unsupported_srlinux_policy_never_match__"

func parseSRLinuxRoutePolicy(routePolicies map[string]*model.RoutePolicy, prefixLists map[string]*model.PrefixList, fields []string) error {
	name := fieldAfter(fields, "policy")
	if name == "" {
		return fmt.Errorf("unsupported SR Linux routing-policy statement")
	}
	if containsSeq(fields, "default-action", "policy-result") {
		action := fields[len(fields)-1]
		if action != "accept" && action != "reject" {
			return fmt.Errorf("unsupported SR Linux routing-policy default-action %q", action)
		}
		addRoutePolicyRule(routePolicies, name, srLinuxPolicyAction(action), 65535)
		return nil
	}
	if !containsAnyField(fields, "statement") {
		return fmt.Errorf("unsupported SR Linux routing-policy statement")
	}
	seq, err := strconv.Atoi(fieldAfter(fields, "statement"))
	if err != nil {
		return err
	}
	policy, rule := ensureRoutePolicyRule(routePolicies, name, seq)
	_ = policy
	switch {
	case containsSeq(fields, "match", "prefix", "prefix-set"):
		rule.MatchPrefixList = fieldAfter(fields, "prefix-set")
	case containsSeq(fields, "action", "policy-result"):
		action := fields[len(fields)-1]
		if action != "accept" && action != "reject" {
			return fmt.Errorf("unsupported SR Linux routing-policy action %q", action)
		}
		rule.Action = srLinuxPolicyAction(action)
	case containsSeq(fields, "action") && fields[len(fields)-1] == "accept":
		rule.Action = "permit"
	case containsSeq(fields, "action") && fields[len(fields)-1] == "reject":
		rule.Action = "deny"
	case containsSeq(fields, "action", "bgp", "local-preference", "set"):
		v, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil {
			return err
		}
		rule.SetLocalPref = intPtr(v)
	case containsSeq(fields, "action", "bgp", "med", "set") ||
		containsSeq(fields, "action", "bgp", "med", "operation", "set") ||
		containsSeq(fields, "action", "bgp", "metric", "set") ||
		containsSeq(fields, "action", "bgp", "metric", "operation", "set"):
		v, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil {
			return err
		}
		rule.SetMED = intPtr(v)
	case containsSeq(fields, "action", "bgp", "next-hop", "set") && strings.EqualFold(fields[len(fields)-1], "self"):
		rule.SetNextHopSelf = true
	default:
		markUnsupportedSRLinuxRoutePolicyRule(prefixLists, rule)
		return fmt.Errorf("unsupported SR Linux routing-policy statement")
	}
	return nil
}

func markUnsupportedSRLinuxRoutePolicyRule(prefixLists map[string]*model.PrefixList, rule *model.RoutePolicyRule) {
	if prefixLists[unsupportedSRLinuxPolicyPrefixList] == nil {
		denyAny, err := parsePrefixListRule(0, "deny", "any", nil)
		if err == nil {
			prefixLists[unsupportedSRLinuxPolicyPrefixList] = &model.PrefixList{Name: unsupportedSRLinuxPolicyPrefixList, Rules: []model.PrefixListRule{denyAny}}
		}
	}
	rule.MatchPrefixList = unsupportedSRLinuxPolicyPrefixList
}

func addSRLinuxDefaultPolicyActions(routePolicies map[string]*model.RoutePolicy) {
	for _, policy := range routePolicies {
		hasDefault := false
		for _, rule := range policy.Rules {
			if rule.Seq == 65535 {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			policy.Rules = append(policy.Rules, model.RoutePolicyRule{Seq: 65535, Action: "permit"})
		}
	}
}

func ensureRoutePolicyRule(routePolicies map[string]*model.RoutePolicy, name string, seq int) (*model.RoutePolicy, *model.RoutePolicyRule) {
	if routePolicies[name] == nil {
		routePolicies[name] = &model.RoutePolicy{Name: name}
	}
	policy := routePolicies[name]
	for i := range policy.Rules {
		if policy.Rules[i].Seq == seq {
			return policy, &policy.Rules[i]
		}
	}
	policy.Rules = append(policy.Rules, model.RoutePolicyRule{Seq: seq, Action: "deny"})
	return policy, &policy.Rules[len(policy.Rules)-1]
}

func srLinuxPolicyAction(action string) string {
	if action == "reject" {
		return "deny"
	}
	return "permit"
}

func parseSRLinuxPolicyBinding(fields []string) (string, error) {
	for i, field := range fields {
		if field != "import-policy" && field != "export-policy" {
			continue
		}
		policies := fields[i+1:]
		if len(policies) == 0 {
			return "", fmt.Errorf("unsupported SR Linux empty BGP policy binding")
		}
		if policies[0] == "[" {
			policies = policies[1:]
			if len(policies) == 0 {
				return "", fmt.Errorf("unsupported SR Linux empty BGP policy binding")
			}
			if len(policies) < 2 || policies[1] != "]" {
				return "", fmt.Errorf("unsupported SR Linux multiple BGP policy binding")
			}
			return policies[0], nil
		}
		if len(policies) > 1 {
			return "", fmt.Errorf("unsupported SR Linux multiple BGP policy binding")
		}
		return policies[0], nil
	}
	return "", fmt.Errorf("unsupported SR Linux BGP policy binding")
}

func srLinuxRoutingPolicyKind(fields []string) string {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "routing-policy" {
			return fields[i+1]
		}
	}
	return ""
}
