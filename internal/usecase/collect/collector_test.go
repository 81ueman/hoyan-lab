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
					Common: observation.RIBRouteCommon{AFI: observation.AFIIPv4, Prefix: "10.0.0.0/24", Protocol: observation.ProtocolStatic, Eligible: true, Best: true},
					Static: &observation.StaticRIBRoute{NextHops: []observation.NextHop{{Address: "192.0.2.1"}}},
				}}},
				FIB: observation.FIB{Node: "r1", VRF: "default", Entries: []observation.FIBEntry{{
					AFI:      observation.AFIIPv4,
					Prefix:   "10.0.0.0/24",
					Source:   observation.RouteSource{Protocol: observation.ProtocolStatic},
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
		fakeFIBCollector{routes: fibusecase.NewExpectedBuilder().Expected(topo)},
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

func (f fakeRIBCollector) CollectBGPRoutes(_ context.Context, nodes []model.Node) ([]observation.RIBRoute, error) {
	return f.routesFor(nodes, true), nil
}

func (f fakeRIBCollector) CollectRouteTableRoutes(_ context.Context, nodes []model.Node) ([]observation.RIBRoute, error) {
	return f.routesFor(nodes, false), nil
}

func (f fakeRIBCollector) routesFor(nodes []model.Node, bgp bool) []observation.RIBRoute {
	allowed := map[string]bool{}
	for _, node := range nodes {
		allowed[node.Name] = true
	}
	var out []observation.RIBRoute
	for _, route := range f.routes {
		route = observation.NormalizeRIBRouteRecord(route)
		if allowed[route.Node] && ((route.Protocol == "bgp") == bgp) {
			out = append(out, route)
		}
	}
	observation.SortRoutes(out)
	return out
}

type fakeFIBCollector struct {
	routes []observation.FIBEntry
}

func (f fakeFIBCollector) Collect(_ context.Context, nodes []model.Node, _ observation.Options) ([]observation.FIBEntry, error) {
	allowed := map[string]bool{}
	for _, node := range nodes {
		allowed[node.Name] = true
	}
	var out []observation.FIBEntry
	for _, route := range f.routes {
		if allowed[route.Node] {
			out = append(out, route)
		}
	}
	observation.SortFIBEntriesForCompare(out)
	return out, nil
}
