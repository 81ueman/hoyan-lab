package intent

import (
	"fmt"
	"strings"
	"testing"
)

func TestParseAggregate(t *testing.T) {
	tests := []struct {
		input string
		want  AggregateFunc
		err   string
	}{
		{"count()", AggregateFunc{Name: "count"}, ""},
		{"distCnt(nexthop)", AggregateFunc{Name: "distCnt", Field: "nexthop"}, ""},
		{"distVals(localPref)", AggregateFunc{Name: "distVals", Field: "localPref"}, ""},
		{"count", AggregateFunc{}, "missing opening parenthesis"},
		{"count(", AggregateFunc{}, "missing closing parenthesis"},
		{"unknown()", AggregateFunc{}, "unknown aggregate function"},
		{"count(field)", AggregateFunc{}, "count() takes no arguments"},
		{"distCnt()", AggregateFunc{}, "requires a field argument"},
		{"distVals()", AggregateFunc{}, "requires a field argument"},
		{"  count()  ", AggregateFunc{Name: "count"}, ""},
		{"distCnt(  nexthop  )", AggregateFunc{Name: "distCnt", Field: "nexthop"}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseAggregate(tt.input)
			if tt.err != "" {
				if err == nil {
					t.Fatalf("ParseAggregate(%q) expected error containing %q, got nil", tt.input, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseAggregate(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("ParseAggregate(%q) = %+v, want %+v", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateVersion(t *testing.T) {
	tests := []struct {
		version string
		wantErr string
	}{
		{"hoyan/v1", ""},
		{"", `version: unsupported or missing version ""`},
		{"hoyan/v2", `version: unsupported or missing version "hoyan/v2"`},
	}
	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			doc := &Document{
				Version: tt.version,
				Intents: []Intent{{
					Name: "test",
					RCL:  &RCLExpr{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}},
				}},
			}
			err := Validate(doc)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("Validate() expected error containing %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("Validate() error = %q, want %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestValidateIntentName(t *testing.T) {
	tests := []struct {
		name    string
		wantErr string
	}{
		{"valid", ""},
		{"", `intents[0].name: required`},
		{"  ", `intents[0].name: required`},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("name=%q", tt.name), func(t *testing.T) {
			doc := &Document{
				Version: "hoyan/v1",
				Intents: []Intent{{
					Name: tt.name,
					RCL:  &RCLExpr{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}},
				}},
			}
			err := Validate(doc)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
			} else {
				if err == nil {
					t.Fatalf("Validate() expected error %q, got nil", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("Validate() error = %q, want %q", err.Error(), tt.wantErr)
				}
			}
		})
	}
}

func TestValidateRCLRequired(t *testing.T) {
	doc := &Document{
		Version: "hoyan/v1",
		Intents: []Intent{{
			Name: "no-rcl",
		}},
	}
	err := Validate(doc)
	if err == nil || err.Error() != `intents[0].rcl: required` {
		t.Fatalf("Validate() error = %v, want 'intents[0].rcl: required'", err)
	}
}

func TestValidateNilExpr(t *testing.T) {
	tests := []struct {
		name string
		expr *RCLExpr
		path string
	}{
		{"nil_guard", &RCLExpr{Guard: &GuardExpr{Where: map[string]any{}, Intent: RCLExpr{}}}, "intents[0].rcl.guard.intent"},
		{"direct_nil", nil, "test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRCLExpr(tt.expr, tt.path, &Document{Version: "hoyan/v1"})
			if err == nil {
				t.Fatalf("validateRCLExpr() expected error, got nil")
			}
		})
	}
}

func TestValidateGuardExpr(t *testing.T) {
	t.Run("missing_where", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			Guard: &GuardExpr{},
		}, "test", &Document{Version: "hoyan/v1"})
		if err == nil || err.Error() != "test.guard.where: required" {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.guard.where: required'", err)
		}
	})

	t.Run("valid_guard", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			Guard: &GuardExpr{
				Where:  map[string]any{"device": "leaf1"},
				Intent: RCLExpr{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}},
			},
		}, "test", &Document{Version: "hoyan/v1"})
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})
}

