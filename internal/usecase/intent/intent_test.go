package intent

import "testing"

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

func ptrBool(v bool) *bool { return &v }
