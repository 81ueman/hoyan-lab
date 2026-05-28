package model_test

import (
	"strings"
	"testing"

	. "github.com/81ueman/hoyan-lab/internal/model"
)

func TestRenderIsolatedTopologyUsesSuffixForNamesAndMgmtSubnet(t *testing.T) {
	data := []byte(`
name: hoyan-wan
prefix: ""
mgmt:
  network: hoyan-wan
  ipv4-subnet: 172.86.86.0/24
topology:
  nodes:
    r1:
      kind: linux
      mgmt-ipv4: 172.86.86.11
  links: []
`)
	out, err := RenderIsolatedTopology(data, TopologyRenderOptions{Suffix: "issue-123"})
	if err != nil {
		t.Fatalf("RenderIsolatedTopology() error = %v", err)
	}
	rendered := string(out)
	for _, want := range []string{
		"name: hoyan-wan-issue-123",
		"network: hoyan-wan-issue-123",
		"ipv4-subnet: 172.86.123.0/24",
		"mgmt-ipv4: 172.86.123.11",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered topology missing %q:\n%s", want, rendered)
		}
	}
	if strings.Contains(rendered, "prefix:") {
		t.Fatalf("rendered topology should omit prefix for containerlab default naming:\n%s", rendered)
	}
}

func TestRenderIsolatedTopologyPreservesRelativeConfigPathsByDefault(t *testing.T) {
	data := []byte(`
name: hoyan-wan
mgmt:
  ipv4-subnet: 172.86.86.0/24
topology:
  nodes:
    frr:
      kind: linux
      binds:
        - configs/frr/r1/frr.conf:/etc/frr/frr.conf:ro
    ceos:
      kind: arista_ceos
      startup-config: configs/ceos/r1.cfg
  links: []
`)
	out, err := RenderIsolatedTopology(data, TopologyRenderOptions{Suffix: "issue-123"})
	if err != nil {
		t.Fatalf("RenderIsolatedTopology() error = %v", err)
	}
	rendered := string(out)
	for _, want := range []string{
		"configs/frr/r1/frr.conf:/etc/frr/frr.conf:ro",
		"startup-config: configs/ceos/r1.cfg",
	} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered topology missing relative path %q:\n%s", want, rendered)
		}
	}
}
