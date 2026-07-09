package configparse

import (
	"bufio"
	"fmt"
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
