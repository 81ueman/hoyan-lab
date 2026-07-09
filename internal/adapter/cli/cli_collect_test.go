package cli

import (
	"testing"
)

func TestTargetTypeParsingAndInference(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		rawType string
		want    TargetType
		wantErr bool
	}{
		{name: "explicit clab", path: "lab.yml", rawType: "clab", want: TargetClab},
		{name: "json snapshot", path: "snapshots/latest.json", want: TargetSnapshot},
		{name: "clab yml model", path: "labs/base-wan/hoyan.clab.yml", want: TargetModel},
		{name: "yaml model", path: "inventory/prod.yaml", want: TargetModel},
		{name: "unknown", path: "target.txt", wantErr: true},
		{name: "bad explicit type", path: "target.json", rawType: "uri", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := newCollectorTarget(tt.path, tt.rawType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("newCollectorTarget() error = nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("newCollectorTarget() error = %v", err)
			}
			if got.Type != tt.want {
				t.Fatalf("target type = %q, want %q", got.Type, tt.want)
			}
		})
	}
}

func TestCollectorTargetInferenceErrorHints(t *testing.T) {
	_, err := newCollectorTarget("target.txt", "")
	if err == nil {
		t.Fatalf("newCollectorTarget() error = nil")
	}
	if got, want := err.Error(), `cannot infer collector type for "target.txt"; set --type`; got != want {
		t.Fatalf("newCollectorTarget() error = %q, want %q", got, want)
	}

	_, err = newCollectorTargetWithTypeHint("target.txt", "", "--left-type, --right-type, or --type")
	if err == nil {
		t.Fatalf("newCollectorTargetWithTypeHint() error = nil")
	}
	if got, want := err.Error(), `cannot infer collector type for "target.txt"; set --left-type, --right-type, or --type`; got != want {
		t.Fatalf("newCollectorTargetWithTypeHint() error = %q, want %q", got, want)
	}
}
