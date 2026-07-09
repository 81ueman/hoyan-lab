package configparse

import (
	"bufio"
	"fmt"
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// srlinuxParser holds all state for parsing SR Linux config files.
type srlinuxParser struct {
	cfg             ParsedConfig
	warnings        []UnsupportedStatement
	path            string
	text            string
	collectWarnings bool

	// Accumulators
	groupAS              map[string]uint32
	groupImportPolicy    map[string]string
	groupExportPolicy    map[string]string
	groupNextHopSelf     map[string]bool
	neighborGroup        map[string]string
	neighborImportPolicy map[string]string
	neighborExportPolicy map[string]string
	neighborNextHopSelf  map[string]bool
	staticNextHopGroups  map[string]string
	prefixLists          map[string]*model.PrefixList
	routePolicies        map[string]*model.RoutePolicy
	srlACLs              map[string]map[int]*parsedACLRule
	aclBindings          []aclBinding
}

func newSRLinuxParser(path, text string, collectWarnings bool) *srlinuxParser {
	return &srlinuxParser{
		path:                 path,
		text:                 text,
		collectWarnings:      collectWarnings,
		groupAS:              make(map[string]uint32),
		groupImportPolicy:    make(map[string]string),
		groupExportPolicy:    make(map[string]string),
		groupNextHopSelf:     make(map[string]bool),
		neighborGroup:        make(map[string]string),
		neighborImportPolicy: make(map[string]string),
		neighborExportPolicy: make(map[string]string),
		neighborNextHopSelf:  make(map[string]bool),
		staticNextHopGroups:  make(map[string]string),
		prefixLists:          make(map[string]*model.PrefixList),
		routePolicies:        make(map[string]*model.RoutePolicy),
		srlACLs:              make(map[string]map[int]*parsedACLRule),
	}
}

func (p *srlinuxParser) parse() (ParseResult, error) {
	scanner := bufio.NewScanner(strings.NewReader(p.text))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "set" {
			continue
		}

		if err := p.dispatch(fields, line, raw, lineNo); err != nil {
			if !p.collectWarnings {
				return ParseResult{}, fmt.Errorf("%s: %w", line, err)
			}
			p.warnings = append(p.warnings, unsupportedStatement("srlinux", p.path, lineNo, line, err.Error()))
		}
	}
	if err := scanner.Err(); err != nil {
		return ParseResult{}, err
	}
	return p.finalize(), nil
}

// dispatch routes a parsed SR Linux line to the appropriate handler.
func (p *srlinuxParser) dispatch(fields []string, line, raw string, lineNo int) error {
	switch {
	case containsSeq(fields, "acl", "interface") && containsAnyField(fields, "input", "output") && containsAnyField(fields, "acl-filter"):
		return p.handleACLBinding(fields, raw, lineNo)
	case containsSeq(fields, "acl", "acl-filter"):
		return p.handleACL(fields, raw, lineNo)
	case srLinuxRoutingPolicyKind(fields) == "prefix-set":
		return p.handlePrefixSet(fields)
	case srLinuxRoutingPolicyKind(fields) == "policy":
		return p.handleRoutePolicy(fields)
	case containsSeq(fields, "system", "name", "host-name"):
		return p.handleHostname(fields)
	case containsSeq(fields, "network-instance") && containsSeq(fields, "interface") && !containsSeq(fields, "protocols"):
		return p.handleNetworkInstanceInterface(fields)
	case containsSeq(fields, "interface") && containsSeq(fields, "ipv4", "address"):
		return p.handleInterfaceAddress(fields)
	case containsSeq(fields, "next-hop-groups", "group") && containsSeq(fields, "nexthop") && containsSeq(fields, "ip-address"):
		return p.handleNextHopGroup(fields)
	case containsSeq(fields, "static-routes", "route"):
		return p.handleStaticRoute(fields, raw, lineNo)
	case containsSeq(fields, "protocols", "ospf"):
		return p.handleOSPF(fields, raw, lineNo)
	case containsSeq(fields, "protocols", "bgp", "autonomous-system"):
		return p.handleBGPASN(fields)
	case containsSeq(fields, "protocols", "bgp", "router-id"):
		return p.handleBGPRouterID(fields)
	case containsSeq(fields, "protocols", "bgp", "group") && containsSeq(fields, "peer-as"):
		return p.handleBGPGroupAS(fields)
	case containsSeq(fields, "protocols", "bgp", "group") && containsAnyField(fields, "import-policy", "export-policy"):
		return p.handleBGPGroupPolicy(fields)
	case containsSeq(fields, "protocols", "bgp", "group") && containsSeq(fields, "next-hop-self"):
		return p.handleBGPGroupNextHopSelf(fields)
	case containsSeq(fields, "protocols", "bgp", "neighbor") && containsSeq(fields, "peer-group"):
		return p.handleBGPNeighborGroup(fields)
	case containsSeq(fields, "protocols", "bgp", "neighbor") && containsAnyField(fields, "import-policy", "export-policy"):
		return p.handleBGPNeighborPolicy(fields)
	case containsSeq(fields, "protocols", "bgp", "neighbor") && containsSeq(fields, "next-hop-self"):
		return p.handleBGPNeighborNextHopSelf(fields)
	case containsSeq(fields, "protocols", "bgp") && (containsAnyField(fields, "aggregate-address") || containsAnyField(fields, "aggregate-routes")):
		return p.handleBGPAggregate()
	}
	return nil
}

