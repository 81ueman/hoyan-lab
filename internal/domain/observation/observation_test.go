package observation

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// --- JSON backward compatibility ---

func TestRIBRouteCommonNewFieldsAreOmitempty(t *testing.T) {
	// A route with only old fields should marshal without new field keys.
	route := RIBRoute{
		Common: RIBRouteCommon{
			AFI:      model.AFIIPv4,
			Prefix:   "10.0.0.0/24",
			Protocol: model.RouteSourceBGP,
			Eligible: true,
			Best:     true,
		},
		BGP: &BGPRIBRoute{Paths: []BGPPath{
			{NextHop: NextHop{Address: "192.0.2.1"}, Eligible: true, Best: true},
		}},
	}
	data, err := json.Marshal(route)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	// Ensure none of the new optional fields appear.
	for _, key := range []string{"safi", "table_id", "table_name", "protocol_instance", "age", "age_seconds", "tag", "installed_reason", "raw"} {
		if containsJSONKey(data, key) {
			t.Errorf("new optional field %q should be omitted when zero", key)
		}
	}

	// Round-trip: should unmarshal into the same structure.
	var decoded RIBRoute
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(route, decoded) {
		t.Fatalf("round-trip mismatch:\n got  %#v\n want %#v", decoded, route)
	}
}

func TestRIBRouteCommonNewFieldsMarshalWhenSet(t *testing.T) {
	route := RIBRoute{
		Common: RIBRouteCommon{
			AFI:              model.AFIIPv4,
			Prefix:           "10.0.0.0/24",
			Protocol:         model.RouteSourceBGP,
			Eligible:         true,
			Best:             true,
			SAFI:             "unicast",
			TableID:          "254",
			TableName:        "main",
			ProtocolInstance: "BGP 65000",
			Age:              "00:12:34",
			AgeSeconds:       754,
			Tag:              42,
			InstalledReason:  "active",
			Raw:              map[string]any{"vendor": "test"},
		},
		BGP: &BGPRIBRoute{Paths: []BGPPath{
			{NextHop: NextHop{Address: "192.0.2.1"}, Eligible: true, Best: true},
		}},
	}
	data, err := json.Marshal(route)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	for _, key := range []string{"safi", "table_id", "table_name", "protocol_instance", "age", "age_seconds", "tag", "installed_reason", "raw"} {
		if !containsJSONKey(data, key) {
			t.Errorf("expected field %q to be present when set", key)
		}
	}

	var decoded RIBRoute
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if decoded.Common.SAFI != "unicast" || decoded.Common.TableID != "254" || decoded.Common.Tag != 42 {
		t.Fatalf("round-trip field values mismatch: %#v", decoded.Common)
	}
}

func TestFIBEntryNewFieldsAreOmitempty(t *testing.T) {
	entry := FIBEntry{
		AFI:      model.AFIIPv4,
		Prefix:   "10.0.0.0/24",
		Source:   RouteSource{Protocol: model.RouteSourceBGP},
		Action:   ActionForward,
		NextHops: []NextHop{{Address: "192.0.2.1", Interface: "eth1"}},
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	for _, key := range []string{"safi", "table_id", "table_name", "protocol_instance", "age", "age_seconds", "tag", "installed_reason", "raw"} {
		if containsJSONKey(data, key) {
			t.Errorf("new optional field %q should be omitted when zero", key)
		}
	}
	var decoded FIBEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(entry, decoded) {
		t.Fatalf("round-trip mismatch:\n got  %#v\n want %#v", decoded, entry)
	}
}

func TestFIBEntryNewFieldsMarshalWhenSet(t *testing.T) {
	entry := FIBEntry{
		AFI:      model.AFIIPv4,
		Prefix:   "10.0.0.0/24",
		Source:   RouteSource{Protocol: model.RouteSourceBGP},
		Action:   ActionForward,
		NextHops: []NextHop{{Address: "192.0.2.1", Interface: "eth1"}},
		SAFI:     "unicast",
		TableID:  "254",
		Tag:      42,
		Raw:      map[string]any{"asic": "trident3"},
	}
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	for _, key := range []string{"safi", "table_id", "tag", "raw"} {
		if !containsJSONKey(data, key) {
			t.Errorf("expected field %q to be present when set", key)
		}
	}
	var decoded FIBEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if decoded.SAFI != "unicast" || decoded.TableID != "254" || decoded.Tag != 42 {
		t.Fatalf("round-trip field values mismatch: %#v", decoded)
	}
}

func TestNextHopNewFieldsAreOmitempty(t *testing.T) {
	hop := NextHop{Address: "192.0.2.1", Interface: "eth1"}
	data, err := json.Marshal(hop)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	for _, key := range []string{"resolved", "raw"} {
		if containsJSONKey(data, key) {
			t.Errorf("new optional field %q should be omitted when zero", key)
		}
	}
	var decoded NextHop
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if !reflect.DeepEqual(hop, decoded) {
		t.Fatalf("round-trip mismatch:\n got  %#v\n want %#v", decoded, hop)
	}
}

func TestNextHopResolvedField(t *testing.T) {
	hop := NextHop{
		Address:   "10.0.0.1",
		Interface: "eth1",
		Resolved: []NextHop{
			{Address: "192.168.1.1", Interface: "eth1"},
		},
	}
	data, err := json.Marshal(hop)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	var decoded NextHop
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if len(decoded.Resolved) != 1 || decoded.Resolved[0].Address != "192.168.1.1" {
		t.Fatalf("resolved next-hop not preserved: %#v", decoded)
	}
}

func TestBGPPathRawField(t *testing.T) {
	path := BGPPath{
		Eligible: true,
		Best:     true,
		Raw:      map[string]any{"origin_code": "?"},
	}
	data, err := json.Marshal(path)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	if !containsJSONKey(data, "raw") {
		t.Error("expected raw field to be present")
	}
	var decoded BGPPath
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal error: %v", err)
	}
	if decoded.Raw["origin_code"] != "?" {
		t.Fatalf("raw value not preserved: %#v", decoded.Raw)
	}
}

