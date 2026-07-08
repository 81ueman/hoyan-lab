package containerlab

import (
	"context"
	"io"
	"time"

	liveadapter "github.com/81ueman/hoyan-lab/internal/adapter/live"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type Environment struct {
	runtime   Runtime
	collector Collector
}

func NewEnvironment(nodes []model.Node, runner liveadapter.Runner, fibOptions observation.Options) Environment {
	return Environment{
		runtime:   Runtime{Runner: runner},
		collector: NewCollector(nodes, runner, fibOptions),
	}
}

func (e Environment) Start(ctx context.Context, topologyPath string, topo *model.Topology, pollInterval time.Duration, out io.Writer) error {
	return e.runtime.Start(ctx, topologyPath, topo, pollInterval, out)
}

func (e Environment) Stop(ctx context.Context, topologyPath string) error {
	return e.runtime.Stop(ctx, topologyPath)
}

func (e Environment) SetLinkLoss(ctx context.Context, topo *model.Topology, node, intf string, lossPercent int) error {
	return e.runtime.SetLinkLoss(ctx, topo, node, intf, lossPercent)
}

func (e Environment) ResetLinkLoss(ctx context.Context, topo *model.Topology, node, intf string) error {
	return e.runtime.ResetLinkLoss(ctx, topo, node, intf)
}

func (e Environment) StopNode(ctx context.Context, node model.Node) error {
	return e.runtime.StopNode(ctx, node)
}

func (e Environment) Metadata(ctx context.Context) observation.CollectorMetadata {
	return e.collector.Metadata(ctx)
}

func (e Environment) Nodes(ctx context.Context) ([]model.NodeID, error) {
	return e.collector.Nodes(ctx)
}

func (e Environment) VRFs(ctx context.Context, node model.NodeID) ([]model.NetworkInstanceID, error) {
	return e.collector.VRFs(ctx, node)
}

func (e Environment) CollectRIB(ctx context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error) {
	return e.collector.CollectRIB(ctx, node, vrf, opts)
}

func (e Environment) CollectFIB(ctx context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts observation.Options) (observation.FIB, error) {
	return e.collector.CollectFIB(ctx, node, vrf, opts)
}
