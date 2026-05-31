package configparse

import (
	"bufio"
	"fmt"
	"net/netip"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type ParsedConfig struct {
	Hostname       string
	ASN            uint32
	RouterID       string
	Loopback       string
	Interfaces     []model.Interface
	Prefixes       []string
	Routes         []model.ConfiguredRoute
	Redistribute   []model.BGPRedistribution
	Neighbors      []model.BGPNeighbor
	PrefixLists    []model.PrefixList
	ASPathLists    []model.ASPathList
	CommunityLists []model.CommunityList
	RoutePolicies  []model.RoutePolicy
	ACLs           []model.ACL
	ACLBindings    []model.ACLBinding
	OSPF           model.OSPFProcess
	OSPFProcesses  []model.OSPFProcess
}

type ParseResult struct {
	Config   ParsedConfig
	Warnings []UnsupportedStatement
}

type UnsupportedStatement struct {
	Vendor string
	File   string
	Line   int
	Text   string
	Reason string
}

type UnsupportedConfigError struct {
	Warnings []UnsupportedStatement
}

// Config parser entrypoints are intentionally per device. FRR and cEOS share
// syntax helpers through the FRR-like helper below, but vendor semantics live in
// the concrete parser/dialect instead of in ParseConfig or a kind-switching loop.
type FRRParser struct{}
type CEOSParser struct{}
type SRLinuxParser struct{}

type frrLikeDialect interface {
	Kind() model.DeviceKind
	VendorName() string
	SupportsRouteMapPolicy() bool
	SupportsOSPFConfig() bool
	SupportsVRF() bool
	SupportsFlatAccessList() bool
	SupportsBGPStringLists() bool
	SupportsAdvancedRouteMapPolicy() bool
	InterfaceVRF(fields []string) (model.NetworkInstanceID, bool)
	OSPFInterfaceVRF(ifaces []model.Interface, name string) model.NetworkInstanceID
	DefaultACLAction(fallback model.ACLDefaultAction) model.ACLDefaultAction
}

type frrDialect struct{}
type ceosDialect struct{}

type aclBinding struct {
	Name      string
	Interface string
	Stage     string
	Source    model.ConfigSource
}

type parsedACLRule struct {
	Name      string
	Stage     string
	Interface string
	Action    model.ACLAction
	Protocol  string
	SrcPrefix model.Prefix
	DstPrefix model.Prefix
	SrcPort   model.PortSet
	DstPort   model.PortSet
	Seq       int
	Source    model.ConfigSource
}

func (w UnsupportedStatement) String() string {
	loc := w.File
	if w.Line > 0 {
		loc = fmt.Sprintf("%s:%d", loc, w.Line)
	}
	if loc == "" {
		loc = w.Vendor
	}
	return fmt.Sprintf("%s: %s: %s", loc, w.Reason, w.Text)
}

func (e UnsupportedConfigError) Error() string {
	if len(e.Warnings) == 0 {
		return "unsupported config statements"
	}
	lines := make([]string, 0, len(e.Warnings)+1)
	lines = append(lines, fmt.Sprintf("unsupported config statements: %d", len(e.Warnings)))
	for _, warning := range e.Warnings {
		lines = append(lines, fmt.Sprintf("vendor=%s file=%s line=%d raw=%q reason=%s", warning.Vendor, warning.File, warning.Line, warning.Text, warning.Reason))
	}
	return strings.Join(lines, "\n")
}

func ParseConfig(kind model.DeviceKind, path string) (ParsedConfig, error) {
	result, err := parseConfig(kind, path, false)
	return result.Config, err
}

func ParseConfigWithWarnings(kind model.DeviceKind, path string) (ParseResult, error) {
	return parseConfig(kind, path, true)
}

type ParseOptions struct {
	CollectWarnings bool
}

func ParseConfigWithOptions(kind model.DeviceKind, path string, opts ParseOptions) (ParseResult, error) {
	return parseConfig(kind, path, opts.CollectWarnings)
}

func ParseNftablesACLConfig(path string) ([]model.ACL, []model.ACLBinding, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	return parseNftables(path, string(data))
}

func parseConfig(kind model.DeviceKind, path string, collectWarnings bool) (ParseResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return ParseResult{}, err
	}
	if !model.ProfileFor(kind).ConfigProfile().SupportsConfigParse() {
		return ParseResult{}, fmt.Errorf("unsupported config kind %q", kind)
	}
	switch kind {
	case model.KindFRR:
		return parseFRR(path, string(data), collectWarnings)
	case model.KindCEOS:
		return parseCEOS(path, string(data), collectWarnings)
	case model.KindSRLinux:
		return parseSRLinux(path, string(data), collectWarnings)
	default:
		return ParseResult{}, fmt.Errorf("unsupported config kind %q", kind)
	}
}

func parseFRR(path, text string, collectWarnings bool) (ParseResult, error) {
	return FRRParser{}.Parse(path, text, collectWarnings)
}

func parseCEOS(path, text string, collectWarnings bool) (ParseResult, error) {
	return CEOSParser{}.Parse(path, text, collectWarnings)
}

func (FRRParser) Parse(path, text string, collectWarnings bool) (ParseResult, error) {
	return parseFRRLike(frrDialect{}, path, text, collectWarnings)
}

func (CEOSParser) Parse(path, text string, collectWarnings bool) (ParseResult, error) {
	return parseFRRLike(ceosDialect{}, path, text, collectWarnings)
}

func (SRLinuxParser) Parse(path, text string, collectWarnings bool) (ParseResult, error) {
	return parseSRLinuxConfig(path, text, collectWarnings)
}

func parseSRLinux(path, text string, collectWarnings bool) (ParseResult, error) {
	return SRLinuxParser{}.Parse(path, text, collectWarnings)
}

func (frrDialect) Kind() model.DeviceKind { return model.KindFRR }

func (frrDialect) VendorName() string {
	return model.ProfileFor(model.KindFRR).ConfigProfile().RouteMapVendorName()
}

func (frrDialect) SupportsRouteMapPolicy() bool {
	return model.ProfileFor(model.KindFRR).ConfigProfile().SupportsRouteMapPolicy()
}

func (frrDialect) SupportsOSPFConfig() bool {
	return model.ProfileFor(model.KindFRR).ConfigProfile().SupportsOSPFConfig()
}

func (frrDialect) SupportsVRF() bool { return true }

func (frrDialect) SupportsFlatAccessList() bool { return true }

func (frrDialect) SupportsBGPStringLists() bool { return true }

func (frrDialect) SupportsAdvancedRouteMapPolicy() bool { return true }

func (frrDialect) InterfaceVRF(fields []string) (model.NetworkInstanceID, bool) {
	if len(fields) >= 3 && fields[1] == "forwarding" {
		return model.NormalizeNetworkInstance(fields[2]), true
	}
	return "", false
}

func (frrDialect) OSPFInterfaceVRF(ifaces []model.Interface, name string) model.NetworkInstanceID {
	return model.ProfileFor(model.KindFRR).ConfigProfile().OSPFInterfaceVRF(ifaces, name)
}

func (frrDialect) DefaultACLAction(fallback model.ACLDefaultAction) model.ACLDefaultAction {
	return model.ProfileFor(model.KindFRR).ACLProfile().DefaultACLAction(fallback)
}

func (ceosDialect) Kind() model.DeviceKind { return model.KindCEOS }

func (ceosDialect) VendorName() string {
	return model.ProfileFor(model.KindCEOS).ConfigProfile().RouteMapVendorName()
}

func (ceosDialect) SupportsRouteMapPolicy() bool {
	return model.ProfileFor(model.KindCEOS).ConfigProfile().SupportsRouteMapPolicy()
}

func (ceosDialect) SupportsOSPFConfig() bool {
	return model.ProfileFor(model.KindCEOS).ConfigProfile().SupportsOSPFConfig()
}

func (ceosDialect) SupportsVRF() bool { return true }

func (ceosDialect) SupportsFlatAccessList() bool { return false }

func (ceosDialect) SupportsBGPStringLists() bool { return false }

func (ceosDialect) SupportsAdvancedRouteMapPolicy() bool { return false }

func (ceosDialect) InterfaceVRF(fields []string) (model.NetworkInstanceID, bool) {
	if len(fields) >= 3 && fields[1] == "forwarding" {
		return model.NormalizeNetworkInstance(fields[2]), true
	}
	if len(fields) >= 2 {
		return model.NormalizeNetworkInstance(fields[1]), true
	}
	return "", false
}

