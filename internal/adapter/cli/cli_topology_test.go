package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTopologyHelpListsRender(t *testing.T) {
	var out bytes.Buffer
	cmd := NewTopologyCommand()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(out.String(), "render") {
		t.Fatalf("help output missing render:\n%s", out.String())
	}
}

func TestRenderTopologyCommandAcceptsIsolationFlags(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.clab.yml")
	output := filepath.Join(dir, "generated.clab.yml")
	input := []byte(`name: hoyan-wan
mgmt:
    ipv4-subnet: 172.86.86.0/24
topology:
    nodes:
        r1:
            kind: linux
            mgmt-ipv4: 172.86.86.11
`)
	if err := os.WriteFile(source, input, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cmd := NewRenderTopologyCommand()
	cmd.SetOut(ioDiscard{})
	cmd.SetErr(ioDiscard{})
	cmd.SetArgs([]string{
		"--topology", source,
		"--output", output,
		"--suffix", "issue-38",
		"--lab-name", "hoyan-custom",
		"--mgmt-network", "hoyan-custom",
		"--mgmt-subnet", "172.86.38.0/24",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	data, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	rendered := string(data)
	for _, want := range []string{"name: hoyan-custom", "network: hoyan-custom", "ipv4-subnet: 172.86.38.0/24", "mgmt-ipv4: 172.86.38.11"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered topology missing %q:\n%s", want, rendered)
		}
	}
}