func TestBGPPathRawOmitempty(t *testing.T) {
	path := BGPPath{Eligible: true, Best: true}
	data, err := json.Marshal(path)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	if containsJSONKey(data, "raw") {
		t.Error("raw field should be omitted when nil")
	}
}

// --- Forwarding action validation ---

func TestFIBEntryValidatesNewActions(t *testing.T) {
	actions := []ForwardingAction{
		ActionReject,
		ActionUnreach,
		ActionPunt,
		ActionTrap,
		ActionLocal,
		ActionGlean,
		ActionDiscard,
	}
	for _, action := range actions {
		entry := FIBEntry{
			AFI:    model.AFIIPv4,
			Prefix: "10.0.0.0/24",
			Source: RouteSource{Protocol: model.RouteSourceBGP},
			Action: action,
		}
		if err := entry.Validate(); err != nil {
			t.Errorf("FIBEntry with action %q should be valid: %v", action, err)
		}
	}
}

func TestFIBEntryNewActionsDoNotRequireNextHops(t *testing.T) {
	entry := FIBEntry{
		AFI:    model.AFIIPv4,
		Prefix: "10.0.0.0/24",
		Source: RouteSource{Protocol: model.RouteSourceBGP},
		Action: ActionReject,
	}
	if err := entry.Validate(); err != nil {
		t.Errorf("reject entry without next-hops should be valid: %v", err)
	}
}

func TestFIBEntryUnknownActionFailsValidation(t *testing.T) {
	entry := FIBEntry{
		AFI:    model.AFIIPv4,
		Prefix: "10.0.0.0/24",
		Source: RouteSource{Protocol: model.RouteSourceBGP},
		Action: ForwardingAction("bogus"),
	}
	if err := entry.Validate(); err == nil {
		t.Fatal("expected validation error for unknown action")
	}
}

// --- Old snapshot JSON compatibility ---

func TestOldSnapshotJSONRoundTrip(t *testing.T) {
	// Simulate an old-style JSON payload without any new fields.
	oldJSON := `{
	  "afi": "ipv4",
	  "prefix": "10.0.0.0/24",
	  "protocol": "bgp",
	  "preference": 0,
	  "metric": 0,
	  "eligible": true,
	  "best": true
	}`
	var common RIBRouteCommon
	if err := json.Unmarshal([]byte(oldJSON), &common); err != nil {
		t.Fatalf("unmarshal old-style JSON error: %v", err)
	}
	if common.AFI != model.AFIIPv4 || common.Prefix != "10.0.0.0/24" {
		t.Fatalf("unexpected values: %#v", common)
	}
	// New fields should be zero-valued.
	if common.SAFI != "" || common.TableID != "" || common.Tag != 0 || common.Raw != nil {
		t.Fatalf("new fields should be zero-valued for old JSON: %#v", common)
	}
}

