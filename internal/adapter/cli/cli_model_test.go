package cli

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestModelHelpListsPacketClasses(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "packet-classes") {
		t.Fatalf("help output missing packet-classes:\n%s", out.String())
	}
}

func TestModelRIBStrictConfigRejectsUnsupportedStatements(t *testing.T) {
	topologyPath, _ := writeUnsupportedConfigLab(t)
	cmd := NewModelRIBCommand()
	cmd.SetOut(ioDiscard{})
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{"--topology", topologyPath, "--strict-config"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil")
	}
	if !strings.Contains(err.Error(), "unsupported config statements") || !strings.Contains(err.Error(), `raw="match source-protocol bgp"`) {
		t.Fatalf("error = %v, want strict config error", err)
	}
}

func TestModelRIBCommandOutputsJSONAndFiltersPrefix(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelRIBCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--node", "bj-edge1",
		"--prefix", "10.4.0.0/16",
		"--format", "json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out.String())
	}
	if len(rows) == 0 {
		t.Fatalf("rows = 0, want modeled RIB entries")
	}
	for _, row := range rows {
		if row["node"] != "bj-edge1" || row["prefix"] != "10.4.0.0/16" {
			t.Fatalf("unexpected row filter result: %#v", row)
		}
		if row["condition"] == "" || row["selected_condition"] == "" {
			t.Fatalf("row missing conditions: %#v", row)
		}
	}
}

func TestModelRIBCommandFiltersProtocolArgument(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelRIBCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--node", "bj-edge1",
		"connected",
		"--format", "json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out.String())
	}
	if len(rows) == 0 {
		t.Fatalf("rows = 0, want connected RIB entries")
	}
	for _, row := range rows {
		if row["source_kind"] != "connected" {
			t.Fatalf("unexpected protocol filter result: %#v", row)
		}
		for _, unexpected := range []string{"as_path", "origin_code", "local_pref", "med", "learned_ibgp", "invalid"} {
			if _, ok := row[unexpected]; ok {
				t.Fatalf("connected row should not include BGP field %q: %#v", unexpected, row)
			}
		}
	}
}

func TestModelRIBCommandUsesRouteSourceTableForNonBGPProtocol(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelRIBCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--node", "bj-edge1",
		"connected",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	if strings.Contains(got, "AS-PATH") || strings.Contains(got, "LOCAL-PREF") || strings.Contains(got, "ORIGIN-CODE") || strings.Contains(got, "IBGP") {
		t.Fatalf("connected table should not include BGP columns:\n%s", got)
	}
	for _, want := range []string{"NODE", "PREFIX", "SOURCE", "IFACE", "connected"} {
		if !strings.Contains(got, want) {
			t.Fatalf("connected table missing %q:\n%s", want, got)
		}
	}
}

func TestModelRIBCommandFiltersOSPFProtocol(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelRIBCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "ospf-multi-area", "hoyan.clab.yml"),
		"--node", "r1",
		"--prefix", "10.255.4.4/32",
		"ospf",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"NODE", "PREFIX", "SOURCE", "OSPF-TYPE", "METRIC", "r1", "10.255.4.4/32", "ospf", "inter-area"} {
		if !strings.Contains(got, want) {
			t.Fatalf("ospf table missing %q:\n%s", want, got)
		}
	}
}

func TestModelRIBCommandRejectsInvalidProtocolArgument(t *testing.T) {
	cmd := NewModelRIBCommand()
	cmd.SetOut(ioDiscard{})
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{"bogus"})

	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil")
	}
	if !strings.Contains(err.Error(), "protocol must be one of bgp, connected, static, ospf, aggregate, or blackhole") {
		t.Fatalf("error = %q, want protocol validation", err.Error())
	}
}

func TestModelFIBCommandOutputsTable(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelFIBCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--node", "bj-edge1",
		"--prefix", "10.4.0.0/16",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"NODE", "PREFIX", "NEXT-HOP", "RANK", "GROUP", "EQUIV", "bj-edge1", "10.4.0.0/16"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "CONDITION") || strings.Contains(got, "link:") || strings.Contains(got, "node:") {
		t.Fatalf("default table output should hide conditions:\n%s", got)
	}
}

