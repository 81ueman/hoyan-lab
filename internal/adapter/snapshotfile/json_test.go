package snapshotfile

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	snapshotdomain "github.com/81ueman/hoyan-lab/internal/domain/snapshot"
	"github.com/81ueman/hoyan-lab/internal/usecase/livesnapshot"
)

func TestMarshalLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshot.json")
	snap := &snapshotdomain.Snapshot{
		Version:     snapshotdomain.Version,
		Lab:         "unit",
		CollectedAt: time.Date(2026, 5, 26, 1, 2, 3, 0, time.UTC),
		Nodes: map[string]snapshotdomain.NodeSnapshot{
			"r1": {
				Kind: model.KindFRR,
				BGPRIB: []observation.RIBRoute{{
					Common: observation.RIBRouteCommon{AFI: observation.AFIIPv4, Prefix: "10.0.0.0/24", Protocol: observation.ProtocolBGP, Eligible: true, Best: true},
					BGP:    &observation.BGPRIBRoute{Paths: []observation.BGPPath{{Eligible: true, Best: true}}},
				}},
			},
		},
	}
	if err := Save(path, snap); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Version != snapshotdomain.Version || loaded.Lab != "unit" {
		t.Fatalf("loaded snapshot = %#v", loaded)
	}
	if got := livesnapshot.BGPRoutes(loaded); len(got) != 1 || got[0].Common.Prefix != "10.0.0.0/24" {
		t.Fatalf("BGPRoutes() = %#v", got)
	}
}