func TestOldFIBEntryJSONRoundTrip(t *testing.T) {
	oldJSON := `{
	  "afi": "ipv4",
	  "prefix": "10.0.0.0/24",
	  "source": {"protocol": "bgp"},
	  "action": "forward",
	  "next_hops": [{"address": "192.0.2.1", "interface": "eth1"}],
	  "preference": 0,
	  "metric": 0
	}`
	var entry FIBEntry
	if err := json.Unmarshal([]byte(oldJSON), &entry); err != nil {
		t.Fatalf("unmarshal old-style FIBEntry error: %v", err)
	}
	if entry.SAFI != "" || entry.TableID != "" || entry.Tag != 0 || entry.Raw != nil {
		t.Fatalf("new fields should be zero-valued for old JSON: %#v", entry)
	}
	// Re-marshal should not include new fields since they're zero.
	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	for _, key := range []string{"safi", "table_id", "tag", "raw"} {
		if containsJSONKey(data, key) {
			t.Errorf("new optional field %q leaked in re-marshal of old-style data", key)
		}
	}
}

// --- Normalization ---

func TestNormalizeFIBEntryPreservesNewFields(t *testing.T) {
	entry := FIBEntry{
		AFI:      model.AFIIPv4,
		Prefix:   "10.0.0.0/24",
		Source:   RouteSource{Protocol: model.RouteSourceBGP},
		Action:   ActionForward,
		NextHops: []NextHop{{Address: "192.0.2.1", Interface: "eth1"}},
		SAFI:     "unicast",
		Tag:      100,
		TableID:  "main",
	}
	normalized := normalizeFIBEntryForCompare(entry)
	if normalized.SAFI != "unicast" || normalized.Tag != 100 || normalized.TableID != "main" {
		t.Fatalf("normalize discarded new fields: %#v", normalized)
	}
}

func TestNormalizeFIBEntryNoDefaultAction(t *testing.T) {
	// Action defaulting has been removed from compare-time normalization.
	// New producers must emit explicit Action; old snapshots are migrated
	// at file-load boundary via migrateSnapshotForCompare.
	entry := FIBEntry{
		AFI:    model.AFIIPv4,
		Prefix: "203.0.113.0/24",
		Source: RouteSource{Protocol: model.RouteSourceBlackhole},
	}
	normalized := normalizeFIBEntryForCompare(entry)
	if normalized.Action != "" {
		t.Fatalf("expected empty Action (no defaulting), got %q", normalized.Action)
	}

	// When action is explicitly set, normalization preserves it.
	entry.Action = ActionDiscard
	normalized = normalizeFIBEntryForCompare(entry)
	if normalized.Action != ActionDiscard {
		t.Fatalf("expected ActionDiscard to be preserved, got %q", normalized.Action)
	}
}

// --- Merge duplicate routes with new fields ---

func TestMergeDuplicateRouteNewFields(t *testing.T) {
	a := FIBEntry{
		AFI:      model.AFIIPv4,
		Prefix:   "10.0.0.0/24",
		Source:   RouteSource{Protocol: model.RouteSourceBGP},
		Action:   ActionForward,
		NextHops: []NextHop{{Address: "192.0.2.1", Interface: "eth1"}},
		SAFI:     "unicast",
		Tag:      42,
	}
	b := FIBEntry{
		AFI:      model.AFIIPv4,
		Prefix:   "10.0.0.0/24",
		Source:   RouteSource{Protocol: model.RouteSourceBGP},
		Action:   ActionForward,
		NextHops: []NextHop{{Address: "192.0.2.2", Interface: "eth2"}},
		TableID:  "254",
		Age:      "00:01:00",
	}
	merged, reason, ok := mergeDuplicateRoute(a, b)
	if !ok {
		t.Fatalf("merge should succeed, got reason=%q", reason)
	}
	if merged.SAFI != "unicast" {
		t.Errorf("expected SAFI=%q from a, got %q", "unicast", merged.SAFI)
	}
	if merged.Tag != 42 {
		t.Errorf("expected Tag=%d from a, got %d", 42, merged.Tag)
	}
	if merged.TableID != "254" {
		t.Errorf("expected TableID=%q from b, got %q", "254", merged.TableID)
	}
	if merged.Age != "00:01:00" {
		t.Errorf("expected Age=%q from b, got %q", "00:01:00", merged.Age)
	}
	if len(merged.NextHops) != 2 {
		t.Errorf("expected 2 next-hops after merge, got %d", len(merged.NextHops))
	}
}

