package fib

import (
	"context"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type Usecase struct {
	collector observation.FIBCollector
}

// New returns the preferred entry point for collector-backed live FIB collection.
func New(collector observation.FIBCollector) Usecase {
	return Usecase{collector: collector}
}

func (u Usecase) CollectFIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID, opts observation.Options) (observation.FIB, error) {
	return u.collector.CollectFIB(ctx, node, vrf, opts)
}

func (u Usecase) Collect(ctx context.Context, nodes []model.Node, opts observation.Options) ([]observation.FIB, error) {
	var out []observation.FIB
	for _, node := range nodes {
		for _, vrf := range model.NetworkInstancesForNode(node) {
			fib, err := u.CollectFIB(ctx, node, model.NormalizeNetworkInstance(vrf), opts)
			if err != nil {
				return nil, err
			}
			out = append(out, fib)
		}
	}
	return out, nil
}
