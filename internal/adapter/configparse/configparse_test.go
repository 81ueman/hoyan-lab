package configparse_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/adapter/configparse"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestParseConfigWithWarningsCurrentLabConfigs(t *testing.T) {
	tests := []struct {
		kind model.DeviceKind
		glob string
	}{
		{kind: model.KindFRR, glob: filepath.Join("..", "..", "..", "labs", "base-wan", "configs", "frr", "*", "frr.conf")},
		{kind: model.KindCEOS, glob: filepath.Join("..", "..", "..", "labs", "base-wan", "configs", "ceos", "*.cfg")},
		{kind: model.KindSRLinux, glob: filepath.Join("..", "..", "..", "labs", "base-wan", "configs", "srlinux", "*.cfg")},
		{kind: model.KindFRR, glob: filepath.Join("..", "..", "..", "labs", "ospf-basic", "configs", "frr", "*", "frr.conf")},
		{kind: model.KindCEOS, glob: filepath.Join("..", "..", "..", "labs", "ospf-basic", "configs", "ceos", "*.cfg")},
		{kind: model.KindSRLinux, glob: filepath.Join("..", "..", "..", "labs", "ospf-basic", "configs", "srlinux", "*.cfg")},
	}
	for _, tt := range tests {
		paths, err := filepath.Glob(tt.glob)
		if err != nil {
			t.Fatalf("Glob(%q) error = %v", tt.glob, err)
		}
		for _, path := range paths {
			t.Run(path, func(t *testing.T) {
				result, err := configparse.ParseConfigWithWarnings(tt.kind, path)
				if err != nil {
					t.Fatalf("configparse.ParseConfigWithWarnings() error = %v", err)
				}
				if len(result.Warnings) != 0 {
					t.Fatalf("Warnings = %#v, want none", result.Warnings)
				}
			})
		}
	}
}

func TestDeviceParserEntrypoints(t *testing.T) {
	frr, err := configparse.FRRParser{}.Parse("frr.conf", `
hostname frr1
interface eth1
 ip address 192.0.2.1/30
!
router bgp 65001
 neighbor 192.0.2.2 remote-as 65002
 address-family ipv4 unicast
  neighbor 192.0.2.2 activate
 exit-address-family
`, false)
	if err != nil {
		t.Fatalf("configparse.FRRParser.Parse() error = %v", err)
	}
	if frr.Config.Hostname != "frr1" || len(frr.Config.Neighbors) != 1 {
		t.Fatalf("FRR parser result = %#v", frr.Config)
	}

	ceos, err := configparse.CEOSParser{}.Parse("ceos.cfg", `
hostname ceos1
interface Ethernet1
   vrf tenant-a
   ip address 192.0.2.1/30
`, false)
	if err != nil {
		t.Fatalf("configparse.CEOSParser.Parse() error = %v", err)
	}
	if ceos.Config.Hostname != "ceos1" || interfaceByName(ceos.Config.Interfaces, "Ethernet1").VRF != "tenant-a" {
		t.Fatalf("cEOS parser result = %#v", ceos.Config)
	}

	srl, err := configparse.SRLinuxParser{}.Parse("srl.cfg", `
set / system name host-name srl1
set / interface ethernet-1/1 subinterface 0 ipv4 address 192.0.2.1/30
set / network-instance tenant-a interface ethernet-1/1.0
`, false)
	if err != nil {
		t.Fatalf("configparse.SRLinuxParser.Parse() error = %v", err)
	}
	if srl.Config.Hostname != "srl1" || interfaceByName(srl.Config.Interfaces, "ethernet-1/1").VRF != "tenant-a" {
		t.Fatalf("SR Linux parser result = %#v", srl.Config)
	}
}

// ---------------------------------------------------------------------------
// Helpers shared across configparse test files
// ---------------------------------------------------------------------------

func prefixListsWithoutMatches(in []model.PrefixList) []model.PrefixList {
	out := make([]model.PrefixList, len(in))
	for i, prefixList := range in {
		out[i] = prefixList
		out[i].Rules = append([]model.PrefixListRule(nil), prefixList.Rules...)
		for j := range out[i].Rules {
			out[i].Rules[j].Match = nil
		}
	}
	return out
}

func interfaceByName(interfaces []model.Interface, name string) model.Interface {
	for _, iface := range interfaces {
		if iface.Name == name {
			return iface
		}
	}
	return model.Interface{}
}

func routePolicyByName(policies []model.RoutePolicy, name string) *model.RoutePolicy {
	for i := range policies {
		if policies[i].Name == name {
			return &policies[i]
		}
	}
	return nil
}

func prefixListByName(prefixLists []model.PrefixList, name string) *model.PrefixList {
	for i := range prefixLists {
		if prefixLists[i].Name == name {
			return &prefixLists[i]
		}
	}
	return nil
}

func asPathListByName(lists []model.ASPathList, name string) *model.ASPathList {
	for i := range lists {
		if lists[i].Name == name {
			return &lists[i]
		}
	}
	return nil
}

func communityListByName(lists []model.CommunityList, name string) *model.CommunityList {
	for i := range lists {
		if lists[i].Name == name {
			return &lists[i]
		}
	}
	return nil
}

func neighborByAddress(neighbors []model.BGPNeighbor, addr string) *model.BGPNeighbor {
	for i := range neighbors {
		if neighbors[i].Address == addr {
			return &neighbors[i]
		}
	}
	return nil
}

func parseFRRConfigText(t *testing.T, config string) configparse.ParsedConfig {
	t.Helper()
	cfg, err := parseFRRConfigTextResult(t, config)
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	return cfg
}

func parseFRRConfigTextResult(t *testing.T, config string) (configparse.ParsedConfig, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "frr.conf")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return configparse.ParseConfig("frr", path)
}

func parseCEOSConfigText(t *testing.T, config string) configparse.ParsedConfig {
	t.Helper()
	cfg, err := parseCEOSConfigTextResult(t, config)
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	return cfg
}

func parseCEOSConfigTextResult(t *testing.T, config string) (configparse.ParsedConfig, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ceos.cfg")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	return configparse.ParseConfig("ceos", path)
}