func (ceosDialect) OSPFInterfaceVRF(ifaces []model.Interface, name string) model.NetworkInstanceID {
	return model.ProfileFor(model.KindCEOS).ConfigProfile().OSPFInterfaceVRF(ifaces, name)
}

func (ceosDialect) DefaultACLAction(fallback model.ACLDefaultAction) model.ACLDefaultAction {
	return model.ProfileFor(model.KindCEOS).ACLProfile().DefaultACLAction(fallback)
}

func parseFRRLike(dialect frrLikeDialect, path, text string, collectWarnings bool) (ParseResult, error) {
	kind := dialect.Kind()
	var cfg ParsedConfig
	var warnings []UnsupportedStatement
	neighbors := map[string]*model.BGPNeighbor{}
	prefixLists := map[string]*model.PrefixList{}
	asPathLists := map[string]*model.ASPathList{}
	communityLists := map[string]*model.CommunityList{}
	routePolicies := map[string]*model.RoutePolicy{}
	aclPolicies := map[string][]parsedACLRule{}
	var aclBindings []aclBinding
	var currentInterface string
	var currentACL string
	var currentRoutePolicy *model.RoutePolicy
	var currentRouteRule *model.RoutePolicyRule
	inBGP := false
	inAF := false
	inOSPF := false
	bgpVRF := model.NetworkInstanceDefault
	currentOSPFVRF := model.NetworkInstanceDefault
	scanner := bufio.NewScanner(strings.NewReader(text))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		if !strings.HasPrefix(raw, " ") && !strings.HasPrefix(line, "route-map ") {
			currentRoutePolicy = nil
			currentRouteRule = nil
			inOSPF = false
			currentOSPFVRF = model.NetworkInstanceDefault
			if !strings.HasPrefix(line, "ip access-list ") {
				currentACL = ""
			}
		}
		if line == "" || line == "!" {
			if line == "!" && !strings.HasPrefix(raw, " ") {
				currentInterface = ""
				currentACL = ""
			}
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch {
		case fields[0] == "hostname" && len(fields) >= 2:
			cfg.Hostname = fields[1]
		case len(fields) >= 3 && fields[0] == "ip" && fields[1] == "access-list":
			currentACL = fields[2]
			if len(fields) >= 4 && (fields[2] == "standard" || fields[2] == "extended") {
				currentACL = fields[3]
			}
			currentInterface = ""
			inBGP = false
			inAF = false
		case currentACL != "" && isACLRuleLine(fields):
			pol, ok, err := parseACLRule(kind, path, lineNo, line, currentACL, fields)
			if err != nil {
				if !collectWarnings {
					return ParseResult{}, err
				}
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, err.Error()))
				continue
			}
			if ok {
				aclPolicies[currentACL] = append(aclPolicies[currentACL], pol)
			}
		case dialect.SupportsFlatAccessList() && len(fields) >= 5 && fields[0] == "access-list" && (fields[2] == "permit" || fields[2] == "deny"):
			pol, ok, err := parseACLRule(kind, path, lineNo, line, fields[1], fields[2:])
			if err != nil {
				if !collectWarnings {
					return ParseResult{}, err
				}
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, err.Error()))
				continue
			}
			if ok {
				aclPolicies[fields[1]] = append(aclPolicies[fields[1]], pol)
			}
		case dialect.SupportsRouteMapPolicy() && len(fields) >= 5 && fields[0] == "ip" && fields[1] == "prefix-list" && (fields[3] == "permit" || fields[3] == "deny"):
			rule, err := parsePrefixListRule(0, fields[3], fields[4], fields[5:])
			if err != nil {
				return ParseResult{}, fmt.Errorf("%s: %w", line, err)
			}
			addPrefixListRule(prefixLists, fields[2], rule)
		case dialect.SupportsRouteMapPolicy() && len(fields) >= 7 && fields[0] == "ip" && fields[1] == "prefix-list" && fields[3] == "seq" && (fields[5] == "permit" || fields[5] == "deny"):
			seq, err := strconv.Atoi(fields[4])
			if err != nil {
				return ParseResult{}, err
			}
			rule, err := parsePrefixListRule(seq, fields[5], fields[6], fields[7:])
			if err != nil {
				return ParseResult{}, fmt.Errorf("%s: %w", line, err)
			}
			addPrefixListRule(prefixLists, fields[2], rule)
		case dialect.SupportsBGPStringLists() && len(fields) >= 6 && fields[0] == "bgp" && fields[1] == "as-path" && fields[2] == "access-list" && (fields[4] == "permit" || fields[4] == "deny"):
			addStringListRule(asPathLists, fields[3], model.StringListRule{Action: fields[4], Pattern: strings.Join(fields[5:], " ")})
		case dialect.SupportsBGPStringLists() && len(fields) >= 6 && fields[0] == "bgp" && fields[1] == "community-list" && fields[2] == "standard" && (fields[4] == "permit" || fields[4] == "deny"):
			addCommunityListRule(communityLists, fields[3], model.StringListRule{Action: fields[4], Pattern: strings.Join(fields[5:], " ")})
		case dialect.SupportsRouteMapPolicy() && len(fields) >= 4 && fields[0] == "route-map" && (fields[2] == "permit" || fields[2] == "deny"):
			seq := 0
			if len(fields) >= 4 {
				var err error
				seq, err = strconv.Atoi(fields[3])
				if err != nil {
					return ParseResult{}, err
				}
			}
			currentRoutePolicy, currentRouteRule = addRoutePolicyRule(routePolicies, fields[1], fields[2], seq)
			currentInterface = ""
			inBGP = false
			inAF = false
		case dialect.SupportsRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 5 && fields[0] == "match" && fields[1] == "ip" && fields[2] == "address" && fields[3] == "prefix-list":
			currentRouteRule.MatchPrefixList = fields[4]
		case dialect.SupportsAdvancedRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 5 && fields[0] == "match" && fields[1] == "ip" && fields[2] == "next-hop" && fields[3] == "prefix-list":
			currentRouteRule.MatchNextHopPrefixList = fields[4]
		case dialect.SupportsAdvancedRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 3 && fields[0] == "match" && fields[1] == "as-path":
			currentRouteRule.MatchASPathList = fields[2]
		case dialect.SupportsAdvancedRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 3 && fields[0] == "match" && fields[1] == "community":
			currentRouteRule.MatchCommunityList = fields[2]
			if len(fields) >= 4 {
				switch fields[3] {
				case "exact-match":
					currentRouteRule.MatchCommunityExact = true
				case "any":
				default:
					if !collectWarnings {
						return ParseResult{}, fmt.Errorf("unsupported %s route-map match statement %q", dialect.VendorName(), line)
					}
					warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, fmt.Sprintf("unsupported %s route-map match statement", dialect.VendorName())))
				}
			}
		case dialect.SupportsRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 1 && fields[0] == "match":
			if !collectWarnings {
				return ParseResult{}, fmt.Errorf("unsupported %s route-map match statement %q", dialect.VendorName(), line)
			}
			warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, fmt.Sprintf("unsupported %s route-map match statement", dialect.VendorName())))
		case dialect.SupportsRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 3 && fields[0] == "set" && fields[1] == "local-preference":
			v, delta, err := parseRouteMapInt(fields[2])
			if err != nil {
				return ParseResult{}, err
			}
			if delta {
				currentRouteRule.SetLocalPrefDelta = intPtr(v)
			} else {
				currentRouteRule.SetLocalPref = intPtr(v)
			}
		case dialect.SupportsRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 3 && fields[0] == "set" && fields[1] == "metric":
			v, delta, err := parseRouteMapInt(fields[2])
			if err != nil {
				return ParseResult{}, err
			}
			if delta {
				currentRouteRule.SetMEDDelta = intPtr(v)
			} else {
				currentRouteRule.SetMED = intPtr(v)
			}
		case dialect.SupportsAdvancedRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 4 && fields[0] == "set" && fields[1] == "as-path" && fields[2] == "prepend":
			path, err := parseASPathFields(fields[3:])
			if err != nil {
				return ParseResult{}, err
			}
			currentRouteRule.SetASPathPrepend = path
		case dialect.SupportsAdvancedRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 3 && fields[0] == "set" && fields[1] == "community":
			communities := append([]string(nil), fields[2:]...)
			if len(communities) > 0 && communities[len(communities)-1] == "additive" {
				currentRouteRule.SetCommunityAdditive = true
				communities = communities[:len(communities)-1]
			}
			currentRouteRule.SetCommunities = communities
		case dialect.SupportsAdvancedRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 3 && fields[0] == "set" && fields[1] == "origin":
			switch fields[2] {
			case "igp", "egp", "incomplete":
				currentRouteRule.SetOriginCode = model.NormalizeBGPOriginCode(model.BGPOriginCode(fields[2]))
			default:
				if !collectWarnings {
					return ParseResult{}, fmt.Errorf("unsupported %s route-map origin %q", dialect.VendorName(), line)
				}
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, fmt.Sprintf("unsupported %s route-map origin", dialect.VendorName())))
			}
		case dialect.SupportsAdvancedRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 4 && fields[0] == "set" && fields[1] == "ip" && fields[2] == "next-hop":
			if _, err := netip.ParseAddr(fields[3]); err != nil {
				if !collectWarnings {
					return ParseResult{}, fmt.Errorf("unsupported %s route-map next-hop %q", dialect.VendorName(), line)
				}
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, fmt.Sprintf("unsupported %s route-map next-hop", dialect.VendorName())))
				continue
			}
			currentRouteRule.SetNextHop = fields[3]
		case dialect.SupportsRouteMapPolicy() && currentRouteRule != nil && len(fields) >= 1 && (fields[0] == "set" || fields[0] == "call" || fields[0] == "continue" || fields[0] == "on-match"):
			if !collectWarnings {
				return ParseResult{}, fmt.Errorf("unsupported %s route-map statement %q", dialect.VendorName(), line)
			}
			warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, fmt.Sprintf("unsupported %s route-map statement", dialect.VendorName())))
		case dialect.SupportsRouteMapPolicy() && currentRoutePolicy != nil:
			if collectWarnings {
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, fmt.Sprintf("unsupported %s route-map statement", dialect.VendorName())))
			}
		case fields[0] == "interface" && len(fields) >= 2:
			currentInterface = fields[1]
			if dialect.SupportsVRF() && len(fields) >= 4 && fields[2] == "vrf" {
				cfg.Interfaces = upsertInterface(cfg.Interfaces, model.Interface{Name: currentInterface, VRF: model.NormalizeNetworkInstance(fields[3])})
			}
			currentACL = ""
			inBGP = false
			inAF = false
			inOSPF = false
		case currentInterface != "" && len(fields) >= 3 && fields[0] == "ip" && fields[1] == "address":
			addr := fields[2]
			cfg.Interfaces = upsertInterface(cfg.Interfaces, model.Interface{Name: currentInterface, Address: addr})
			if strings.EqualFold(currentInterface, "lo") || strings.HasPrefix(strings.ToLower(currentInterface), "loopback") {
				cfg.Loopback = addr
			}
		case dialect.SupportsVRF() && currentInterface != "" && len(fields) >= 2 && fields[0] == "vrf":
			if vrf, ok := dialect.InterfaceVRF(fields); ok {
				cfg.Interfaces = upsertInterface(cfg.Interfaces, model.Interface{Name: currentInterface, VRF: vrf})
			}
		case currentInterface != "" && len(fields) >= 4 && fields[0] == "ip" && fields[1] == "access-group":
			stage, ok := aclStage(fields[3])
			if ok {
				aclBindings = append(aclBindings, aclBinding{Name: fields[2], Interface: currentInterface, Stage: stage, Source: model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: line}})
			}
		case dialect.SupportsOSPFConfig() && currentInterface != "" && len(fields) >= 4 && fields[0] == "ip" && fields[1] == "ospf" && fields[2] == "area":
			vrf := dialect.OSPFInterfaceVRF(cfg.Interfaces, currentInterface)
			ospf := ospfProcess(&cfg, vrf)
			oi := ospfInterface(ospf, currentInterface)
			oi.Area = normalizeOSPFAreaID(fields[3])
			oi.Source = model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: line}
			ospf.Interfaces[currentInterface] = *oi
			ospf.Enabled = true
		case dialect.SupportsOSPFConfig() && currentInterface != "" && len(fields) >= 4 && fields[0] == "ip" && fields[1] == "ospf" && fields[2] == "cost":
			cost, err := strconv.Atoi(fields[3])
			if err != nil || cost <= 0 {
				if !collectWarnings {
					return ParseResult{}, fmt.Errorf("unsupported %s OSPF interface cost %q", dialect.VendorName(), line)
				}
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, fmt.Sprintf("unsupported %s OSPF interface cost", dialect.VendorName())))
				continue
			}
			vrf := dialect.OSPFInterfaceVRF(cfg.Interfaces, currentInterface)
			ospf := ospfProcess(&cfg, vrf)
			oi := ospfInterface(ospf, currentInterface)
			oi.Cost = cost
			oi.Source = model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: line}
			ospf.Interfaces[currentInterface] = *oi
			ospf.Enabled = true
		case dialect.SupportsOSPFConfig() && currentInterface != "" && len(fields) >= 4 && fields[0] == "ip" && fields[1] == "ospf" && fields[2] == "network" && isSupportedOSPFNetworkType(fields[3]):
			vrf := dialect.OSPFInterfaceVRF(cfg.Interfaces, currentInterface)
			ospf := ospfProcess(&cfg, vrf)
			oi := ospfInterface(ospf, currentInterface)
			oi.NetworkType = normalizeOSPFNetworkType(fields[3])
			oi.Source = model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: line}
			ospf.Interfaces[currentInterface] = *oi
			ospf.Enabled = true
		case dialect.SupportsOSPFConfig() && currentInterface != "" && len(fields) >= 4 && fields[0] == "ip" && fields[1] == "ospf" && (fields[2] == "hello-interval" || fields[2] == "dead-interval"):
			ospfProcess(&cfg, dialect.OSPFInterfaceVRF(cfg.Interfaces, currentInterface)).Enabled = true
		case dialect.SupportsOSPFConfig() && currentInterface != "" && len(fields) >= 3 && fields[0] == "ip" && fields[1] == "ospf" && fields[2] == "mtu-ignore":
			ospfProcess(&cfg, dialect.OSPFInterfaceVRF(cfg.Interfaces, currentInterface)).Enabled = true
		case dialect.SupportsOSPFConfig() && currentInterface != "" && len(fields) >= 3 && fields[0] == "ip" && fields[1] == "ospf":
			if !collectWarnings {
				return ParseResult{}, fmt.Errorf("unsupported %s OSPF interface statement %q", dialect.VendorName(), line)
			}
			warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, fmt.Sprintf("unsupported %s OSPF interface statement", dialect.VendorName())))
		case len(fields) >= 4 && fields[0] == "ip" && fields[1] == "route":
			route, err := parseFRRLikeStaticRoute(kind, path, lineNo, line, fields)
			if err != nil {
				if !collectWarnings {
					return ParseResult{}, err
				}
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, err.Error()))
				continue
			}
			cfg.Routes = append(cfg.Routes, route)
		case len(fields) >= 3 && fields[0] == "router" && fields[1] == "bgp":
			asn, err := strconv.ParseUint(fields[2], 10, 32)
			if err != nil {
				return ParseResult{}, err
			}
			cfg.ASN = uint32(asn)
			bgpVRF = model.NetworkInstanceDefault
			if len(fields) >= 5 && fields[3] == "vrf" {
				bgpVRF = model.NormalizeNetworkInstance(fields[4])
			}
			inBGP = true
			inAF = false
			inOSPF = false
			currentInterface = ""
		case dialect.SupportsOSPFConfig() && len(fields) >= 2 && fields[0] == "router" && fields[1] == "ospf":
			currentOSPFVRF = parseFRRLikeOSPFVRF(fields)
			ospf := ospfProcess(&cfg, currentOSPFVRF)
			ospf.Enabled = true
			inOSPF = true
			inBGP = false
			inAF = false
			currentInterface = ""
		case dialect.SupportsOSPFConfig() && inOSPF && len(fields) >= 3 && fields[0] == "ospf" && fields[1] == "router-id":
			ospfProcess(&cfg, currentOSPFVRF).RouterID = fields[2]
		case dialect.SupportsOSPFConfig() && inOSPF && len(fields) >= 2 && fields[0] == "router-id":
			ospfProcess(&cfg, currentOSPFVRF).RouterID = fields[1]
		case dialect.SupportsOSPFConfig() && inOSPF && len(fields) >= 4 && fields[0] == "network" && fields[2] == "area":
			prefix, err := model.ParsePrefix(fields[1])
			if err != nil {
				if !collectWarnings {
					return ParseResult{}, fmt.Errorf("unsupported %s OSPF network %q", dialect.VendorName(), line)
				}
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, fmt.Sprintf("unsupported %s OSPF network", dialect.VendorName())))
				continue
			}
			ospf := ospfProcess(&cfg, currentOSPFVRF)
			ospf.Networks = append(ospf.Networks, model.OSPFNetwork{Prefix: prefix, Area: normalizeOSPFAreaID(fields[3]), Source: model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: line}})
		case dialect.SupportsOSPFConfig() && inOSPF && len(fields) >= 3 && fields[0] == "area":
			area, err := parseFRRLikeOSPFArea(kind, path, lineNo, line, fields)
			if err != nil {
				if !collectWarnings {
					return ParseResult{}, err
				}
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, err.Error()))
				continue
			}
			ospf := ospfProcess(&cfg, currentOSPFVRF)
			ospf.Areas[area.ID] = area
		case dialect.SupportsOSPFConfig() && inOSPF && len(fields) >= 2 && fields[0] == "passive-interface":
			ospf := ospfProcess(&cfg, currentOSPFVRF)
			ospf.PassiveInterfaces = appendUnique(ospf.PassiveInterfaces, fields[1])
			oi := ospfInterface(ospf, fields[1])
			oi.Passive = true
			oi.Source = model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: line}
			ospf.Interfaces[fields[1]] = *oi
		case dialect.SupportsOSPFConfig() && inOSPF && len(fields) >= 1 && fields[0] == "redistribute":
			redist, err := parseFRRLikeOSPFRedistribution(kind, path, lineNo, line, fields)
			if err != nil {
				if !collectWarnings {
					return ParseResult{}, err
				}
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, err.Error()))
				continue
			}
			ospf := ospfProcess(&cfg, currentOSPFVRF)
			ospf.Redistribute = append(ospf.Redistribute, redist)
		case dialect.SupportsOSPFConfig() && inOSPF:
			if !collectWarnings {
				return ParseResult{}, fmt.Errorf("unsupported %s OSPF statement %q", dialect.VendorName(), line)
			}
			warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, fmt.Sprintf("unsupported %s OSPF statement", dialect.VendorName())))
		case inBGP && len(fields) >= 3 && (fields[0] == "bgp" || fields[0] == "router-id") && fields[len(fields)-2] == "router-id":
			cfg.RouterID = fields[len(fields)-1]
		case inBGP && len(fields) >= 2 && fields[0] == "router-id":
			cfg.RouterID = fields[1]
		case inBGP && len(fields) >= 2 && fields[0] == "address-family":
			inAF = true
		case inBGP && fields[0] == "exit-address-family":
			inAF = false
		case inBGP && len(fields) >= 4 && fields[0] == "neighbor" && fields[2] == "remote-as":
			asn, err := strconv.ParseUint(fields[3], 10, 32)
			if err != nil {
				return ParseResult{}, err
			}
			n := getNeighbor(neighbors, bgpVRF, fields[1])
			n.RemoteAS = uint32(asn)
		case inBGP && inAF && len(fields) >= 2 && fields[0] == "network":
			if bgpVRF == model.NetworkInstanceDefault {
				cfg.Prefixes = appendUnique(cfg.Prefixes, fields[1])
				continue
			}
			prefix, err := model.ParsePrefix(fields[1])
			if err != nil {
				return ParseResult{}, err
			}
			cfg.Routes = append(cfg.Routes, model.ConfiguredRoute{
				NetworkInstance: bgpVRF,
				AFI:             model.AFIIPv4,
				Prefix:          prefix,
				Kind:            model.RouteSourceBGP,
				AdminDistance:   200,
				Source:          model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: line},
			})
		case inBGP && inAF && len(fields) >= 2 && fields[0] == "aggregate-address":
			route, err := parseAggregateRoute(kind, path, lineNo, line, fields)
			if err != nil {
				if !collectWarnings {
					return ParseResult{}, err
				}
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, err.Error()))
				continue
			}
			route.NetworkInstance = bgpVRF
			cfg.Routes = append(cfg.Routes, route)
		case inBGP && inAF && len(fields) >= 2 && fields[0] == "redistribute":
			redist, err := parseFRRLikeRedistribution(kind, path, lineNo, line, fields)
			if err != nil {
				if !collectWarnings {
					return ParseResult{}, err
				}
				warnings = append(warnings, unsupportedStatement(string(kind), path, lineNo, line, err.Error()))
				continue
			}
			redist.NetworkInstance = bgpVRF
			cfg.Redistribute = append(cfg.Redistribute, redist)
		case inBGP && inAF && len(fields) >= 3 && fields[0] == "neighbor" && fields[2] == "activate":
			getNeighbor(neighbors, bgpVRF, fields[1]).Activated = true
		case inBGP && inAF && len(fields) >= 3 && fields[0] == "neighbor" && fields[2] == "next-hop-self":
			getNeighbor(neighbors, bgpVRF, fields[1]).NextHopSelf = true
		case dialect.SupportsRouteMapPolicy() && inBGP && inAF && len(fields) >= 5 && fields[0] == "neighbor" && fields[2] == "route-map":
			n := getNeighbor(neighbors, bgpVRF, fields[1])
			switch fields[4] {
			case "in":
				n.ImportPolicy = fields[3]
			case "out":
				n.ExportPolicy = fields[3]
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return ParseResult{}, err
	}
	for _, n := range neighbors {
		if n.Activated {
			cfg.Neighbors = append(cfg.Neighbors, *n)
		}
	}
	cfg.PrefixLists = sortedPrefixLists(prefixLists)
	cfg.ASPathLists = sortedASPathLists(asPathLists)
	cfg.CommunityLists = sortedCommunityLists(communityLists)
	cfg.RoutePolicies = sortedRoutePolicies(routePolicies)
	cfg.ACLs = normalizedACLs(kind, aclPolicies, dialect.DefaultACLAction(model.ACLDefaultDeny))
	cfg.ACLBindings = normalizedACLBindings(aclBindings)
	cfg.OSPFProcesses = compactOSPFProcesses(cfg.OSPFProcesses)
	if cfg.Loopback == "" && cfg.RouterID != "" {
		cfg.Loopback = cfg.RouterID + "/32"
	}
	return ParseResult{Config: cfg, Warnings: warnings}, nil
}

