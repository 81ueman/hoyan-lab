package configparse

import (
	"fmt"
	"sort"
	"strconv"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func supportedACLPortTail(fields []string) bool {
	_, err := parseACLPortTail(fields)
	return err == nil
}

func parseACLPortTail(fields []string) (model.PortSet, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	if len(fields) == 2 && fields[0] == "eq" {
		port, err := parseACLPort(fields[1])
		if err != nil {
			return nil, err
		}
		return model.ExactPort(port), nil
	}
	return nil, fmt.Errorf("unsupported port tail")
}

func parseACLPort(raw string) (int, error) {
	switch raw {
	case "www", "http":
		return 80, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("unsupported port %q", raw)
	}
	return port, nil
}

func aclPolicyProtocol(protocol string) string {
	if protocol == "ip" {
		return ""
	}
	return protocol
}

func aclStage(raw string) (string, bool) {
	switch raw {
	case "in", "input":
		return "ingress", true
	case "out", "output":
		return "egress", true
	default:
		return "", false
	}
}

func normalizedACLs(kind model.DeviceKind, aclPolicies map[string][]parsedACLRule, defaultAction model.ACLDefaultAction) []model.ACL {
	var out []model.ACL
	for name, policies := range aclPolicies {
		acl := model.ACL{
			Name:          name,
			Vendor:        kind,
			DefaultAction: defaultAction,
		}
		for _, policy := range policies {
			acl.Rules = append(acl.Rules, parsedACLRuleToACLRule(policy))
			if acl.Source.Raw == "" && policy.Source.Raw != "" {
				acl.Source = policy.Source
			}
		}
		sort.Slice(acl.Rules, func(i, j int) bool {
			return acl.Rules[i].Seq < acl.Rules[j].Seq
		})
		out = append(out, acl)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func normalizedACLBindings(bindings []aclBinding) []model.ACLBinding {
	out := make([]model.ACLBinding, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, model.ACLBinding{
			Interface: binding.Interface,
			Direction: binding.Stage,
			ACLName:   binding.Name,
			Source:    binding.Source,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ACLName == out[j].ACLName {
			if out[i].Interface == out[j].Interface {
				return out[i].Direction < out[j].Direction
			}
			return out[i].Interface < out[j].Interface
		}
		return out[i].ACLName < out[j].ACLName
	})
	return out
}

func parsedACLRuleToACLRule(policy parsedACLRule) model.ACLRule {
	action := policy.Action
	if action == "" {
		action = model.ACLDeny
	}
	return model.ACLRule{
		Seq:    policy.Seq,
		Action: action,
		Match: model.PacketSpec{
			SrcSet:   prefixSetOrNil(policy.SrcPrefix),
			DstSet:   prefixSetOrNil(policy.DstPrefix),
			Protocol: policy.Protocol,
			SrcPort:  policy.SrcPort,
			DstPort:  policy.DstPort,
		},
		Source: policy.Source,
	}
}

func prefixSetOrNil(prefix model.Prefix) model.PrefixSet {
	if prefix.IsZero() {
		return nil
	}
	return model.ExactPrefixSet{Prefix: prefix}
}
