package livecheck

import (
	"context"
	"io"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	snapshotdomain "github.com/81ueman/hoyan-lab/internal/domain/snapshot"
	"github.com/81ueman/hoyan-lab/internal/usecase/collect"
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

type RIBCollector interface {
	CollectRIB(ctx context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error)
}

type FIBCollector interface {
	CollectFIB(ctx context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts observation.Options) (observation.FIB, error)
}

type SnapshotRepository interface {
	Load(path string) (*snapshotdomain.Snapshot, error)
}

type InputHashChecker interface {
	CheckHashes(topologyPath string, snap *snapshotdomain.Snapshot) (snapshotdomain.HashCheckResult, error)
}

type Dependencies struct {
	Runtime            Runtime
	Collector          collect.Collector
	SnapshotRepository SnapshotRepository
	InputHashChecker   InputHashChecker
}
