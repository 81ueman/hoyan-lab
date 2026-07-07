package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	liveadapter "github.com/81ueman/hoyan-lab/internal/adapter/live"
	clabcollector "github.com/81ueman/hoyan-lab/internal/adapter/live/containerlab"
	"github.com/81ueman/hoyan-lab/internal/adapter/snapshotfile"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	collectusecase "github.com/81ueman/hoyan-lab/internal/usecase/collect"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

var newLiveRunner = func() liveadapter.Runner {
	return liveadapter.ExecRunner{}
}

type TargetType string

const (
	TargetModel    TargetType = "model"
	TargetClab     TargetType = "clab"
	TargetSnapshot TargetType = "snapshot"
	TargetDevice   TargetType = "device"
)

type CollectorTarget struct {
	Type TargetType
	Path string
}

func newCollectorTarget(path string, typeRaw string) (CollectorTarget, error) {
	return newCollectorTargetWithTypeHint(path, typeRaw, "--type")
}

func newCollectorTargetWithTypeHint(path string, typeRaw string, typeFlagHint string) (CollectorTarget, error) {
	targetType, err := parseTargetType(typeRaw)
	if err != nil {
		return CollectorTarget{}, err
	}
	if targetType == "" {
		targetType, err = inferTargetType(path, typeFlagHint)
		if err != nil {
			return CollectorTarget{}, err
		}
	}
	return CollectorTarget{Type: targetType, Path: path}, nil
}

func parseTargetType(raw string) (TargetType, error) {
	switch TargetType(strings.ToLower(strings.TrimSpace(raw))) {
	case "":
		return "", nil
	case TargetModel:
		return TargetModel, nil
	case TargetClab:
		return TargetClab, nil
	case TargetSnapshot:
		return TargetSnapshot, nil
	case TargetDevice:
		return TargetDevice, nil
	default:
		return "", fmt.Errorf("collector type must be one of model, clab, snapshot, or device")
	}
}

func inferTargetType(path string, typeFlagHint string) (TargetType, error) {
	lower := strings.ToLower(filepath.Clean(path))
	switch {
	case strings.HasSuffix(lower, ".json"):
		return TargetSnapshot, nil
	case strings.HasSuffix(lower, ".clab.yml"),
		strings.HasSuffix(lower, ".clab.yaml"),
		strings.HasSuffix(lower, ".yml"),
		strings.HasSuffix(lower, ".yaml"):
		return TargetModel, nil
	default:
		return "", fmt.Errorf("cannot infer collector type for %q; set %s", path, typeFlagHint)
	}
}

func resolveCollector(ctx context.Context, target CollectorTarget) (collectusecase.Collector, error) {
	_ = ctx
	switch target.Type {
	case TargetSnapshot:
		snap, err := snapshotfile.LoadObservation(target.Path)
		if err != nil {
			return nil, err
		}
		return observation.NewSnapshotBackedCollector(snap), nil
	case TargetModel:
		topo, err := topology.LoadTopology(target.Path)
		if err != nil {
			return nil, err
		}
		return collectusecase.NewSimulator(topo)
	case TargetClab:
		topo, err := topology.LoadTopology(target.Path)
		if err != nil {
			return nil, err
		}
		runner := newLiveRunner()
		return clabcollector.NewCollector(
			topo.Nodes,
			runner,
			observation.Options{AllowUnsupported: true, UnresolvedPolicy: observation.UnresolvedPolicyWarn},
		), nil
	case TargetDevice:
		return nil, fmt.Errorf("collector type %q is not implemented yet", target.Type)
	default:
		return nil, fmt.Errorf("unsupported collector type %q", target.Type)
	}
}
