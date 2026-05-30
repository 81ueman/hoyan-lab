package intent

import "github.com/81ueman/hoyan-lab/internal/usecase/facts"

type SnapshotProvider interface {
	LoadSnapshot(name string, def Snapshot) (facts.Snapshot, error)
}

type DefaultSnapshotProvider struct{}

func (DefaultSnapshotProvider) LoadSnapshot(name string, def Snapshot) (facts.Snapshot, error) {
	return facts.Build(def.Lab, name)
}
