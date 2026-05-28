package topology

import (
	"path/filepath"

	"github.com/81ueman/hoyan-lab/internal/adapter/configparse"
	"github.com/81ueman/hoyan-lab/internal/adapter/labfile"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type LoadOptions struct {
	CollectWarnings bool
	StrictConfig    bool
}

func LoadTopology(clabPath string) (*model.Topology, error) {
	topo, _, err := LoadTopologyWithOptions(clabPath, LoadOptions{})
	return topo, err
}

func LoadTopologyWithWarnings(clabPath string) (*model.Topology, []configparse.UnsupportedStatement, error) {
	return LoadTopologyWithOptions(clabPath, LoadOptions{CollectWarnings: true})
}

func LoadTopologyWithOptions(clabPath string, opts LoadOptions) (*model.Topology, []configparse.UnsupportedStatement, error) {
	raw, err := labfile.Load(clabPath)
	if err != nil {
		return nil, nil, err
	}
	root := filepath.Dir(clabPath)
	topo := &model.Topology{Name: raw.Name, ManagementSubnet: raw.Mgmt.IPv4Subnet}

	nodes, aclAttachments, transitNodes, warnings, err := buildNodes(raw, root, opts)
	if err != nil {
		return nil, nil, err
	}
	topo.Nodes = nodes
	topo.ACLs = aclAttachments.ACLs
	topo.ACLBindings = aclAttachments.Bindings
	if opts.StrictConfig && len(warnings) > 0 {
		return nil, warnings, configparse.UnsupportedConfigError{Warnings: warnings}
	}

	links, transitAttachments, err := buildDirectLinks(raw.Topology.Links, topo, transitNodes)
	if err != nil {
		return nil, nil, err
	}
	topo.Links = append(topo.Links, links...)

	transitLinks, err := expandTransitLinks(transitAttachments, topo)
	if err != nil {
		return nil, nil, err
	}
	topo.Links = append(topo.Links, transitLinks...)

	resolveNeighborNodes(topo)
	if err := topo.Validate(); err != nil {
		return nil, nil, err
	}
	return topo, warnings, nil
}