func TestValidateForallExpr(t *testing.T) {
	t.Run("missing_var", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			Forall: &ForallExpr{},
		}, "test", &Document{Version: "hoyan/v1"})
		if err == nil || err.Error() != "test.forall.var: required" {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.forall.var: required'", err)
		}
	})

	t.Run("valid_forall", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			Forall: &ForallExpr{
				Var:    "device",
				Intent: RCLExpr{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}},
			},
		}, "test", &Document{Version: "hoyan/v1"})
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})
}

func TestValidateAndOr(t *testing.T) {
	tests := []struct {
		name    string
		expr    *RCLExpr
		wantErr string
	}{
		{
			name:    "and_single",
			expr:    &RCLExpr{And: []RCLExpr{{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}}}},
			wantErr: "test.and: must have at least 2 elements",
		},
		{
			name: "and_valid",
			expr: &RCLExpr{And: []RCLExpr{{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}}, {RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{2}}}}},
		},
		{
			name:    "or_single",
			expr:    &RCLExpr{Or: []RCLExpr{{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}}}},
			wantErr: "test.or: must have at least 2 elements",
		},
		{
			name: "or_valid",
			expr: &RCLExpr{Or: []RCLExpr{{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}}, {RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{2}}}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRCLExpr(tt.expr, "test", &Document{Version: "hoyan/v1"})
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("validateRCLExpr() unexpected error: %v", err)
				}
			} else {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("validateRCLExpr() error = %v, want %q", err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidateNot(t *testing.T) {
	t.Run("nil_subexpr", func(t *testing.T) {
		// Not is a pointer, so if Not is non-nil but points to a nil, that's a sub-expr issue
		err := validateRCLExpr(&RCLExpr{
			Not: &RCLExpr{},
		}, "test", &Document{Version: "hoyan/v1"})
		if err == nil || err.Error() != "test.not: empty expression (no fields set)" {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.not: empty expression (no fields set)'", err)
		}
	})

	t.Run("valid_not", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			Not: &RCLExpr{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}},
		}, "test", &Document{Version: "hoyan/v1"})
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})
}

func TestValidateImply(t *testing.T) {
	t.Run("nil_element", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			Imply: [2]*RCLExpr{nil, {RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}}},
		}, "test", &Document{Version: "hoyan/v1"})
		if err == nil || err.Error() != "test.imply: must have exactly 2 sub-expressions" {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.imply: must have exactly 2 sub-expressions'", err)
		}
	})

	t.Run("valid_imply", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			Imply: [2]*RCLExpr{
				{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}},
				{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{2}}},
			},
		}, "test", &Document{Version: "hoyan/v1"})
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})
}

func TestValidateRIBEqExpr(t *testing.T) {
	doc := &Document{
		Version:   "hoyan/v1",
		Snapshots: map[string]Snapshot{"pre": {Lab: "labs/pre"}, "post": {Lab: "labs/post"}},
	}

	t.Run("missing_left", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEq: &RIBEqExpr{Right: "post"},
		}, "test", doc)
		if err == nil || err.Error() != "test.diff.left: required" {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.diff.left: required'", err)
		}
	})

	t.Run("missing_right", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEq: &RIBEqExpr{Left: "pre"},
		}, "test", doc)
		if err == nil || err.Error() != "test.diff.right: required" {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.diff.right: required'", err)
		}
	})

	t.Run("unknown_left_snapshot", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEq: &RIBEqExpr{Left: "unknown", Right: "post"},
		}, "test", doc)
		if err == nil || err.Error() != `test.diff.left: unknown snapshot "unknown"` {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.diff.left: unknown snapshot \"unknown\"'", err)
		}
	})

	t.Run("unknown_right_snapshot", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEq: &RIBEqExpr{Left: "pre", Right: "unknown"},
		}, "test", doc)
		if err == nil || err.Error() != `test.diff.right: unknown snapshot "unknown"` {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.diff.right: unknown snapshot \"unknown\"'", err)
		}
	})

	t.Run("valid_rib_eq", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEq: &RIBEqExpr{Left: "pre", Right: "post"},
		}, "test", doc)
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})
}

