package configparse

import (
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

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
		if err != nil {
			return model.Prefix{}, 0, err
		}
		return pfx, 1, nil
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
