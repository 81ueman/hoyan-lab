package configparse

import (
	"fmt"
	"net/netip"
	"strconv"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func (p *frrLikeParser) handleRouteMapStart(fields []string) error {
	if !p.dialect.SupportsRouteMapPolicy() {
		return nil
	}
	if len(fields) < 4 || (fields[2] != "permit" && fields[2] != "deny") {
		return nil
	}
	seq := 0
	if len(fields) >= 4 {
		var err error
		seq, err = strconv.Atoi(fields[3])
		if err != nil {
			return err
		}
	}
	p.currentRoutePolicy, p.currentRouteRule = addRoutePolicyRule(p.routePolicies, fields[1], fields[2], seq)
	p.currentInterface = ""
	p.inBGP = false
	p.inAF = false
	return nil
}

// handleRouteMapMatch handles "match ..." clauses inside a route-map rule.
func (p *frrLikeParser) handleRouteMapMatch(fields []string, line, raw string, lineNo int) error {
	if !p.dialect.SupportsRouteMapPolicy() || p.currentRouteRule == nil {
		return nil
	}

	// match ip address prefix-list NAME
	if len(fields) >= 5 && fields[1] == "ip" && fields[2] == "address" && fields[3] == "prefix-list" {
		p.currentRouteRule.MatchPrefixList = fields[4]
		return nil
	}
	// match ip next-hop prefix-list NAME
	if p.dialect.SupportsAdvancedRouteMapPolicy() && len(fields) >= 5 && fields[1] == "ip" && fields[2] == "next-hop" && fields[3] == "prefix-list" {
		p.currentRouteRule.MatchNextHopPrefixList = fields[4]
		return nil
	}
	// match as-path NAME
	if p.dialect.SupportsAdvancedRouteMapPolicy() && len(fields) >= 3 && fields[1] == "as-path" {
		p.currentRouteRule.MatchASPathList = fields[2]
		return nil
	}
	// match community NAME [exact-match|any]
	if p.dialect.SupportsAdvancedRouteMapPolicy() && len(fields) >= 3 && fields[1] == "community" {
		p.currentRouteRule.MatchCommunityList = fields[2]
		if len(fields) >= 4 {
			switch fields[3] {
			case "exact-match":
				p.currentRouteRule.MatchCommunityExact = true
			case "any":
				// no-op
			default:
				return fmt.Errorf("unsupported %s route-map match statement %q", p.dialect.VendorName(), line)
			}
		}
		return nil
	}
	// Unsupported match — collect as warning in warning mode
	if p.collectWarnings {
		p.warnings = append(p.warnings, unsupportedStatement(
			string(p.dialect.Kind()), p.path, lineNo, line,
			fmt.Sprintf("unsupported %s route-map match statement", p.dialect.VendorName()),
		))
		return nil
	}
	return fmt.Errorf("unsupported %s route-map match statement %q", p.dialect.VendorName(), line)
}

// handleRouteMapSet handles "set ..." clauses inside a route-map rule.
func (p *frrLikeParser) handleRouteMapSet(fields []string, line, raw string, lineNo int) error {
	if !p.dialect.SupportsRouteMapPolicy() || p.currentRouteRule == nil {
		return nil
	}

	// set local-preference VALUE
	if len(fields) >= 3 && fields[1] == "local-preference" {
		v, delta, err := parseRouteMapInt(fields[2])
		if err != nil {
			return err
		}
		if delta {
			p.currentRouteRule.SetLocalPrefDelta = intPtr(v)
		} else {
			p.currentRouteRule.SetLocalPref = intPtr(v)
		}
		return nil
	}
	// set metric VALUE
	if len(fields) >= 3 && fields[1] == "metric" {
		v, delta, err := parseRouteMapInt(fields[2])
		if err != nil {
			return err
		}
		if delta {
			p.currentRouteRule.SetMEDDelta = intPtr(v)
		} else {
			p.currentRouteRule.SetMED = intPtr(v)
		}
		return nil
	}
	// set as-path prepend ASN...
	if p.dialect.SupportsAdvancedRouteMapPolicy() && len(fields) >= 4 && fields[1] == "as-path" && fields[2] == "prepend" {
		path, err := parseASPathFields(fields[3:])
		if err != nil {
			return err
		}
		p.currentRouteRule.SetASPathPrepend = path
		return nil
	}
	// set community VALUE [additive]
	if p.dialect.SupportsAdvancedRouteMapPolicy() && len(fields) >= 3 && fields[1] == "community" {
		communities := append([]string(nil), fields[2:]...)
		if len(communities) > 0 && communities[len(communities)-1] == "additive" {
			p.currentRouteRule.SetCommunityAdditive = true
			communities = communities[:len(communities)-1]
		}
		p.currentRouteRule.SetCommunities = communities
		return nil
	}
	// set origin (igp|egp|incomplete)
	if p.dialect.SupportsAdvancedRouteMapPolicy() && len(fields) >= 3 && fields[1] == "origin" {
		switch fields[2] {
		case "igp", "egp", "incomplete":
			p.currentRouteRule.SetOriginCode = model.NormalizeBGPOriginCode(model.BGPOriginCode(fields[2]))
		default:
			return fmt.Errorf("unsupported %s route-map origin %q", p.dialect.VendorName(), line)
		}
		return nil
	}
	// set ip next-hop ADDRESS
	if p.dialect.SupportsAdvancedRouteMapPolicy() && len(fields) >= 4 && fields[1] == "ip" && fields[2] == "next-hop" {
		if _, err := netip.ParseAddr(fields[3]); err != nil {
			return fmt.Errorf("unsupported %s route-map next-hop %q", p.dialect.VendorName(), line)
		}
		p.currentRouteRule.SetNextHop = fields[3]
		return nil
	}

	// Unsupported set — collect as warning in warning mode
	if p.collectWarnings {
		p.warnings = append(p.warnings, unsupportedStatement(
			string(p.dialect.Kind()), p.path, lineNo, line,
			fmt.Sprintf("unsupported %s route-map statement", p.dialect.VendorName()),
		))
		return nil
	}
	return fmt.Errorf("unsupported %s route-map statement %q", p.dialect.VendorName(), line)
}

// handleRouteMapMisc handles "call", "continue", "on-match" inside a route-map.
func (p *frrLikeParser) handleRouteMapMisc(fields []string, line, raw string, lineNo int) error {
	if !p.dialect.SupportsRouteMapPolicy() {
		return nil
	}
	if p.collectWarnings {
		p.warnings = append(p.warnings, unsupportedStatement(
			string(p.dialect.Kind()), p.path, lineNo, line,
			fmt.Sprintf("unsupported %s route-map statement", p.dialect.VendorName()),
		))
		return nil
	}
	return fmt.Errorf("unsupported %s route-map statement %q", p.dialect.VendorName(), line)
}

// handleRouteMapCatchAll handles any unrecognized line inside a route-map block.
func (p *frrLikeParser) handleRouteMapCatchAll(line string, lineNo int) error {
	if p.collectWarnings {
		p.warnings = append(p.warnings, unsupportedStatement(
			string(p.dialect.Kind()), p.path, lineNo, line,
			fmt.Sprintf("unsupported %s route-map statement", p.dialect.VendorName()),
		))
	}
	return nil
}

// handleInterfaceAddress handles "ip address ADDR" under an interface.
