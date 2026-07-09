package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/snapshotfile"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	collectusecase "github.com/81ueman/hoyan-lab/internal/usecase/collect"
)

func TestCompareHelpListsTargetFlags(t *testing.T) {
	var out bytes.Buffer
	cmd := NewCompareCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	help := out.String()
	for _, want := range []string{"--left-type", "--right-type", "--check", "--save-snapshots"} {
		if !strings.Contains(help, want) {
			t.Fatalf("help output missing %q:\n%s", want, help)
		}
	}
}

func TestCompareSnapshotWithSnapshot(t *testing.T) {
	dir := t.TempDir()
	snapshotPath := filepath.Join(dir, "snapshot.json")
	if err := snapshotfile.SaveObservation(snapshotPath, minimalObservationSnapshot()); err != nil {
		t.Fatalf("SaveObservation() error = %v", err)
	}

	var out bytes.Buffer
	cmd := NewCompareCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{snapshotPath, snapshotPath})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "snapshots match") {
		t.Fatalf("output missing success line:\n%s", out.String())
	}
}

func TestCompareModelWithSnapshot(t *testing.T) {
	topologyPath := filepath.Join("..", "..", "..", "labs", "ospf-basic", "hoyan.clab.yml")
	collector, err := resolveCollector(t.Context(), CollectorTarget{Type: TargetModel, Path: topologyPath})
	if err != nil {
		t.Fatalf("resolveCollector(model) error = %v", err)
	}
	snap, err := collectusecase.CollectSnapshot(t.Context(), collector, observation.CollectOptions{})
	if err != nil {
		t.Fatalf("CollectSnapshot(model) error = %v", err)
	}
	snapshotPath := filepath.Join(t.TempDir(), "expected.json")
	if err := snapshotfile.SaveObservation(snapshotPath, snap); err != nil {
		t.Fatalf("SaveObservation() error = %v", err)
	}

	var out bytes.Buffer
	cmd := NewCompareCommand()
	cmd.SetOut(&out)
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{topologyPath, snapshotPath, "--left-type", "model", "--right-type", "snapshot"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestStartCompareClabTargetsDeploysExplicitClabSide(t *testing.T) {
	topologyPath := filepath.Join("..", "..", "usecase", "livecheck", "testdata", "live.clab.yml")
	runner := &recordingRunner{}

	err := startCompareClabTargets(t.Context(), []CollectorTarget{
		{Type: TargetModel, Path: topologyPath},
		{Type: TargetClab, Path: topologyPath},
	}, ioDiscard{}, 0, runner)
	if err != nil {
		t.Fatalf("startCompareClabTargets() error = %v", err)
	}

	if got, want := countCommand(runner.commands, "containerlab deploy --reconfigure -t "+topologyPath), 1; got != want {
		t.Fatalf("containerlab deploy count = %d, want %d; commands=%v", got, want, runner.commands)
	}
}

func TestStartCompareClabTargetsDeploysSharedClabTopologyOnce(t *testing.T) {
	topologyPath := filepath.Join("..", "..", "usecase", "livecheck", "testdata", "live.clab.yml")
	runner := &recordingRunner{}

	err := startCompareClabTargets(t.Context(), []CollectorTarget{
		{Type: TargetClab, Path: topologyPath},
		{Type: TargetClab, Path: topologyPath},
	}, ioDiscard{}, 0, runner)
	if err != nil {
		t.Fatalf("startCompareClabTargets() error = %v", err)
	}

	if got, want := countCommand(runner.commands, "containerlab deploy --reconfigure -t "+topologyPath), 1; got != want {
		t.Fatalf("containerlab deploy count = %d, want %d; commands=%v", got, want, runner.commands)
	}
}
