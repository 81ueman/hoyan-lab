package topology

import (
	"path/filepath"

	"github.com/81ueman/hoyan-lab/internal/adapter/configparse"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type LoadOptions struct {
	CollectWarnings bool
	StrictConfig    bool
}

func LoadTopology(clabPath string) (*model.Topology, error) {
	topo, _, err := defaultBuilder().LoadTopologyWithOptions(clabPath, LoadOptions{})
	return topo, err
}

func LoadTopologyWithWarnings(clabPath string) (*model.Topology, []configparse.UnsupportedStatement, error) {
	return defaultBuilder().LoadTopologyWithOptions(clabPath, LoadOptions{CollectWarnings: true})
}

func LoadTopologyWithOptions(clabPath string, opts LoadOptions) (*model.Topology, []configparse.UnsupportedStatement, error) {
	return defaultBuilder().LoadTopologyWithOptions(clabPath, opts)
}

func LoadDomainTopologyWithRuntime(clabPath string, opts LoadOptions) (*model.Topology, RuntimeTopology, []configparse.UnsupportedStatement, error) {
	return defaultBuilder().LoadDomainTopologyWithRuntime(clabPath, opts)
}

func (b Builder) LoadTopology(clabPath string) (*model.Topology, error) {
	topo, _, err := b.LoadTopologyWithOptions(clabPath, LoadOptions{})
	return topo, err
}

func (b Builder) LoadTopologyWithWarnings(clabPath string) (*model.Topology, []configparse.UnsupportedStatement, error) {
	return b.LoadTopologyWithOptions(clabPath, LoadOptions{CollectWarnings: true})
}

func (b Builder) LoadTopologyWithOptions(clabPath string, opts LoadOptions) (*model.Topology, []configparse.UnsupportedStatement, error) {
	topo, runtime, warnings, err := b.LoadDomainTopologyWithRuntime(clabPath, opts)
	if err != nil {
		return nil, warnings, err
	}
	applyRuntimeMetadata(topo, runtime)
	return topo, warnings, nil
}

func (b Builder) LoadDomainTopologyWithRuntime(clabPath string, opts LoadOptions) (*model.Topology, RuntimeTopology, []configparse.UnsupportedStatement, error) {
	raw, err := b.loader.Load(clabPath)
	if err != nil {
		return nil, RuntimeTopology{}, nil, err
	}
	root := filepath.Dir(clabPath)
	topo := &model.Topology{Name: raw.Name, ManagementSubnet: raw.Mgmt.IPv4Subnet}

	nodes, runtime, aclAttachments, transitNodes, warnings, err := buildNodes(raw, root, opts, b.parser)
	if err != nil {
		return nil, RuntimeTopology{}, nil, err
	}
	topo.Nodes = nodes
	topo.ACLs = aclAttachments.ACLs
	topo.ACLBindings = aclAttachments.Bindings
	if opts.StrictConfig && len(warnings) > 0 {
		return nil, runtime, warnings, configparse.UnsupportedConfigError{Warnings: warnings}
	}

	links, transitAttachments, err := buildDirectLinks(raw.Topology.Links, topo, transitNodes)
	if err != nil {
		return nil, RuntimeTopology{}, nil, err
	}
	topo.Links = append(topo.Links, links...)

	transitLinks, err := expandTransitLinks(transitAttachments, topo)
	if err != nil {
		return nil, RuntimeTopology{}, nil, err
	}
	topo.Links = append(topo.Links, transitLinks...)

	resolveNeighborNodes(topo)
	if err := topo.Validate(); err != nil {
		return nil, RuntimeTopology{}, nil, err
	}
	return topo, runtime, warnings, nil
}
