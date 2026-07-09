package configparse

import (
	"bufio"
	"fmt"
	"net/netip"
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// frrLikeParser holds all state for parsing FRR-like (FRR, cEOS) config files.
type frrLikeParser struct {
	dialect         frrLikeDialect
	cfg             ParsedConfig
	warnings        []UnsupportedStatement
	path            string
	text            string
	collectWarnings bool

	// Accumulators
	neighbors      map[string]*model.BGPNeighbor
	prefixLists    map[string]*model.PrefixList
	asPathLists    map[string]*model.ASPathList
	communityLists map[string]*model.CommunityList
	routePolicies  map[string]*model.RoutePolicy
	aclPolicies    map[string][]parsedACLRule
	aclBindings    []aclBinding

	// Context tracking
	currentInterface   string
	currentACL         string
	currentRoutePolicy *model.RoutePolicy
	currentRouteRule   *model.RoutePolicyRule
	inBGP              bool
	inAF               bool
	inOSPF             bool
	bgpVRF             model.NetworkInstanceID
	currentOSPFVRF     model.NetworkInstanceID
}

func newFRRLikeParser(dialect frrLikeDialect, path, text string, collectWarnings bool) *frrLikeParser {
	return &frrLikeParser{
		dialect:         dialect,
		path:            path,
		text:            text,
		collectWarnings: collectWarnings,
		neighbors:       make(map[string]*model.BGPNeighbor),
		prefixLists:     make(map[string]*model.PrefixList),
		asPathLists:     make(map[string]*model.ASPathList),
		communityLists:  make(map[string]*model.CommunityList),
		routePolicies:   make(map[string]*model.RoutePolicy),
		aclPolicies:     make(map[string][]parsedACLRule),
	}
}

func (p *frrLikeParser) parse() (ParseResult, error) {
	scanner := bufio.NewScanner(strings.NewReader(p.text))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)

		// Reset context for top-level statements (before empty/comment check)
		p.resetContext(line, raw)

		// Skip empty lines and comments
		if line == "" || line == "!" {
			if line == "!" && !strings.HasPrefix(raw, " ") {
				p.currentInterface = ""
				p.currentACL = ""
			}
			continue
		}

		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		if err := p.dispatch(fields, line, raw, lineNo); err != nil {
			if !p.collectWarnings {
				return ParseResult{}, err
			}
			p.warnings = append(p.warnings, unsupportedStatement(string(p.dialect.Kind()), p.path, lineNo, line, err.Error()))
		}
	}
	if err := scanner.Err(); err != nil {
		return ParseResult{}, err
	}
	return p.finalize(), nil
}

// resetContext clears per-block state when entering a new top-level statement.
func (p *frrLikeParser) resetContext(line, raw string) {
	if !strings.HasPrefix(raw, " ") && !strings.HasPrefix(line, "route-map ") {
		p.currentRoutePolicy = nil
		p.currentRouteRule = nil
		p.inOSPF = false
		p.currentOSPFVRF = model.NetworkInstanceDefault
		if !strings.HasPrefix(line, "ip access-list ") {
			p.currentACL = ""
		}
	}
}

