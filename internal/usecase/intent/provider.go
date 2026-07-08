package intent

import "github.com/81ueman/hoyan-lab/internal/engine/sim"

type SnapshotProvider interface {
	LoadSnapshot(name string, def Snapshot) (SnapshotContext, error)
}

type DefaultSnapshotProvider struct {
	GraphOptions []sim.GraphOption
}

var defaultGraphOptions []sim.GraphOption

func SetDefaultGraphOptions(opts ...sim.GraphOption) {
	defaultGraphOptions = append([]sim.GraphOption(nil), opts...)
}

func (p DefaultSnapshotProvider) LoadSnapshot(name string, def Snapshot) (SnapshotContext, error) {
	return BuildSnapshotWithGraphOptions(def.Lab, name, p.graphOptions()...)
}

func (p DefaultSnapshotProvider) graphOptions() []sim.GraphOption {
	if p.GraphOptions != nil {
		return p.GraphOptions
	}
	return append([]sim.GraphOption(nil), defaultGraphOptions...)
}
