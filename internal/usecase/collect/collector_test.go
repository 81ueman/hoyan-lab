package collect

import (
	"context"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func TestSimulatorCollectorCanCompareWithSnapshotBackedCollector(t *testing.T) {
	ctx := context.Background()
	simulator, err := NewSimulator(testTopology())
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	snapshot, err := observation.CollectSnapshot(ctx, simulator, observation.CollectOptions{})
	if err != nil {
		t.Fatalf("CollectSnapshot(simulator) error = %v", err)
	}
	snapshotCollector := observation.NewSnapshotBackedCollector(snapshot)

	result, err := observation.CompareCollectors(ctx, simulator, snapshotCollector, observation.CollectOptions{}, observation.SnapshotCompareOptions{IgnoreMetadata: true})
	if err != nil {
		t.Fatalf("CompareCollectors() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("simulator vs snapshot mismatch: %#v", result)
	}
}

func TestNewSimulatorRejectsNilTopology(t *testing.T) {
	if _, err := NewSimulator(nil); err == nil {
		t.Fatalf("NewSimulator(nil) succeeded unexpectedly")
	}
}

func TestSnapshotBackedCollectorsCanCompareWithEachOther(t *testing.T) {
	ctx := context.Background()
	snapshot := observation.NetworkSnapshot{
		Nodes: []observation.NodeSnapshot{{
			Node: "r1",
			VRFs: []observation.VRFSnapshot{{
				VRF: "default",
				RIB: observation.RIB{Node: "r1", VRF: "default", Routes: []observation.RIBRoute{{
					Common: observation.RIBRouteCommon{AFI: model.AFIIPv4, Prefix: "10.0.0.0/24", Protocol: model.RouteSourceStatic, Eligible: true, Best: true},
					Static: &observation.StaticRIBRoute{NextHops: []observation.NextHop{{Address: "192.0.2.1"}}},
				}}},
				FIB: observation.FIB{Node: "r1", VRF: "default", Entries: []observation.FIBEntry{{
					AFI:      model.AFIIPv4,
					Prefix:   "10.0.0.0/24",
					Source:   observation.RouteSource{Protocol: model.RouteSourceStatic},
					Action:   observation.ActionForward,
					NextHops: []observation.NextHop{{Address: "192.0.2.1"}},
				}}},
			}},
		}},
	}

	a := observation.NewSnapshotBackedCollector(snapshot)
	b := observation.NewSnapshotBackedCollector(snapshot)
	result, err := observation.CompareCollectors(ctx, a, b, observation.CollectOptions{}, observation.SnapshotCompareOptions{IgnoreMetadata: true})
	if err != nil {
		t.Fatalf("CompareCollectors() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("snapshot vs snapshot mismatch: %#v", result)
	}
}

func testTopology() *model.Topology {
	staticPrefix := model.MustPrefix("203.0.113.0/24")
	return &model.Topology{
		Nodes: []model.Node{{
			Name:       "r1",
			Kind:       model.KindFRR,
			Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.1/31"}},
			Routes: []model.ConfiguredRoute{{
				Prefix:  staticPrefix,
				Kind:    model.RouteSourceStatic,
				NextHop: "192.0.2.0",
			}},
		}, {
			Name:       "r2",
			Kind:       model.KindFRR,
			Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.0/31"}},
		}},
		Links: []model.Link{{Name: "r1-r2", A: "r1", B: "r2", AIntf: "eth1", BIntf: "eth1", Cost: 1, Subnet: "192.0.2.0/31"}},
	}
}
