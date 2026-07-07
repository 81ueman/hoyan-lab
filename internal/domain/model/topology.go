package model

type Topology struct {
	Name             string
	ManagementSubnet string
	Nodes            []Node
	Links            []Link
	ACLs             []ACL
	ACLBindings      []ACLBinding
}

type Node struct {
	Name string
	// Deprecated: runtime metadata belongs in usecase/topology.RuntimeNode.
	// Kept temporarily for compatibility with callers that still use LoadTopology.
	ContainerName string
	Kind          DeviceKind
	Role          string
	ASN           uint32
	// Deprecated: runtime metadata belongs in usecase/topology.RuntimeNode.
	// Kept temporarily for compatibility with callers that still use LoadTopology.
	MgmtIPv4 string
	Loopback string
	// Deprecated: config source metadata belongs in usecase/topology.RuntimeNode.
	// Kept temporarily for compatibility with callers that still use LoadTopology.
	ConfigPath     string
	Prefixes       []Prefix
	Routes         []ConfiguredRoute
	Interfaces     []Interface
	Neighbors      []BGPNeighbor
	Redistribute   []BGPRedistribution
	OSPF           OSPFProcess
	OSPFProcesses  []OSPFProcess
	PrefixLists    []PrefixList
	ASPathLists    []ASPathList
	CommunityLists []CommunityList
	RoutePolicies  []RoutePolicy
}

// Deprecated: runtime naming belongs in adapter/usecase DTOs. Kept for
// compatibility while live adapters migrate from model.Node.ContainerName.
func (n Node) RuntimeName() string {
	if n.ContainerName != "" {
		return n.ContainerName
	}
	return n.Name
}

type Link struct {
	Name   string
	A      string
	B      string
	Role   string
	Cost   int
	Subnet string
	AIntf  string
	BIntf  string
}

type Interface struct {
	Name    string
	Address string
	VRF     NetworkInstanceID
}

func (t *Topology) Node(name string) (Node, bool) {
	idx, err := BuildTopologyIndex(t)
	if err != nil {
		return Node{}, false
	}
	return idx.Node(name)
}

func (t *Topology) OriginForPrefix(prefix string) (string, bool) {
	idx, err := BuildTopologyIndex(t)
	if err != nil {
		return "", false
	}
	return idx.OriginForPrefix(prefix)
}

func (t *Topology) OriginForIP(addr string) (string, Prefix, bool) {
	idx, err := BuildTopologyIndex(t)
	if err != nil {
		return "", Prefix{}, false
	}
	return idx.OriginForIP(addr)
}
