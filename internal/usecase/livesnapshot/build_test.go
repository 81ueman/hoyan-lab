package livesnapshot

import (
	"context"
	"testing"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	observationfib "github.com/81ueman/hoyan-lab/internal/domain/observation/fib"
	observationrib "github.com/81ueman/hoyan-lab/internal/domain/observation/rib"
	snapshotdomain "github.com/81ueman/hoyan-lab/internal/domain/snapshot"
)

func TestBuildUsesInjectedMetadataProviders(t *testing.T) {
	now := time.Date(2026, 5, 30, 7, 30, 0, 0, time.UTC)
	u := New(
		fakeRIBCollector{},
		fakeFIBCollector{},
		WithHashProvider(fakeHashProvider{}),
		WithCommitProvider(fakeCommitProvider{}),
		WithClock(func() time.Time { return now }),
	)
	snap, err := u.Build(context.Background(), "../livecheck/testdata/live.clab.yml", "unit-lab", observationfib.Options{})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if snap.TopologyHash != "topology-sha" || snap.ConfigHashes["frr.conf"] != "config-sha" {
		t.Fatalf("snapshot hashes = %#v, %#v", snap.TopologyHash, snap.ConfigHashes)
	}
	if snap.GitCommit != "commit-sha" {
		t.Fatalf("GitCommit = %q", snap.GitCommit)
	}
	if !snap.CollectedAt.Equal(now) {
		t.Fatalf("CollectedAt = %s, want %s", snap.CollectedAt, now)
	}
	if got := BGPRoutes(snap); len(got) != 1 || got[0].Prefix != "10.0.0.0/24" {
		t.Fatalf("BGPRoutes() = %#v", got)
	}
}

type fakeHashProvider struct{}

func (fakeHashProvider) InputHashes(string) (snapshotdomain.InputHashSet, error) {
	return snapshotdomain.InputHashSet{
		TopologyHash: "topology-sha",
		ConfigHashes: map[string]string{"frr.conf": "config-sha"},
	}, nil
}

type fakeCommitProvider struct{}

func (fakeCommitProvider) Commit() string {
	return "commit-sha"
}

type fakeRIBCollector struct{}

func (fakeRIBCollector) CollectBGPRoutes(context.Context, []model.Node) ([]observationrib.NormalizedRoute, error) {
	return []observationrib.NormalizedRoute{{
		Node:            "r1",
		NetworkInstance: "default",
		AFI:             "ipv4",
		Prefix:          "10.0.0.0/24",
		Protocol:        "bgp",
	}}, nil
}

func (fakeRIBCollector) CollectOSPFRoutes(context.Context, []model.Node) ([]observationrib.NormalizedRoute, error) {
	return nil, nil
}

func (fakeRIBCollector) CollectRouteTableRoutes(context.Context, []model.Node) ([]observationrib.NormalizedRoute, error) {
	return nil, nil
}

type fakeFIBCollector struct{}

func (fakeFIBCollector) Collect(context.Context, []model.Node, observationfib.Options) ([]observationfib.NormalizedFIBRoute, error) {
	return nil, nil
}

func (fakeFIBCollector) SupportedNodes(nodes []model.Node) []model.Node {
	return nodes
}