func parseSRLinuxConfig(path, text string, collectWarnings bool) (ParseResult, error) {
	var cfg ParsedConfig
	var warnings []UnsupportedStatement
	groupAS := map[string]uint32{}
	groupImportPolicy := map[string]string{}
	groupExportPolicy := map[string]string{}
	groupNextHopSelf := map[string]bool{}
	neighborGroup := map[string]string{}
	neighborImportPolicy := map[string]string{}
	neighborExportPolicy := map[string]string{}
	neighborNextHopSelf := map[string]bool{}
	staticNextHopGroups := map[string]string{}
	prefixLists := map[string]*model.PrefixList{}
	routePolicies := map[string]*model.RoutePolicy{}
	srlACLs := map[string]map[int]*parsedACLRule{}
	var aclBindings []aclBinding
	scanner := bufio.NewScanner(strings.NewReader(text))
	lineNo := 0
	for scanner.Scan() {
		lineNo++
		raw := scanner.Text()
		line := strings.TrimSpace(raw)
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != "set" {
			continue
		}
		switch {
		case containsSeq(fields, "acl", "interface") && containsAnyField(fields, "input", "output") && containsAnyField(fields, "acl-filter"):
			binding, ok := parseSRLinuxACLBinding(path, lineNo, line, fields)
			if ok {
				aclBindings = append(aclBindings, binding)
			}
		case containsSeq(fields, "acl", "acl-filter"):
			if err := parseSRLinuxACL(srlACLs, path, lineNo, line, fields); err != nil {
				if !collectWarnings {
					return ParseResult{}, fmt.Errorf("%s: %w", line, err)
				}
				warnings = append(warnings, unsupportedStatement("srlinux", path, lineNo, line, err.Error()))
			}
		case srLinuxRoutingPolicyKind(fields) == "prefix-set":
			if err := parseSRLinuxPrefixSet(prefixLists, fields); err != nil {
				if !collectWarnings {
					return ParseResult{}, fmt.Errorf("%s: %w", line, err)
				}
				warnings = append(warnings, unsupportedStatement("srlinux", path, lineNo, line, err.Error()))
			}
		case srLinuxRoutingPolicyKind(fields) == "policy":
			if err := parseSRLinuxRoutePolicy(routePolicies, prefixLists, fields); err != nil {
				if !collectWarnings {
					return ParseResult{}, fmt.Errorf("%s: %w", line, err)
				}
				warnings = append(warnings, unsupportedStatement("srlinux", path, lineNo, line, err.Error()))
			}
		case containsSeq(fields, "system", "name", "host-name") && len(fields) > 0:
			cfg.Hostname = fields[len(fields)-1]
		case containsSeq(fields, "network-instance") && containsSeq(fields, "interface") && !containsSeq(fields, "protocols"):
			ni := model.NormalizeNetworkInstance(fieldAfter(fields, "network-instance"))
			iface := srlinuxConfigInterfaceName(fieldAfter(fields, "interface"))
			if iface != "" {
				cfg.Interfaces = upsertInterface(cfg.Interfaces, model.Interface{Name: iface, VRF: ni})
			}
		case containsSeq(fields, "interface") && containsSeq(fields, "ipv4", "address") && len(fields) > 0:
			iface := fieldAfter(fields, "interface")
			addr := fields[len(fields)-1]
			cfg.Interfaces = upsertInterface(cfg.Interfaces, model.Interface{Name: iface, Address: addr})
			if strings.HasPrefix(strings.ToLower(iface), "lo") {
				cfg.Loopback = addr
			}
		case containsSeq(fields, "next-hop-groups", "group") && containsSeq(fields, "nexthop") && containsSeq(fields, "ip-address"):
			ni := fieldAfter(fields, "network-instance")
			group := fieldAfter(fields, "group")
			addr := fieldAfter(fields, "ip-address")
			if ni != "" && group != "" {
				staticNextHopGroups[srlinuxNextHopGroupKey(ni, group)] = addr
			}
		case containsSeq(fields, "static-routes", "route"):
			route, err := parseSRLinuxStaticRoute(path, lineNo, line, fields, staticNextHopGroups)
			if err != nil {
				if !collectWarnings {
					return ParseResult{}, fmt.Errorf("%s: %w", line, err)
				}
				warnings = append(warnings, unsupportedStatement("srlinux", path, lineNo, line, err.Error()))
				continue
			}
			cfg.Routes = append(cfg.Routes, route)
		case containsSeq(fields, "protocols", "ospf"):
			if err := parseSRLinuxOSPF(&cfg, path, lineNo, line, fields); err != nil {
				if !collectWarnings {
					return ParseResult{}, fmt.Errorf("%s: %w", line, err)
				}
				warnings = append(warnings, unsupportedStatement("srlinux", path, lineNo, line, err.Error()))
			}
		case containsSeq(fields, "protocols", "bgp", "autonomous-system") && len(fields) > 0:
			asn, err := strconv.ParseUint(fields[len(fields)-1], 10, 32)
			if err != nil {
				return ParseResult{}, err
			}
			cfg.ASN = uint32(asn)
		case containsSeq(fields, "protocols", "bgp", "router-id") && len(fields) > 0:
			cfg.RouterID = fields[len(fields)-1]
			cfg.Loopback = cfg.RouterID + "/32"
		case containsSeq(fields, "protocols", "bgp", "group") && containsSeq(fields, "peer-as"):
			group := fieldAfter(fields, "group")
			asn, err := strconv.ParseUint(fields[len(fields)-1], 10, 32)
			if err != nil {
				return ParseResult{}, err
			}
			groupAS[group] = uint32(asn)
		case containsSeq(fields, "protocols", "bgp", "group") && containsAnyField(fields, "import-policy", "export-policy"):
			group := fieldAfter(fields, "group")
			policy, err := parseSRLinuxPolicyBinding(fields)
			if err != nil {
				if !collectWarnings {
					return ParseResult{}, fmt.Errorf("%s: %w", line, err)
				}
				warnings = append(warnings, unsupportedStatement("srlinux", path, lineNo, line, err.Error()))
				continue
			}
			if containsAnyField(fields, "import-policy") {
				groupImportPolicy[group] = policy
			} else {
				groupExportPolicy[group] = policy
			}
		case containsSeq(fields, "protocols", "bgp", "group") && containsSeq(fields, "next-hop-self"):
			group := fieldAfter(fields, "group")
			groupNextHopSelf[group] = true
		case containsSeq(fields, "protocols", "bgp", "neighbor") && containsSeq(fields, "peer-group"):
			addr := fieldAfter(fields, "neighbor")
			neighborGroup[addr] = fields[len(fields)-1]
		case containsSeq(fields, "protocols", "bgp", "neighbor") && containsAnyField(fields, "import-policy", "export-policy"):
			addr := fieldAfter(fields, "neighbor")
			policy, err := parseSRLinuxPolicyBinding(fields)
			if err != nil {
				if !collectWarnings {
					return ParseResult{}, fmt.Errorf("%s: %w", line, err)
				}
				warnings = append(warnings, unsupportedStatement("srlinux", path, lineNo, line, err.Error()))
				continue
			}
			if containsAnyField(fields, "import-policy") {
				neighborImportPolicy[addr] = policy
			} else {
				neighborExportPolicy[addr] = policy
			}
		case containsSeq(fields, "protocols", "bgp", "neighbor") && containsSeq(fields, "next-hop-self"):
			addr := fieldAfter(fields, "neighbor")
			neighborNextHopSelf[addr] = true
		case containsSeq(fields, "protocols", "bgp") && (containsAnyField(fields, "aggregate-address") || containsAnyField(fields, "aggregate-routes")):
			err := fmt.Errorf("unsupported SR Linux BGP aggregate route statement")
			if !collectWarnings {
				return ParseResult{}, fmt.Errorf("%s: %w", line, err)
			}
			warnings = append(warnings, unsupportedStatement("srlinux", path, lineNo, line, err.Error()))
		}
	}
	if err := scanner.Err(); err != nil {
		return ParseResult{}, err
	}
	for addr, group := range neighborGroup {
		neighbor := model.BGPNeighbor{
			Address:      addr,
			RemoteAS:     groupAS[group],
			Activated:    true,
			ImportPolicy: groupImportPolicy[group],
			ExportPolicy: groupExportPolicy[group],
			NextHopSelf:  groupNextHopSelf[group],
		}
		if policy := neighborImportPolicy[addr]; policy != "" {
			neighbor.ImportPolicy = policy
		}
		if policy := neighborExportPolicy[addr]; policy != "" {
			neighbor.ExportPolicy = policy
		}
		if neighborNextHopSelf[addr] {
			neighbor.NextHopSelf = true
		}
		cfg.Neighbors = append(cfg.Neighbors, neighbor)
	}
	addSRLinuxDefaultPolicyActions(routePolicies)
	cfg.PrefixLists = sortedPrefixLists(prefixLists)
	cfg.RoutePolicies = sortedRoutePolicies(routePolicies)
	cfg.ACLs = normalizedACLs(model.KindSRLinux, flattenSRLinuxACLs(srlACLs), model.ACLDefaultDeny)
	cfg.ACLBindings = normalizedACLBindings(aclBindings)
	return ParseResult{Config: cfg, Warnings: warnings}, nil
}

