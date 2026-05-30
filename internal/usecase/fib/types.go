package fib

import (
	"context"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	observationfib "github.com/81ueman/hoyan-lab/internal/domain/observation/fib"
)

type Usecase struct {
	collector observationfib.Collector
}

func New(collector observationfib.Collector) Usecase {
	return Usecase{collector: collector}
}

func (u Usecase) Collect(ctx context.Context, nodes []model.Node, opts observationfib.Options) ([]observationfib.NormalizedFIBRoute, error) {
	return u.collector.Collect(ctx, nodes, opts)
}

func (u Usecase) SupportedNodes(nodes []model.Node) []model.Node {
	return u.collector.SupportedNodes(nodes)
}

func (u Usecase) Expected(topo *model.Topology) []observationfib.NormalizedFIBRoute {
	return Expected(topo)
}

func (u Usecase) ExpectedForNodes(topo *model.Topology, nodes []model.Node) []observationfib.NormalizedFIBRoute {
	return ExpectedForNodes(topo, nodes)
}
