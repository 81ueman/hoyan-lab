package intent

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func TestValidateCompareRelation(t *testing.T) {
	snapshots := map[string]Snapshot{
		"pre":  {Lab: "labs/pre"},
		"post": {Lab: "labs/post"},
	}
	scenarios := map[string]Scenario{}

	validRelations := []string{"equal", "added_count", "removed_count", "changed_count", "no_change"}
	for _, rel := range validRelations {
		t.Run("valid/"+rel, func(t *testing.T) {
			doc := &Document{
				Version:   "hoyan/v1",
				Snapshots: snapshots,
				Scenarios: scenarios,
				Intents: []Intent{{
					Name: "test",
					Check: Check{
						Compare: &CompareCheck{
							Table:    "rib",
							Left:     CompareSide{Snapshot: "pre"},
							Right:    CompareSide{Snapshot: "post"},
							Relation: rel,
						},
					},
				}},
			}
			if err := Validate(doc); err != nil {
				t.Fatalf("Validate() error for relation %q: %v", rel, err)
			}
		})
	}

	t.Run("invalid/unknown", func(t *testing.T) {
		doc := &Document{
			Version:   "hoyan/v1",
			Snapshots: snapshots,
			Scenarios: scenarios,
			Intents: []Intent{{
				Name: "test",
				Check: Check{
					Compare: &CompareCheck{
						Table:    "rib",
						Left:     CompareSide{Snapshot: "pre"},
						Right:    CompareSide{Snapshot: "post"},
						Relation: "invalid_relation",
					},
				},
			}},
		}
		err := Validate(doc)
		if err == nil {
			t.Fatal("Validate() expected error for unknown relation, got nil")
		}
	})
}

func TestEvaluateCompareRelationPass(t *testing.T) {
	// Identical routes on both sides → all relations pass (no diff).
	route := observation.RIBRoute{
		Common: observation.RIBRouteCommon{
			AFI:      model.AFIIPv4,
			Prefix:   "10.0.0.0/24",
			Protocol: model.RouteSourceStatic,
			Eligible: true,
			Best:     true,
		},
		Static: &observation.StaticRIBRoute{},
	}
	ctx := SnapshotContext{
		Name:    "snap",
		LabPath: "labs/snap",
		Network: observation.NetworkSnapshot{
			Nodes: []observation.NodeSnapshot{{
				Node: "leaf1",
				VRFs: []observation.VRFSnapshot{{
					VRF: "default",
					RIB: observation.RIB{Node: "leaf1", VRF: "default", Routes: []observation.RIBRoute{route}},
					FIB: observation.FIB{Node: "leaf1", VRF: "default"},
				}},
			}},
		},
	}
	provider := &fakeSnapshotProvider{
		snapshots: map[string]SnapshotContext{"snap": ctx},
	}

	relations := []string{"equal", "added_count", "removed_count", "changed_count", "no_change"}
	for _, rel := range relations {
		t.Run("pass/"+rel, func(t *testing.T) {
			doc := &Document{
				Version:   "hoyan/v1",
				Snapshots: map[string]Snapshot{"snap": {Lab: "labs/snap"}},
				Intents: []Intent{{
					Name: "test-" + rel,
					Check: Check{
						Compare: &CompareCheck{
							Table:    "rib",
							Left:     CompareSide{Snapshot: "snap"},
							Right:    CompareSide{Snapshot: "snap"},
							Relation: rel,
						},
					},
				}},
			}
			report, err := VerifyWithProvider(doc, provider)
			if err != nil {
				t.Fatalf("VerifyWithProvider() error: %v", err)
			}
			if len(report.Results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(report.Results))
			}
			if report.Results[0].Status != "pass" {
				t.Fatalf("relation %q: expected pass, got %q with reason %q", rel, report.Results[0].Status, report.Results[0].Actual.Reason)
			}
			if report.Results[0].Actual.AddedCount != 0 || report.Results[0].Actual.RemovedCount != 0 || report.Results[0].Actual.ChangedCount != 0 {
				t.Fatalf("relation %q: expected all counts 0, got added=%d removed=%d changed=%d", rel, report.Results[0].Actual.AddedCount, report.Results[0].Actual.RemovedCount, report.Results[0].Actual.ChangedCount)
			}
		})
	}
}

