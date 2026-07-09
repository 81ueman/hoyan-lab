package configparse_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/configparse"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestParseSRLinuxRoutingPolicies(t *testing.T) {
	config := `
set / system name host-name core1
set / network-instance default protocols bgp autonomous-system 65100
set / routing-policy prefix-set LOCAL prefix 10.0.0.0/24 mask-length-range 24..32
set / routing-policy policy IMPORT statement 10 match prefix prefix-set LOCAL
set / routing-policy policy IMPORT statement 10 action bgp local-preference set 250
set / routing-policy policy IMPORT statement 10 action policy-result accept
set / routing-policy policy EXPORT statement 20 action bgp med operation set value 77
set / routing-policy policy EXPORT statement 20 action policy-result accept
set / routing-policy policy REJECT-ALL default-action policy-result reject
set / network-instance default protocols bgp group edge import-policy [ IMPORT ]
set / network-instance default protocols bgp group edge export-policy [ EXPORT ]
set / network-instance default protocols bgp group edge peer-as 65001
set / network-instance default protocols bgp neighbor 192.0.2.1 peer-group edge
set / network-instance default protocols bgp neighbor 192.0.2.1 export-policy [ REJECT-ALL ]
`
	path := filepath.Join(t.TempDir(), "core.cfg")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := configparse.ParseConfig("srlinux", path)
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if cfg.Hostname != "core1" || cfg.ASN != 65100 {
		t.Fatalf("Config = %#v", cfg)
	}
	if got, want := prefixListsWithoutMatches(cfg.PrefixLists), []model.PrefixList{{Name: "LOCAL", Rules: []model.PrefixListRule{{Action: "permit", Prefix: "10.0.0.0/24", Le: 32}}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("PrefixLists = %#v, want %#v", got, want)
	}
	importPolicy := routePolicyByName(cfg.RoutePolicies, "IMPORT")
	if importPolicy == nil || len(importPolicy.Rules) != 2 || importPolicy.Rules[0].Action != "permit" || importPolicy.Rules[0].MatchPrefixList != "LOCAL" || importPolicy.Rules[0].SetLocalPref == nil || *importPolicy.Rules[0].SetLocalPref != 250 || importPolicy.Rules[1].Action != "permit" {
		t.Fatalf("IMPORT = %#v", importPolicy)
	}
	exportPolicy := routePolicyByName(cfg.RoutePolicies, "EXPORT")
	if exportPolicy == nil || len(exportPolicy.Rules) != 2 || exportPolicy.Rules[0].Action != "permit" || exportPolicy.Rules[0].SetMED == nil || *exportPolicy.Rules[0].SetMED != 77 || exportPolicy.Rules[1].Action != "permit" {
		t.Fatalf("EXPORT = %#v", exportPolicy)
	}
	rejectPolicy := routePolicyByName(cfg.RoutePolicies, "REJECT-ALL")
	if rejectPolicy == nil || len(rejectPolicy.Rules) != 1 || rejectPolicy.Rules[0].Action != "deny" {
		t.Fatalf("REJECT-ALL = %#v", rejectPolicy)
	}
	if len(cfg.Neighbors) != 1 || cfg.Neighbors[0].ImportPolicy != "IMPORT" || cfg.Neighbors[0].ExportPolicy != "REJECT-ALL" {
		t.Fatalf("Neighbors = %#v, want group import and neighbor export override", cfg.Neighbors)
	}
}

func TestParseSRLinuxRoutingPolicyRejectsUnsupportedMatch(t *testing.T) {
	config := `
set / routing-policy policy IMPORT statement 10 match protocol bgp
set / routing-policy policy IMPORT statement 10 action policy-result accept
`
	path := filepath.Join(t.TempDir(), "core.cfg")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := configparse.ParseConfig("srlinux", path)
	if err == nil || !strings.Contains(err.Error(), "unsupported SR Linux routing-policy statement") {
		t.Fatalf("configparse.ParseConfig() error = %v, want unsupported SR Linux routing-policy statement", err)
	}
	result, err := configparse.ParseConfigWithWarnings("srlinux", path)
	if err != nil {
		t.Fatalf("configparse.ParseConfigWithWarnings() error = %v", err)
	}
	want := []configparse.UnsupportedStatement{{Vendor: "srlinux", File: path, Line: 2, Text: "set / routing-policy policy IMPORT statement 10 match protocol bgp", Reason: "unsupported SR Linux routing-policy statement"}}
	if !reflect.DeepEqual(result.Warnings, want) {
		t.Fatalf("Warnings = %#v, want %#v", result.Warnings, want)
	}
	policy := routePolicyByName(result.Config.RoutePolicies, "IMPORT")
	if policy == nil || len(policy.Rules) != 2 || policy.Rules[0].MatchPrefixList == "" {
		t.Fatalf("unsupported match should not become match-any: %#v", policy)
	}
}

func TestParseSRLinuxBGPAggregateWarnsUnsupported(t *testing.T) {
	config := `set / network-instance default protocols bgp aggregate-routes route 10.0.0.0/16`
	path := filepath.Join(t.TempDir(), "core.cfg")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, err := configparse.ParseConfig(model.KindSRLinux, path)
	if err == nil || !strings.Contains(err.Error(), "unsupported SR Linux BGP aggregate route statement") {
		t.Fatalf("configparse.ParseConfig() error = %v, want unsupported SR Linux aggregate", err)
	}
	result, err := configparse.ParseConfigWithWarnings(model.KindSRLinux, path)
	if err != nil {
		t.Fatalf("configparse.ParseConfigWithWarnings() error = %v", err)
	}
	want := []configparse.UnsupportedStatement{{Vendor: "srlinux", File: path, Line: 1, Text: config, Reason: "unsupported SR Linux BGP aggregate route statement"}}
	if !reflect.DeepEqual(result.Warnings, want) {
		t.Fatalf("Warnings = %#v, want %#v", result.Warnings, want)
	}
}