// dispatch routes a parsed line to the appropriate handler method.
func (p *frrLikeParser) dispatch(fields []string, line, raw string, lineNo int) error {
	// 1. ACL rule lines within a current ACL block
	if p.currentACL != "" && isACLRuleLine(fields) {
		return p.handleACLRuleLine(fields, raw, lineNo)
	}

	// 2. Route-map match/set/misc clauses (context-sensitive)
	if p.currentRouteRule != nil {
		switch fields[0] {
		case "match":
			return p.handleRouteMapMatch(fields, line, raw, lineNo)
		case "set":
			return p.handleRouteMapSet(fields, line, raw, lineNo)
		case "call", "continue", "on-match":
			return p.handleRouteMapMisc(fields, line, raw, lineNo)
		case "route-map":
			// A new route-map entry always starts a new rule, even inside another route-map.
			// Fall through to the top-level dispatch.
		default:
			return p.handleRouteMapCatchAll(line, raw, lineNo)
		}
	}

	// 3. Route-map catch-all — any unrecognized line inside a route-map (no current rule)
	if p.currentRoutePolicy != nil && fields[0] != "route-map" {
		return p.handleRouteMapCatchAll(line, raw, lineNo)
	}

	// 4. Top-level dispatch by first field token
	switch fields[0] {
	case "hostname":
		return p.handleHostname(fields)
	case "interface":
		return p.handleInterface(fields)
	case "router":
		return p.handleRouter(fields, line, raw, lineNo)
	case "ip":
		return p.handleIP(fields, line, raw, lineNo)
	case "access-list":
		return p.handleFlatAccessList(fields, raw, lineNo)
	case "bgp":
		return p.handleBGP(fields, line, raw, lineNo)
	case "route-map":
		return p.handleRouteMapStart(fields)
	case "router-id":
		if p.inOSPF {
			return p.handleOSPFRouterID(fields)
		}
		if p.inBGP {
			return p.handleBGPCommon(fields)
		}
		return nil
	case "address-family":
		return p.handleBGPAddressFamily(fields)
	case "exit-address-family":
		return p.handleBGPExitAddressFamily()
	case "neighbor":
		return p.handleBGPNeighbor(fields, raw, lineNo)
	case "network":
		if p.inOSPF {
			return p.handleOSPFNetwork(fields, raw, lineNo)
		}
		return p.handleBGPNetwork(fields, raw, lineNo)
	case "aggregate-address":
		return p.handleBGPAggregateAddress(fields, raw, lineNo)
	case "redistribute":
		if p.inOSPF {
			return p.handleOSPFRedistribute(fields, raw, lineNo)
		}
		if p.inBGP && p.inAF {
			return p.handleBGPRedistribute(fields, raw, lineNo)
		}
		return nil
	case "ospf":
		return p.handleOSPFRouterIDOrStatement(fields, line, raw, lineNo)
	case "passive-interface":
		return p.handleOSPFPassiveInterface(fields, raw, lineNo)
	case "area":
		return p.handleOSPFArea(fields, line, raw, lineNo)
	case "vrf":
		return p.handleInterfaceVRF(fields)
	}
	return nil
}

