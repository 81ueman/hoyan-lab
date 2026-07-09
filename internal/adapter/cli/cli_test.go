package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/intent"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

// ---------------------------------------------------------------------------
// Root command tests
// ---------------------------------------------------------------------------

func TestRootHelpListsSubcommands(t *testing.T) {
	var out bytes.Buffer
	cmd := NewRootCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	help := out.String()
	for _, want := range []string{"compare", "collect", "topology", "labs", "model", "intent"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}
}

func TestRootCommandsUseGroupedHierarchy(t *testing.T) {
	cmd := NewRootCommand()
	names := map[string]bool{}
	for _, child := range cmd.Commands() {
		if !child.IsAvailableCommand() {
			continue
		}
		names[child.Name()] = true
	}
	for _, want := range []string{"compare", "collect", "topology", "labs", "model", "intent"} {
		if !names[want] {
			t.Fatalf("root command missing %q; got %v", want, names)
		}
	}
}

// ---------------------------------------------------------------------------
// Lab flag resolution tests
// ---------------------------------------------------------------------------

func TestLabFlagResolvesDefaultInputs(t *testing.T) {
	dir := t.TempDir()
	labDir := filepath.Join(dir, "labs", "base-wan")
	if err := os.MkdirAll(filepath.Join(labDir, "intent"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(labDir, "lab.yml"), []byte("name: base-wan\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(lab.yml) error = %v", err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	// Use NewModelPrefixClassesCommand as a stand-in: every model-inspect
	// subcommand registers the same --lab/--topology flags via addLabFlag +
	// addTopologyFlag, so any of them can drive resolveLabInputs tests.
	cmd := NewModelPrefixClassesCommand()
	cmd.SetArgs([]string{"--lab", "base-wan"})
	if err := cmd.ParseFlags([]string{"--lab", "base-wan"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	topology := defaultTopologyPath
	if err := resolveLabInputs(cmd, "base-wan", &topology); err != nil {
		t.Fatalf("resolveLabInputs() error = %v", err)
	}
	if want := filepath.Join("labs", "base-wan", "hoyan.clab.yml"); topology != want {
		t.Fatalf("topology = %q, want %q", topology, want)
	}
}

func TestLabFlagKeepsExplicitTopology(t *testing.T) {
	dir := t.TempDir()
	labDir := filepath.Join(dir, "labs", "base-wan")
	if err := os.MkdirAll(labDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	// See TestLabFlagResolvesDefaultInputs for rationale on using
	// NewModelPrefixClassesCommand as a --lab/--topology flag stand-in.
	cmd := NewModelPrefixClassesCommand()
	if err := cmd.ParseFlags([]string{"--lab", "base-wan", "--topology", "custom.yml"}); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	topology := "custom.yml"
	if err := resolveLabInputs(cmd, "base-wan", &topology); err != nil {
		t.Fatalf("resolveLabInputs() error = %v", err)
	}
	if topology != "custom.yml" {
		t.Fatalf("topology = %q, want explicit custom.yml", topology)
	}
}

func TestIntentSnapshotLabsResolveAtCLIBoundary(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "labs", "base-wan"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	directLab := filepath.Join(dir, "direct-lab")
	if err := os.MkdirAll(directLab, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldwd); err != nil {
			t.Fatalf("restore cwd: %v", err)
		}
	})

	doc := &intent.Document{
		Snapshots: map[string]intent.Snapshot{
			"short":  {Lab: "base-wan"},
			"direct": {Lab: directLab},
		},
	}
	if err := resolveIntentSnapshotLabs(doc); err != nil {
		t.Fatalf("resolveIntentSnapshotLabs() error = %v", err)
	}
	if want := filepath.Join("labs", "base-wan"); doc.Snapshots["short"].Lab != want {
		t.Fatalf("short lab = %q, want %q", doc.Snapshots["short"].Lab, want)
	}
	if doc.Snapshots["direct"].Lab != directLab {
		t.Fatalf("direct lab = %q, want %q", doc.Snapshots["direct"].Lab, directLab)
	}
}

// ---------------------------------------------------------------------------
// Helpers shared across CLI test files
// ---------------------------------------------------------------------------

func minimalObservationSnapshot() observation.NetworkSnapshot {
	return observation.NetworkSnapshot{
		Nodes: []observation.NodeSnapshot{{
			Node: "r1",
			VRFs: []observation.VRFSnapshot{{
				VRF: "default",
				RIB: observation.RIB{Node: "r1", VRF: "default"},
				FIB: observation.FIB{Node: "r1", VRF: "default"},
			}},
		}},
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) {
	return len(p), nil
}

type recordingRunner struct {
	commands []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, strings.Join(append([]string{name}, args...), " "))
	if name == "docker" && len(args) >= 2 && args[0] == "inspect" {
		return []byte("true\n"), nil
	}
	return nil, nil
}

func countCommand(commands []string, want string) int {
	count := 0
	for _, command := range commands {
		if command == want {
			count++
		}
	}
	return count
}

func writeUnsupportedConfigLab(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "frr.conf")
	if err := os.WriteFile(configPath, []byte(`
hostname r1
route-map RM permit 10
 match source-protocol bgp
 set local-preference 200
`), 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	topologyPath := filepath.Join(dir, "lab.clab.yml")
	if err := os.WriteFile(topologyPath, []byte(`name: strict-test
topology:
  nodes:
    r1:
      kind: linux
      binds:
        - frr.conf:/etc/frr/frr.conf
`), 0o644); err != nil {
		t.Fatalf("WriteFile(topology) error = %v", err)
	}
	return topologyPath, configPath
}
