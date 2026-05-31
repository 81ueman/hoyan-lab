package collect

import (
	"context"
	"fmt"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	fibusecase "github.com/81ueman/hoyan-lab/internal/usecase/fib"
	ribusecase "github.com/81ueman/hoyan-lab/internal/usecase/rib"
)

type Simulator struct {
	topo     *model.Topology
	failures sim.FailureSet
}

func NewSimulator(topo *model.Topology) Simulator {
	return Simulator{topo: topo, failures: sim.NoFailures()}
}

func (s Simulator) CollectorFor(failures sim.FailureSet) observation.Collector {
	s.failures = failures
	return s
}

func (s Simulator) Metadata(context.Context) observation.CollectorMetadata {
	return observation.CollectorMetadata{Source: "simulator"}
}

func (s Simulator) Nodes(context.Context) ([]model.NodeID, error) {
	if s.topo == nil {
		return nil, fmt.Errorf("simulator collector has no topology")
	}
	out := make([]model.NodeID, 0, len(s.topo.Nodes))
	for _, node := range s.topo.Nodes {
		out = append(out, model.NodeID(node.Name))
	}
	return out, nil
}

func (s Simulator) VRFs(_ context.Context, node model.NodeID) ([]model.NetworkInstanceID, error) {
	n, ok := s.node(node)
	if !ok {
		return nil, fmt.Errorf("simulator node %q not found", node)
	}
	vrfs := model.NetworkInstancesForNode(n)
	out := make([]model.NetworkInstanceID, 0, len(vrfs))
	for _, vrf := range vrfs {
		out = append(out, model.NormalizeNetworkInstance(vrf))
	}
	return out, nil
}

func (s Simulator) CollectRIB(_ context.Context, node model.Node, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	nodeID := model.NodeID(node.Name)
	n, ok := s.node(nodeID)
	if !ok {
		return observation.RIB{}, fmt.Errorf("simulator node %q not found", node.Name)
	}
	routes := (ribusecase.ExpectedBuilder{}).BuildForNodesWithFailureSet(s.topo, []model.Node{n}, s.failures)
	routes = filterObservationRIBRoutes(routes, string(nodeID), string(vrf))
	observation.SortRIBRoutes(routes)
	return observation.FilterRIB(observation.RIB{Node: nodeID, VRF: vrf, Routes: routes}, opts), nil
}

func (s Simulator) CollectFIB(_ context.Context, node model.Node, vrf model.NetworkInstanceID, _ observation.Options) (observation.FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	n, ok := s.node(model.NodeID(node.Name))
	if !ok {
		return observation.FIB{}, fmt.Errorf("simulator node %q not found", node.Name)
	}
	fibs := fibusecase.NewExpectedBuilder().ExpectedFIBsForNodesWithFailureSet(s.topo, []model.Node{n}, s.failures)
	return filterFIBs(fibs, model.NodeID(node.Name), vrf), nil
}

func (s Simulator) node(node model.NodeID) (model.Node, bool) {
	if s.topo == nil {
		return model.Node{}, false
	}
	for _, n := range s.topo.Nodes {
		if n.Name == string(node) {
			return n, true
		}
	}
	return model.Node{}, false
}