func TestValidateRIBEvalExpr(t *testing.T) {
	doc := &Document{
		Version:   "hoyan/v1",
		Snapshots: map[string]Snapshot{"current": {Lab: "labs/current"}},
	}

	t.Run("missing_aggregate", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEval: &RIBEvalExpr{Eq: []any{1}},
		}, "test", doc)
		if err == nil || err.Error() != "test.rib_eval.aggregate: required" {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.rib_eval.aggregate: required'", err)
		}
	})

	t.Run("invalid_aggregate", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEval: &RIBEvalExpr{Aggregate: "invalid()", Eq: []any{1}},
		}, "test", doc)
		if err == nil || !contains(err.Error(), "unknown aggregate function") {
			t.Fatalf("validateRCLExpr() error = %v, want containing 'unknown aggregate function'", err)
		}
	})

	t.Run("unknown_snapshot", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEval: &RIBEvalExpr{Snapshot: "unknown", Aggregate: "count()", Eq: []any{1}},
		}, "test", doc)
		if err == nil || err.Error() != `test.rib_eval.snapshot: unknown snapshot "unknown"` {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.rib_eval.snapshot: unknown snapshot \"unknown\"'", err)
		}
	})

	t.Run("missing_comparison_operator", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEval: &RIBEvalExpr{Aggregate: "count()"},
		}, "test", doc)
		if err == nil || !contains(err.Error(), "at least one comparison operator") {
			t.Fatalf("validateRCLExpr() error = %v, want containing 'at least one comparison operator'", err)
		}
	})

	t.Run("count_with_non_numeric_eq", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{"not-a-number"}},
		}, "test", doc)
		if err == nil || !contains(err.Error(), "count() comparison requires numeric value") {
			t.Fatalf("validateRCLExpr() error = %v, want containing 'count() comparison requires numeric value'", err)
		}
	})

	t.Run("distVals_with_non_array_eq", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEval: &RIBEvalExpr{Aggregate: "distVals(localPref)", Eq: []any{"not-an-array"}},
		}, "test", doc)
		if err == nil || !contains(err.Error(), "distVals() comparison requires array value") {
			t.Fatalf("validateRCLExpr() error = %v, want containing 'distVals() comparison requires array value'", err)
		}
	})

	t.Run("valid_count", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}},
		}, "test", doc)
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})

	t.Run("valid_count_with_gt", func(t *testing.T) {
		v := 1
		err := validateRCLExpr(&RCLExpr{
			RIBEval: &RIBEvalExpr{Aggregate: "count()", Gt: &v},
		}, "test", doc)
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})

	t.Run("valid_distCnt", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEval: &RIBEvalExpr{Aggregate: "distCnt(nexthop)", Eq: []any{3}},
		}, "test", doc)
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})

	t.Run("valid_distVals", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			RIBEval: &RIBEvalExpr{Aggregate: "distVals(localPref)", Eq: []any{[]any{100, 200}}},
		}, "test", doc)
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})
}

