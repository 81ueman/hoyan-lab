package configparse

import (
	"fmt"
	"strconv"
	"strings"
)

func (p *frrLikeParser) handleIPAccessList(fields []string) error {
	if len(fields) < 3 {
		return nil
	}
	p.currentACL = fields[2]
	if len(fields) >= 4 && (fields[2] == "standard" || fields[2] == "extended") {
		p.currentACL = fields[3]
	}
	p.currentInterface = ""
	p.inBGP = false
	p.inAF = false
	return nil
}

// handleACLRuleLine parses an ACL rule within the current ACL.
func (p *frrLikeParser) handleACLRuleLine(fields []string, raw string, lineNo int) error {
	kind := p.dialect.Kind()
	pol, ok, err := parseACLRule(kind, p.path, lineNo, raw, p.currentACL, fields)
	if err != nil {
		return err
	}
	if ok {
		p.aclPolicies[p.currentACL] = append(p.aclPolicies[p.currentACL], pol)
	}
	return nil
}

// handleFlatAccessList handles FRR-style flat "access-list ..." statements.
func (p *frrLikeParser) handleFlatAccessList(fields []string, raw string, lineNo int) error {
	if !p.dialect.SupportsFlatAccessList() {
		return nil
	}
	if len(fields) < 5 || (fields[2] != "permit" && fields[2] != "deny") {
		return nil
	}
	kind := p.dialect.Kind()
	pol, ok, err := parseACLRule(kind, p.path, lineNo, raw, fields[1], fields[2:])
	if err != nil {
		return err
	}
	if ok {
		p.aclPolicies[fields[1]] = append(p.aclPolicies[fields[1]], pol)
	}
	return nil
}

// handlePrefixList handles "ip prefix-list ..." statements.
func (p *frrLikeParser) handlePrefixList(fields []string) error {
	if !p.dialect.SupportsRouteMapPolicy() || len(fields) < 5 {
		return nil
	}
	// Check for "ip prefix-list NAME seq N (permit|deny) PREFIX [ge|le ...]"
	if fields[3] == "seq" && len(fields) >= 7 && (fields[5] == "permit" || fields[5] == "deny") {
		seq, err := strconv.Atoi(fields[4])
		if err != nil {
			return err
		}
		rule, err := parsePrefixListRule(seq, fields[5], fields[6], fields[7:])
		if err != nil {
			return fmt.Errorf("%s: %w", strings.Join(fields, " "), err)
		}
		addPrefixListRule(p.prefixLists, fields[2], rule)
		return nil
	}
	// "ip prefix-list NAME (permit|deny) PREFIX [ge|le ...]"
	if len(fields) >= 5 && (fields[3] == "permit" || fields[3] == "deny") {
		rule, err := parsePrefixListRule(0, fields[3], fields[4], fields[5:])
		if err != nil {
			return fmt.Errorf("%s: %w", strings.Join(fields, " "), err)
		}
		addPrefixListRule(p.prefixLists, fields[2], rule)
		return nil
	}
	return nil
}
