package livecheck

import (
	"context"
	"io"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	observationfib "github.com/81ueman/hoyan-lab/internal/domain/observation/fib"
	observationrib "github.com/81ueman/hoyan-lab/internal/domain/observation/rib"
	"github.com/81ueman/hoyan-lab/internal/domain/query"
)

type Runtime interface {
	BuildLocalImages(ctx context.Context, topologyPath string, out io.Writer) error
	Deploy(ctx context.Context, topologyPath string) error
	Destroy(ctx context.Context, topologyPath string) error
	WaitContainers(ctx context.Context, nodes []model.Node, interval time.Duration) error
	WaitSRLinuxCLI(ctx context.Context, nodes []model.Node, interval time.Duration) error
	ApplyNftablesPolicies(ctx context.Context, topo *model.Topology, out io.Writer) error
}

type FailureRuntime interface {
	SetLinkLoss(ctx context.Context, topo *model.Topology, node, intf string, lossPercent int) error
	ResetLinkLoss(ctx context.Context, topo *model.Topology, node, intf string) error
	StopNode(ctx context.Context, node model.Node) error
}

type QueryLoader interface {
	Load(path string) (*query.Queries, error)
}

type RIBCollector interface {
	SupportedNodes(nodes []model.Node) []model.Node
	Collect(ctx context.Context, nodes []model.Node) ([]observationrib.NormalizedRoute, error)
	CollectBGPRoutes(ctx context.Context, nodes []model.Node) ([]observationrib.NormalizedRoute, error)
}

type FIBCollector interface {
	SupportedNodes(nodes []model.Node) []model.Node
	Collect(ctx context.Context, nodes []model.Node, opts observationfib.Options) ([]observationfib.NormalizedFIBRoute, error)
}

type DataplaneProber interface {
	Probe(ctx context.Context, topo *model.Topology, check query.PacketCheck) (bool, error)
}

type Dependencies struct {
	Runtime         Runtime
	QueryLoader     QueryLoader
	RIBCollector    RIBCollector
	FIBCollector    FIBCollector
	DataplaneProber DataplaneProber
}
