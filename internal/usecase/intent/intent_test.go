package intent

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

func TestDefaultSnapshotProviderUsesRegisteredDefaultGraphOptions(t *testing.T) {
	orig := defaultGraphOptions
	defer func() { defaultGraphOptions = orig }()
	SetDefaultGraphOptions(sim.WithSolverBackend(nil))
	if got := (DefaultSnapshotProvider{}).graphOptions(); len(got) == 0 {
		t.Fatalf("DefaultSnapshotProvider graph options = %#v, want registered default option", got)
	}
	if got := (DefaultSnapshotProvider{GraphOptions: []sim.GraphOption{}}).graphOptions(); len(got) != 0 {
		t.Fatalf("explicit graph options = %#v, want explicit empty options preserved", got)
	}
}


