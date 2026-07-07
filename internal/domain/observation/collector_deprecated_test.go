package observation

import (
	"context"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type deprecatedSnapshotCollector struct{}

func (deprecatedSnapshotCollector) Metadata(context.Context) CollectorMetadata {
	return CollectorMetadata{Source: "test", Labels: map[string]string{"env": "unit"}}
}

func (deprecatedSnapshotCollector) Nodes(context.Context) ([]model.NodeID, error) {
	return []model.NodeID{"r2", "r1"}, nil
}

func (deprecatedSnapshotCollector) VRFs(context.Context, model.NodeID) ([]model.NetworkInstanceID, error) {
	return []model.NetworkInstanceID{model.NetworkInstanceDefault}, nil
}

func (deprecatedSnapshotCollector) CollectRIB(_ context.Context, node model.Node, vrf model.NetworkInstanceID, _ CollectOptions) (RIB, error) {
	return RIB{Node: model.NodeID(node.Name), VRF: vrf, Routes: []RIBRoute{{
		Common: RIBRouteCommon{AFI: model.AFIIPv4, Prefix: "10.0.0.0/24", Protocol: model.RouteSourceStatic, Eligible: true, Best: true},
	}}}, nil
}

func (deprecatedSnapshotCollector) CollectFIB(_ context.Context, node model.Node, vrf model.NetworkInstanceID, _ Options) (FIB, error) {
	return FIB{Node: model.NodeID(node.Name), VRF: vrf, Entries: []FIBEntry{{
		AFI:    model.AFIIPv4,
		Prefix: "10.0.0.0/24",
		Source: RouteSource{Protocol: model.RouteSourceStatic},
		Action: ActionForward,
	}}}, nil
}

func TestDeprecatedCollectSnapshotCompatibility(t *testing.T) {
	snapshot, err := CollectSnapshot(t.Context(), deprecatedSnapshotCollector{}, CollectOptions{})
	if err != nil {
		t.Fatalf("CollectSnapshot() error = %v", err)
	}
	if got, want := snapshot.Metadata.Source, "test"; got != want {
		t.Fatalf("Metadata.Source = %q, want %q", got, want)
	}
	if got, want := snapshot.Metadata.Labels["env"], "unit"; got != want {
		t.Fatalf("Metadata.Labels[env] = %q, want %q", got, want)
	}
	if len(snapshot.Nodes) != 2 {
		t.Fatalf("len(snapshot.Nodes) = %d, want 2", len(snapshot.Nodes))
	}
	if got, want := snapshot.Nodes[0].Node, model.NodeID("r1"); got != want {
		t.Fatalf("first node = %q, want %q", got, want)
	}
	if len(snapshot.Nodes[0].VRFs) != 1 {
		t.Fatalf("len(snapshot.Nodes[0].VRFs) = %d, want 1", len(snapshot.Nodes[0].VRFs))
	}
}