func parseFRRLikeStaticRoute(kind model.DeviceKind, path string, lineNo int, raw string, fields []string) (model.ConfiguredRoute, error) {
	vrf := model.NetworkInstanceDefault
	if len(fields) >= 5 && fields[2] == "vrf" {
		vrf = model.NormalizeNetworkInstance(fields[3])
		fields = append([]string{fields[0], fields[1]}, fields[4:]...)
	}
	if len(fields) == 6 && fields[4] == "vrf" {
		vrf = model.NormalizeNetworkInstance(fields[5])
		fields = fields[:4]
	}
	if len(fields) != 4 {
		return model.ConfiguredRoute{}, fmt.Errorf("unsupported %s static route statement", routeMapVendorName(kind))
	}
	prefix, err := model.ParsePrefix(fields[2])
	if err != nil {
		return model.ConfiguredRoute{}, err
	}
	route := model.ConfiguredRoute{
		NetworkInstance: vrf,
		AFI:             model.AFIIPv4,
		Prefix:          prefix,
		Kind:            model.RouteSourceStatic,
		AdminDistance:   1,
		Source:          model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: raw},
	}
	target := fields[3]
	if strings.EqualFold(target, "Null0") {
		route.Kind = model.RouteSourceBlackhole
		route.Interface = target
		return route, nil
	}
	if _, err := netip.ParseAddr(target); err == nil {
		route.NextHop = target
		return route, nil
	}
	route.Interface = target
	return route, nil
}

