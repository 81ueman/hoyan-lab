package topology

import (
	"fmt"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// LabFile is the topology input DTO consumed by the topology usecase.
// It intentionally contains only fields needed by this package, independent of
// any adapter-specific containerlab YAML representation.
type LabFile struct {
	Name             string
	Prefix           *string
	ManagementSubnet string
	Nodes            map[string]LabNode
	Links            []LabLink
}

type LabNode struct {
	Kind          string
	Group         string
	NetworkMode   string
	MgmtIPv4      string
	Binds         []string
	StartupConfig string
}

type LabLink struct {
	Endpoints []string
}

// ParsedConfig is the neutral parsed configuration DTO consumed by node assembly.
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

type ParseOptions struct {
	CollectWarnings bool
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

type LabFileLoader interface {
	Load(path string) (LabFile, error)
}

type ConfigParser interface {
	Parse(kind model.DeviceKind, path string, opts ParseOptions) (ParseResult, error)
	ParseNftablesACL(path string) ([]model.ACL, []model.ACLBinding, error)
}

type Builder struct {
	loader LabFileLoader
	parser ConfigParser
}

func NewBuilder(loader LabFileLoader, parser ConfigParser) Builder {
	return Builder{loader: loader, parser: parser}
}
