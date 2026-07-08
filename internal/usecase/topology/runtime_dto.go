package topology

// RuntimeTopology carries adapter/runtime metadata derived from the lab file.
// It is intentionally kept outside internal/domain/model so callers can start
// depending on a pure domain Topology plus this DTO instead of runtime fields on
// model.Node.
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