func ospfProcess(cfg *ParsedConfig, vrf model.NetworkInstanceID) *model.OSPFProcess {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	if vrf == model.NetworkInstanceDefault {
		cfg.OSPF.NetworkInstance = model.NetworkInstanceDefault
		if cfg.OSPF.Interfaces == nil {
			cfg.OSPF.Interfaces = map[string]model.OSPFInterface{}
		}
		if cfg.OSPF.Areas == nil {
			cfg.OSPF.Areas = map[string]model.OSPFArea{}
		}
		return &cfg.OSPF
	}
	for i := range cfg.OSPFProcesses {
		if model.NormalizeNetworkInstance(string(cfg.OSPFProcesses[i].NetworkInstance)) == vrf {
			if cfg.OSPFProcesses[i].Interfaces == nil {
				cfg.OSPFProcesses[i].Interfaces = map[string]model.OSPFInterface{}
			}
			if cfg.OSPFProcesses[i].Areas == nil {
				cfg.OSPFProcesses[i].Areas = map[string]model.OSPFArea{}
			}
			return &cfg.OSPFProcesses[i]
		}
	}
	cfg.OSPFProcesses = append(cfg.OSPFProcesses, model.OSPFProcess{
		NetworkInstance: vrf,
		Interfaces:      map[string]model.OSPFInterface{},
		Areas:           map[string]model.OSPFArea{},
	})
	return &cfg.OSPFProcesses[len(cfg.OSPFProcesses)-1]
}