func TestModelFIBCommandShowsConditionsWhenRequested(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelFIBCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--node", "bj-edge1",
		"--prefix", "10.4.0.0/16",
		"--show-conditions",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"CONDITION", "link:", "node:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestModelFIBCommandOutputsECMPMetadataJSON(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelFIBCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--node", "bj-edge1",
		"--prefix", "10.4.0.0/16",
		"--format", "json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out.String())
	}
	if len(rows) == 0 {
		t.Fatalf("rows = 0, want modeled FIB entries")
	}
	first := rows[0]
	for _, key := range []string{"rank", "group_id", "equivalent"} {
		if _, ok := first[key]; !ok {
			t.Fatalf("FIB JSON missing %q metadata: %#v", key, first)
		}
	}
}

func TestModelFIBCommandOutputsDiscardJSON(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelFIBCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--node", "hz-edge1",
		"--prefix", "10.4.0.0/16",
		"--format", "json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out.String())
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %#v, want one local discard FIB entry", rows)
	}
	if rows[0]["source_kind"] != "blackhole" || rows[0]["discard"] != true || rows[0]["interface"] != "Null0" {
		t.Fatalf("discard FIB JSON = %#v, want blackhole discard via Null0", rows[0])
	}
}

func TestModelPrefixClassesCommandOutputsJSONAndFiltersPrefix(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelPrefixClassesCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--prefix", "10.4.0.0/16",
		"--format", "json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var rows []map[string]any
	if err := json.Unmarshal(out.Bytes(), &rows); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out.String())
	}
	if len(rows) < 2 {
		t.Fatalf("rows = %d, want prefix split into multiple classes", len(rows))
	}
	for _, row := range rows {
		if _, ok := row["class_id"]; !ok {
			t.Fatalf("row missing class_id: %#v", row)
		}
		if row["space"] == "" {
			t.Fatalf("row missing space: %#v", row)
		}
		predicates, ok := row["matched_predicates"].([]any)
		if !ok || len(predicates) == 0 {
			t.Fatalf("row missing matched_predicates: %#v", row)
		}
	}
	var foundRIB, foundFIB bool
	for _, row := range rows {
		predicates, _ := row["matched_predicates"].([]any)
		for _, raw := range predicates {
			source, _ := raw.(string)
			if strings.HasPrefix(source, "rib:") {
				foundRIB = true
			}
			if strings.HasPrefix(source, "fib:") {
				foundFIB = true
			}
		}
	}
	if !foundRIB || !foundFIB {
		t.Fatalf("prefix-classes JSON missing RIB/FIB predicates: rib=%v fib=%v", foundRIB, foundFIB)
	}
}

func TestModelPrefixClassesCommandOutputsTable(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelPrefixClassesCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--prefix", "10.4.1.10/32",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"CLASS", "SPACE", "pc-", "10.4.1.10/32"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "MATCHED-PREDICATES") || strings.Contains(got, "request:prefix-classes:") {
		t.Fatalf("default table output should hide matched predicates:\n%s", got)
	}
}

func TestModelPrefixClassesCommandShowsPredicatesWhenRequested(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelPrefixClassesCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--prefix", "10.4.1.10/32",
		"--show-predicates",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"MATCHED-PREDICATES", "request:prefix-classes:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}

func TestModelPrefixClassesCommandSummary(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelPrefixClassesCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--prefix", "10.4.0.0/16",
		"--summary",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"predicates=", "unique=", "classes=", "sources:", "CLASS", "SPACE"} {
		if !strings.Contains(got, want) {
			t.Fatalf("summary output missing %q:\n%s", want, got)
		}
	}
}

func TestModelPrefixClassesCommandThresholdFails(t *testing.T) {
	cmd := NewModelPrefixClassesCommand()
	cmd.SetOut(ioDiscard{})
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--max-prefix-classes", "1",
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatalf("Execute() error = nil")
	}
	if !strings.Contains(err.Error(), "prefix universe class count") || !strings.Contains(err.Error(), "exceeds --max-prefix-classes 1") {
		t.Fatalf("error = %v, want threshold error", err)
	}
}

