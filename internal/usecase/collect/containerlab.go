package collect

import (
	"context"
	"fmt"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type RIBCollector interface {
	CollectBGPRoutes(ctx context.Context, nodes []model.Node) ([]observation.RIBRoute, error)
	CollectRouteTableRoutes(ctx context.Context, nodes []model.Node) ([]observation.RIBRoute, error)
}

type FIBCollector interface {
	Collect(ctx context.Context, nodes []model.Node, opts observation.Options) ([]observation.FIB, error)
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

func (c ContainerlabCollector) CollectRIB(ctx context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	n, ok := c.node(node)
	if !ok {
		return observation.RIB{}, fmt.Errorf("containerlab node %q not found", node)
	}
	if c.ribCollector == nil {
		return observation.RIB{Node: node, VRF: vrf}, nil
	}
	bgp, err := c.ribCollector.CollectBGPRoutes(ctx, []model.Node{n})
	if err != nil {
		return observation.RIB{}, err
	}
	routeTable, err := c.ribCollector.CollectRouteTableRoutes(ctx, []model.Node{n})
	if err != nil {
		return observation.RIB{}, err
	}
	routes := append(bgp, routeTable...)
	routes = filterObservationRIBRoutes(routes, string(node), string(vrf))
	observation.SortRIBRoutes(routes)
	return observation.FilterRIB(observation.RIB{Node: node, VRF: vrf, Routes: routes}, opts), nil
}

func (c ContainerlabCollector) CollectFIB(ctx context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	n, ok := c.node(node)
	if !ok {
		return observation.FIB{}, fmt.Errorf("containerlab node %q not found", node)
	}
	if c.fibCollector == nil {
		return observation.FIB{Node: node, VRF: vrf}, nil
	}
	fibOpts := c.fibOptions
	fibOpts.AllowUnsupported = true
	routes, err := c.fibCollector.Collect(ctx, []model.Node{n}, fibOpts)
	if err != nil {
		return observation.FIB{}, err
	}
	fib := filterFIBs(routes, node, vrf)
	return observation.FilterFIB(fib, opts), nil
}

func (c ContainerlabCollector) node(node model.NodeID) (model.Node, bool) {
	for _, n := range c.nodes {
		if n.Name == string(node) {
			return n, true
		}
	}
	return model.Node{}, false
}