// --- Migration ---

func TestMigrateSnapshotForCompare(t *testing.T) {
	snapshot := NetworkSnapshot{
		Nodes: []NodeSnapshot{{
			Node: "r1",
			VRFs: []VRFSnapshot{{
				VRF: "default",
				RIB: RIB{
					Routes: []RIBRoute{
						{Common: RIBRouteCommon{Prefix: "10.0.0.0/24", Protocol: model.RouteSourceBGP}}, // missing AFI
						{Common: RIBRouteCommon{AFI: model.AFIIPv6, Prefix: "2001::/32", Protocol: model.RouteSourceBGP}},
					},
				},
				FIB: FIB{
					Entries: []FIBEntry{
						{Prefix: "10.0.0.0/24", Source: RouteSource{Protocol: model.RouteSourceBlackhole}}, // missing AFI+Action
						{AFI: model.AFIIPv4, Prefix: "10.0.0.0/24", Source: RouteSource{Protocol: model.RouteSourceConnected}, Action: ActionForward},
						{Prefix: "10.0.1.0/24", Source: RouteSource{Protocol: model.RouteSourceConnected}},                                           // missing Action, no next-hops
						{Prefix: "10.0.2.0/24", Source: RouteSource{Protocol: model.RouteSourceStatic}, NextHops: []NextHop{{Address: "192.0.2.1"}}}, // missing Action
					},
				},
			}},
		}},
	}
	migrated := migrateSnapshotForCompare(snapshot)

	// RIB: empty AFI should be set to IPv4
	if migrated.Nodes[0].VRFs[0].RIB.Routes[0].Common.AFI != model.AFIIPv4 {
		t.Errorf("expected AFI=ipv4 for RIB route with empty AFI, got %q", migrated.Nodes[0].VRFs[0].RIB.Routes[0].Common.AFI)
	}
	// RIB: explicit AFI should be preserved
	if migrated.Nodes[0].VRFs[0].RIB.Routes[1].Common.AFI != model.AFIIPv6 {
		t.Errorf("expected AFI=ipv6 for RIB route, got %q", migrated.Nodes[0].VRFs[0].RIB.Routes[1].Common.AFI)
	}

	// FIB: empty AFI should be set to IPv4
	if migrated.Nodes[0].VRFs[0].FIB.Entries[0].AFI != model.AFIIPv4 {
		t.Errorf("expected AFI=ipv4 for FIB entry with empty AFI, got %q", migrated.Nodes[0].VRFs[0].FIB.Entries[0].AFI)
	}
	// FIB: empty Action for blackhole should be set to drop
	if migrated.Nodes[0].VRFs[0].FIB.Entries[0].Action != ActionDrop {
		t.Errorf("expected Action=drop for blackhole, got %q", migrated.Nodes[0].VRFs[0].FIB.Entries[0].Action)
	}
	// FIB: explicit Action should be preserved
	if migrated.Nodes[0].VRFs[0].FIB.Entries[1].Action != ActionForward {
		t.Errorf("expected Action=forward (preserved), got %q", migrated.Nodes[0].VRFs[0].FIB.Entries[1].Action)
	}
	// FIB: empty Action for connected without next-hops → receive
	if migrated.Nodes[0].VRFs[0].FIB.Entries[2].Action != ActionReceive {
		t.Errorf("expected Action=receive for connected without next-hops, got %q", migrated.Nodes[0].VRFs[0].FIB.Entries[2].Action)
	}
	// FIB: empty Action for static with next-hops → forward
	if migrated.Nodes[0].VRFs[0].FIB.Entries[3].Action != ActionForward {
		t.Errorf("expected Action=forward for static with next-hops, got %q", migrated.Nodes[0].VRFs[0].FIB.Entries[3].Action)
	}
}

// --- Helpers ---

func containsJSONKey(data []byte, key string) bool {
	// Simple check: look for `"key":` in the JSON byte stream.
	target := []byte(`"` + key + `":`)
	// We want to avoid matching substrings like "raw_attributes", so check
	// that the byte before the key is either '{' or ','.
	for i := 0; i <= len(data)-len(target); i++ {
		if data[i] == '"' && string(data[i:i+len(target)]) == string(target) {
			return true
		}
	}
	return false
}
