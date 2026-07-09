package configparse

import (
	"fmt"
	"strconv"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func parseSRLinuxACL(aclPolicies map[string]map[int]*parsedACLRule, path string, lineNo int, raw string, fields []string) error {
	name := fieldAfter(fields, "acl-filter")
	if name == "" || fieldAfter(fields, "type") != "ipv4" {
		return nil
	}
	entryText := fieldAfter(fields, "entry")
	if entryText == "" {
		return nil
	}
	seq, err := strconv.Atoi(entryText)
	if err != nil {
		return err
	}
	if aclPolicies[name] == nil {
		aclPolicies[name] = map[int]*parsedACLRule{}
	}
	policy := aclPolicies[name][seq]
	if policy == nil {
		policy = &parsedACLRule{
			Name:   name,
			Seq:    seq,
			Source: model.ConfigSource{Vendor: "srlinux", File: path, Line: lineNo, Raw: raw},
		}
		aclPolicies[name][seq] = policy
	}
	if containsSeq(fields, "match", "ipv4", "protocol") {
		proto := fields[len(fields)-1]
		if proto != "tcp" && proto != "udp" && proto != "icmp" && proto != "ip" {
			return fmt.Errorf("unsupported SR Linux ACL protocol %q", proto)
		}
		policy.Protocol = aclPolicyProtocol(proto)
		return nil
	}
	if containsSeq(fields, "match", "ipv4", "destination-ip", "prefix") {
		pfx, err := model.ParsePrefix(fields[len(fields)-1])
		if err != nil {
			return err
		}
		policy.DstPrefix = pfx
		return nil
	}
	if containsSeq(fields, "match", "transport", "destination-port", "value") {
		if !supportedACLPortTail([]string{"eq", fields[len(fields)-1]}) {
			return fmt.Errorf("unsupported SR Linux ACL destination port %q", fields[len(fields)-1])
		}
		port, err := parseACLPort(fields[len(fields)-1])
		if err != nil {
			return err
		}
		policy.DstPort = model.ExactPort(port)
		return nil
	}
	if containsSeq(fields, "action") {
		switch fields[len(fields)-1] {
		case "drop":
			policy.Action = model.ACLDeny
		case "accept":
			policy.Action = model.ACLPermit
		default:
			return fmt.Errorf("unsupported SR Linux ACL action %q", fields[len(fields)-1])
		}
		return nil
	}
	return fmt.Errorf("unsupported SR Linux ACL statement")
}

func parseSRLinuxACLBinding(path string, lineNo int, raw string, fields []string) (aclBinding, bool) {
	name := fieldAfter(fields, "acl-filter")
	if name == "" || fieldAfter(fields, "type") != "ipv4" {
		return aclBinding{}, false
	}
	iface := fieldAfter(fields, "interface")
	stage := ""
	if containsAnyField(fields, "input") {
		stage = "ingress"
	}
	if containsAnyField(fields, "output") {
		stage = "egress"
	}
	if iface == "" || stage == "" {
		return aclBinding{}, false
	}
	return aclBinding{Name: name, Interface: iface, Stage: stage, Source: model.ConfigSource{Vendor: "srlinux", File: path, Line: lineNo, Raw: raw}}, true
}

func flattenSRLinuxACLs(raw map[string]map[int]*parsedACLRule) map[string][]parsedACLRule {
	out := map[string][]parsedACLRule{}
	for name, entries := range raw {
		for _, policy := range entries {
			if policy.Action != model.ACLDeny && policy.Action != model.ACLPermit {
				continue
			}
			out[name] = append(out[name], *policy)
		}
	}
	return out
}
