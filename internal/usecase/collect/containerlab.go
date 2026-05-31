package collect

import (
	"context"
	"fmt"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type RIBCollector interface {
	CollectRIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error)
}

type FIBCollector interface {
	CollectFIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID, opts observation.Options) (observation.FIB, error)
}

type ContainerlabCollector struct {
	nodes        []model.Node
	ribCollector RIBCollector
	fibCollector FIBCollector
	fibOptions   observation.Options
}

func NewContainerlabCollector(nodes []model.Node, ribCollector RIBCollector, fibCollector FIBCollector, fibOptions observation.Options) ContainerlabCollector {
	return ContainerlabCollector{
		nodes:        append([]model.Node(nil), nodes...),
		ribCollector: ribCollector,
		fibCollector: fibCollector,
		fibOptions:   fibOptions,
	}
}

func (c ContainerlabCollector) Metadata(context.Context) observation.CollectorMetadata {
	return observation.CollectorMetadata{Source: "containerlab"}
}

func (c ContainerlabCollector) Nodes(context.Context) ([]model.NodeID, error) {
	out := make([]model.NodeID, 0, len(c.nodes))
	for _, node := range c.nodes {
		out = append(out, model.NodeID(node.Name))
	}
	return out, nil
}

func (c ContainerlabCollector) VRFs(_ context.Context, node model.NodeID) ([]model.NetworkInstanceID, error) {
	n, ok := c.node(node)
	if !ok {
		return nil, fmt.Errorf("containerlab node %q not found", node)
	}
	vrfs := model.NetworkInstancesForNode(n)
	out := make([]model.NetworkInstanceID, 0, len(vrfs))
	for _, vrf := range vrfs {
		out = append(out, model.NormalizeNetworkInstance(vrf))
	}
	return out, nil
}

func (c ContainerlabCollector) CollectRIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	nodeID := model.NodeID(node.Name)
	n, ok := c.node(nodeID)
	if !ok {
		return observation.RIB{}, fmt.Errorf("containerlab node %q not found", node.Name)
	}
	if c.ribCollector == nil {
		return observation.RIB{Node: nodeID, VRF: vrf}, nil
	}
	return c.ribCollector.CollectRIB(ctx, n, vrf, opts)
}

func (c ContainerlabCollector) CollectFIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID, opts observation.Options) (observation.FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	n, ok := c.node(model.NodeID(node.Name))
	if !ok {
		return observation.FIB{}, fmt.Errorf("containerlab node %q not found", node.Name)
	}
	if c.fibCollector == nil {
		return observation.FIB{Node: model.NodeID(node.Name), VRF: vrf}, nil
	}
	fibOpts := c.fibOptions
	fibOpts.AllowUnsupported = true
	fib, err := c.fibCollector.CollectFIB(ctx, n, vrf, fibOpts)
	if err != nil {
		return observation.FIB{}, err
	}
	return fib, nil
}

func (c ContainerlabCollector) node(node model.NodeID) (model.Node, bool) {
	for _, n := range c.nodes {
		if n.Name == string(node) {
			return n, true
		}
	}
	return model.Node{}, false
}
