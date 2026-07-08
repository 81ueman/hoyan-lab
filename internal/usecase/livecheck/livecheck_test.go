package livecheck

import (
	"bytes"
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	snapshotdomain "github.com/81ueman/hoyan-lab/internal/domain/snapshot"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/collect"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

type fakeRuntime struct {
	calls []string
}

func (f *fakeRuntime) BuildLocalImages(ctx context.Context, topologyPath string, out io.Writer) error {
	f.calls = append(f.calls, "build "+topologyPath)
	return nil
}

func (f *fakeRuntime) Start(ctx context.Context, topologyPath string, topo *model.Topology, pollInterval time.Duration, out io.Writer) error {
	f.calls = append(f.calls, "start "+topologyPath)
	return nil
}

func (f *fakeRuntime) Stop(ctx context.Context, topologyPath string) error {
	f.calls = append(f.calls, "destroy "+topologyPath)
	return nil
}

func (f *fakeRuntime) Deploy(ctx context.Context, topologyPath string) error {
	f.calls = append(f.calls, "deploy "+topologyPath)
	return nil
}

func (f *fakeRuntime) Destroy(ctx context.Context, topologyPath string) error {
	f.calls = append(f.calls, "destroy "+topologyPath)
	return nil
}

func (f *fakeRuntime) WaitContainers(ctx context.Context, nodes []model.Node, interval time.Duration) error {
	f.calls = append(f.calls, "wait-containers")
	return nil
}

func (f *fakeRuntime) WaitSRLinuxCLI(ctx context.Context, nodes []model.Node, interval time.Duration) error {
	f.calls = append(f.calls, "wait-srlinux")
	return nil
}

func (f *fakeRuntime) ApplyNftablesPolicies(ctx context.Context, topo *model.Topology, out io.Writer) error {
	f.calls = append(f.calls, "nftables")
	return nil
}

func (f *fakeRuntime) SetLinkLoss(ctx context.Context, topo *model.Topology, node, intf string, lossPercent int) error {
	f.calls = append(f.calls, "loss "+node+" "+intf)
	return nil
}

func (f *fakeRuntime) ResetLinkLoss(ctx context.Context, topo *model.Topology, node, intf string) error {
	f.calls = append(f.calls, "reset "+node+" "+intf)
	return nil
}

func (f *fakeRuntime) StopNode(ctx context.Context, node model.Node) error {
	f.calls = append(f.calls, "stop "+node.Name)
	return nil
}

type fakeRIBCollector struct {
	supported []model.Node
	routes    [][]observation.RIBRoute
	errs      []error
	polls     int
}

type fakeSnapshotRepository struct {
	snap *snapshotdomain.Snapshot
	err  error
}

func (f fakeSnapshotRepository) Load(path string) (*snapshotdomain.Snapshot, error) {
	return f.snap, f.err
}

type fakeInputHashChecker struct {
	result snapshotdomain.HashCheckResult
	err    error
}

func (f fakeInputHashChecker) CheckHashes(topologyPath string, snap *snapshotdomain.Snapshot) (snapshotdomain.HashCheckResult, error) {
	return f.result, f.err
}

func (f *fakeRIBCollector) CollectRIB(ctx context.Context, node model.NodeID, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error) {
	routes, err := f.next()
	if err != nil {
		return observation.RIB{}, err
	}
	return observation.FilterRIB(observation.RIB{Node: node, VRF: model.NormalizeNetworkInstance(string(vrf)), Routes: routes}, opts), nil
}

func (f *fakeRIBCollector) next() ([]observation.RIBRoute, error) {
	i := f.polls
	f.polls++
	if i < len(f.errs) && f.errs[i] != nil {
		return nil, f.errs[i]
	}
	if i >= len(f.routes) {
		i = len(f.routes) - 1
	}
	if i < 0 {
		return nil, nil
	}
	return f.routes[i], nil
}

func deps(runtime *fakeRuntime, collector collect.Collector) Dependencies {
	return Dependencies{
		Runtime:   runtime,
		Collector: collector,
	}
}

func newSnapshotCollector(snap observation.NetworkSnapshot) collect.Collector {
	return observation.NewSnapshotBackedCollector(snap)
}

func newTestUsecase(t *testing.T, deps Dependencies) Usecase {
	t.Helper()
	u, err := New(deps)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return u
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	valid := deps(&fakeRuntime{}, newSnapshotCollector(observation.NetworkSnapshot{}))
	tests := []struct {
		name string
		deps Dependencies
		want string
	}{
		{name: "runtime", deps: Dependencies{Collector: valid.Collector}, want: "runtime"},
		{name: "collector", deps: Dependencies{Runtime: valid.Runtime}, want: "collector"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.deps)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("New() error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestHasExpectedRoutes(t *testing.T) {
	expected := []observation.RIBRoute{
		testRIBRoute("10.0.0.0/24", "", true),
		testRIBRoute("10.1.0.0/24", "", true),
	}
	actual := []observation.RIBRoute{testRIBRoute("10.0.0.0/24", "", true)}
	if HasExpectedRoutes(expected, actual) {
		t.Fatalf("routes should be incomplete")
	}
	actual = append(actual, testRIBRoute("10.1.0.0/24", "", true))
	if !HasExpectedRoutes(expected, actual) {
		t.Fatalf("routes should be complete")
	}
	if got := CountExpectedRoutes(expected, actual); got != 2 {
		t.Fatalf("CountExpectedRoutes() = %d, want 2", got)
	}
}

func TestRunDestroysOnSuccess(t *testing.T) {
	rt := &fakeRuntime{}
	topo, err := topology.LoadTopology("testdata/live.clab.yml")
	if err != nil {
		t.Fatalf("LoadTopology() error = %v", err)
	}
	simulator, err := collect.NewSimulator(topo)
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	expected, err := collect.CollectSnapshot(context.Background(), simulator, observation.CollectOptions{IncludeInactive: true, IncludeModelInfo: true})
	if err != nil {
		t.Fatalf("CollectSnapshot() error = %v", err)
	}
	collector := newSnapshotCollector(expected)
	opts := Options{
		Topology:     "testdata/live.clab.yml",
		Timeout:      time.Second,
		PollInterval: time.Millisecond,
		Out:          io.Discard,
	}
	if err := newTestUsecase(t, deps(rt, collector)).Run(context.Background(), opts); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := rt.calls, []string{"start testdata/live.clab.yml", "destroy testdata/live.clab.yml"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime calls = %v, want %v", got, want)
	}
}

func TestRunSnapshotOfflineDoesNotCallRuntimeOrCollectors(t *testing.T) {
	topologyPath := "testdata/live.clab.yml"
	topo, err := topology.LoadTopology(topologyPath)
	if err != nil {
		t.Fatalf("LoadTopology() error = %v", err)
	}
	simulator, err := collect.NewSimulator(topo)
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	expected, err := collectRIBRoutes(context.Background(), simulator, topo.Nodes, observation.CollectOptions{IncludeInactive: true, IncludeModelInfo: true})
	if err != nil {
		t.Fatalf("collectRIBRoutes() error = %v", err)
	}
	snap := &snapshotdomain.Snapshot{
		Version:      snapshotdomain.Version,
		TopologyPath: topologyPath,
		CollectedAt:  time.Now().UTC(),
		Nodes:        map[string]snapshotdomain.NodeSnapshot{},
		Network:      observation.NetworkSnapshot{Nodes: make([]observation.NodeSnapshot, 0, len(topo.Nodes))},
	}
	for _, node := range topo.Nodes {
		snap.Nodes[node.Name] = snapshotdomain.NodeSnapshot{Kind: node.Kind}
		vrf := observation.VRFSnapshot{
			VRF: model.NetworkInstanceDefault,
			RIB: observation.RIB{Node: model.NodeID(node.Name), VRF: model.NetworkInstanceDefault},
		}
		for _, route := range expected {
			if route.ModelInfo != nil && string(route.ModelInfo.Provenance.FromNode) == node.Name {
				vrf.RIB.Routes = append(vrf.RIB.Routes, route)
			}
		}
		snap.Network.Nodes = append(snap.Network.Nodes, observation.NodeSnapshot{Node: model.NodeID(node.Name), VRFs: []observation.VRFSnapshot{vrf}})
	}
	rt := &fakeRuntime{}
	opts := Options{Topology: topologyPath, Snapshot: "snapshot.json", Offline: true, CheckFIB: false, Out: io.Discard}
	usecase, err := New(Dependencies{
		Runtime:            rt,
		Collector:          newSnapshotCollector(observation.NetworkSnapshot{}),
		SnapshotRepository: fakeSnapshotRepository{snap: snap},
		InputHashChecker:   fakeInputHashChecker{},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if err := usecase.Run(context.Background(), opts); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rt.calls) != 0 {
		t.Fatalf("runtime calls = %v, want none", rt.calls)
	}
}

func TestWaitForExpectedRoutesStopsAfterMaxPolls(t *testing.T) {
	rib := &fakeRIBCollector{routes: [][]observation.RIBRoute{
		{testRIBRoute("10.0.0.0/24", "", true)},
		{testRIBRoute("10.0.0.0/24", "", true)},
	}}
	nodes := []model.Node{{Name: "r1", Kind: "frr"}}
	expected := []observation.RIBRoute{testRIBRoute("10.0.0.0/24", "", true), testRIBRoute("10.1.0.0/24", "", true)}
	actual, err := WaitForExpectedRoutes(context.Background(), rib, nodes, expected, time.Millisecond, 2)
	if err == nil {
		t.Fatalf("WaitForExpectedRoutes() succeeded unexpectedly")
	}
	if len(actual) != 1 {
		t.Fatalf("actual routes = %d, want last successful collection", len(actual))
	}
	if rib.polls != 2 {
		t.Fatalf("polls = %d, want 2", rib.polls)
	}
}

func TestWaitForMatchingRIBsPollsUntilDiffsClear(t *testing.T) {
	expected := []observation.RIBRoute{testRIBRoute("10.0.0.0/24", "198.51.100.1", true)}
	rib := &fakeRIBCollector{routes: [][]observation.RIBRoute{
		{testRIBRoute("10.0.0.0/24", "192.0.2.1", true)},
		expected,
	}}
	_, diffs, err := WaitForMatchingRIBs(context.Background(), rib, []model.Node{{Name: "r1", Kind: "frr"}}, expected, time.Millisecond, 2, observation.DefaultCompareOptions())
	if err != nil {
		t.Fatalf("WaitForMatchingRIBs() error = %v", err)
	}
	if !diffs.OK {
		t.Fatalf("diffs = %v, want none", diffs)
	}
	if rib.polls != 2 {
		t.Fatalf("polls = %d, want 2", rib.polls)
	}
}

func TestWaitForMatchingRIBsClearsTransientCollectionError(t *testing.T) {
	expected := []observation.RIBRoute{testRIBRoute("10.0.0.0/24", "192.0.2.1", true)}
	rib := &fakeRIBCollector{
		errs:   []error{errors.New("transient collector error"), nil},
		routes: [][]observation.RIBRoute{nil, {testRIBRoute("10.0.0.0/24", "192.0.2.1", false)}},
	}
	_, diffs, err := WaitForMatchingRIBs(context.Background(), rib, []model.Node{{Name: "r1", Kind: "frr"}}, expected, time.Millisecond, 2, observation.DefaultCompareOptions())
	if err == nil {
		t.Fatalf("WaitForMatchingRIBs() succeeded unexpectedly")
	}
	if strings.Contains(err.Error(), "transient collector error") {
		t.Fatalf("WaitForMatchingRIBs() retained stale collection error: %v", err)
	}
	if len(diffs.Mismatched) != 1 || diffs.Mismatched[0].Field != "best" {
		t.Fatalf("diffs = %#v", diffs)
	}
}

func TestLinkFailureScenarioUsesRuntimeForBothEndpoints(t *testing.T) {
	topo := &model.Topology{
		Name: "test-lab",
		Nodes: []model.Node{
			{Name: "a", Kind: model.KindFRR, Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.1/24"}}},
			{Name: "b", Kind: model.KindCEOS, Interfaces: []model.Interface{{Name: "Ethernet2", Address: "192.0.2.2/24"}}},
		},
		Links: []model.Link{{Name: "a-b", A: "a", B: "b", AIntf: "eth1", BIntf: "eth2"}},
	}
	scenario, err := LinkFailureScenario(topo, "a-b")
	if err != nil {
		t.Fatalf("LinkFailureScenario() error = %v", err)
	}
	rt := &fakeRuntime{}
	if err := scenario.Inject(context.Background(), rt); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if err := scenario.Cleanup(context.Background(), rt); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	want := []string{"loss a eth1", "loss b eth2", "reset a eth1", "reset b eth2"}
	if !reflect.DeepEqual(rt.calls, want) {
		t.Fatalf("calls = %v, want %v", rt.calls, want)
	}
	if !scenario.Failures.Links["a-b"] {
		t.Fatalf("scenario failures = %#v, want link a-b", scenario.Failures)
	}
}

func TestNodeFailureScenarioStopsNodeThroughRuntime(t *testing.T) {
	topo := &model.Topology{Nodes: []model.Node{{Name: "r1", Kind: "frr"}, {Name: "r2", Kind: "frr"}}}
	scenario, err := NodeFailureScenario(topo, "r1")
	if err != nil {
		t.Fatalf("NodeFailureScenario() error = %v", err)
	}
	rt := &fakeRuntime{}
	if err := scenario.Inject(context.Background(), rt); err != nil {
		t.Fatalf("Inject() error = %v", err)
	}
	if got, want := rt.calls, []string{"stop r1"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if !scenario.Failures.Nodes["r1"] {
		t.Fatalf("scenario failures = %#v, want node r1", scenario.Failures)
	}
	if got, want := scenario.ActiveNodes, []model.Node{{Name: "r2", Kind: "frr"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("active nodes = %#v, want %#v", got, want)
	}
}

func TestCompareRIBsWithFailuresUsesFailureAwareExpectedRoutes(t *testing.T) {
	topo := &model.Topology{Nodes: []model.Node{
		{Name: "r1", Kind: "frr", ASN: 65001, Prefixes: model.MustPrefixes("10.0.0.0/24")},
		{Name: "r2", Kind: "frr", ASN: 65002, Prefixes: model.MustPrefixes("10.1.0.0/24")},
	}}
	active := []model.Node{{Name: "r2", Kind: "frr", ASN: 65002, Prefixes: model.MustPrefixes("10.1.0.0/24")}}
	simulator, err := collect.NewSimulator(topo)
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	expected, err := collectRIBRoutes(context.Background(), simulator.CollectorFor(sim.NodeFailures("r1")), active, observation.CollectOptions{IncludeInactive: true, IncludeModelInfo: true})
	if err != nil {
		t.Fatalf("collectRIBRoutes() error = %v", err)
	}
	rib := &fakeRIBCollector{supported: active, routes: [][]observation.RIBRoute{expected}}
	err = CompareRIBsWithFailures(context.Background(), &fakeRuntime{}, rib, topo, RIBFailureScenario{
		Name:        "node-r1",
		Failures:    sim.NodeFailures("r1"),
		ActiveNodes: active,
	}, RIBFailureCheckOptions{Interval: time.Millisecond, MaxPolls: 1, Out: io.Discard})
	if err != nil {
		t.Fatalf("CompareRIBsWithFailures() error = %v", err)
	}
	if rib.polls != 1 {
		t.Fatalf("polls = %d, want 1", rib.polls)
	}
}

func TestRunCheckFIBUsesInjectedCollector(t *testing.T) {
	var out bytes.Buffer
	rt := &fakeRuntime{}
	topo, err := topology.LoadTopology("testdata/live.clab.yml")
	if err != nil {
		t.Fatalf("LoadTopology() error = %v", err)
	}
	simulator, err := collect.NewSimulator(topo)
	if err != nil {
		t.Fatalf("NewSimulator() error = %v", err)
	}
	expected, err := collect.CollectSnapshot(context.Background(), simulator, observation.CollectOptions{IncludeInactive: true, IncludeModelInfo: true})
	if err != nil {
		t.Fatalf("CollectSnapshot() error = %v", err)
	}
	actual := expected
	actual.Nodes[0].VRFs[0].FIB = observation.FIB{Node: actual.Nodes[0].Node, VRF: actual.Nodes[0].VRFs[0].VRF, Entries: []observation.FIBEntry{{AFI: "ipv4", Prefix: "10.255.1.1/32", Source: observation.RouteSource{Protocol: model.RouteSourceConnected}, Action: observation.ActionReceive}}}
	deps := deps(rt, newSnapshotCollector(actual))
	opts := Options{Topology: "testdata/live.clab.yml", Timeout: time.Second, PollInterval: time.Millisecond, CheckFIB: true, FIBOptions: observation.Options{UnresolvedPolicy: observation.UnresolvedPolicyIgnore}, Out: &out}
	err = newTestUsecase(t, deps).Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "collector snapshots did not converge") {
		t.Fatalf("Run() error = %v, want FIB diff from injected collector", err)
	}
}

func testRIBRoute(prefix, nextHop string, best bool) observation.RIBRoute {
	path := observation.BGPPath{
		Best:      best,
		Eligible:  true,
		NextHop:   observation.NextHop{Address: nextHop},
		Origin:    "igp",
		LocalPref: 100,
	}
	return observation.RIBRoute{
		Common: observation.RIBRouteCommon{AFI: model.AFIIPv4, Prefix: prefix, Protocol: model.RouteSourceBGP, Eligible: true, Best: best},
		BGP:    &observation.BGPRIBRoute{Paths: []observation.BGPPath{path}},
	}
}
