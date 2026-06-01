package intent

type SnapshotProvider interface {
	LoadSnapshot(name string, def Snapshot) (SnapshotContext, error)
}

type DefaultSnapshotProvider struct{}

func (DefaultSnapshotProvider) LoadSnapshot(name string, def Snapshot) (SnapshotContext, error) {
	return BuildSnapshot(def.Lab, name)
}
