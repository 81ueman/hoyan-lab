package livecheck

import (
	"context"
	"io"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/domain/query"
)

type Runtime interface {
	Start(ctx context.Context, topologyPath string, topo *model.Topology, pollInterval time.Duration, out io.Writer) error
	Stop(ctx context.Context, topologyPath string) error
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
	CollectRIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error)
}

type FIBCollector interface {
	CollectFIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID, opts observation.Options) (observation.FIB, error)
}

type DataplaneProber interface {
	Probe(ctx context.Context, topo *model.Topology, check query.PacketCheck) (bool, error)
}

type Dependencies struct {
	Runtime         Runtime
	QueryLoader     QueryLoader
	Collector       observation.Collector
	DataplaneProber DataplaneProber
}
