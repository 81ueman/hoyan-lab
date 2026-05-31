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

func (u Usecase) Collect(ctx context.Context, nodes []model.Node, opts observation.Options) ([]observation.FIB, error) {
	return u.collector.Collect(ctx, nodes, opts)
}

func (u Usecase) SupportedNodes(nodes []model.Node) []model.Node {
	return u.collector.SupportedNodes(nodes)
}
