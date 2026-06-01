package intent

import (
	"reflect"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func TestExpandForallSubstitutesLoopVarsDeterministically(t *testing.T) {
	doc := &Document{
		Version: "hoyan/v1",
		Vars: map[string]any{
			"edges":          []any{"bj-edge1", "sh-edge1", "gz-edge1"},
			"service_prefix": "10.4.0.0/16",
		},
		Snapshots: map[string]Snapshot{"current": {Lab: "labs/base-wan"}},
		Scenarios: map[string]Scenario{"normal": {Snapshot: "current"}},
		Intents: []Intent{{
			Name:   "service-prefix-visible",
			Forall: map[string]any{"edge": "${edges}"},
			Check: Check{
				Table:    "rib",
				Scenario: "normal",
				Where: map[string]any{
					"device": "${edge}",
					"prefix": "${service_prefix}",
				},
			},
			Assert: Assertion{Exists: ptrBool(true)},
		}},
	}
	expanded, err := Expand(doc)
	if err != nil {
		t.Fatalf("Expand() error = %v", err)
	}
	got := make([]string, 0, len(expanded.Intents))
	for _, in := range expanded.Intents {
		got = append(got, in.Name)
		if in.Check.Where["device"] == "${edge}" || in.Check.Where["prefix"] == "${service_prefix}" {
			t.Fatalf("unexpanded where: %#v", in.Check.Where)
		}
	}
	want := []string{
		"service-prefix-visible[edge=bj-edge1]",
		"service-prefix-visible[edge=sh-edge1]",
		"service-prefix-visible[edge=gz-edge1]",
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expanded names = %#v, want %#v", got, want)
		}
	}
}

func TestValidateUnknownTableIncludesPath(t *testing.T) {
	doc := &Document{
		Version:   "hoyan/v1",
		Snapshots: map[string]Snapshot{"current": {Lab: "labs/base-wan"}},
		Scenarios: map[string]Scenario{"normal": {Snapshot: "current"}},
		Intents: []Intent{{
			Name:   "bad",
			Check:  Check{Table: "routes", Scenario: "normal"},
			Assert: Assertion{Exists: ptrBool(true)},
		}},
	}
	err := Validate(doc)
	if err == nil || err.Error() != `intents[0].check.table: unsupported table "routes"` {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestVerifyWithProviderCachesAndLazilyLoadsSnapshots(t *testing.T) {
	doc := &Document{
		Version: "hoyan/v1",
		Snapshots: map[string]Snapshot{
			"current": {Lab: "labs/current"},
			"unused":  {Lab: "labs/unused"},
		},
		Scenarios: map[string]Scenario{
			"normal": {Snapshot: "current"},
			"again":  {Snapshot: "current"},
		},
		Intents: []Intent{
			{
				Name:  "route-present",
				Check: Check{Table: "rib", Scenario: "normal", Where: map[string]any{"device": "leaf1"}},
				Assert: Assertion{
					Exists: ptrBool(true),
				},
			},
			{
				Name:  "route-still-present",
				Check: Check{Table: "rib", Scenario: "again", Where: map[string]any{"prefix": "10.0.0.0/24"}},
				Assert: Assertion{
					Exists: ptrBool(true),
				},
			},
		},
	}
	provider := &fakeSnapshotProvider{
		snapshots: map[string]SnapshotContext{
			"current": {
				Name:    "current",
				LabPath: "labs/current",
				Network: observation.NetworkSnapshot{
					Nodes: []observation.NodeSnapshot{{
						Node: "leaf1",
						VRFs: []observation.VRFSnapshot{{
							VRF: "default",
							RIB: observation.RIB{
								Node: "leaf1",
								VRF:  "default",
								Routes: []observation.RIBRoute{{
									Common: observation.RIBRouteCommon{
										AFI:      model.AFIIPv4,
										Prefix:   "10.0.0.0/24",
										Protocol: model.RouteSourceStatic,
										Eligible: true,
										Best:     true,
									},
									Static: &observation.StaticRIBRoute{},
								}},
							},
							FIB: observation.FIB{Node: "leaf1", VRF: "default"},
						}},
					}},
				},
			},
		},
	}
	report, err := VerifyWithProvider(doc, provider)
	if err != nil {
		t.Fatalf("VerifyWithProvider() error = %v", err)
	}
	if report.Summary.Total != 2 || report.Summary.Passed != 2 || report.Summary.Failed != 0 {
		t.Fatalf("VerifyWithProvider() summary = %+v", report.Summary)
	}
	if !reflect.DeepEqual(provider.calls, []string{"current"}) {
		t.Fatalf("provider calls = %#v, want current loaded once", provider.calls)
	}
	if !reflect.DeepEqual(provider.defs, map[string]Snapshot{"current": {Lab: "labs/current"}}) {
		t.Fatalf("provider defs = %#v", provider.defs)
	}
}

type fakeSnapshotProvider struct {
	snapshots map[string]SnapshotContext
	calls     []string
	defs      map[string]Snapshot
}

func (p *fakeSnapshotProvider) LoadSnapshot(name string, def Snapshot) (SnapshotContext, error) {
	p.calls = append(p.calls, name)
	if p.defs == nil {
		p.defs = map[string]Snapshot{}
	}
	p.defs[name] = def
	return p.snapshots[name], nil
}

func ptrBool(v bool) *bool { return &v }
