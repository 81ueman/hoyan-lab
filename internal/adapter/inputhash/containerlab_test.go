package inputhash

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	snapshotdomain "github.com/81ueman/hoyan-lab/internal/domain/snapshot"
)

func TestInputHashesAndCheckHashesReportConfigMismatch(t *testing.T) {
	topologyPath, configPath := writeHashLab(t)
	hashes, err := InputHashes(topologyPath)
	if err != nil {
		t.Fatalf("InputHashes() error = %v", err)
	}
	if hashes.TopologyHash == "" {
		t.Fatalf("TopologyHash is empty")
	}
	if _, ok := hashes.ConfigHashes["frr.conf"]; !ok {
		t.Fatalf("ConfigHashes missing frr.conf: %#v", hashes.ConfigHashes)
	}
	snap := &snapshotdomain.Snapshot{
		Version:      snapshotdomain.Version,
		TopologyHash: hashes.TopologyHash,
		ConfigHashes: hashes.ConfigHashes,
		CollectedAt:  time.Now().UTC(),
		Nodes:        map[string]snapshotdomain.NodeSnapshot{},
	}
	appendConfig(t, configPath, "\ninterface lo\n")
	result, err := CheckHashes(topologyPath, snap)
	if err != nil {
		t.Fatalf("CheckHashes() error = %v", err)
	}
	if len(result.Mismatches) != 1 || result.Mismatches[0].Path != "frr.conf" {
		t.Fatalf("mismatches = %#v, want frr.conf mismatch", result.Mismatches)
	}
}

func writeHashLab(t *testing.T) (string, string) {
	t.Helper()
	dir := t.TempDir()
	configPath := filepath.Join(dir, "frr.conf")
	writeFile(t, configPath, "hostname r1\nrouter bgp 65001\n")
	topologyPath := filepath.Join(dir, "lab.clab.yml")
	writeFile(t, topologyPath, `name: hash-test
topology:
  nodes:
    r1:
      kind: linux
      binds:
        - frr.conf:/etc/frr/frr.conf:ro
`)
	return topologyPath, configPath
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func appendConfig(t *testing.T, path, body string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("append %s: %v", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(strings.TrimPrefix(body, "\n") + "\n"); err != nil {
		t.Fatalf("append %s: %v", path, err)
	}
}
