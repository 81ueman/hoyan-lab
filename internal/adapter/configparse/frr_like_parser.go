package configparse

import (
	"bufio"
	"fmt"
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
			return p.handleRouteMapCatchAll(line, lineNo)
		}
	}

	// 3. Route-map catch-all — any unrecognized line inside a route-map (no current rule)
	if p.currentRoutePolicy != nil && fields[0] != "route-map" {
		return p.handleRouteMapCatchAll(line, lineNo)
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
			return p.handleOSPFNetwork(fields, line, raw, lineNo)
		}
		return p.handleBGPNetwork(fields, line, raw, lineNo)
	case "aggregate-address":
		return p.handleBGPAggregateAddress(fields, raw, lineNo)
	case "redistribute":
		if p.inOSPF {
			return p.handleOSPFRedistribute(fields, line, raw, lineNo)
		}
		if p.inBGP && p.inAF {
			return p.handleBGPRedistribute(fields, raw, lineNo)
		}
		return nil
	case "ospf":
		return p.handleOSPFRouterIDOrStatement(fields, line, raw, lineNo)
	case "passive-interface":
		return p.handleOSPFPassiveInterface(fields, line, raw, lineNo)
	case "area":
		return p.handleOSPFArea(fields, line, raw, lineNo)
	case "vrf":
		return p.handleInterfaceVRF(fields)
	}

	// Catch-all for unrecognized statements inside an active OSPF context
	if p.inOSPF && p.dialect.SupportsOSPFConfig() {
		return fmt.Errorf("unsupported %s OSPF statement %q", p.dialect.VendorName(), line)
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
		return p.handleInterfaceACLBinding(fields, line, raw, lineNo)
	case "route":
		return p.handleStaticRoute(fields, raw, lineNo)
	case "ospf":
		return p.handleInterfaceOSPF(fields, line, raw, lineNo)
	}
	return nil
}

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
func (p *frrLikeParser) handleInterfaceACLBinding(fields []string, line, raw string, lineNo int) error {
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
		Source:    model.ConfigSource{Vendor: string(p.dialect.Kind()), File: p.path, Line: lineNo, Raw: line},
	})
	return nil
}

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