func TestEvaluateCompareRelationFail(t *testing.T) {
	// Two routes in pre, one different route in post → added=routeB, removed=routeA.
	routeA := observation.RIBRoute{
		Common: observation.RIBRouteCommon{
			AFI:      model.AFIIPv4,
			Prefix:   "10.0.0.0/24",
			Protocol: model.RouteSourceStatic,
			Eligible: true,
			Best:     true,
		},
		Static: &observation.StaticRIBRoute{},
	}
	routeB := observation.RIBRoute{
		Common: observation.RIBRouteCommon{
			AFI:      model.AFIIPv4,
			Prefix:   "10.1.0.0/24",
			Protocol: model.RouteSourceStatic,
			Eligible: true,
			Best:     true,
		},
		Static: &observation.StaticRIBRoute{},
	}
	routeC := observation.RIBRoute{
		Common: observation.RIBRouteCommon{
			AFI:      model.AFIIPv4,
			Prefix:   "10.2.0.0/24",
			Protocol: model.RouteSourceStatic,
			Eligible: true,
			Best:     true,
		},
		Static: &observation.StaticRIBRoute{},
	}

	// pre has routeA + routeB, post has routeA + routeC
	// → added=[routeC], removed=[routeB] → addedCount=1, removedCount=1, changedCount=2
	preCtx := SnapshotContext{
		Name:    "pre",
		LabPath: "labs/pre",
		Network: observation.NetworkSnapshot{
			Nodes: []observation.NodeSnapshot{{
				Node: "leaf1",
				VRFs: []observation.VRFSnapshot{{
					VRF: "default",
					RIB: observation.RIB{Node: "leaf1", VRF: "default", Routes: []observation.RIBRoute{routeA, routeB}},
					FIB: observation.FIB{Node: "leaf1", VRF: "default"},
				}},
			}},
		},
	}
	postCtx := SnapshotContext{
		Name:    "post",
		LabPath: "labs/post",
		Network: observation.NetworkSnapshot{
			Nodes: []observation.NodeSnapshot{{
				Node: "leaf1",
				VRFs: []observation.VRFSnapshot{{
					VRF: "default",
					RIB: observation.RIB{Node: "leaf1", VRF: "default", Routes: []observation.RIBRoute{routeA, routeC}},
					FIB: observation.FIB{Node: "leaf1", VRF: "default"},
				}},
			}},
		},
	}
	provider := &fakeSnapshotProvider{
		snapshots: map[string]SnapshotContext{"pre": preCtx, "post": postCtx},
	}

	tests := []struct {
		name      string
		relation  string
		wantPass  bool
		wantAdded int
		wantRemoved int
		wantChanged int
	}{
		// equal / changed_count / no_change: any change → fail
		{"equal", "equal", false, 1, 1, 2},
		{"changed_count", "changed_count", false, 1, 1, 2},
		{"no_change", "no_change", false, 1, 1, 2},
		// added_count: only checks added → fail (1 added)
		{"added_count", "added_count", false, 1, 1, 2},
		// removed_count: only checks removed → fail (1 removed)
		{"removed_count", "removed_count", false, 1, 1, 2},
	}

	for _, tt := range tests {
		t.Run("fail/"+tt.name, func(t *testing.T) {
			doc := &Document{
				Version:   "hoyan/v1",
				Snapshots: map[string]Snapshot{"pre": {Lab: "labs/pre"}, "post": {Lab: "labs/post"}},
				Intents: []Intent{{
					Name: "test-" + tt.relation,
					Check: Check{
						Compare: &CompareCheck{
							Table:    "rib",
							Left:     CompareSide{Snapshot: "pre"},
							Right:    CompareSide{Snapshot: "post"},
							Relation: tt.relation,
						},
					},
				}},
			}
			report, err := VerifyWithProvider(doc, provider)
			if err != nil {
				t.Fatalf("VerifyWithProvider() error: %v", err)
			}
			if len(report.Results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(report.Results))
			}
			gotPass := report.Results[0].Status == "pass"
			if gotPass != tt.wantPass {
				t.Fatalf("relation %q: status=%s, want pass=%v", tt.relation, report.Results[0].Status, tt.wantPass)
			}
			a := report.Results[0].Actual
			if a.AddedCount != tt.wantAdded || a.RemovedCount != tt.wantRemoved || a.ChangedCount != tt.wantChanged {
				t.Fatalf("relation %q: counts (added=%d, removed=%d, changed=%d), want (added=%d, removed=%d, changed=%d)",
					tt.relation, a.AddedCount, a.RemovedCount, a.ChangedCount, tt.wantAdded, tt.wantRemoved, tt.wantChanged)
			}
			if !gotPass && len(report.Results[0].Counterexamples) == 0 {
				t.Fatalf("relation %q: failed result should have counterexamples", tt.relation)
			}
		})
	}
}

