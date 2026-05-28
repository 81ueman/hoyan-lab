package queryfile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadIncludesFailureDomain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "queries.yml")
	if err := os.WriteFile(path, []byte(`
route_checks:
  - name: explicit-domain
    from: edge
    prefix: 10.0.0.0/24
    max_failures: 1
    failure_domain:
      include_node_roles: [core]
      exclude_node_roles: [customer]
      include_link_roles: [backbone]
      exclude_link_roles: [access]
      include_nodes: [core1]
      exclude_nodes: [client1]
      include_links: [core1-core2]
      exclude_links: [edge-client1]
packet_checks:
  - name: multi-port
    from: src
    to: 10.0.0.10
    protocol: tcp
    dst_ports: [80, 443]
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	queries, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	got := queries.RouteChecks[0].FailureDomain
	if !reflect.DeepEqual(got.IncludeNodeRoles, []string{"core"}) ||
		!reflect.DeepEqual(got.ExcludeNodeRoles, []string{"customer"}) ||
		!reflect.DeepEqual(got.IncludeLinkRoles, []string{"backbone"}) ||
		!reflect.DeepEqual(got.ExcludeLinkRoles, []string{"access"}) ||
		!reflect.DeepEqual(got.IncludeNodes, []string{"core1"}) ||
		!reflect.DeepEqual(got.ExcludeNodes, []string{"client1"}) ||
		!reflect.DeepEqual(got.IncludeLinks, []string{"core1-core2"}) ||
		!reflect.DeepEqual(got.ExcludeLinks, []string{"edge-client1"}) {
		t.Fatalf("FailureDomain = %#v", got)
	}
	if ports := queries.PacketChecks[0].DstPortValues(); !reflect.DeepEqual(ports, []int{80, 443}) {
		t.Fatalf("DstPortValues() = %#v, want [80 443]", ports)
	}
}