func TestValidatePacketReachableExpr(t *testing.T) {
	t.Run("missing_from", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			PacketReachable: &PacketReachableExpr{To: "leaf2", Protocol: "icmp"},
		}, "test", &Document{Version: "hoyan/v1"})
		if err == nil || err.Error() != "test.packet_reachable.from: required" {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.packet_reachable.from: required'", err)
		}
	})

	t.Run("missing_to", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			PacketReachable: &PacketReachableExpr{From: "leaf1", Protocol: "icmp"},
		}, "test", &Document{Version: "hoyan/v1"})
		if err == nil || err.Error() != "test.packet_reachable.to: required" {
			t.Fatalf("validateRCLExpr() error = %v, want 'test.packet_reachable.to: required'", err)
		}
	})

	t.Run("unsupported_protocol", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			PacketReachable: &PacketReachableExpr{From: "leaf1", To: "leaf2", Protocol: "gre"},
		}, "test", &Document{Version: "hoyan/v1"})
		if err == nil || !contains(err.Error(), "unsupported protocol") {
			t.Fatalf("validateRCLExpr() error = %v, want containing 'unsupported protocol'", err)
		}
	})

	t.Run("dst_port_out_of_range_negative", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			PacketReachable: &PacketReachableExpr{From: "leaf1", To: "leaf2", Protocol: "tcp", DstPort: -1},
		}, "test", &Document{Version: "hoyan/v1"})
		if err == nil || !contains(err.Error(), "out of range") {
			t.Fatalf("validateRCLExpr() error = %v, want containing 'out of range'", err)
		}
	})

	t.Run("dst_port_out_of_range_overflow", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			PacketReachable: &PacketReachableExpr{From: "leaf1", To: "leaf2", Protocol: "tcp", DstPort: 65536},
		}, "test", &Document{Version: "hoyan/v1"})
		if err == nil || !contains(err.Error(), "out of range") {
			t.Fatalf("validateRCLExpr() error = %v, want containing 'out of range'", err)
		}
	})

	t.Run("valid_icmp", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			PacketReachable: &PacketReachableExpr{From: "leaf1", To: "leaf2", Protocol: "icmp"},
		}, "test", &Document{Version: "hoyan/v1"})
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})

	t.Run("valid_tcp_with_port", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			PacketReachable: &PacketReachableExpr{From: "leaf1", To: "leaf2", Protocol: "tcp", DstPort: 443},
		}, "test", &Document{Version: "hoyan/v1"})
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})

	t.Run("valid_udp", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{
			PacketReachable: &PacketReachableExpr{From: "leaf1", To: "leaf2", Protocol: "udp"},
		}, "test", &Document{Version: "hoyan/v1"})
		if err != nil {
			t.Fatalf("validateRCLExpr() unexpected error: %v", err)
		}
	})
}

func TestValidateScenarioReference(t *testing.T) {
	doc := &Document{
		Version: "hoyan/v1",
		Scenarios: map[string]Scenario{
			"normal": {Snapshot: "current"},
		},
		Snapshots: map[string]Snapshot{
			"current": {Lab: "labs/current"},
		},
		Intents: []Intent{{
			Name:     "test",
			Scenario: "normal",
			RCL:      &RCLExpr{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}},
		}},
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidateUnknownScenario(t *testing.T) {
	doc := &Document{
		Version: "hoyan/v1",
		Intents: []Intent{{
			Name:     "test",
			Scenario: "nonexistent",
			RCL:      &RCLExpr{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}},
		}},
	}
	err := Validate(doc)
	if err == nil || err.Error() != `intents[0].scenario: unknown scenario "nonexistent"` {
		t.Fatalf("Validate() error = %v, want 'intents[0].scenario: unknown scenario \"nonexistent\"'", err)
	}
}

func TestValidateScenarioReferencesUnknownSnapshot(t *testing.T) {
	doc := &Document{
		Version: "hoyan/v1",
		Scenarios: map[string]Scenario{
			"normal": {Snapshot: "nonexistent"},
		},
		Intents: []Intent{{
			Name:     "test",
			Scenario: "normal",
			RCL:      &RCLExpr{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}}},
		}},
	}
	err := Validate(doc)
	if err == nil || !contains(err.Error(), "references unknown snapshot") {
		t.Fatalf("Validate() error = %v, want containing 'references unknown snapshot'", err)
	}
}

