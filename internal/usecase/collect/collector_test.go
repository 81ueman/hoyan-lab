package collect

import (
	"context"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	fibusecase "github.com/81ueman/hoyan-lab/internal/usecase/fib"
	ribusecase "github.com/81ueman/hoyan-lab/internal/usecase/rib"
)

func TestSimulatorCollectorCanCompareWithSnapshotBackedCollector(t *testing.T) {
	ctx := context.Background()
	simulator := NewSimulator(testTopology())
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

func TestSimulatorAndContainerlabStyleCollectorsUseSameInterface(t *testing.T) {
	ctx := context.Background()
	topo := testTopology()
	simulator := NewSimulator(topo)
	containerlab := NewContainerlabCollector(
		topo.Nodes,
		fakeRIBCollector{routes: (ribusecase.ExpectedBuilder{}).Build(topo)},
		fakeFIBCollector{fibs: fibusecase.NewExpectedBuilder().ExpectedFIBs(topo)},
		observation.Options{},
	)

	var _ observation.Collector = simulator
	var _ observation.Collector = containerlab

	result, err := observation.CompareCollectors(ctx, simulator, containerlab, observation.CollectOptions{}, observation.SnapshotCompareOptions{IgnoreMetadata: true})
	if err != nil {
		t.Fatalf("CompareCollectors() error = %v", err)
	}
	if !result.OK {
		t.Fatalf("simulator vs containerlab-style mismatch: %#v", result)
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

type fakeRIBCollector struct {
	routes []observation.RIBRoute
}

func (f fakeRIBCollector) CollectRIB(_ context.Context, node model.Node, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error) {
	var out []observation.RIBRoute
	for _, route := range f.routes {
		if route.ModelInfo == nil || route.ModelInfo.Provenance.FromNode == model.NodeID(node.Name) {
			out = append(out, route)
		}
	}
	observation.SortRoutes(out)
	return observation.FilterRIB(observation.RIB{Node: model.NodeID(node.Name), VRF: model.NormalizeNetworkInstance(string(vrf)), Routes: out}, opts), nil
}

type fakeFIBCollector struct {
	fibs []observation.FIB
}

func (f fakeFIBCollector) CollectFIB(_ context.Context, node model.Node, vrf model.NetworkInstanceID, _ observation.Options) (observation.FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	for _, fib := range f.fibs {
		if fib.Node == model.NodeID(node.Name) && fib.VRF == vrf {
			return fib, nil
		}
	}
	return observation.FIB{Node: model.NodeID(node.Name), VRF: vrf}, nil
}
