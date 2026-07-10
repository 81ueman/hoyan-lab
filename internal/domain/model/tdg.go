package model

import "fmt"

// TDGNode represents one forwarding decision point in the graph.
type TDGNode struct {
	ID            int
	Node          string // router name
	VRF           string
	Stage         string // "ingress_acl", "fib_lookup", "egress_acl", "next_hop"
	PacketClassID PrefixClassID
}

// TDGEdge represents traffic flow between two TDG nodes with a weight.
type TDGEdge struct {
	From   *TDGNode
	To     *TDGNode
	Weight float64 // 0.0-1.0 (ECMP distribution)
}

// TDG is the Traffic Distribution Graph for a single packet class.
type TDG struct {
	Nodes []*TDGNode
	Edges []*TDGEdge
	Root  *TDGNode   // ingress node for this class
	Sinks []*TDGNode // destination nodes (origination points)

	nodeIndex map[string]*TDGNode   // name -> node for fast lookup
	outEdges  map[string][]*TDGEdge // name -> outgoing edges
}

// NewTDG creates an empty Traffic Distribution Graph.
func NewTDG() *TDG {
	return &TDG{
		Nodes:     []*TDGNode{},
		Edges:     []*TDGEdge{},
		Sinks:     []*TDGNode{},
		nodeIndex: map[string]*TDGNode{},
		outEdges:  map[string][]*TDGEdge{},
	}
}

// AddNode adds a node to the TDG or returns an existing one with the same name.
func (tdg *TDG) AddNode(name, vrf, stage string, classID PrefixClassID) *TDGNode {
	if existing, ok := tdg.nodeIndex[name]; ok {
		return existing
	}
	node := &TDGNode{
		ID:            len(tdg.Nodes),
		Node:          name,
		VRF:           vrf,
		Stage:         stage,
		PacketClassID: classID,
	}
	tdg.Nodes = append(tdg.Nodes, node)
	tdg.nodeIndex[name] = node
	return node
}

// AddEdge adds a directed edge between two named nodes.
func (tdg *TDG) AddEdge(fromName, toName string, weight float64) (*TDGEdge, error) {
	from, ok := tdg.nodeIndex[fromName]
	if !ok {
		return nil, fmt.Errorf("node %q not found", fromName)
	}
	to, ok := tdg.nodeIndex[toName]
	if !ok {
		return nil, fmt.Errorf("node %q not found", toName)
	}
	edge := &TDGEdge{
		From:   from,
		To:     to,
		Weight: weight,
	}
	tdg.Edges = append(tdg.Edges, edge)
	tdg.outEdges[fromName] = append(tdg.outEdges[fromName], edge)
	return edge, nil
}

// SetRoot sets the root (ingress) node for this TDG.
func (tdg *TDG) SetRoot(name string) error {
	node, ok := tdg.nodeIndex[name]
	if !ok {
		return fmt.Errorf("node %q not found", name)
	}
	tdg.Root = node
	return nil
}

// AddSink adds a sink (destination) node.
func (tdg *TDG) AddSink(name string) {
	if node, ok := tdg.nodeIndex[name]; ok {
		tdg.Sinks = append(tdg.Sinks, node)
	}
}

// OutEdges returns all outgoing edges from a named node.
func (tdg *TDG) OutEdges(name string) []*TDGEdge {
	return tdg.outEdges[name]
}

// TopologicalOrder returns nodes in topological order (BFS from root).
func (tdg *TDG) TopologicalOrder() []*TDGNode {
	if tdg.Root == nil {
		return nil
	}

	var order []*TDGNode
	visited := map[string]bool{}
	queue := []*TDGNode{tdg.Root}

	for len(queue) > 0 {
		node := queue[0]
		queue = queue[1:]
		if visited[node.Node] {
			continue
		}
		visited[node.Node] = true
		order = append(order, node)

		for _, edge := range tdg.outEdges[node.Node] {
			if !visited[edge.To.Node] {
				queue = append(queue, edge.To)
			}
		}
	}
	return order
}
