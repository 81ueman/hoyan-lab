package model

type Topology struct {
	Name             string       `yaml:"name"`
	ManagementSubnet string       `yaml:"management_subnet"`
	Nodes            []Node       `yaml:"nodes"`
	Links            []Link       `yaml:"links"`
	ACLs             []ACL        `yaml:"acls,omitempty" json:"acls,omitempty"`
	ACLBindings      []ACLBinding `yaml:"acl_bindings,omitempty" json:"acl_bindings,omitempty"`
}

type Node struct {
	Name           string              `yaml:"name"`
	ContainerName  string              `yaml:"container_name"`
	Kind           DeviceKind          `yaml:"kind"`
	Role           string              `yaml:"role"`
	ASN            uint32              `yaml:"asn"`
	MgmtIPv4       string              `yaml:"mgmt_ipv4"`
	Loopback       string              `yaml:"loopback"`
	ConfigPath     string              `yaml:"config_path"`
	Prefixes       []Prefix            `yaml:"prefixes"`
	Routes         []ConfiguredRoute   `yaml:"routes,omitempty"`
	Interfaces     []Interface         `yaml:"interfaces"`
	Neighbors      []BGPNeighbor       `yaml:"neighbors"`
	Redistribute   []BGPRedistribution `yaml:"redistribute,omitempty"`
	OSPF           OSPFProcess         `yaml:"ospf,omitempty"`
	OSPFProcesses  []OSPFProcess       `yaml:"ospf_processes,omitempty" json:"ospf_processes,omitempty"`
	PrefixLists    []PrefixList        `yaml:"prefix_lists"`
	ASPathLists    []ASPathList        `yaml:"as_path_lists"`
	CommunityLists []CommunityList     `yaml:"community_lists"`
	RoutePolicies  []RoutePolicy       `yaml:"route_policies"`
}

func (n Node) RuntimeName() string {
	if n.ContainerName != "" {
		return n.ContainerName
	}
	return n.Name
}

type Link struct {
	Name   string `yaml:"name"`
	A      string `yaml:"a"`
	B      string `yaml:"b"`
	Role   string `yaml:"role,omitempty"`
	Cost   int    `yaml:"cost"`
	Subnet string `yaml:"subnet"`
	AIntf  string `yaml:"a_intf"`
	BIntf  string `yaml:"b_intf"`
}

type Interface struct {
	Name    string            `yaml:"name"`
	Address string            `yaml:"address"`
	VRF     NetworkInstanceID `yaml:"vrf,omitempty" json:"vrf,omitempty"`
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
