package configparse

import (
	"bufio"
	"fmt"
	"net/netip"
	"os"
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
