package rib

import (
	"context"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type Usecase struct {
	collector observation.RIBCollector
}

func New(collector observation.RIBCollector) Usecase {
	return Usecase{collector: collector}
}

func (u Usecase) Collect(ctx context.Context, nodes []model.Node) ([]observation.RIBRoute, error) {
	if u.collector == nil {
		return nil, nil
	}
	var out []observation.RIBRoute
	for _, node := range nodes {
		for _, vrf := range model.NetworkInstancesForNode(node) {
			rib, err := u.collector.CollectRIB(ctx, node, model.NormalizeNetworkInstance(vrf), observation.CollectOptions{IncludeInactive: true, IncludeModelInfo: true})
			if err != nil {
				return nil, err
			}
			out = append(out, rib.Routes...)
		}
	}
	observation.SortRoutes(out)
	return out, nil
}
