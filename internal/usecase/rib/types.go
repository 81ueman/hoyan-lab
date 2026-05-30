package rib

import (
	"context"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	observationrib "github.com/81ueman/hoyan-lab/internal/domain/observation/rib"
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