func TestModelCommandRejectsUnknownNode(t *testing.T) {
	cmd := NewModelRIBCommand()
	cmd.SetOut(ioDiscard{})
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--node", "missing-node",
	})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), `unknown node "missing-node"`) {
		t.Fatalf("Execute() error = %v, want unknown node", err)
	}
}

func TestModelSymbolicPacketCommandOutputsJSON(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelSymbolicPacketCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--from", "cust-bj",
		"--to", "10.4.1.10",
		"--protocol", "tcp",
		"--dst-port", "80",
		"--format", "json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var result map[string]any
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out.String())
	}
	if result["from"] != "cust-bj" || result["to"] != "10.4.1.10" || result["protocol"] != "tcp" {
		t.Fatalf("unexpected symbolic packet metadata: %#v", result)
	}
	if result["dst_port"] != float64(80) {
		t.Fatalf("unexpected symbolic packet dst_port: %#v", result)
	}
	if result["reachable_condition"] == "" || result["unreachable_condition"] == "" {
		t.Fatalf("missing reachability conditions: %#v", result)
	}
	blocked, ok := result["blocked_paths"].([]any)
	if !ok || len(blocked) == 0 {
		t.Fatalf("missing symbolic policy blocked paths: %#v", result)
	}
	first, ok := blocked[0].(map[string]any)
	if !ok || first["acl"] != "BLOCK-HTTP-TO-HZ" || first["node"] != "core-hz" {
		t.Fatalf("unexpected symbolic blocked path metadata: %#v", first)
	}
	source, ok := first["source"].(map[string]any)
	if !ok || source["vendor"] != "nftables" || source["file"] == "" || source["raw"] == "" {
		t.Fatalf("missing symbolic blocked path source: %#v", first)
	}
}

func TestModelSymbolicRouteCommandOutputsJSON(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelSymbolicRouteCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--from", "bj-edge1",
		"--prefix", "10.4.0.0/16",
		"--format", "json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var results []map[string]any
	if err := json.Unmarshal(out.Bytes(), &results); err != nil {
		t.Fatalf("json.Unmarshal() error = %v\n%s", err, out.String())
	}
	if len(results) < 2 {
		t.Fatalf("results = %d, want prefix split into multiple classes", len(results))
	}
	result := results[0]
	if result["from"] != "bj-edge1" || result["prefix"] != "10.4.0.0/16" {
		t.Fatalf("unexpected symbolic route metadata: %#v", result)
	}
	if _, ok := result["class_id"]; !ok {
		t.Fatalf("missing class_id: %#v", result)
	}
	if result["space"] == "" {
		t.Fatalf("missing class space: %#v", result)
	}
	predicates, ok := result["matched_predicates"].([]any)
	if !ok || len(predicates) == 0 {
		t.Fatalf("missing matched predicates: %#v", result)
	}
	if result["reachable_condition"] == "" || result["unreachable_condition"] == "" {
		t.Fatalf("missing reachability conditions: %#v", result)
	}
	paths, ok := result["paths"].([]any)
	if !ok || len(paths) == 0 {
		t.Fatalf("missing symbolic route paths: %#v", result)
	}
}

func TestModelSymbolicRouteCommandOutputsClassTable(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelSymbolicRouteCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--from", "bj-edge1",
		"--prefix", "10.4.0.0/16",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"class: pc-", "space:", "PATH"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
	for _, hidden := range []string{"matched predicates:", "reachable:", "unreachable:", "CONDITION", "link:", "node:"} {
		if strings.Contains(got, hidden) {
			t.Fatalf("default table output should hide %q:\n%s", hidden, got)
		}
	}
}

func TestModelSymbolicRouteCommandShowsPredicatesWhenRequested(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelSymbolicRouteCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--from", "bj-edge1",
		"--prefix", "10.4.0.0/16",
		"--show-predicates",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "matched predicates:") {
		t.Fatalf("output missing matched predicates:\n%s", got)
	}
}

func TestModelSymbolicRouteCommandShowsConditionsWhenRequested(t *testing.T) {
	var out bytes.Buffer
	cmd := NewModelSymbolicRouteCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"),
		"--from", "bj-edge1",
		"--prefix", "10.4.0.0/16",
		"--show-conditions",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"reachable:", "unreachable:", "CONDITION", "link:", "node:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q:\n%s", want, got)
		}
	}
}