func TestValidateVarRefsInRCLExpr(t *testing.T) {
	t.Run("undefined_var_in_where", func(t *testing.T) {
		doc := &Document{
			Version: "hoyan/v1",
			Vars:    map[string]any{},
			Intents: []Intent{{
				Name: "test",
				RCL: &RCLExpr{
					Guard: &GuardExpr{
						Where: map[string]any{"device": "${undefined_var}"},
						Intent: RCLExpr{
							RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{1}},
						},
					},
				},
			}},
		}
		err := Validate(doc)
		if err == nil || !contains(err.Error(), "undefined var") {
			t.Fatalf("Validate() error = %v, want containing 'undefined var'", err)
		}
	})

	t.Run("var_in_rcl_pkt_fields", func(t *testing.T) {
		doc := &Document{
			Version: "hoyan/v1",
			Vars:    map[string]any{"src": "leaf1", "dst": "leaf2"},
			Intents: []Intent{{
				Name: "test",
				RCL: &RCLExpr{
					PacketReachable: &PacketReachableExpr{
						From:     "${src}",
						To:       "${dst}",
						Protocol: "icmp",
					},
				},
			}},
		}
		err := Validate(doc)
		if err != nil {
			t.Fatalf("Validate() unexpected error: %v", err)
		}
	})

	t.Run("empty_expr_error", func(t *testing.T) {
		err := validateRCLExpr(&RCLExpr{}, "test", &Document{Version: "hoyan/v1"})
		if err == nil || err.Error() != "test: empty expression (no fields set)" {
			t.Fatalf("validateRCLExpr() error = %v, want 'test: empty expression (no fields set)'", err)
		}
	})
}

func TestValidateValidDocument(t *testing.T) {
	doc := &Document{
		Version: "hoyan/v1",
		Vars: map[string]any{
			"devices": []any{"leaf1", "leaf2"},
		},
		Snapshots: map[string]Snapshot{
			"pre":  {Lab: "labs/pre"},
			"post": {Lab: "labs/post"},
		},
		Scenarios: map[string]Scenario{
			"normal": {Snapshot: "pre"},
		},
		Intents: []Intent{
			{
				Name:     "test-compare",
				Scenario: "normal",
				RCL: &RCLExpr{
					RIBEq: &RIBEqExpr{
						Left: "pre", Right: "post",
					},
				},
			},
			{
				Name: "test-packet",
				RCL: &RCLExpr{
					Not: &RCLExpr{
						PacketReachable: &PacketReachableExpr{
							From: "leaf1", To: "leaf2", Protocol: "tcp", DstPort: 80,
						},
					},
				},
			},
			{
				Name: "test-complex",
				RCL: &RCLExpr{
					And: []RCLExpr{
						{RIBEval: &RIBEvalExpr{Aggregate: "distCnt(nexthop)", Eq: []any{1}}},
						{RIBEval: &RIBEvalExpr{Aggregate: "distVals(localPref)", Eq: []any{[]any{100, 200}}}},
					},
				},
			},
			{
				Name: "test-imply",
				RCL: &RCLExpr{
					Imply: [2]*RCLExpr{
						{RIBEval: &RIBEvalExpr{Aggregate: "count()", Gt: intPtr(0)}},
						{RIBEval: &RIBEvalExpr{Aggregate: "count()", Eq: []any{0}}},
					},
				},
			},
		},
	}
	if err := Validate(doc); err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
}

func TestValidateMultipleErrors(t *testing.T) {
	// First error wins (no-name before nil-rcl)
	doc := &Document{
		Version: "hoyan/v1",
		Intents: []Intent{
			{Name: "", RCL: nil},
			{Name: "ok", RCL: &RCLExpr{}},
		},
	}
	err := Validate(doc)
	if err == nil || err.Error() != `intents[0].name: required` {
		t.Fatalf("Validate() error = %v, want 'intents[0].name: required'", err)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

func intPtr(v int) *int { return &v }
