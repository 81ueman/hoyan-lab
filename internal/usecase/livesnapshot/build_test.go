package livesnapshot

import (
	"context"
	"testing"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
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
	snap, err := u.Build(context.Background(), "../livecheck/testdata/live.clab.yml", "unit-lab", observation.Options{})
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
	if got := BGPRoutes(snap); len(got) != 1 || got[0].Common.Prefix != "10.0.0.0/24" {
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

func (fakeRIBCollector) CollectRIB(_ context.Context, node model.Node, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error) {
	routes := []observation.RIBRoute{{
		Common: observation.RIBRouteCommon{AFI: model.AFIIPv4, Prefix: "10.0.0.0/24", Protocol: model.RouteSourceBGP, Eligible: true, Best: true},
		BGP:    &observation.BGPRIBRoute{Paths: []observation.BGPPath{{Eligible: true, Best: true}}},
	}}
	return observation.FilterRIB(observation.RIB{Node: model.NodeID(node.Name), VRF: model.NormalizeNetworkInstance(string(vrf)), Routes: routes}, opts), nil
}

type fakeFIBCollector struct{}

func (fakeFIBCollector) CollectFIB(context.Context, model.Node, model.NetworkInstanceID, observation.Options) (observation.FIB, error) {
	return observation.FIB{}, nil
}