func TestParseSRLinuxNetworkInstanceInterfacesAndStaticRoutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "srl.cfg")
	config := strings.Join([]string{
		"set / interface ethernet-1/1 subinterface 0 ipv4 address 192.0.2.1/30",
		"set / network-instance tenant-a interface ethernet-1/1.0",
		"set / network-instance tenant-a next-hop-groups group tenant-a-nhg nexthop 1 ip-address 192.0.2.2",
		"set / network-instance tenant-a static-routes route 10.0.0.0/24 next-hop-group tenant-a-nhg admin-state enable",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := configparse.ParseConfig(model.KindSRLinux, path)
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if got := interfaceByName(cfg.Interfaces, "ethernet-1/1").VRF; got != "tenant-a" {
		t.Fatalf("ethernet-1/1 VRF = %q, want tenant-a", got)
	}
	if len(cfg.Routes) != 1 || cfg.Routes[0].NetworkInstance != "tenant-a" {
		t.Fatalf("routes = %#v, want one tenant-a route", cfg.Routes)
	}
}

func TestParseSRLinuxConfig(t *testing.T) {
	cfg, err := configparse.ParseConfig("srlinux", filepath.Join("..", "..", "..", "labs", "base-wan", "configs", "srlinux", "core-gz.cfg"))
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if cfg.ASN != 65100 || cfg.RouterID != "10.255.100.3" {
		t.Fatalf("BGP = ASN %d router-id %s", cfg.ASN, cfg.RouterID)
	}
	if len(cfg.Interfaces) != 6 || len(cfg.Neighbors) != 6 {
		t.Fatalf("interfaces/neighbors = %d/%d, want 6/6", len(cfg.Interfaces), len(cfg.Neighbors))
	}
	policy := routePolicyByName(cfg.RoutePolicies, "GZ-NH-SELF")
	if policy == nil || len(policy.Rules) < 1 || !policy.Rules[0].SetNextHopSelf {
		t.Fatalf("GZ-NH-SELF policy = %#v, want set next-hop self", policy)
	}
	neighbor := neighborByAddress(cfg.Neighbors, "198.18.20.8")
	if neighbor == nil || neighbor.ExportPolicy != "GZ-NH-SELF" {
		t.Fatalf("neighbor 198.18.20.8 = %#v, want export policy GZ-NH-SELF", neighbor)
	}
	for _, addr := range []string{"198.18.20.5", "198.18.20.2"} {
		neighbor := neighborByAddress(cfg.Neighbors, addr)
		if neighbor == nil || neighbor.ExportPolicy != "" {
			t.Fatalf("neighbor %s = %#v, want no export policy", addr, neighbor)
		}
	}
}

func TestParseSRLinuxOSPFConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "srl.cfg")
	if err := os.WriteFile(path, []byte(`
set / system name host-name srl1
set / interface ethernet-1/1 subinterface 0 ipv4 address 198.51.100.0/31
set / interface lo0 subinterface 0 ipv4 address 10.255.1.1/32
set / network-instance default protocols ospf instance default admin-state enable
set / network-instance default protocols ospf instance default router-id 10.255.1.1
set / network-instance default protocols ospf instance default area 0.0.0.0 interface ethernet-1/1.0 admin-state enable
set / network-instance default protocols ospf instance default area 0.0.0.0 interface ethernet-1/1.0 metric 30
set / network-instance default protocols ospf instance default area 0.0.0.0 interface lo0.0 passive true
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := configparse.ParseConfig("srlinux", path)
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if !cfg.OSPF.Enabled || cfg.OSPF.RouterID != "10.255.1.1" {
		t.Fatalf("OSPF = %#v, want enabled router-id", cfg.OSPF)
	}
	if got := cfg.OSPF.Interfaces["ethernet-1/1.0"]; got.Area != "0" || got.Cost != 30 {
		t.Fatalf("ethernet-1/1.0 OSPF = %#v, want area 0 cost 30", got)
	}
	if got := cfg.OSPF.Interfaces["lo0.0"]; got.Area != "0" || !got.Passive {
		t.Fatalf("lo0.0 OSPF = %#v, want passive area", got)
	}
}

func TestParseSRLinuxNextHopSelf(t *testing.T) {
	path := filepath.Join(t.TempDir(), "srl.cfg")
	if err := os.WriteFile(path, []byte(`
set / network-instance default protocols bgp autonomous-system 65100
set / network-instance default protocols bgp group core peer-as 65100
set / network-instance default protocols bgp group core afi-safi ipv4-unicast ipv4-unicast next-hop-self true
set / network-instance default protocols bgp neighbor 198.18.20.8 peer-group core
set / network-instance default protocols bgp neighbor 198.18.20.8 admin-state enable
set / network-instance default protocols bgp neighbor 198.18.20.9 peer-group core
set / network-instance default protocols bgp neighbor 198.18.20.9 afi-safi ipv4-unicast ipv4-unicast next-hop-self true
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := configparse.ParseConfig("srlinux", path)
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if len(cfg.Neighbors) != 2 {
		t.Fatalf("neighbors = %#v", cfg.Neighbors)
	}
	for _, neighbor := range cfg.Neighbors {
		if !neighbor.NextHopSelf {
			t.Fatalf("neighbor %s NextHopSelf = false", neighbor.Address)
		}
	}
}
