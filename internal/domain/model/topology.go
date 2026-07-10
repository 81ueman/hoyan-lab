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
	Name           string
	Kind           DeviceKind
	Role           string
	RouterID       string
	ASN            uint32
	Loopback       string
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
