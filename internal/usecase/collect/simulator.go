package collect

import (
	"context"
	"fmt"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

type Simulator struct {
	idx      *model.TopologyIndex
	graph    *sim.Graph
	failures sim.FailureSet
}

var _ Collector = Simulator{}

func NewSimulator(topo *model.Topology) (Simulator, error) {
	return NewSimulatorWithGraphOptions(topo)
}

func NewSimulatorWithGraphOptions(topo *model.Topology, graphOpts ...sim.GraphOption) (Simulator, error) {
	if topo == nil {
		return Simulator{}, fmt.Errorf("simulator topology is required")
	}
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		return Simulator{}, err
	}
	g, err := sim.NewGraph(topo, graphOpts...)
	if err != nil {
		return Simulator{}, err
	}
	return Simulator{idx: idx, graph: g, failures: sim.NoFailures()}, nil
}

func (s Simulator) Graph() *sim.Graph {
	return s.graph
}

func (s Simulator) CollectorFor(failures sim.FailureSet) Collector {
	s.failures = failures
	return s
}

func (s Simulator) Metadata(context.Context) observation.CollectorMetadata {
	return observation.CollectorMetadata{Source: "simulator"}
}

func (s Simulator) Nodes(context.Context) ([]model.NodeID, error) {
	out := make([]model.NodeID, 0, len(s.idx.Topology.Nodes))
	for _, node := range s.idx.Topology.Nodes {
		out = append(out, model.NodeID(node.Name))
	}
	return out, nil
}

func (s Simulator) VRFs(_ context.Context, node model.NodeID) ([]model.NetworkInstanceID, error) {
	vrfs, ok := s.idx.NetworkInstancesForNode(node)
	if !ok {
		return nil, fmt.Errorf("simulator node %q not found", node)
	}
	return vrfs, nil
}

func (s Simulator) CollectRIB(_ context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error) {
	n, ok := s.idx.Node(string(node))
	if !ok {
		return observation.RIB{}, fmt.Errorf("simulator node %q not found", node)
	}
	rib := s.expectedRIB(n, vrf, s.failures)
	return observation.FilterRIB(rib, opts), nil
}

func (s Simulator) CollectFIB(_ context.Context, node model.NodeID, vrf model.NetworkInstanceID, _ observation.Options) (observation.FIB, error) {
	n, ok := s.idx.Node(string(node))
	if !ok {
		return observation.FIB{}, fmt.Errorf("simulator node %q not found", node)
	}
	return s.expectedFIB(n, vrf, s.failures), nil
}
