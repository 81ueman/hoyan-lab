package configparse

import (
	"bufio"
	"fmt"
	"net/netip"
	"sort"
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func parseNftables(path, text string) ([]model.ACL, []model.ACLBinding, error) {
	var rules []parsedACLRule
	var tableName string
	var chainDefault model.ACLDefaultAction = model.ACLDefaultPermit
	inForward := false
	scanner := bufio.NewScanner(strings.NewReader(text))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if i := strings.Index(line, "#"); i >= 0 {
			line = strings.TrimSpace(line[:i])
		}
		if line == "" || line == "}" {
			if line == "}" && inForward {
				inForward = false
			}
			continue
		}
		fields := strings.Fields(strings.NewReplacer("{", " { ", ";", " ; ").Replace(line))
		if len(fields) == 0 {
			continue
		}
		switch {
		case len(fields) >= 4 && fields[0] == "table" && fields[1] == "inet":
			tableName = fields[2]
		case len(fields) >= 3 && fields[0] == "chain" && fields[1] == "forward":
			inForward = true
		case inForward && len(fields) >= 8 && fields[0] == "type" && fields[1] == "filter" && fields[2] == "hook" && fields[3] == "forward":
			if action, ok := nftablesChainPolicy(fields); ok {
				chainDefault = action
			}
			continue
		case inForward:
			policy, ok, err := parseNftablesForwardRule(path, lineNo, line, tableName, fields)
			if err != nil {
				return nil, nil, err
			}
			if ok {
				rules = append(rules, policy)
			}
		default:
			return nil, nil, fmt.Errorf("%s:%d: unsupported nftables statement %q", path, lineNo, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, err
	}
	aclName := nftablesPolicyName(tableName)
	acl := model.ACL{
		Name:          aclName,
		Vendor:        model.KindFRR,
		DefaultAction: chainDefault,
		Source:        model.ConfigSource{Vendor: "nftables", File: path},
	}
	bindingSeen := map[string]bool{}
	var bindings []model.ACLBinding
	for _, rule := range rules {
		acl.Rules = append(acl.Rules, parsedACLRuleToACLRule(rule))
		key := rule.Stage + "\x00" + rule.Interface
		if !bindingSeen[key] {
			bindings = append(bindings, model.ACLBinding{
				Interface: rule.Interface,
				Direction: rule.Stage,
				ACLName:   aclName,
				Source:    rule.Source,
			})
			bindingSeen[key] = true
		}
	}
	return []model.ACL{acl}, bindings, nil
}

func nftablesChainPolicy(fields []string) (model.ACLDefaultAction, bool) {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] != "policy" {
			continue
		}
		switch strings.TrimSuffix(fields[i+1], ";") {
		case "accept":
			return model.ACLDefaultPermit, true
		case "drop":
			return model.ACLDefaultDeny, true
		}
	}
	return "", false
}

func parseNftablesForwardRule(path string, lineNo int, raw, tableName string, fields []string) (parsedACLRule, bool, error) {
	stage := ""
	iface := ""
	protocol := ""
	dstPrefix := model.Prefix{}
	dstPort := model.PortSet(nil)
	action := model.ACLAction("")
	for i := 0; i < len(fields); i++ {
		switch fields[i] {
		case ";":
			continue
		case "iifname", "oifname":
			if i+1 >= len(fields) {
				return parsedACLRule{}, false, fmt.Errorf("%s:%d: unsupported nftables interface match %q", path, lineNo, raw)
			}
			if fields[i] == "iifname" {
				stage = "ingress"
			} else {
				stage = "egress"
			}
			iface = strings.Trim(fields[i+1], `"`)
			i++
		case "ip":
			if i+2 >= len(fields) {
				return parsedACLRule{}, false, fmt.Errorf("%s:%d: unsupported nftables ip match %q", path, lineNo, raw)
			}
			switch fields[i+1] {
			case "protocol":
				protocol = fields[i+2]
			case "daddr":
				pfx, err := model.ParsePrefix(fields[i+2])
				if err != nil {
					return parsedACLRule{}, false, fmt.Errorf("%s:%d: %w", path, lineNo, err)
				}
				dstPrefix = pfx
			default:
				return parsedACLRule{}, false, fmt.Errorf("%s:%d: unsupported nftables ip match %q", path, lineNo, raw)
			}
			i += 2
		case "tcp", "udp":
			if i+2 >= len(fields) || fields[i+1] != "dport" || !supportedACLPortTail([]string{"eq", fields[i+2]}) {
				return parsedACLRule{}, false, fmt.Errorf("%s:%d: unsupported nftables transport match %q", path, lineNo, raw)
			}
			if protocol == "" {
				protocol = fields[i]
			}
			port, err := parseACLPort(fields[i+2])
			if err != nil {
				return parsedACLRule{}, false, fmt.Errorf("%s:%d: %w", path, lineNo, err)
			}
			dstPort = model.ExactPort(port)
			i += 2
		case "drop":
			action = model.ACLDeny
		case "accept":
			action = model.ACLPermit
		default:
			return parsedACLRule{}, false, fmt.Errorf("%s:%d: unsupported nftables forward statement %q", path, lineNo, raw)
		}
	}
	if stage == "" || iface == "" || protocol == "" || dstPrefix.IsZero() || action == "" {
		return parsedACLRule{}, false, fmt.Errorf("%s:%d: incomplete nftables forward rule %q", path, lineNo, raw)
	}
	if protocol != "tcp" && protocol != "udp" && protocol != "icmp" && protocol != "ip" {
		return parsedACLRule{}, false, fmt.Errorf("%s:%d: unsupported nftables protocol %q", path, lineNo, protocol)
	}
	return parsedACLRule{
		Name:      nftablesPolicyName(tableName),
		Stage:     stage,
		Interface: iface,
		Action:    action,
		Protocol:  aclPolicyProtocol(protocol),
		DstPrefix: dstPrefix,
		DstPort:   dstPort,
		Seq:       lineNo,
		Source: model.ConfigSource{
			Vendor: "nftables",
			File:   path,
			Line:   lineNo,
			Raw:    raw,
		},
	}, true, nil
}

