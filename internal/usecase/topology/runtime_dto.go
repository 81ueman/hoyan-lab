package topology

import "github.com/81ueman/hoyan-lab/internal/domain/model"

// RuntimeTopology carries adapter/runtime metadata derived from the lab file.
// It is intentionally kept outside internal/domain/model so callers can start
// depending on a pure domain Topology plus this DTO instead of runtime fields on
// model.Node. Existing LoadTopology APIs still copy this data into model.Node
// for backwards compatibility during the migration.
type RuntimeTopology struct {
	Name             string
	ManagementSubnet string
	Nodes            map[string]RuntimeNode
}

type RuntimeNode struct {
	Name          string
	ContainerName string
	MgmtIPv4      string
	ConfigPath    string
}

func (n RuntimeNode) RuntimeName() string {
	if n.ContainerName != "" {
		return n.ContainerName
	}
	return n.Name
}

func (t RuntimeTopology) RuntimeName(nodeName string) string {
	if n, ok := t.Nodes[nodeName]; ok {
		return n.RuntimeName()
	}
	return nodeName
}

func applyRuntimeMetadata(topo *model.Topology, runtime RuntimeTopology) {
	if topo == nil {
		return
	}
	for i := range topo.Nodes {
		metadata, ok := runtime.Nodes[topo.Nodes[i].Name]
		if !ok {
			continue
		}
		topo.Nodes[i].ContainerName = metadata.ContainerName
		topo.Nodes[i].MgmtIPv4 = metadata.MgmtIPv4
		topo.Nodes[i].ConfigPath = metadata.ConfigPath
	}
}