// finalize runs post-processing and returns the final ParseResult.
func (p *srlinuxParser) finalize() ParseResult {
	cfg := p.cfg
	for addr, group := range p.neighborGroup {
		neighbor := model.BGPNeighbor{
			Address:      addr,
			RemoteAS:     p.groupAS[group],
			Activated:    true,
			ImportPolicy: p.groupImportPolicy[group],
			ExportPolicy: p.groupExportPolicy[group],
			NextHopSelf:  p.groupNextHopSelf[group],
		}
		if policy := p.neighborImportPolicy[addr]; policy != "" {
			neighbor.ImportPolicy = policy
		}
		if policy := p.neighborExportPolicy[addr]; policy != "" {
			neighbor.ExportPolicy = policy
		}
		if p.neighborNextHopSelf[addr] {
			neighbor.NextHopSelf = true
		}
		cfg.Neighbors = append(cfg.Neighbors, neighbor)
	}
	addSRLinuxDefaultPolicyActions(p.routePolicies)
	cfg.PrefixLists = sortedPrefixLists(p.prefixLists)
	cfg.RoutePolicies = sortedRoutePolicies(p.routePolicies)
	cfg.ACLs = normalizedACLs(model.KindSRLinux, flattenSRLinuxACLs(p.srlACLs), model.ACLDefaultDeny)
	cfg.ACLBindings = normalizedACLBindings(p.aclBindings)
	return ParseResult{Config: cfg, Warnings: p.warnings}
}

// ---------------------------------------------------------------------------
// Handler methods
// ---------------------------------------------------------------------------

func (p *srlinuxParser) handleHostname(fields []string) error {
	if len(fields) > 0 {
		p.cfg.Hostname = fields[len(fields)-1]
	}
	return nil
}

func (p *srlinuxParser) handleNetworkInstanceInterface(fields []string) error {
	ni := model.NormalizeNetworkInstance(fieldAfter(fields, "network-instance"))
	iface := srlinuxConfigInterfaceName(fieldAfter(fields, "interface"))
	if iface != "" {
		p.cfg.Interfaces = upsertInterface(p.cfg.Interfaces, model.Interface{Name: iface, VRF: ni})
	}
	return nil
}

func (p *srlinuxParser) handleInterfaceAddress(fields []string) error {
	iface := fieldAfter(fields, "interface")
	addr := fields[len(fields)-1]
	p.cfg.Interfaces = upsertInterface(p.cfg.Interfaces, model.Interface{Name: iface, Address: addr})
	if strings.HasPrefix(strings.ToLower(iface), "lo") {
		p.cfg.Loopback = addr
	}
	return nil
}