func nftablesPolicyName(tableName string) string {
	if tableName == "" {
		return "NFTABLES-FORWARD"
	}
	return strings.ReplaceAll(tableName, "_", "-")
}

func isACLRuleLine(fields []string) bool {
	if len(fields) == 0 {
		return false
	}
	if len(fields) >= 3 && fields[0] == "seq" && (fields[2] == "permit" || fields[2] == "deny") {
		return true
	}
	if fields[0] == "permit" || fields[0] == "deny" {
		return true
	}
	if _, err := strconv.Atoi(fields[0]); err == nil && len(fields) >= 2 && (fields[1] == "permit" || fields[1] == "deny") {
		return true
	}
	return false
}

func parseACLRule(kind model.DeviceKind, path string, lineNo int, raw, name string, fields []string) (parsedACLRule, bool, error) {
	seq := 0
	if len(fields) >= 2 && fields[0] == "seq" {
		fields = fields[1:]
	}
	if n, err := strconv.Atoi(fields[0]); err == nil {
		seq = n
		fields = fields[1:]
	}
	if len(fields) < 4 {
		return parsedACLRule{}, false, fmt.Errorf("unsupported %s ACL statement", routeMapVendorName(kind))
	}
	action := model.ACLAction(fields[0])
	if action != model.ACLPermit && action != model.ACLDeny {
		return parsedACLRule{}, false, fmt.Errorf("unsupported %s ACL action %q", routeMapVendorName(kind), fields[0])
	}
	protocol := fields[1]
	if protocol != "ip" && protocol != "tcp" && protocol != "udp" && protocol != "icmp" {
		return parsedACLRule{}, false, fmt.Errorf("unsupported %s ACL protocol %q", routeMapVendorName(kind), protocol)
	}
	rest := fields[2:]
	srcPrefix, srcEnd, err := parseACLAddress(rest)
	if err != nil {
		return parsedACLRule{}, false, err
	}
	if srcEnd >= len(rest) {
		return parsedACLRule{}, false, fmt.Errorf("unsupported %s ACL destination", routeMapVendorName(kind))
	}
	dstPrefix, dstEnd, err := parseACLAddress(rest[srcEnd:])
	if err != nil {
		return parsedACLRule{}, false, err
	}
	dstPort, err := parseACLPortTail(rest[srcEnd+dstEnd:])
	if err != nil {
		return parsedACLRule{}, false, fmt.Errorf("unsupported %s ACL port match", routeMapVendorName(kind))
	}
	return parsedACLRule{
		Name:      name,
		Action:    action,
		Protocol:  aclPolicyProtocol(protocol),
		SrcPrefix: srcPrefix,
		DstPrefix: dstPrefix,
		DstPort:   dstPort,
		Seq:       seq,
		Source: model.ConfigSource{
			Vendor: string(kind),
			File:   path,
			Line:   lineNo,
			Raw:    raw,
		},
	}, true, nil
}

func parseACLAddress(fields []string) (model.Prefix, int, error) {
	if len(fields) == 0 {
		return model.Prefix{}, 0, fmt.Errorf("unsupported ACL empty address")
	}
	switch fields[0] {
	case "any":
		pfx, err := model.ParsePrefix("0.0.0.0/0")
		return pfx, 1, err
	case "host":
		if len(fields) < 2 {
			return model.Prefix{}, 0, fmt.Errorf("unsupported ACL host address")
		}
		pfx, err := model.ParsePrefix(fields[1] + "/32")
		return pfx, 2, err
	}
	if strings.Contains(fields[0], "/") {
		pfx, err := model.ParsePrefix(fields[0])
		return pfx, 1, err
	}
	if len(fields) >= 2 {
		if pfx, ok := wildcardPrefix(fields[0], fields[1]); ok {
			return pfx, 2, nil
		}
	}
	return model.Prefix{}, 0, fmt.Errorf("unsupported ACL address %q", strings.Join(fields, " "))
}

func wildcardPrefix(addr, wildcard string) (model.Prefix, bool) {
	ip, err := netip.ParseAddr(addr)
	if err != nil || !ip.Is4() {
		return model.Prefix{}, false
	}
	w, err := netip.ParseAddr(wildcard)
	if err != nil || !w.Is4() {
		return model.Prefix{}, false
	}
	wb := w.As4()
	bits := 0
	seenOne := false
	for _, octet := range wb {
		for bit := 7; bit >= 0; bit-- {
			one := octet&(1<<bit) != 0
			if one {
				seenOne = true
				continue
			}
			if seenOne {
				return model.Prefix{}, false
			}
			bits++
		}
	}
	pfx := netip.PrefixFrom(ip, bits).Masked()
	return model.PrefixFromNetIP(pfx), true
}

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