func ospfInterface(ospf *model.OSPFProcess, name string) *model.OSPFInterface {
	if ospf.Interfaces == nil {
		ospf.Interfaces = map[string]model.OSPFInterface{}
	}
	oi := ospf.Interfaces[name]
	if oi.Name == "" {
		oi.Name = name
	}
	ospf.Interfaces[name] = oi
	return &oi
}

func parseFRRLikeOSPFVRF(fields []string) model.NetworkInstanceID {
	for i := 2; i+1 < len(fields); i++ {
		if fields[i] == "vrf" {
			return model.NormalizeNetworkInstance(fields[i+1])
		}
	}
	return model.NetworkInstanceDefault
}

func compactOSPFProcesses(processes []model.OSPFProcess) []model.OSPFProcess {
	out := processes[:0]
	for _, process := range processes {
		process.NetworkInstance = model.NormalizeNetworkInstance(string(process.NetworkInstance))
		if process.NetworkInstance == model.NetworkInstanceDefault || !process.Enabled {
			continue
		}
		out = append(out, process)
	}
	return out
}

func parseFRRLikeOSPFArea(kind model.DeviceKind, path string, lineNo int, raw string, fields []string) (model.OSPFArea, error) {
	area := model.OSPFArea{ID: normalizeOSPFAreaID(fields[1]), Source: model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: raw}}
	switch fields[2] {
	case "stub":
		area.Kind = model.OSPFAreaStub
	case "nssa":
		area.Kind = model.OSPFAreaNSSA
	default:
		return model.OSPFArea{}, fmt.Errorf("unsupported %s OSPF area statement", routeMapVendorName(kind))
	}
	for _, opt := range fields[3:] {
		switch opt {
		case "no-summary":
			area.NoSummary = true
		case "default-information-originate":
			if area.Kind != model.OSPFAreaNSSA {
				return model.OSPFArea{}, fmt.Errorf("unsupported %s OSPF area option %q", routeMapVendorName(kind), opt)
			}
			area.DefaultInformationOriginate = true
		default:
			return model.OSPFArea{}, fmt.Errorf("unsupported %s OSPF area option %q", routeMapVendorName(kind), opt)
		}
	}
	return area, nil
}

func parseFRRLikeOSPFRedistribution(kind model.DeviceKind, path string, lineNo int, raw string, fields []string) (model.OSPFRedistribution, error) {
	if len(fields) < 2 {
		return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute statement", routeMapVendorName(kind))
	}
	redist := model.OSPFRedistribution{MetricType: 2, Source: model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: raw}}
	switch fields[1] {
	case "connected":
		redist.Kind = model.RouteSourceConnected
	case "static":
		redist.Kind = model.RouteSourceStatic
	case "bgp":
		redist.Kind = model.RouteSourceBGP
	default:
		return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute source %q", routeMapVendorName(kind), fields[1])
	}
	for i := 2; i < len(fields); {
		switch fields[i] {
		case "route-map":
			if i+1 >= len(fields) {
				return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute statement", routeMapVendorName(kind))
			}
			redist.RouteMap = fields[i+1]
			i += 2
		case "metric":
			if i+1 >= len(fields) {
				return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute metric", routeMapVendorName(kind))
			}
			metric, err := strconv.Atoi(fields[i+1])
			if err != nil || metric < 0 {
				return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute metric", routeMapVendorName(kind))
			}
			redist.Metric = metric
			i += 2
		case "metric-type":
			if i+1 >= len(fields) {
				return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute metric-type", routeMapVendorName(kind))
			}
			metricType, err := strconv.Atoi(fields[i+1])
			if err != nil || (metricType != 1 && metricType != 2) {
				return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute metric-type", routeMapVendorName(kind))
			}
			redist.MetricType = metricType
			i += 2
		default:
			return model.OSPFRedistribution{}, fmt.Errorf("unsupported %s OSPF redistribute option %q", routeMapVendorName(kind), fields[i])
		}
	}
	return redist, nil
}

func parseSRLinuxOSPF(cfg *ParsedConfig, path string, lineNo int, raw string, fields []string) error {
	ospf := ospfProcess(cfg, model.NetworkInstanceDefault)
	ospf.Enabled = true
	source := model.ConfigSource{Vendor: string(model.KindSRLinux), File: path, Line: lineNo, Raw: raw}
	if containsAnyField(fields, "router-id") {
		ospf.RouterID = fields[len(fields)-1]
		return nil
	}
	if containsSeq(fields, "area", "interface") {
		iface := fieldAfter(fields, "interface")
		area := normalizeOSPFAreaID(fieldAfter(fields, "area"))
		if iface == "" || area == "" {
			return fmt.Errorf("unsupported SR Linux OSPF interface statement")
		}
		oi := ospfInterface(ospf, iface)
		oi.Area = area
		oi.Source = source
		if containsAnyField(fields, "metric") {
			cost, err := strconv.Atoi(fields[len(fields)-1])
			if err != nil || cost <= 0 {
				return fmt.Errorf("unsupported SR Linux OSPF interface metric")
			}
			oi.Cost = cost
		}
		if containsAnyField(fields, "interface-type") {
			networkType := normalizeOSPFNetworkType(fields[len(fields)-1])
			if !isSupportedOSPFNetworkType(networkType) {
				return fmt.Errorf("unsupported SR Linux OSPF interface type")
			}
			oi.NetworkType = networkType
		}
		if containsAnyField(fields, "passive") && parseConfigBool(fields[len(fields)-1]) {
			oi.Passive = true
			ospf.PassiveInterfaces = appendUnique(ospf.PassiveInterfaces, iface)
		}
		ospf.Interfaces[iface] = *oi
		return nil
	}
	if containsAnyField(fields, "admin-state", "version") {
		return nil
	}
	return fmt.Errorf("unsupported SR Linux OSPF statement")
}

func isSupportedOSPFNetworkType(raw string) bool {
	switch normalizeOSPFNetworkType(raw) {
	case "", "broadcast", "point-to-point":
		return true
	default:
		return false
	}
}

func normalizeOSPFNetworkType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "point-to-point", "p2p":
		return "point-to-point"
	case "broadcast":
		return "broadcast"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func normalizeOSPFAreaID(area string) string {
	switch strings.TrimSpace(area) {
	case "0.0.0.0":
		return "0"
	default:
		return strings.TrimSpace(area)
	}
}

func parseConfigBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "yes", "enable", "enabled":
		return true
	default:
		return false
	}
}

func parseAggregateRoute(kind model.DeviceKind, path string, lineNo int, raw string, fields []string) (model.ConfiguredRoute, error) {
	if len(fields) < 2 {
		return model.ConfiguredRoute{}, fmt.Errorf("unsupported %s aggregate-address statement", routeMapVendorName(kind))
	}
	prefixText := fields[1]
	prefix, err := model.ParsePrefix(prefixText)
	if err != nil {
		return model.ConfiguredRoute{}, err
	}
	route := model.ConfiguredRoute{
		NetworkInstance: model.NetworkInstanceDefault,
		AFI:             model.AFIIPv4,
		Prefix:          prefix,
		Kind:            model.RouteSourceAggregate,
		AdminDistance:   200,
		Source:          model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: raw},
	}
	for _, opt := range fields[2:] {
		switch opt {
		case "summary-only":
			route.SummaryOnly = true
		default:
			return model.ConfiguredRoute{}, fmt.Errorf("unsupported %s aggregate-address option %q", routeMapVendorName(kind), opt)
		}
	}
	return route, nil
}