func (p *srlinuxParser) handleNextHopGroup(fields []string) error {
	ni := fieldAfter(fields, "network-instance")
	group := fieldAfter(fields, "group")
	addr := fieldAfter(fields, "ip-address")
	if ni != "" && group != "" {
		p.staticNextHopGroups[srlinuxNextHopGroupKey(ni, group)] = addr
	}
	return nil
}

func (p *srlinuxParser) handleStaticRoute(fields []string, raw string, lineNo int) error {
	route, err := parseSRLinuxStaticRoute(p.path, lineNo, raw, fields, p.staticNextHopGroups)
	if err != nil {
		return err
	}
	p.cfg.Routes = append(p.cfg.Routes, route)
	return nil
}

func (p *srlinuxParser) handleOSPF(fields []string, raw string, lineNo int) error {
	return parseSRLinuxOSPF(&p.cfg, p.path, lineNo, raw, fields)
}

func (p *srlinuxParser) handleBGPASN(fields []string) error {
	asn, err := strconv.ParseUint(fields[len(fields)-1], 10, 32)
	if err != nil {
		return err
	}
	p.cfg.ASN = uint32(asn)
	return nil
}

func (p *srlinuxParser) handleBGPRouterID(fields []string) error {
	p.cfg.RouterID = fields[len(fields)-1]
	p.cfg.Loopback = p.cfg.RouterID + "/32"
	return nil
}

func (p *srlinuxParser) handleBGPGroupAS(fields []string) error {
	group := fieldAfter(fields, "group")
	asn, err := strconv.ParseUint(fields[len(fields)-1], 10, 32)
	if err != nil {
		return err
	}
	p.groupAS[group] = uint32(asn)
	return nil
}

func (p *srlinuxParser) handleBGPGroupPolicy(fields []string) error {
	group := fieldAfter(fields, "group")
	policy, err := parseSRLinuxPolicyBinding(fields)
	if err != nil {
		return err
	}
	if containsAnyField(fields, "import-policy") {
		p.groupImportPolicy[group] = policy
	} else {
		p.groupExportPolicy[group] = policy
	}
	return nil
}

func (p *srlinuxParser) handleBGPGroupNextHopSelf(fields []string) error {
	group := fieldAfter(fields, "group")
	p.groupNextHopSelf[group] = true
	return nil
}

func (p *srlinuxParser) handleBGPNeighborGroup(fields []string) error {
	addr := fieldAfter(fields, "neighbor")
	p.neighborGroup[addr] = fields[len(fields)-1]
	return nil
}

func (p *srlinuxParser) handleBGPNeighborPolicy(fields []string) error {
	addr := fieldAfter(fields, "neighbor")
	policy, err := parseSRLinuxPolicyBinding(fields)
	if err != nil {
		return err
	}
	if containsAnyField(fields, "import-policy") {
		p.neighborImportPolicy[addr] = policy
	} else {
		p.neighborExportPolicy[addr] = policy
	}
	return nil
}

func (p *srlinuxParser) handleBGPNeighborNextHopSelf(fields []string) error {
	addr := fieldAfter(fields, "neighbor")
	p.neighborNextHopSelf[addr] = true
	return nil
}

func (p *srlinuxParser) handleBGPAggregate() error {
	return fmt.Errorf("unsupported SR Linux BGP aggregate route statement")
}

func (p *srlinuxParser) handlePrefixSet(fields []string) error {
	return parseSRLinuxPrefixSet(p.prefixLists, fields)
}

func (p *srlinuxParser) handleRoutePolicy(fields []string) error {
	return parseSRLinuxRoutePolicy(p.routePolicies, p.prefixLists, fields)
}

func (p *srlinuxParser) handleACL(fields []string, raw string, lineNo int) error {
	return parseSRLinuxACL(p.srlACLs, p.path, lineNo, raw, fields)
}

func (p *srlinuxParser) handleACLBinding(fields []string, raw string, lineNo int) error {
	binding, ok := parseSRLinuxACLBinding(p.path, lineNo, raw, fields)
	if ok {
		p.aclBindings = append(p.aclBindings, binding)
	}
	return nil
}
