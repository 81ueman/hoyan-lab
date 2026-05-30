package rib

import (
	"context"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	observationrib "github.com/81ueman/hoyan-lab/internal/domain/observation/rib"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

type Usecase struct {
	collector observationrib.Collector
}

func New(collector observationrib.Collector) Usecase {
	return Usecase{collector: collector}
}

func (u Usecase) Collect(ctx context.Context, nodes []model.Node) ([]observationrib.NormalizedRoute, error) {
	if u.collector == nil {
		return nil, nil
	}
	out, err := u.collector.CollectBGPRoutes(ctx, nodes)
	if err != nil {
		return nil, err
	}
	nonBGP, err := u.collector.CollectRouteTableRoutes(ctx, nodes)
	if err != nil {
		return nil, err
	}
	out = append(out, nonBGP...)
	observationrib.SortRoutes(out)
	return out, nil
}

func (u Usecase) CollectBGPRoutes(ctx context.Context, nodes []model.Node) ([]observationrib.NormalizedRoute, error) {
	return u.collector.CollectBGPRoutes(ctx, nodes)
}

func (u Usecase) CollectOSPFRoutes(ctx context.Context, nodes []model.Node) ([]observationrib.NormalizedRoute, error) {
	return u.collector.CollectOSPFRoutes(ctx, nodes)
}

func (u Usecase) CollectRouteTableRoutes(ctx context.Context, nodes []model.Node) ([]observationrib.NormalizedRoute, error) {
	return u.collector.CollectRouteTableRoutes(ctx, nodes)
}

func (u Usecase) Expected(topo *model.Topology) []observationrib.NormalizedRoute {
	return u.ExpectedWithFailureSet(topo, sim.NoFailures())
}

func (u Usecase) ExpectedForNodes(topo *model.Topology, nodes []model.Node) []observationrib.NormalizedRoute {
	return u.ExpectedForNodesWithFailureSet(topo, nodes, sim.NoFailures())
}

func (u Usecase) ExpectedWithFailureSet(topo *model.Topology, failures sim.FailureSet) []observationrib.NormalizedRoute {
	return expected(topo, nil, failures)
}

func (u Usecase) ExpectedForNodesWithFailureSet(topo *model.Topology, nodes []model.Node, failures sim.FailureSet) []observationrib.NormalizedRoute {
	allowed := map[string]bool{}
	for _, n := range nodes {
		allowed[n.Name] = true
	}
	return expected(topo, allowed, failures)
}
