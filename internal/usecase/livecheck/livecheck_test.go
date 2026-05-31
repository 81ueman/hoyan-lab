package livecheck

import (
	"bytes"
	"context"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/81ueman/hoyan-lab/internal/adapter/inputhash"
	"github.com/81ueman/hoyan-lab/internal/adapter/snapshotfile"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/domain/query"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/livesnapshot"
	ribcompare "github.com/81ueman/hoyan-lab/internal/usecase/rib"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

type fakeRuntime struct {
	calls []string
}

func (f *fakeRuntime) BuildLocalImages(ctx context.Context, topologyPath string, out io.Writer) error {
	f.calls = append(f.calls, "build "+topologyPath)
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

type fakeQueryLoader struct {
	queries *query.Queries
}

func (f fakeQueryLoader) Load(path string) (*query.Queries, error) {
	if f.queries != nil {
		return f.queries, nil
	}
	return &query.Queries{}, nil
}

type fakeRIBCollector struct {
	supported []model.Node
	routes    [][]observation.RIBRoute
	errs      []error
	polls     int
}

func (f *fakeRIBCollector) SupportedNodes(nodes []model.Node) []model.Node {
	if f.supported != nil {
		return f.supported
	}
	return nodes
}

func (f *fakeRIBCollector) Collect(ctx context.Context, nodes []model.Node) ([]observation.RIBRoute, error) {
	return f.next()
}

func (f *fakeRIBCollector) CollectBGPRoutes(ctx context.Context, nodes []model.Node) ([]observation.RIBRoute, error) {
	return f.next()
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

type fakeFIBCollector struct {
	routes []observation.FIBEntry
}

func (f fakeFIBCollector) SupportedNodes(nodes []model.Node) []model.Node { return nodes }
func (f fakeFIBCollector) Collect(ctx context.Context, nodes []model.Node, opts observation.Options) ([]observation.FIBEntry, error) {
	return f.routes, nil
}

type fakeProber struct {
	reachable bool
}

func (f fakeProber) Probe(ctx context.Context, topo *model.Topology, check query.PacketCheck) (bool, error) {
	return f.reachable, nil
}

func deps(runtime *fakeRuntime, rib *fakeRIBCollector) Dependencies {
	return Dependencies{
		Runtime:         runtime,
		QueryLoader:     fakeQueryLoader{},
		RIBCollector:    rib,
		FIBCollector:    fakeFIBCollector{},
		DataplaneProber: fakeProber{reachable: true},
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
	nodes := topo.Nodes
	expected := (ribcompare.ExpectedBuilder{}).BuildForNodes(topo, nodes)
	rib := &fakeRIBCollector{supported: nodes, routes: [][]observation.RIBRoute{expected}}
	opts := Options{
		Topology:     "testdata/live.clab.yml",
		Timeout:      time.Second,
		PollInterval: time.Millisecond,
		Out:          io.Discard,
	}
	if err := New(deps(rt, rib)).Run(context.Background(), opts); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := rt.calls, []string{"build testdata/live.clab.yml", "deploy testdata/live.clab.yml", "wait-containers", "wait-srlinux", "nftables", "destroy testdata/live.clab.yml"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime calls = %v, want %v", got, want)
	}
}

func TestRunSnapshotOfflineDoesNotCallRuntimeOrCollectors(t *testing.T) {
	topologyPath := "testdata/live.clab.yml"
	topo, err := topology.LoadTopology(topologyPath)
	if err != nil {
		t.Fatalf("LoadTopology() error = %v", err)
	}
	expected := (ribcompare.ExpectedBuilder{}).BuildForNodes(topo, topo.Nodes)
	hashes, err := inputhash.InputHashes(topologyPath)
	if err != nil {
		t.Fatalf("InputHashes() error = %v", err)
	}
	snap := &livesnapshot.Snapshot{
		Version:      livesnapshot.Version,
		TopologyPath: topologyPath,
		TopologyHash: hashes.TopologyHash,
		ConfigHashes: hashes.ConfigHashes,
		CollectedAt:  time.Now().UTC(),
		Nodes:        map[string]livesnapshot.NodeSnapshot{},
	}
	for _, node := range topo.Nodes {
		ns := livesnapshot.NodeSnapshot{Kind: node.Kind}
		for _, route := range expected {
			if route.ModelInfo != nil && string(route.ModelInfo.Provenance.FromNode) == node.Name {
				ns.BGPRIB = append(ns.BGPRIB, route)
			}
		}
		snap.Nodes[node.Name] = ns
	}
	snapshotPath := filepath.Join(t.TempDir(), "snapshot.json")
	if err := snapshotfile.Save(snapshotPath, snap); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	rt := &fakeRuntime{}
	rib := &fakeRIBCollector{supported: topo.Nodes}
	opts := Options{Topology: topologyPath, Snapshot: snapshotPath, Offline: true, CheckFIB: false, Out: io.Discard}
	if err := New(deps(rt, rib)).Run(context.Background(), opts); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rt.calls) != 0 {
		t.Fatalf("runtime calls = %v, want none", rt.calls)
	}
	if rib.polls != 0 {
		t.Fatalf("collector polls = %d, want none", rib.polls)
	}
}

func TestRunDataplaneChecksUsesInjectedProber(t *testing.T) {
	reachable := true
	topo := &model.Topology{
		Nodes: []model.Node{{Name: "dst", Kind: model.KindFRR, Prefixes: model.MustPrefixes("10.0.0.10/32")}},
	}
	queries := &query.Queries{PacketChecks: []query.PacketCheck{{Name: "icmp-ok", From: "dst", To: "10.0.0.10", Protocol: "icmp", ExpectReachable: &reachable}}}
	if err := RunDataplaneChecks(context.Background(), fakeProber{reachable: true}, topo, queries, io.Discard); err != nil {
		t.Fatalf("RunDataplaneChecks() error = %v", err)
	}
	err := RunDataplaneChecks(context.Background(), fakeProber{reachable: false}, topo, queries, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "live dataplane reachable=false expected=true") {
		t.Fatalf("RunDataplaneChecks() error = %v", err)
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
	expected := (ribcompare.ExpectedBuilder{}).BuildForNodesWithFailureSet(topo, active, sim.NodeFailures("r1"))
	rib := &fakeRIBCollector{supported: active, routes: [][]observation.RIBRoute{expected}}
	err := CompareRIBsWithFailures(context.Background(), &fakeRuntime{}, rib, topo, RIBFailureScenario{
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
	expected := (ribcompare.ExpectedBuilder{}).BuildForNodes(topo, topo.Nodes)
	rib := &fakeRIBCollector{supported: topo.Nodes, routes: [][]observation.RIBRoute{expected}}
	deps := deps(rt, rib)
	deps.FIBCollector = fakeFIBCollector{routes: []observation.FIBEntry{{Node: "r1", VRF: "default", AFI: "ipv4", Prefix: "10.255.1.1/32", Protocol: "connected", Installed: true}}}
	opts := Options{Topology: "testdata/live.clab.yml", Timeout: time.Second, PollInterval: time.Millisecond, CheckFIB: true, FIBOptions: observation.Options{UnresolvedPolicy: observation.UnresolvedPolicyIgnore}, Out: &out}
	err = New(deps).Run(context.Background(), opts)
	if err == nil || !strings.Contains(err.Error(), "live FIB comparison found diff") {
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
		Common: observation.RIBRouteCommon{AFI: observation.AFIIPv4, Prefix: prefix, Protocol: observation.ProtocolBGP, Eligible: true, Best: best},
		BGP:    &observation.BGPRIBRoute{Paths: []observation.BGPPath{path}},
	}
}
