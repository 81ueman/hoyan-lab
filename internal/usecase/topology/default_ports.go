package topology

import (
	"github.com/81ueman/hoyan-lab/internal/adapter/configparse"
	"github.com/81ueman/hoyan-lab/internal/adapter/labfile"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type labFileLoader struct{}

func (labFileLoader) Load(path string) (LabFile, error) {
	raw, err := labfile.Load(path)
	if err != nil {
		return LabFile{}, err
	}
	return labFileFromAdapter(raw), nil
}

type configParser struct{}

func (configParser) Parse(kind model.DeviceKind, path string, opts ParseOptions) (ParseResult, error) {
	result, err := configparse.ParseConfigWithOptions(kind, path, configparse.ParseOptions{CollectWarnings: opts.CollectWarnings})
	if err != nil {
		return ParseResult{}, err
	}
	return parseResultFromAdapter(result), nil
}

func (configParser) ParseNftablesACL(path string) ([]model.ACL, []model.ACLBinding, error) {
	return configparse.ParseNftablesACLConfig(path)
}

func defaultBuilder() Builder {
	return NewBuilder(labFileLoader{}, configParser{})
}

func labFileFromAdapter(raw labfile.File) LabFile {
	out := LabFile{
		Name:             raw.Name,
		Prefix:           raw.Prefix,
		ManagementSubnet: raw.Mgmt.IPv4Subnet,
		Nodes:            make(map[string]LabNode, len(raw.Topology.Nodes)),
		Links:            make([]LabLink, 0, len(raw.Topology.Links)),
	}
	for name, node := range raw.Topology.Nodes {
		out.Nodes[name] = LabNode{
			Kind:          node.Kind,
			Group:         node.Group,
			NetworkMode:   node.NetworkMode,
			MgmtIPv4:      node.MgmtIPv4,
			Binds:         node.Binds,
			StartupConfig: node.StartupConfig,
		}
	}
	for _, link := range raw.Topology.Links {
		out.Links = append(out.Links, LabLink{Endpoints: link.Endpoints})
	}
	return out
}

func parseResultFromAdapter(result configparse.ParseResult) ParseResult {
	warnings := make([]UnsupportedStatement, 0, len(result.Warnings))
	for _, warning := range result.Warnings {
		warnings = append(warnings, UnsupportedStatement{
			Vendor: warning.Vendor,
			File:   warning.File,
			Line:   warning.Line,
			Text:   warning.Text,
			Reason: warning.Reason,
		})
	}
	return ParseResult{
		Config: ParsedConfig{
			Hostname:       result.Config.Hostname,
			ASN:            result.Config.ASN,
			RouterID:       result.Config.RouterID,
			Loopback:       result.Config.Loopback,
			Interfaces:     result.Config.Interfaces,
			Prefixes:       result.Config.Prefixes,
			Routes:         result.Config.Routes,
			Redistribute:   result.Config.Redistribute,
			Neighbors:      result.Config.Neighbors,
			PrefixLists:    result.Config.PrefixLists,
			ASPathLists:    result.Config.ASPathLists,
			CommunityLists: result.Config.CommunityLists,
			RoutePolicies:  result.Config.RoutePolicies,
			ACLs:           result.Config.ACLs,
			ACLBindings:    result.Config.ACLBindings,
			OSPF:           result.Config.OSPF,
			OSPFProcesses:  result.Config.OSPFProcesses,
		},
		Warnings: warnings,
	}
}
