package configparse

import (
	"fmt"
	"os"
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
	p := newFRRLikeParser(frrDialect{}, path, text, collectWarnings)
	return p.parse()
}

func (CEOSParser) Parse(path, text string, collectWarnings bool) (ParseResult, error) {
	p := newFRRLikeParser(ceosDialect{}, path, text, collectWarnings)
	return p.parse()
}

func (SRLinuxParser) Parse(path, text string, collectWarnings bool) (ParseResult, error) {
	p := newSRLinuxParser(path, text, collectWarnings)
	return p.parse()
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
