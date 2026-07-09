package intent

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func testRouteAndRIB() (observation.RIBRoute, observation.RIB) {
	return observation.RIBRoute{
		Common: observation.RIBRouteCommon{
			Prefix:   "10.4.0.0/16",
			Protocol: model.RouteSourceKind("bgp"),
			Best:     true,
		},
	}, observation.RIB{Node: model.NodeID("r1")}
}

func TestMatchWhereAcceptsParserCompoundShapes(t *testing.T) {
	route, rib := testRouteAndRIB()
	tests := []struct {
		name  string
		where map[string]any
		want  bool
	}{
		{
			name: "and false condition",
			where: map[string]any{"and": []map[string]any{
				{"device": "r1"},
				{"protocol": "ospf"},
			}},
			want: false,
		},
		{
			name: "or all false",
			where: map[string]any{"or": []map[string]any{
				{"device": "r2"},
				{"protocol": "ospf"},
			}},
			want: false,
		},
		{
			name: "imply true antecedent false consequent",
			where: map[string]any{"imply": []map[string]any{
				{"device": "r1"},
				{"protocol": "ospf"},
			}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchWhere(route, rib, tt.where)
			if err != nil {
				t.Fatalf("matchWhere() error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("matchWhere() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMatchWhereDSLNotEqualAndWithinOperators(t *testing.T) {
	route, rib := testRouteAndRIB()
	tests := []struct {
		name  string
		where map[string]any
		want  bool
	}{
		{
			name:  "device ne rejects matching device",
			where: map[string]any{"device": map[string]any{"ne": "r1"}},
			want:  false,
		},
		{
			name:  "device ne accepts different device",
			where: map[string]any{"device": map[string]any{"ne": "r2"}},
			want:  true,
		},
		{
			name:  "prefix within accepts containing prefix",
			where: map[string]any{"prefix": map[string]any{"within": "10.0.0.0/8"}},
			want:  true,
		},
		{
			name:  "prefix within rejects disjoint prefix",
			where: map[string]any{"prefix": map[string]any{"within": "192.0.2.0/24"}},
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := matchWhere(route, rib, tt.where)
			if err != nil {
				t.Fatalf("matchWhere() error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("matchWhere() = %v, want %v", got, tt.want)
			}
		})
	}
}