func TestEvaluateCompareRelationPartialDiff(t *testing.T) {
	// Test added-only and removed-only scenarios.
	routeCommon := observation.RIBRoute{
		Common: observation.RIBRouteCommon{
			AFI:      model.AFIIPv4,
			Prefix:   "10.0.0.0/24",
			Protocol: model.RouteSourceStatic,
			Eligible: true,
			Best:     true,
		},
		Static: &observation.StaticRIBRoute{},
	}
	routeExtra := observation.RIBRoute{
		Common: observation.RIBRouteCommon{
			AFI:      model.AFIIPv4,
			Prefix:   "10.1.0.0/24",
			Protocol: model.RouteSourceStatic,
			Eligible: true,
			Best:     true,
		},
		Static: &observation.StaticRIBRoute{},
	}

	t.Run("added_only/added_count_fails_removed_count_passes", func(t *testing.T) {
		// pre has only routeCommon, post has routeCommon + routeExtra
		// → added=[routeExtra], removed=[] → addedCount=1, removedCount=0
		preCtx := SnapshotContext{
			Name:    "pre",
			LabPath: "labs/pre",
			Network: observation.NetworkSnapshot{
				Nodes: []observation.NodeSnapshot{{
					Node: "leaf1",
					VRFs: []observation.VRFSnapshot{{
						VRF: "default",
						RIB: observation.RIB{Node: "leaf1", VRF: "default", Routes: []observation.RIBRoute{routeCommon}},
						FIB: observation.FIB{Node: "leaf1", VRF: "default"},
					}},
				}},
			},
		}
		postCtx := SnapshotContext{
			Name:    "post",
			LabPath: "labs/post",
			Network: observation.NetworkSnapshot{
				Nodes: []observation.NodeSnapshot{{
					Node: "leaf1",
					VRFs: []observation.VRFSnapshot{{
						VRF: "default",
						RIB: observation.RIB{Node: "leaf1", VRF: "default", Routes: []observation.RIBRoute{routeCommon, routeExtra}},
						FIB: observation.FIB{Node: "leaf1", VRF: "default"},
					}},
				}},
			},
		}
		provider := &fakeSnapshotProvider{
			snapshots: map[string]SnapshotContext{"pre": preCtx, "post": postCtx},
		}
		doc := &Document{
			Version:   "hoyan/v1",
			Snapshots: map[string]Snapshot{"pre": {Lab: "labs/pre"}, "post": {Lab: "labs/post"}},
			Intents: []Intent{
				{
					Name: "added-should-fail",
					Check: Check{
						Compare: &CompareCheck{
							Table: "rib", Left: CompareSide{Snapshot: "pre"}, Right: CompareSide{Snapshot: "post"},
							Relation: "added_count",
						},
					},
				},
				{
					Name: "removed-should-pass",
					Check: Check{
						Compare: &CompareCheck{
							Table: "rib", Left: CompareSide{Snapshot: "pre"}, Right: CompareSide{Snapshot: "post"},
							Relation: "removed_count",
						},
					},
				},
			},
		}
		report, err := VerifyWithProvider(doc, provider)
		if err != nil {
			t.Fatalf("VerifyWithProvider() error: %v", err)
		}
		if len(report.Results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(report.Results))
		}
		for _, r := range report.Results {
			switch r.Name {
			case "added-should-fail":
				if r.Status != "fail" {
					t.Errorf("added_count: expected fail, got %s", r.Status)
				}
			case "removed-should-pass":
				if r.Status != "pass" {
					t.Errorf("removed_count: expected pass, got %s", r.Status)
				}
			}
		}
	})

	t.Run("removed_only/removed_count_fails_added_count_passes", func(t *testing.T) {
		// pre has routeCommon + routeExtra, post has only routeCommon
		// → added=[], removed=[routeExtra] → addedCount=0, removedCount=1
		preCtx := SnapshotContext{
			Name:    "pre",
			LabPath: "labs/pre",
			Network: observation.NetworkSnapshot{
				Nodes: []observation.NodeSnapshot{{
					Node: "leaf1",
					VRFs: []observation.VRFSnapshot{{
						VRF: "default",
						RIB: observation.RIB{Node: "leaf1", VRF: "default", Routes: []observation.RIBRoute{routeCommon, routeExtra}},
						FIB: observation.FIB{Node: "leaf1", VRF: "default"},
					}},
				}},
			},
		}
		postCtx := SnapshotContext{
			Name:    "post",
			LabPath: "labs/post",
			Network: observation.NetworkSnapshot{
				Nodes: []observation.NodeSnapshot{{
					Node: "leaf1",
					VRFs: []observation.VRFSnapshot{{
						VRF: "default",
						RIB: observation.RIB{Node: "leaf1", VRF: "default", Routes: []observation.RIBRoute{routeCommon}},
						FIB: observation.FIB{Node: "leaf1", VRF: "default"},
					}},
				}},
			},
		}
		provider := &fakeSnapshotProvider{
			snapshots: map[string]SnapshotContext{"pre": preCtx, "post": postCtx},
		}
		doc := &Document{
			Version:   "hoyan/v1",
			Snapshots: map[string]Snapshot{"pre": {Lab: "labs/pre"}, "post": {Lab: "labs/post"}},
			Intents: []Intent{
				{
					Name: "removed-should-fail",
					Check: Check{
						Compare: &CompareCheck{
							Table: "rib", Left: CompareSide{Snapshot: "pre"}, Right: CompareSide{Snapshot: "post"},
							Relation: "removed_count",
						},
					},
				},
				{
					Name: "added-should-pass",
					Check: Check{
						Compare: &CompareCheck{
							Table: "rib", Left: CompareSide{Snapshot: "pre"}, Right: CompareSide{Snapshot: "post"},
							Relation: "added_count",
						},
					},
				},
			},
		}
		report, err := VerifyWithProvider(doc, provider)
		if err != nil {
			t.Fatalf("VerifyWithProvider() error: %v", err)
		}
		if len(report.Results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(report.Results))
		}
		for _, r := range report.Results {
			switch r.Name {
			case "removed-should-fail":
				if r.Status != "fail" {
					t.Errorf("removed_count: expected fail, got %s", r.Status)
				}
			case "added-should-pass":
				if r.Status != "pass" {
					t.Errorf("added_count: expected pass, got %s", r.Status)
				}
			}
		}
	})
}