// finalize runs post-processing and returns the final ParseResult.
func (p *frrLikeParser) finalize() ParseResult {
	cfg := p.cfg
	for _, n := range p.neighbors {
		if n.Activated {
			cfg.Neighbors = append(cfg.Neighbors, *n)
		}
	}
	cfg.PrefixLists = sortedPrefixLists(p.prefixLists)
	cfg.ASPathLists = sortedASPathLists(p.asPathLists)
	cfg.CommunityLists = sortedCommunityLists(p.communityLists)
	cfg.RoutePolicies = sortedRoutePolicies(p.routePolicies)
	cfg.ACLs = normalizedACLs(p.dialect.Kind(), p.aclPolicies, p.dialect.DefaultACLAction(model.ACLDefaultDeny))
	cfg.ACLBindings = normalizedACLBindings(p.aclBindings)
	cfg.OSPFProcesses = compactOSPFProcesses(cfg.OSPFProcesses)
	if cfg.Loopback == "" && cfg.RouterID != "" {
		cfg.Loopback = cfg.RouterID + "/32"
	}
	return ParseResult{Config: cfg, Warnings: p.warnings}
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

func (p *frrLikeParser) handleHostname(fields []string) error {
	if len(fields) >= 2 {
		p.cfg.Hostname = fields[1]
	}
	return nil
}

func (p *frrLikeParser) handleInterface(fields []string) error {
	if len(fields) < 2 {
		return nil
	}
	p.currentInterface = fields[1]
	if p.dialect.SupportsVRF() && len(fields) >= 4 && fields[2] == "vrf" {
		p.cfg.Interfaces = upsertInterface(p.cfg.Interfaces, model.Interface{Name: p.currentInterface, VRF: model.NormalizeNetworkInstance(fields[3])})
	}
	p.currentACL = ""
	p.inBGP = false
	p.inAF = false
	p.inOSPF = false
	return nil
}

func (p *frrLikeParser) handleRouter(fields []string, line, raw string, lineNo int) error {
	if len(fields) < 2 {
		return nil
	}
	switch fields[1] {
	case "bgp":
		return p.handleRouterBGP(fields, raw, lineNo)
	case "ospf":
		return p.handleRouterOSPF(fields)
	}
	return nil
}

func (p *frrLikeParser) handleRouterBGP(fields []string, raw string, lineNo int) error {
	if len(fields) < 3 {
		return nil
	}
	asn, err := strconv.ParseUint(fields[2], 10, 32)
	if err != nil {
		return err
	}
	p.cfg.ASN = uint32(asn)
	p.bgpVRF = model.NetworkInstanceDefault
	if len(fields) >= 5 && fields[3] == "vrf" {
		p.bgpVRF = model.NormalizeNetworkInstance(fields[4])
	}
	p.inBGP = true
	p.inAF = false
	p.inOSPF = false
	p.currentInterface = ""
	return nil
}

func (p *frrLikeParser) handleRouterOSPF(fields []string) error {
	if !p.dialect.SupportsOSPFConfig() {
		return nil
	}
	p.currentOSPFVRF = parseFRRLikeOSPFVRF(fields)
	ospf := ospfProcess(&p.cfg, p.currentOSPFVRF)
	ospf.Enabled = true
	p.inOSPF = true
	p.inBGP = false
	p.inAF = false
	p.currentInterface = ""
	return nil
}

func (p *frrLikeParser) handleIP(fields []string, line, raw string, lineNo int) error {
	if len(fields) < 2 {
		return nil
	}
	switch fields[1] {
	case "access-list":
		return p.handleIPAccessList(fields)
	case "prefix-list":
		return p.handlePrefixList(fields)
	case "address":
		return p.handleInterfaceAddress(fields)
	case "access-group":
		return p.handleInterfaceACLBinding(fields, raw, lineNo)
	case "route":
		return p.handleStaticRoute(fields, raw, lineNo)
	case "ospf":
		return p.handleInterfaceOSPF(fields, line, raw, lineNo)
	}
	return nil
}

// handleIPAccessList starts a new ACL context.
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

// handleBGP handles "bgp ..." statements — global lists or inside-BGP settings.
func (p *frrLikeParser) handleBGP(fields []string, line, raw string, lineNo int) error {
	// bgp as-path access-list (global, outside BGP context)
	if len(fields) >= 6 && p.dialect.SupportsBGPStringLists() && fields[1] == "as-path" && fields[2] == "access-list" && (fields[4] == "permit" || fields[4] == "deny") {
		addStringListRule(p.asPathLists, fields[3], model.StringListRule{Action: fields[4], Pattern: strings.Join(fields[5:], " ")})
		return nil
	}
	// bgp community-list standard (global, outside BGP context)
	if len(fields) >= 6 && p.dialect.SupportsBGPStringLists() && fields[1] == "community-list" && fields[2] == "standard" && (fields[4] == "permit" || fields[4] == "deny") {
		addCommunityListRule(p.communityLists, fields[3], model.StringListRule{Action: fields[4], Pattern: strings.Join(fields[5:], " ")})
		return nil
	}
	// "bgp router-id X" inside BGP context
	if p.inBGP && len(fields) >= 3 && fields[len(fields)-2] == "router-id" {
		p.cfg.RouterID = fields[len(fields)-1]
		return nil
	}
	return nil
}

// handleRouteMapStart starts a new route-map entry.
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
func (p *frrLikeParser) handleRouteMapCatchAll(line, raw string, lineNo int) error {
	if p.collectWarnings {
		p.warnings = append(p.warnings, unsupportedStatement(
			string(p.dialect.Kind()), p.path, lineNo, line,
			fmt.Sprintf("unsupported %s route-map statement", p.dialect.VendorName()),
		))
	}
	return nil
}

// handleInterfaceAddress handles "ip address ADDR" under an interface.
func (p *frrLikeParser) handleInterfaceAddress(fields []string) error {
	if p.currentInterface == "" || len(fields) < 3 {
		return nil
	}
	addr := fields[2]
	p.cfg.Interfaces = upsertInterface(p.cfg.Interfaces, model.Interface{Name: p.currentInterface, Address: addr})
	if strings.EqualFold(p.currentInterface, "lo") || strings.HasPrefix(strings.ToLower(p.currentInterface), "loopback") {
		p.cfg.Loopback = addr
	}
	return nil
}

// handleInterfaceVRF handles "vrf NAME" under an interface.
func (p *frrLikeParser) handleInterfaceVRF(fields []string) error {
	if !p.dialect.SupportsVRF() || p.currentInterface == "" || len(fields) < 2 {
		return nil
	}
	if vrf, ok := p.dialect.InterfaceVRF(fields); ok {
		p.cfg.Interfaces = upsertInterface(p.cfg.Interfaces, model.Interface{Name: p.currentInterface, VRF: vrf})
	}
	return nil
}

// handleInterfaceACLBinding handles "ip access-group NAME (in|out)" under an interface.
func (p *frrLikeParser) handleInterfaceACLBinding(fields []string, raw string, lineNo int) error {
	if p.currentInterface == "" || len(fields) < 4 {
		return nil
	}
	stage, ok := aclStage(fields[3])
	if !ok {
		return nil
	}
	p.aclBindings = append(p.aclBindings, aclBinding{
		Name:      fields[2],
		Interface: p.currentInterface,
		Stage:     stage,
		Source:    model.ConfigSource{Vendor: string(p.dialect.Kind()), File: p.path, Line: lineNo, Raw: raw},
	})
	return nil
}

// handleInterfaceOSPF handles "ip ospf ..." sub-statements under an interface.
func (p *frrLikeParser) handleInterfaceOSPF(fields []string, line, raw string, lineNo int) error {
	if !p.dialect.SupportsOSPFConfig() || p.currentInterface == "" || len(fields) < 3 {
		return nil
	}
	kind := p.dialect.Kind()
	ospfSubCmd := fields[2]

	switch {
	case ospfSubCmd == "area" && len(fields) >= 4:
		vrf := p.dialect.OSPFInterfaceVRF(p.cfg.Interfaces, p.currentInterface)
		ospf := ospfProcess(&p.cfg, vrf)
		oi := ospfInterface(ospf, p.currentInterface)
		oi.Area = normalizeOSPFAreaID(fields[3])
		oi.Source = model.ConfigSource{Vendor: string(kind), File: p.path, Line: lineNo, Raw: raw}
		ospf.Interfaces[p.currentInterface] = *oi
		ospf.Enabled = true
		return nil

	case ospfSubCmd == "cost" && len(fields) >= 4:
		cost, err := strconv.Atoi(fields[3])
		if err != nil || cost <= 0 {
			return fmt.Errorf("unsupported %s OSPF interface cost %q", p.dialect.VendorName(), line)
		}
		vrf := p.dialect.OSPFInterfaceVRF(p.cfg.Interfaces, p.currentInterface)
		ospf := ospfProcess(&p.cfg, vrf)
		oi := ospfInterface(ospf, p.currentInterface)
		oi.Cost = cost
		oi.Source = model.ConfigSource{Vendor: string(kind), File: p.path, Line: lineNo, Raw: raw}
		ospf.Interfaces[p.currentInterface] = *oi
		ospf.Enabled = true
		return nil

	case ospfSubCmd == "network" && len(fields) >= 4 && isSupportedOSPFNetworkType(fields[3]):
		vrf := p.dialect.OSPFInterfaceVRF(p.cfg.Interfaces, p.currentInterface)
		ospf := ospfProcess(&p.cfg, vrf)
		oi := ospfInterface(ospf, p.currentInterface)
		oi.NetworkType = normalizeOSPFNetworkType(fields[3])
		oi.Source = model.ConfigSource{Vendor: string(kind), File: p.path, Line: lineNo, Raw: raw}
		ospf.Interfaces[p.currentInterface] = *oi
		ospf.Enabled = true
		return nil

	case ospfSubCmd == "hello-interval" || ospfSubCmd == "dead-interval":
		ospfProcess(&p.cfg, p.dialect.OSPFInterfaceVRF(p.cfg.Interfaces, p.currentInterface)).Enabled = true
		return nil

	case ospfSubCmd == "mtu-ignore":
		ospfProcess(&p.cfg, p.dialect.OSPFInterfaceVRF(p.cfg.Interfaces, p.currentInterface)).Enabled = true
		return nil

	default:
		return fmt.Errorf("unsupported %s OSPF interface statement %q", p.dialect.VendorName(), line)
	}
}

// handleStaticRoute handles "ip route ..." statements.
func (p *frrLikeParser) handleStaticRoute(fields []string, raw string, lineNo int) error {
	if len(fields) < 4 {
		return nil
	}
	route, err := parseFRRLikeStaticRoute(p.dialect.Kind(), p.path, lineNo, raw, fields)
	if err != nil {
		return err
	}
	p.cfg.Routes = append(p.cfg.Routes, route)
	return nil
}

// handleBGPCommon handles "router-id X" under BGP.
func (p *frrLikeParser) handleBGPCommon(fields []string) error {
	if len(fields) >= 2 {
		p.cfg.RouterID = fields[1]
	}
	return nil
}

// handleBGPAddressFamily sets the address-family context.
func (p *frrLikeParser) handleBGPAddressFamily(fields []string) error {
	if !p.inBGP {
		return nil
	}
	p.inAF = true
	return nil
}

// handleBGPExitAddressFamily leaves the address-family context.
func (p *frrLikeParser) handleBGPExitAddressFamily() error {
	if !p.inBGP {
		return nil
	}
	p.inAF = false
	return nil
}

// handleBGPNeighbor handles "neighbor ..." statements under BGP.
func (p *frrLikeParser) handleBGPNeighbor(fields []string, raw string, lineNo int) error {
	if !p.inBGP || len(fields) < 3 {
		return nil
	}

	switch {
	case fields[2] == "remote-as":
		asn, err := strconv.ParseUint(fields[3], 10, 32)
		if err != nil {
			return err
		}
		n := getNeighbor(p.neighbors, p.bgpVRF, fields[1])
		n.RemoteAS = uint32(asn)
		return nil

	case p.inAF && fields[2] == "activate":
		getNeighbor(p.neighbors, p.bgpVRF, fields[1]).Activated = true
		return nil

	case p.inAF && fields[2] == "next-hop-self":
		getNeighbor(p.neighbors, p.bgpVRF, fields[1]).NextHopSelf = true
		return nil

	case p.dialect.SupportsRouteMapPolicy() && p.inAF && fields[2] == "route-map" && len(fields) >= 5:
		n := getNeighbor(p.neighbors, p.bgpVRF, fields[1])
		switch fields[4] {
		case "in":
			n.ImportPolicy = fields[3]
		case "out":
			n.ExportPolicy = fields[3]
		}
		return nil
	}
	return nil
}

// handleBGPNetwork handles "network PREFIX" within BGP address-family.
func (p *frrLikeParser) handleBGPNetwork(fields []string, raw string, lineNo int) error {
	if !p.inBGP || !p.inAF || len(fields) < 2 {
		return nil
	}
	kind := p.dialect.Kind()
	if p.bgpVRF == model.NetworkInstanceDefault {
		p.cfg.Prefixes = appendUnique(p.cfg.Prefixes, fields[1])
		return nil
	}
	prefix, err := model.ParsePrefix(fields[1])
	if err != nil {
		return err
	}
	p.cfg.Routes = append(p.cfg.Routes, model.ConfiguredRoute{
		NetworkInstance: p.bgpVRF,
		AFI:             model.AFIIPv4,
		Prefix:          prefix,
		Kind:            model.RouteSourceBGP,
		AdminDistance:   200,
		Source:          model.ConfigSource{Vendor: string(kind), File: p.path, Line: lineNo, Raw: raw},
	})
	return nil
}

// handleBGPAggregateAddress handles "aggregate-address ..." in BGP address-family.
func (p *frrLikeParser) handleBGPAggregateAddress(fields []string, raw string, lineNo int) error {
	if !p.inBGP || !p.inAF || len(fields) < 2 {
		return nil
	}
	route, err := parseAggregateRoute(p.dialect.Kind(), p.path, lineNo, raw, fields)
	if err != nil {
		return err
	}
	route.NetworkInstance = p.bgpVRF
	p.cfg.Routes = append(p.cfg.Routes, route)
	return nil
}

// handleBGPRedistribute handles "redistribute ..." in BGP address-family.
func (p *frrLikeParser) handleBGPRedistribute(fields []string, raw string, lineNo int) error {
	if len(fields) < 2 {
		return nil
	}
	redist, err := parseFRRLikeRedistribution(p.dialect.Kind(), p.path, lineNo, raw, fields)
	if err != nil {
		return err
	}
	redist.NetworkInstance = p.bgpVRF
	p.cfg.Redistribute = append(p.cfg.Redistribute, redist)
	return nil
}

// handleOSPFRouterID handles "router-id X" inside OSPF context.
func (p *frrLikeParser) handleOSPFRouterID(fields []string) error {
	if len(fields) < 2 {
		return nil
	}
	ospfProcess(&p.cfg, p.currentOSPFVRF).RouterID = fields[1]
	return nil
}

// handleOSPFRouterIDOrStatement handles "ospf router-id X" inside OSPF context.
func (p *frrLikeParser) handleOSPFRouterIDOrStatement(fields []string, line, raw string, lineNo int) error {
	if !p.inOSPF || len(fields) < 2 {
		return nil
	}
	// "ospf router-id X"
	if fields[1] == "router-id" && len(fields) >= 3 {
		ospfProcess(&p.cfg, p.currentOSPFVRF).RouterID = fields[2]
		return nil
	}
	// Unsupported ospf statement
	return fmt.Errorf("unsupported %s OSPF statement %q", p.dialect.VendorName(), line)
}

// handleOSPFPassiveInterface handles "passive-interface NAME" inside OSPF.
func (p *frrLikeParser) handleOSPFPassiveInterface(fields []string, raw string, lineNo int) error {
	if !p.inOSPF || len(fields) < 2 {
		return nil
	}
	ospf := ospfProcess(&p.cfg, p.currentOSPFVRF)
	ospf.PassiveInterfaces = appendUnique(ospf.PassiveInterfaces, fields[1])
	oi := ospfInterface(ospf, fields[1])
	oi.Passive = true
	oi.Source = model.ConfigSource{Vendor: string(p.dialect.Kind()), File: p.path, Line: lineNo, Raw: raw}
	ospf.Interfaces[fields[1]] = *oi
	return nil
}

// handleOSPFArea handles "area ..." inside OSPF.
func (p *frrLikeParser) handleOSPFArea(fields []string, line, raw string, lineNo int) error {
	if !p.inOSPF || len(fields) < 3 {
		return nil
	}
	area, err := parseFRRLikeOSPFArea(p.dialect.Kind(), p.path, lineNo, raw, fields)
	if err != nil {
		return err
	}
	ospf := ospfProcess(&p.cfg, p.currentOSPFVRF)
	ospf.Areas[area.ID] = area
	return nil
}

// handleOSPFRedistribute handles "redistribute ..." inside OSPF context.
func (p *frrLikeParser) handleOSPFRedistribute(fields []string, raw string, lineNo int) error {
	if len(fields) < 2 {
		return nil
	}
	redist, err := parseFRRLikeOSPFRedistribution(p.dialect.Kind(), p.path, lineNo, raw, fields)
	if err != nil {
		return err
	}
	ospf := ospfProcess(&p.cfg, p.currentOSPFVRF)
	ospf.Redistribute = append(ospf.Redistribute, redist)
	return nil
}

// handleOSPFNetwork handles "network PREFIX area AREA" inside OSPF context.
func (p *frrLikeParser) handleOSPFNetwork(fields []string, raw string, lineNo int) error {
	if !p.dialect.SupportsOSPFConfig() || !p.inOSPF || len(fields) < 4 || fields[2] != "area" {
		return nil
	}
	prefix, err := model.ParsePrefix(fields[1])
	if err != nil {
		return fmt.Errorf("unsupported %s OSPF network %q", p.dialect.VendorName(), raw)
	}
	ospf := ospfProcess(&p.cfg, p.currentOSPFVRF)
	ospf.Networks = append(ospf.Networks, model.OSPFNetwork{
		Prefix: prefix,
		Area:   normalizeOSPFAreaID(fields[3]),
		Source: model.ConfigSource{Vendor: string(p.dialect.Kind()), File: p.path, Line: lineNo, Raw: raw},
	})
	return nil
}
