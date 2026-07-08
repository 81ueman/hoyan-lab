package containerlab

import (
	"context"
	"fmt"

	liveadapter "github.com/81ueman/hoyan-lab/internal/adapter/live"
	livefib "github.com/81ueman/hoyan-lab/internal/adapter/live/fib"
	liverib "github.com/81ueman/hoyan-lab/internal/adapter/live/rib"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type Collector struct {
	nodes      []model.Node
	runner     liveadapter.Runner
	fibOptions observation.Options
}

func NewCollector(nodes []model.Node, runner liveadapter.Runner, fibOptions observation.Options) Collector {
	return Collector{
		nodes:      append([]model.Node(nil), nodes...),
		runner:     runner,
		fibOptions: fibOptions,
	}
}

func (c Collector) Metadata(context.Context) observation.CollectorMetadata {
	return observation.CollectorMetadata{Source: "containerlab"}
}

func (c Collector) Nodes(context.Context) ([]model.NodeID, error) {
	out := make([]model.NodeID, 0, len(c.nodes))
	for _, node := range c.nodes {
		out = append(out, model.NodeID(node.Name))
	}
	return out, nil
}

func (c Collector) VRFs(_ context.Context, node model.NodeID) ([]model.NetworkInstanceID, error) {
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

func (c Collector) CollectRIB(ctx context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error) {
	n, ok := c.node(node)
	if !ok {
		return observation.RIB{}, fmt.Errorf("containerlab node %q not found", node)
	}
	return liverib.NewCollector(c.runner).CollectRIB(ctx, n, vrf, opts)
}

func (c Collector) CollectFIB(ctx context.Context, node model.NodeID, vrf model.NetworkInstanceID, _ observation.Options) (observation.FIB, error) {
	n, ok := c.node(node)
	if !ok {
		return observation.FIB{}, fmt.Errorf("containerlab node %q not found", node)
	}
	fibOpts := c.fibOptions
	fibOpts.AllowUnsupported = true
	fib, err := livefib.NewCollector(c.runner).CollectFIB(ctx, n, vrf, fibOpts)
	if err != nil {
		return observation.FIB{}, err
	}
	return fib, nil
}

func (c Collector) node(node model.NodeID) (model.Node, bool) {
	for _, n := range c.nodes {
		if n.Name == string(node) {
			return n, true
		}
	}
	return model.Node{}, false
}