func parseFRRLikeRedistribution(kind model.DeviceKind, path string, lineNo int, raw string, fields []string) (model.BGPRedistribution, error) {
	redist := model.BGPRedistribution{Source: model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: raw}}
	switch fields[1] {
	case "connected":
		redist.Kind = model.RouteSourceConnected
	case "static":
		redist.Kind = model.RouteSourceStatic
	default:
		return model.BGPRedistribution{}, fmt.Errorf("unsupported %s redistribute source %q", routeMapVendorName(kind), fields[1])
	}
	if len(fields) == 2 {
		return redist, nil
	}
	if len(fields) == 4 && fields[2] == "route-map" {
		redist.RouteMap = fields[3]
		return redist, nil
	}
	return model.BGPRedistribution{}, fmt.Errorf("unsupported %s redistribute statement", routeMapVendorName(kind))
}

func parseSRLinuxStaticRoute(path string, lineNo int, raw string, fields []string, nextHopGroups map[string]string) (model.ConfiguredRoute, error) {
	prefixText := fieldAfter(fields, "route")
	if prefixText == "" {
		return model.ConfiguredRoute{}, fmt.Errorf("unsupported SR Linux static route statement")
	}
	prefix, err := model.ParsePrefix(prefixText)
	if err != nil {
		return model.ConfiguredRoute{}, err
	}
	route := model.ConfiguredRoute{
		NetworkInstance: model.NormalizeNetworkInstance(fieldAfter(fields, "network-instance")),
		AFI:             model.AFIIPv4,
		Prefix:          prefix,
		Kind:            model.RouteSourceStatic,
		AdminDistance:   5,
		Source:          model.ConfigSource{Vendor: "srlinux", File: path, Line: lineNo, Raw: raw},
	}
	if nh := fieldAfter(fields, "next-hop"); nh != "" {
		if _, err := netip.ParseAddr(nh); err == nil {
			route.NextHop = nh
			return route, nil
		}
	}
	if group := fieldAfter(fields, "next-hop-group"); group != "" {
		nh := nextHopGroups[srlinuxNextHopGroupKey(string(route.NetworkInstance), group)]
		if _, err := netip.ParseAddr(nh); err == nil {
			route.NextHop = nh
			return route, nil
		}
		return model.ConfiguredRoute{}, fmt.Errorf("unsupported SR Linux static route next-hop-group")
	}
	if iface := fieldAfter(fields, "interface"); iface != "" {
		route.Interface = iface
		return route, nil
	}
	if containsAnyField(fields, "blackhole") || containsAnyField(fields, "discard") {
		route.Kind = model.RouteSourceBlackhole
		return route, nil
	}
	return model.ConfiguredRoute{}, fmt.Errorf("unsupported SR Linux static route next-hop")
}

func srlinuxNextHopGroupKey(networkInstance, group string) string {
	return string(model.NormalizeNetworkInstance(networkInstance)) + "\x00" + group
}

func srlinuxConfigInterfaceName(iface string) string {
	if base, ok := strings.CutSuffix(iface, ".0"); ok && (strings.HasPrefix(base, "ethernet-1/") || strings.HasPrefix(base, "lo")) {
		return base
	}
	return iface
}

func parseSRLinuxPrefixSet(prefixLists map[string]*model.PrefixList, fields []string) error {
	name := fieldAfter(fields, "prefix-set")
	prefix := fieldAfter(fields, "prefix")
	if name == "" || prefix == "" {
		return fmt.Errorf("unsupported SR Linux prefix-set statement")
	}
	ge, le, err := parseSRLinuxMaskLengthRange(prefix, fieldAfter(fields, "mask-length-range"))
	if err != nil {
		return err
	}
	rule, err := parsePrefixListRule(0, "permit", prefix, prefixRangeFields(ge, le))
	if err != nil {
		return err
	}
	addPrefixListRule(prefixLists, name, rule)
	return nil
}

func parseSRLinuxMaskLengthRange(prefix, raw string) (int, int, error) {
	if raw == "" || raw == "exact" {
		return 0, 0, nil
	}
	parts := strings.Split(raw, "..")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unsupported SR Linux mask-length-range %q", raw)
	}
	ge, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, err
	}
	le, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, err
	}
	parsed, err := netip.ParsePrefix(prefix)
	if err != nil {
		return 0, 0, err
	}
	if ge == parsed.Bits() {
		ge = 0
	}
	if le == parsed.Bits() {
		le = 0
	}
	return ge, le, nil
}

func prefixRangeFields(ge, le int) []string {
	var fields []string
	if ge > 0 {
		fields = append(fields, "ge", strconv.Itoa(ge))
	}
	if le > 0 {
		fields = append(fields, "le", strconv.Itoa(le))
	}
	return fields
}

const unsupportedSRLinuxPolicyPrefixList = "__unsupported_srlinux_policy_never_match__"

func parseSRLinuxRoutePolicy(routePolicies map[string]*model.RoutePolicy, prefixLists map[string]*model.PrefixList, fields []string) error {
	name := fieldAfter(fields, "policy")
	if name == "" {
		return fmt.Errorf("unsupported SR Linux routing-policy statement")
	}
	if containsSeq(fields, "default-action", "policy-result") {
		action := fields[len(fields)-1]
		if action != "accept" && action != "reject" {
			return fmt.Errorf("unsupported SR Linux routing-policy default-action %q", action)
		}
		addRoutePolicyRule(routePolicies, name, srLinuxPolicyAction(action), 65535)
		return nil
	}
	if !containsAnyField(fields, "statement") {
		return fmt.Errorf("unsupported SR Linux routing-policy statement")
	}
	seq, err := strconv.Atoi(fieldAfter(fields, "statement"))
	if err != nil {
		return err
	}
	policy, rule := ensureRoutePolicyRule(routePolicies, name, seq)
	_ = policy
	switch {
	case containsSeq(fields, "match", "prefix", "prefix-set"):
		rule.MatchPrefixList = fieldAfter(fields, "prefix-set")
	case containsSeq(fields, "action", "policy-result"):
		action := fields[len(fields)-1]
		if action != "accept" && action != "reject" {
			return fmt.Errorf("unsupported SR Linux routing-policy action %q", action)
		}
		rule.Action = srLinuxPolicyAction(action)
	case containsSeq(fields, "action") && fields[len(fields)-1] == "accept":
		rule.Action = "permit"
	case containsSeq(fields, "action") && fields[len(fields)-1] == "reject":
		rule.Action = "deny"
	case containsSeq(fields, "action", "bgp", "local-preference", "set"):
		v, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil {
			return err
		}
		rule.SetLocalPref = intPtr(v)
	case containsSeq(fields, "action", "bgp", "med", "set") ||
		containsSeq(fields, "action", "bgp", "med", "operation", "set") ||
		containsSeq(fields, "action", "bgp", "metric", "set") ||
		containsSeq(fields, "action", "bgp", "metric", "operation", "set"):
		v, err := strconv.Atoi(fields[len(fields)-1])
		if err != nil {
			return err
		}
		rule.SetMED = intPtr(v)
	case containsSeq(fields, "action", "bgp", "next-hop", "set") && strings.EqualFold(fields[len(fields)-1], "self"):
		rule.SetNextHopSelf = true
	default:
		markUnsupportedSRLinuxRoutePolicyRule(prefixLists, rule)
		return fmt.Errorf("unsupported SR Linux routing-policy statement")
	}
	return nil
}

func markUnsupportedSRLinuxRoutePolicyRule(prefixLists map[string]*model.PrefixList, rule *model.RoutePolicyRule) {
	if prefixLists[unsupportedSRLinuxPolicyPrefixList] == nil {
		denyAny, err := parsePrefixListRule(0, "deny", "any", nil)
		if err == nil {
			prefixLists[unsupportedSRLinuxPolicyPrefixList] = &model.PrefixList{Name: unsupportedSRLinuxPolicyPrefixList, Rules: []model.PrefixListRule{denyAny}}
		}
	}
	rule.MatchPrefixList = unsupportedSRLinuxPolicyPrefixList
}

