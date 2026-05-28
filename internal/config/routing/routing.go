package routing

import "github.com/81ueman/hoyan-lab/internal/core/topology"

type RouteSourceKind = topology.RouteSourceKind
type ConnectedRouteClass = topology.ConnectedRouteClass
type ConfiguredRoute = topology.ConfiguredRoute
type BGPRedistribution = topology.BGPRedistribution
type BGPNeighbor = topology.BGPNeighbor
type OSPFProcess = topology.OSPFProcess
type OSPFNetwork = topology.OSPFNetwork
type OSPFInterface = topology.OSPFInterface
type OSPFAreaKind = topology.OSPFAreaKind
type OSPFArea = topology.OSPFArea
type OSPFRedistribution = topology.OSPFRedistribution
type PrefixList = topology.PrefixList
type PrefixListRule = topology.PrefixListRule
type ASPathList = topology.ASPathList
type CommunityList = topology.CommunityList
type StringListRule = topology.StringListRule
type RoutePolicy = topology.RoutePolicy
type RoutePolicyRule = topology.RoutePolicyRule
type ACLAction = topology.ACLAction
type ACLDefaultAction = topology.ACLDefaultAction
type ACLRule = topology.ACLRule
type ACL = topology.ACL
type ACLBinding = topology.ACLBinding
type ConfigSource = topology.ConfigSource
type FailureDomain = topology.FailureDomain

type NodeRouting struct {
	Node           string
	ASN            uint32
	Routes         []ConfiguredRoute
	Neighbors      []BGPNeighbor
	Redistribute   []BGPRedistribution
	OSPF           OSPFProcess
	OSPFProcesses  []OSPFProcess
	PrefixLists    []PrefixList
	ASPathLists    []ASPathList
	CommunityLists []CommunityList
	RoutePolicies  []RoutePolicy
	ConfigPath     string
}

type TopologyRouting struct {
	Nodes       map[string]NodeRouting
	ACLs        []ACL
	ACLBindings []ACLBinding
}

func EmptyTopologyRouting() TopologyRouting {
	return TopologyRouting{Nodes: map[string]NodeRouting{}}
}

func FromTopology(topo *topology.Topology) TopologyRouting {
	out := EmptyTopologyRouting()
	if topo == nil {
		return out
	}
	for _, node := range topo.Nodes {
		out.Nodes[node.Name] = NodeRouting{
			Node:           node.Name,
			ASN:            node.ASN,
			Routes:         append([]ConfiguredRoute(nil), node.Routes...),
			Neighbors:      append([]BGPNeighbor(nil), node.Neighbors...),
			Redistribute:   append([]BGPRedistribution(nil), node.Redistribute...),
			OSPF:           node.OSPF,
			OSPFProcesses:  append([]OSPFProcess(nil), node.OSPFProcesses...),
			PrefixLists:    append([]PrefixList(nil), node.PrefixLists...),
			ASPathLists:    append([]ASPathList(nil), node.ASPathLists...),
			CommunityLists: append([]CommunityList(nil), node.CommunityLists...),
			RoutePolicies:  append([]RoutePolicy(nil), node.RoutePolicies...),
			ConfigPath:     node.ConfigPath,
		}
	}
	out.ACLs = append([]ACL(nil), topo.ACLs...)
	out.ACLBindings = append([]ACLBinding(nil), topo.ACLBindings...)
	return out
}

func StripTopology(topo *topology.Topology) *topology.Topology {
	if topo == nil {
		return nil
	}
	out := *topo
	out.Nodes = append([]topology.Node(nil), topo.Nodes...)
	for i := range out.Nodes {
		out.Nodes[i].ASN = 0
		out.Nodes[i].ConfigPath = ""
		out.Nodes[i].Routes = nil
		out.Nodes[i].Neighbors = nil
		out.Nodes[i].Redistribute = nil
		out.Nodes[i].OSPF = topology.OSPFProcess{}
		out.Nodes[i].OSPFProcesses = nil
		out.Nodes[i].PrefixLists = nil
		out.Nodes[i].ASPathLists = nil
		out.Nodes[i].CommunityLists = nil
		out.Nodes[i].RoutePolicies = nil
	}
	out.ACLs = nil
	out.ACLBindings = nil
	return &out
}

func (r TopologyRouting) ForNode(name string) NodeRouting {
	if r.Nodes == nil {
		return NodeRouting{Node: name}
	}
	node, ok := r.Nodes[name]
	if !ok {
		return NodeRouting{Node: name}
	}
	if node.Node == "" {
		node.Node = name
	}
	return node
}

const (
	RouteSourceConnected = topology.RouteSourceConnected
	RouteSourceStatic    = topology.RouteSourceStatic
	RouteSourceBGP       = topology.RouteSourceBGP
	RouteSourceOSPF      = topology.RouteSourceOSPF
	RouteSourceAggregate = topology.RouteSourceAggregate
	RouteSourceBlackhole = topology.RouteSourceBlackhole

	ConnectedRouteClassLink     = topology.ConnectedRouteClassLink
	ConnectedRouteClassLoopback = topology.ConnectedRouteClassLoopback
	ConnectedRouteClassService  = topology.ConnectedRouteClassService
	ConnectedRouteClassHost     = topology.ConnectedRouteClassHost

	OSPFAreaNormal = topology.OSPFAreaNormal
	OSPFAreaStub   = topology.OSPFAreaStub
	OSPFAreaNSSA   = topology.OSPFAreaNSSA

	ACLPermit = topology.ACLPermit
	ACLDeny   = topology.ACLDeny

	ACLDefaultPermit = topology.ACLDefaultPermit
	ACLDefaultDeny   = topology.ACLDefaultDeny
)