func addSRLinuxDefaultPolicyActions(routePolicies map[string]*model.RoutePolicy) {
	for _, policy := range routePolicies {
		hasDefault := false
		for _, rule := range policy.Rules {
			if rule.Seq == 65535 {
				hasDefault = true
				break
			}
		}
		if !hasDefault {
			policy.Rules = append(policy.Rules, model.RoutePolicyRule{Seq: 65535, Action: "permit"})
		}
	}
}

func ensureRoutePolicyRule(routePolicies map[string]*model.RoutePolicy, name string, seq int) (*model.RoutePolicy, *model.RoutePolicyRule) {
	if routePolicies[name] == nil {
		routePolicies[name] = &model.RoutePolicy{Name: name}
	}
	policy := routePolicies[name]
	for i := range policy.Rules {
		if policy.Rules[i].Seq == seq {
			return policy, &policy.Rules[i]
		}
	}
	policy.Rules = append(policy.Rules, model.RoutePolicyRule{Seq: seq, Action: "deny"})
	return policy, &policy.Rules[len(policy.Rules)-1]
}

func srLinuxPolicyAction(action string) string {
	if action == "reject" {
		return "deny"
	}
	return "permit"
}

func parseSRLinuxPolicyBinding(fields []string) (string, error) {
	for i, field := range fields {
		if field != "import-policy" && field != "export-policy" {
			continue
		}
		policies := fields[i+1:]
		if len(policies) == 0 {
			return "", fmt.Errorf("unsupported SR Linux empty BGP policy binding")
		}
		if policies[0] == "[" {
			policies = policies[1:]
			if len(policies) == 0 {
				return "", fmt.Errorf("unsupported SR Linux empty BGP policy binding")
			}
			if len(policies) < 2 || policies[1] != "]" {
				return "", fmt.Errorf("unsupported SR Linux multiple BGP policy binding")
			}
			return policies[0], nil
		}
		if len(policies) > 1 {
			return "", fmt.Errorf("unsupported SR Linux multiple BGP policy binding")
		}
		return policies[0], nil
	}
	return "", fmt.Errorf("unsupported SR Linux BGP policy binding")
}

func unsupportedStatement(vendor, file string, line int, text, reason string) UnsupportedStatement {
	return UnsupportedStatement{
		Vendor: vendor,
		File:   file,
		Line:   line,
		Text:   text,
		Reason: reason,
	}
}

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

func srLinuxRoutingPolicyKind(fields []string) string {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == "routing-policy" {
			return fields[i+1]
		}
	}
	return ""
}

func routeMapVendorName(kind model.DeviceKind) string {
	return model.ProfileFor(kind).ConfigProfile().RouteMapVendorName()
}

func getNeighbor(neighbors map[string]*model.BGPNeighbor, vrf model.NetworkInstanceID, addr string) *model.BGPNeighbor {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	key := string(vrf) + "|" + addr
	if neighbors[key] == nil {
		neighbors[key] = &model.BGPNeighbor{NetworkInstance: vrf, Address: addr}
	}
	return neighbors[key]
}

func parsePrefixListRule(seq int, action, prefix string, fields []string) (model.PrefixListRule, error) {
	rule := model.PrefixListRule{Seq: seq, Action: action, Prefix: prefix}
	for i := 0; i < len(fields); i += 2 {
		if i+1 >= len(fields) {
			return model.PrefixListRule{}, fmt.Errorf("invalid prefix-list range")
		}
		v, err := strconv.Atoi(fields[i+1])
		if err != nil {
			return model.PrefixListRule{}, err
		}
		switch fields[i] {
		case "ge":
			rule.Ge = v
		case "le":
			rule.Le = v
		default:
			return model.PrefixListRule{}, fmt.Errorf("unsupported prefix-list option %q", fields[i])
		}
	}
	match, err := model.NewPrefixSet(rule.Prefix, rule.Ge, rule.Le)
	if err != nil {
		return model.PrefixListRule{}, err
	}
	rule.Match = match
	return rule, nil
}

func parseRouteMapInt(raw string) (int, bool, error) {
	delta := strings.HasPrefix(raw, "+") || strings.HasPrefix(raw, "-")
	v, err := strconv.Atoi(raw)
	return v, delta, err
}

func parseASPathFields(fields []string) ([]uint32, error) {
	var out []uint32
	for _, field := range fields {
		asn, err := strconv.ParseUint(field, 10, 32)
		if err != nil {
			return nil, err
		}
		out = append(out, uint32(asn))
	}
	return out, nil
}

func addPrefixListRule(prefixLists map[string]*model.PrefixList, name string, rule model.PrefixListRule) {
	if prefixLists[name] == nil {
		prefixLists[name] = &model.PrefixList{Name: name}
	}
	prefixLists[name].Rules = append(prefixLists[name].Rules, rule)
}

func addStringListRule(asPathLists map[string]*model.ASPathList, name string, rule model.StringListRule) {
	if asPathLists[name] == nil {
		asPathLists[name] = &model.ASPathList{Name: name}
	}
	asPathLists[name].Rules = append(asPathLists[name].Rules, rule)
}

func addCommunityListRule(communityLists map[string]*model.CommunityList, name string, rule model.StringListRule) {
	if communityLists[name] == nil {
		communityLists[name] = &model.CommunityList{Name: name}
	}
	communityLists[name].Rules = append(communityLists[name].Rules, rule)
}

func addRoutePolicyRule(routePolicies map[string]*model.RoutePolicy, name string, action string, seq int) (*model.RoutePolicy, *model.RoutePolicyRule) {
	if routePolicies[name] == nil {
		routePolicies[name] = &model.RoutePolicy{Name: name}
	}
	routePolicies[name].Rules = append(routePolicies[name].Rules, model.RoutePolicyRule{Seq: seq, Action: action})
	policy := routePolicies[name]
	return policy, &policy.Rules[len(policy.Rules)-1]
}

func sortedPrefixLists(prefixLists map[string]*model.PrefixList) []model.PrefixList {
	var out []model.PrefixList
	for _, prefixList := range prefixLists {
		cp := *prefixList
		cp.Rules = append([]model.PrefixListRule(nil), prefixList.Rules...)
		sort.Slice(cp.Rules, func(i, j int) bool {
			return cp.Rules[i].Seq < cp.Rules[j].Seq
		})
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func sortedASPathLists(asPathLists map[string]*model.ASPathList) []model.ASPathList {
	var out []model.ASPathList
	for _, list := range asPathLists {
		cp := *list
		cp.Rules = append([]model.StringListRule(nil), list.Rules...)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func sortedCommunityLists(communityLists map[string]*model.CommunityList) []model.CommunityList {
	var out []model.CommunityList
	for _, list := range communityLists {
		cp := *list
		cp.Rules = append([]model.StringListRule(nil), list.Rules...)
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func sortedRoutePolicies(routePolicies map[string]*model.RoutePolicy) []model.RoutePolicy {
	var out []model.RoutePolicy
	for _, routePolicy := range routePolicies {
		cp := *routePolicy
		cp.Rules = append([]model.RoutePolicyRule(nil), routePolicy.Rules...)
		sort.Slice(cp.Rules, func(i, j int) bool {
			return cp.Rules[i].Seq < cp.Rules[j].Seq
		})
		out = append(out, cp)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func intPtr(v int) *int {
	return &v
}

func upsertInterface(xs []model.Interface, iface model.Interface) []model.Interface {
	for i := range xs {
		if xs[i].Name == iface.Name {
			if iface.Address != "" {
				xs[i].Address = iface.Address
			}
			if iface.VRF != "" {
				xs[i].VRF = iface.VRF
			}
			return xs
		}
	}
	return append(xs, iface)
}

func appendUnique(xs []string, x string) []string {
	for _, existing := range xs {
		if existing == x {
			return xs
		}
	}
	return append(xs, x)
}

func containsSeq(fields []string, seq ...string) bool {
	pos := 0
	for _, f := range fields {
		if f == seq[pos] {
			pos++
			if pos == len(seq) {
				return true
			}
		}
	}
	return false
}

func containsAnyField(fields []string, matches ...string) bool {
	for _, field := range fields {
		for _, match := range matches {
			if field == match {
				return true
			}
		}
	}
	return false
}

func fieldAfter(fields []string, marker string) string {
	for i := 0; i+1 < len(fields); i++ {
		if fields[i] == marker {
			return strings.Trim(fields[i+1], "[]")
		}
	}
	return ""
}
