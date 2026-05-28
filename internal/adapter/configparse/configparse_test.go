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

func TestParseFRRConfig(t *testing.T) {
	cfg, err := configparse.ParseConfig("frr", filepath.Join("..", "..", "..", "labs", "base-wan", "configs", "frr", "bj-edge1", "frr.conf"))
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if cfg.ASN != 65001 || cfg.RouterID != "10.255.1.1" {
		t.Fatalf("BGP = ASN %d router-id %s", cfg.ASN, cfg.RouterID)
	}
	if len(cfg.Interfaces) != 4 {
		t.Fatalf("interfaces = %d, want 4", len(cfg.Interfaces))
	}
	if len(cfg.Neighbors) != 3 {
		t.Fatalf("neighbors = %d, want 3", len(cfg.Neighbors))
	}
}

func TestParseFRROSPFConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frr.conf")
	if err := os.WriteFile(path, []byte(`hostname r1
interface lo
 ip address 10.255.1.1/32
interface eth1
 ip address 198.51.100.0/31
 ip ospf area 0
 ip ospf cost 10
router ospf
 ospf router-id 10.255.1.1
 passive-interface lo
 network 10.255.1.1/32 area 0
 network 198.51.100.0/31 area 0
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := configparse.ParseConfig(model.KindFRR, path)
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if !cfg.OSPF.Enabled || cfg.OSPF.RouterID != "10.255.1.1" {
		t.Fatalf("OSPF = %#v, want enabled router-id", cfg.OSPF)
	}
	if len(cfg.OSPF.Networks) != 2 {
		t.Fatalf("OSPF networks = %#v, want two", cfg.OSPF.Networks)
	}
	if got := cfg.OSPF.Interfaces["eth1"]; got.Area != "0" || got.Cost != 10 {
		t.Fatalf("eth1 OSPF = %#v, want area 0 cost 10", got)
	}
	if got := cfg.OSPF.Interfaces["eth1"]; got.NetworkType != "" {
		t.Fatalf("eth1 OSPF network type = %q, want empty", got.NetworkType)
	}
	if got := cfg.OSPF.Interfaces["lo"]; !got.Passive {
		t.Fatalf("lo OSPF = %#v, want passive", got)
	}
}

func TestParseFRROSPFNetworkType(t *testing.T) {
	cfg := parseFRRConfigText(t, `
interface eth1
 ip address 198.51.100.1/29
 ip ospf area 0
 ip ospf network broadcast
router ospf
 network 198.51.100.0/29 area 0
`)
	got := cfg.OSPF.Interfaces["eth1"]
	if got.NetworkType != "broadcast" {
		t.Fatalf("eth1 OSPF network type = %q, want broadcast", got.NetworkType)
	}
}

func TestParseFRRVRFScopedOSPFConfig(t *testing.T) {
	cfg := parseFRRConfigText(t, `
hostname r1
interface eth1 vrf tenant-a
 ip address 192.0.2.1/30
 ip ospf area 0
 ip ospf cost 10
interface a-svc vrf tenant-a
 ip address 10.10.0.1/32
router ospf vrf tenant-a
 ospf router-id 10.255.1.1
 passive-interface a-svc
 network 192.0.2.0/30 area 0
 network 10.10.0.1/32 area 0
`)
	if cfg.OSPF.Enabled {
		t.Fatalf("default OSPF = %#v, want disabled", cfg.OSPF)
	}
	if len(cfg.OSPFProcesses) != 1 {
		t.Fatalf("OSPFProcesses = %#v, want one VRF process", cfg.OSPFProcesses)
	}
	ospf := cfg.OSPFProcesses[0]
	if ospf.NetworkInstance != "tenant-a" || !ospf.Enabled || ospf.RouterID != "10.255.1.1" {
		t.Fatalf("VRF OSPF = %#v, want tenant-a enabled router-id", ospf)
	}
	if len(ospf.Networks) != 2 {
		t.Fatalf("VRF OSPF networks = %#v, want two", ospf.Networks)
	}
	if got := ospf.Interfaces["eth1"]; got.Area != "0" || got.Cost != 10 {
		t.Fatalf("eth1 VRF OSPF = %#v, want area 0 cost 10", got)
	}
	if got := ospf.Interfaces["a-svc"]; !got.Passive {
		t.Fatalf("a-svc VRF OSPF = %#v, want passive", got)
	}
}

func TestParseCEOSOSPFConfig(t *testing.T) {
	cfg := parseCEOSConfigText(t, `
hostname ceos1
interface Loopback0
   ip address 10.255.1.1/32
interface Ethernet1
   ip address 198.51.100.0/31
   ip ospf area 0.0.0.0
   ip ospf cost 20
router ospf 1
   router-id 10.255.1.1
   passive-interface Loopback0
   network 10.255.1.1/32 area 0.0.0.0
   network 198.51.100.0/31 area 0.0.0.0
`)
	if !cfg.OSPF.Enabled || cfg.OSPF.RouterID != "10.255.1.1" {
		t.Fatalf("OSPF = %#v, want enabled router-id", cfg.OSPF)
	}
	if len(cfg.OSPF.Networks) != 2 {
		t.Fatalf("OSPF networks = %#v, want two", cfg.OSPF.Networks)
	}
	if got := cfg.OSPF.Interfaces["Ethernet1"]; got.Area != "0" || got.Cost != 20 {
		t.Fatalf("Ethernet1 OSPF = %#v, want area 0 cost 20", got)
	}
	if got := cfg.OSPF.Interfaces["Loopback0"]; !got.Passive {
		t.Fatalf("Loopback0 OSPF = %#v, want passive", got)
	}
}

func TestParseFRROSPFStubNSSAAreasAndRedistribute(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frr.conf")
	if err := os.WriteFile(path, []byte(`router ospf
 area 1 stub
 area 2 nssa default-information-originate
 redistribute connected
 redistribute static metric 44 metric-type 1 route-map STATIC-TO-OSPF
 redistribute bgp metric-type 2 metric 12
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := configparse.ParseConfig(model.KindFRR, path)
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if cfg.OSPF.Areas["1"].Kind != model.OSPFAreaStub {
		t.Fatalf("area 1 = %#v, want stub", cfg.OSPF.Areas["1"])
	}
	if area := cfg.OSPF.Areas["2"]; area.Kind != model.OSPFAreaNSSA || !area.DefaultInformationOriginate {
		t.Fatalf("area 2 = %#v, want NSSA default-information-originate", area)
	}
	if len(cfg.OSPF.Redistribute) != 3 ||
		cfg.OSPF.Redistribute[0].Kind != model.RouteSourceConnected ||
		cfg.OSPF.Redistribute[1].Kind != model.RouteSourceStatic ||
		cfg.OSPF.Redistribute[1].Metric != 44 ||
		cfg.OSPF.Redistribute[1].MetricType != 1 ||
		cfg.OSPF.Redistribute[1].RouteMap != "STATIC-TO-OSPF" ||
		cfg.OSPF.Redistribute[2].Kind != model.RouteSourceBGP ||
		cfg.OSPF.Redistribute[2].Metric != 12 ||
		cfg.OSPF.Redistribute[2].MetricType != 2 {
		t.Fatalf("redistribute = %#v, want connected, static options, and bgp", cfg.OSPF.Redistribute)
	}
}

func TestParseFRROSPFUnsupportedAreaOptionWarns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frr.conf")
	if err := os.WriteFile(path, []byte(`router ospf
 area 1 stub totally
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	result, err := configparse.ParseConfigWithWarnings(model.KindFRR, path)
	if err != nil {
		t.Fatalf("configparse.ParseConfigWithWarnings() error = %v", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0].Reason, "unsupported FRR OSPF area option") {
		t.Fatalf("warnings = %#v, want unsupported area option warning", result.Warnings)
	}
}

func TestParseNftablesConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nftables.conf")
	if err := os.WriteFile(path, []byte(`table inet BLOCK_HTTP_TO_HZ {
  chain forward {
    type filter hook forward priority 0; policy accept;
    oifname "eth1" ip protocol tcp ip daddr 10.4.0.0/16 tcp dport 80 drop
    iifname "eth2" ip protocol icmp ip daddr 10.5.0.0/16 drop
  }
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	acls, bindings, err := configparse.ParseNftablesACLConfig(path)
	if err != nil {
		t.Fatalf("configparse.ParseNftablesACLConfig() error = %v", err)
	}
	if len(acls) != 1 || len(acls[0].Rules) != 2 {
		t.Fatalf("ACLs = %#v, want one ACL with two rules", acls)
	}
	acl := acls[0]
	if acl.Name != "BLOCK-HTTP-TO-HZ" || acl.DefaultAction != model.ACLDefaultPermit {
		t.Fatalf("acl metadata = %#v", acl)
	}
	if acl.Rules[0].Match.Protocol != "tcp" || acl.Rules[0].Action != model.ACLDeny || !acl.Rules[0].Match.DstPort.Contains(80) {
		t.Fatalf("first rule = %#v", acl.Rules[0])
	}
	if len(bindings) != 2 || bindings[0].Direction != "egress" || bindings[0].Interface != "eth1" || bindings[1].Direction != "ingress" || bindings[1].Interface != "eth2" {
		t.Fatalf("bindings = %#v", bindings)
	}
	if acl.Rules[0].Source.Vendor != "nftables" || acl.Rules[0].Source.File != path || acl.Rules[0].Source.Raw == "" {
		t.Fatalf("source = %#v", acl.Rules[0].Source)
	}
}

func TestParseFRRLikeACLsBuildNormalizedIRWithPermitAndBinding(t *testing.T) {
	cfg := parseCEOSConfigText(t, `
hostname r1
interface Ethernet1
   no switchport
   ip address 192.0.2.1/31
   ip access-group WEB-FILTER out
!
ip access-list WEB-FILTER
   10 permit tcp any 10.0.0.0/24 eq 443
   20 deny tcp any 10.0.0.0/24 eq 80
!
`)
	if len(cfg.ACLs) != 1 {
		t.Fatalf("ACLs = %#v, want one ACL", cfg.ACLs)
	}
	acl := cfg.ACLs[0]
	if acl.Name != "WEB-FILTER" || acl.Vendor != model.KindCEOS || acl.DefaultAction != model.ACLDefaultDeny {
		t.Fatalf("ACL metadata = %#v", acl)
	}
	if len(acl.Rules) != 2 || acl.Rules[0].Action != model.ACLPermit || acl.Rules[0].Seq != 10 || acl.Rules[1].Action != model.ACLDeny || acl.Rules[1].Seq != 20 {
		t.Fatalf("ACL rules = %#v, want permit seq 10 then deny seq 20", acl.Rules)
	}
	if acl.Rules[0].Match.Protocol != "tcp" || !acl.Rules[0].Match.DstPort.Contains(443) {
		t.Fatalf("permit rule match = %#v, want tcp/443", acl.Rules[0].Match)
	}
	if len(cfg.ACLBindings) != 1 || cfg.ACLBindings[0].ACLName != "WEB-FILTER" || cfg.ACLBindings[0].Interface != "Ethernet1" || cfg.ACLBindings[0].Direction != "egress" {
		t.Fatalf("ACL bindings = %#v", cfg.ACLBindings)
	}
}

func TestParseNftablesRejectsUnsupportedStatement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nftables.conf")
	if err := os.WriteFile(path, []byte(`table inet T {
  chain forward {
    type filter hook forward priority 0; policy accept;
    oifname "eth1" ip saddr 10.0.0.0/8 drop
  }
}
`), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, _, err := configparse.ParseNftablesACLConfig(path)
	if err == nil || !strings.Contains(err.Error(), "unsupported nftables ip match") {
		t.Fatalf("configparse.ParseNftablesACLConfig() error = %v", err)
	}
}

func TestParseCoreBJRouteMapConfig(t *testing.T) {
	cfg, err := configparse.ParseConfig("frr", filepath.Join("..", "..", "..", "labs", "base-wan", "configs", "frr", "core-bj", "frr.conf"))
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if prefixListByName(cfg.PrefixLists, "BJ-LOCAL") == nil {
		t.Fatalf("BJ-LOCAL prefix-list not parsed: %#v", cfg.PrefixLists)
	}
	policy := routePolicyByName(cfg.RoutePolicies, "PREFER-BJ-LOCAL")
	if policy == nil || len(policy.Rules) != 3 || policy.Rules[0].SetLocalPrefDelta == nil || *policy.Rules[0].SetLocalPrefDelta != 125 || policy.Rules[1].SetLocalPref == nil || *policy.Rules[1].SetLocalPref != 200 {
		t.Fatalf("PREFER-BJ-LOCAL = %#v", policy)
	}
	if asPathListByName(cfg.ASPathLists, "FROM-BJ") == nil {
		t.Fatalf("FROM-BJ as-path list not parsed: %#v", cfg.ASPathLists)
	}
	if communityListByName(cfg.CommunityLists, "BJ-DIRECT") == nil {
		t.Fatalf("BJ-DIRECT community-list not parsed: %#v", cfg.CommunityLists)
	}
	for _, addr := range []string{"198.18.10.0", "198.18.10.2"} {
		neighbor := neighborByAddress(cfg.Neighbors, addr)
		if neighbor == nil || neighbor.ImportPolicy != "PREFER-BJ-LOCAL" {
			t.Fatalf("neighbor %s = %#v", addr, neighbor)
		}
	}
}

func TestParseFRRRouteMaps(t *testing.T) {
	config := `
hostname r1
ip prefix-list PL-IN seq 10 permit 10.0.0.0/24
ip prefix-list PL-OUT permit 10.0.1.0/24
route-map RM-IN permit 10
 match ip address prefix-list PL-IN
 set local-preference 250
route-map RM-OUT permit 20
 match ip address prefix-list PL-OUT
 set metric 77
 set as-path prepend 65002 65002
 set community 65001:100 additive
 set origin incomplete
route-map RM-DENY deny 30
 match ip address prefix-list PL-IN
router bgp 65001
 neighbor 192.0.2.1 remote-as 65002
 address-family ipv4 unicast
  neighbor 192.0.2.1 activate
  neighbor 192.0.2.1 route-map RM-IN in
  neighbor 192.0.2.1 route-map RM-OUT out
 exit-address-family
`
	path := filepath.Join(t.TempDir(), "frr.conf")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	cfg, err := configparse.ParseConfig("frr", path)
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if got, want := prefixListsWithoutMatches(cfg.PrefixLists), []model.PrefixList{
		{Name: "PL-IN", Rules: []model.PrefixListRule{{Seq: 10, Action: "permit", Prefix: "10.0.0.0/24"}}},
		{Name: "PL-OUT", Rules: []model.PrefixListRule{{Seq: 0, Action: "permit", Prefix: "10.0.1.0/24"}}},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("PrefixLists = %#v, want %#v", got, want)
	}
	if len(cfg.RoutePolicies) != 3 {
		t.Fatalf("RoutePolicies = %#v, want 3 policies", cfg.RoutePolicies)
	}
	rmIn := routePolicyByName(cfg.RoutePolicies, "RM-IN")
	if rmIn == nil || len(rmIn.Rules) != 1 || rmIn.Rules[0].MatchPrefixList != "PL-IN" || rmIn.Rules[0].SetLocalPref == nil || *rmIn.Rules[0].SetLocalPref != 250 {
		t.Fatalf("RM-IN = %#v", rmIn)
	}
	rmOut := routePolicyByName(cfg.RoutePolicies, "RM-OUT")
	if rmOut == nil || len(rmOut.Rules) != 1 || rmOut.Rules[0].MatchPrefixList != "PL-OUT" || rmOut.Rules[0].SetMED == nil || *rmOut.Rules[0].SetMED != 77 || !reflect.DeepEqual(rmOut.Rules[0].SetASPathPrepend, []uint32{65002, 65002}) || !reflect.DeepEqual(rmOut.Rules[0].SetCommunities, []string{"65001:100"}) || !rmOut.Rules[0].SetCommunityAdditive || rmOut.Rules[0].SetOriginCode != "incomplete" {
		t.Fatalf("RM-OUT = %#v", rmOut)
	}
	rmDeny := routePolicyByName(cfg.RoutePolicies, "RM-DENY")
	if rmDeny == nil || len(rmDeny.Rules) != 1 || rmDeny.Rules[0].Action != "deny" || rmDeny.Rules[0].MatchPrefixList != "PL-IN" {
		t.Fatalf("RM-DENY = %#v", rmDeny)
	}
	if len(cfg.Neighbors) != 1 || cfg.Neighbors[0].ImportPolicy != "RM-IN" || cfg.Neighbors[0].ExportPolicy != "RM-OUT" {
		t.Fatalf("Neighbors = %#v", cfg.Neighbors)
	}
}

func TestParseFRRRouteMapWithoutMatchIsMatchAny(t *testing.T) {
	cfg := parseFRRConfigText(t, `
route-map RM permit 10
 set metric 12
`)
	policy := routePolicyByName(cfg.RoutePolicies, "RM")
	if policy == nil || len(policy.Rules) != 1 || policy.Rules[0].MatchPrefixList != "" || policy.Rules[0].SetMED == nil || *policy.Rules[0].SetMED != 12 {
		t.Fatalf("RM = %#v", policy)
	}
}

func TestParseFRRRouteMapRejectsUnsupportedMatch(t *testing.T) {
	for _, stmt := range []string{
		"match source-protocol bgp",
		"match ip next-hop address 192.0.2.1",
	} {
		t.Run(stmt, func(t *testing.T) {
			_, err := parseFRRConfigTextResult(t, "route-map RM permit 10\n "+stmt+"\n set local-preference 200\n")
			if err == nil || !strings.Contains(err.Error(), "unsupported FRR route-map match statement") {
				t.Fatalf("configparse.ParseConfig() error = %v, want unsupported match", err)
			}
		})
	}
}

func TestParseConfigWithWarningsReportsUnsupportedFRRRouteMapStatements(t *testing.T) {
	config := `
hostname r1
route-map RM permit 10
 match source-protocol bgp
 set weight 50
 set local-preference 200
`
	path := filepath.Join(t.TempDir(), "frr.conf")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	result, err := configparse.ParseConfigWithWarnings("frr", path)
	if err != nil {
		t.Fatalf("configparse.ParseConfigWithWarnings() error = %v", err)
	}
	if result.Config.Hostname != "r1" {
		t.Fatalf("Hostname = %q, want r1", result.Config.Hostname)
	}
	policy := routePolicyByName(result.Config.RoutePolicies, "RM")
	if policy == nil || len(policy.Rules) != 1 || policy.Rules[0].SetLocalPref == nil || *policy.Rules[0].SetLocalPref != 200 {
		t.Fatalf("RM = %#v", policy)
	}
	want := []configparse.UnsupportedStatement{
		{Vendor: "frr", File: path, Line: 4, Text: "match source-protocol bgp", Reason: "unsupported FRR route-map match statement"},
		{Vendor: "frr", File: path, Line: 5, Text: "set weight 50", Reason: "unsupported FRR route-map statement"},
	}
	if !reflect.DeepEqual(result.Warnings, want) {
		t.Fatalf("Warnings = %#v, want %#v", result.Warnings, want)
	}
}

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

func TestParseFRRRouteMapMatchExtensions(t *testing.T) {
	cfg := parseFRRConfigText(t, `
bgp as-path access-list FROM-BJ permit ^65001$
bgp community-list standard BJ-COMM permit 65001:100
ip prefix-list NH seq 10 permit 198.18.10.0/30
route-map RM permit 10
 match as-path FROM-BJ
 match community BJ-COMM exact-match
 match ip next-hop prefix-list NH
 set local-preference +50
 set metric -10
`)
	if got, want := cfg.ASPathLists, []model.ASPathList{{Name: "FROM-BJ", Rules: []model.StringListRule{{Action: "permit", Pattern: "^65001$"}}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ASPathLists = %#v, want %#v", got, want)
	}
	if got, want := cfg.CommunityLists, []model.CommunityList{{Name: "BJ-COMM", Rules: []model.StringListRule{{Action: "permit", Pattern: "65001:100"}}}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("CommunityLists = %#v, want %#v", got, want)
	}
	policy := routePolicyByName(cfg.RoutePolicies, "RM")
	if policy == nil || len(policy.Rules) != 1 {
		t.Fatalf("RM = %#v", policy)
	}
	rule := policy.Rules[0]
	if rule.MatchASPathList != "FROM-BJ" || rule.MatchCommunityList != "BJ-COMM" || !rule.MatchCommunityExact || rule.MatchNextHopPrefixList != "NH" {
		t.Fatalf("match fields = %#v", rule)
	}
	if rule.SetLocalPrefDelta == nil || *rule.SetLocalPrefDelta != 50 || rule.SetMEDDelta == nil || *rule.SetMEDDelta != -10 {
		t.Fatalf("delta fields = %#v", rule)
	}
}

func TestParseFRRRouteMapSetIPAddressNextHop(t *testing.T) {
	cfg := parseFRRConfigText(t, `
route-map RM permit 10
 set ip next-hop 192.0.2.1
`)
	policy := routePolicyByName(cfg.RoutePolicies, "RM")
	if policy == nil || len(policy.Rules) != 1 {
		t.Fatalf("RM = %#v", policy)
	}
	if got := policy.Rules[0].SetNextHop; got != "192.0.2.1" {
		t.Fatalf("SetNextHop = %q, want 192.0.2.1", got)
	}
}

func TestParseFRRPrefixListDenyAndOrder(t *testing.T) {
	cfg := parseFRRConfigText(t, `
ip prefix-list PL seq 20 permit 10.1.0.0/16
ip prefix-list PL seq 10 deny 10.0.0.0/8
`)
	got := prefixListsWithoutMatches(cfg.PrefixLists)
	want := []model.PrefixList{{Name: "PL", Rules: []model.PrefixListRule{
		{Seq: 10, Action: "deny", Prefix: "10.0.0.0/8"},
		{Seq: 20, Action: "permit", Prefix: "10.1.0.0/16"},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PrefixLists = %#v, want %#v", got, want)
	}
}

func TestParseFRRPrefixListLeGe(t *testing.T) {
	cfg := parseFRRConfigText(t, `
ip prefix-list PL permit any
ip prefix-list PL seq 10 permit 10.0.0.0/8 ge 16 le 24
`)
	got := prefixListsWithoutMatches(cfg.PrefixLists)
	want := []model.PrefixList{{Name: "PL", Rules: []model.PrefixListRule{
		{Seq: 0, Action: "permit", Prefix: "any"},
		{Seq: 10, Action: "permit", Prefix: "10.0.0.0/8", Ge: 16, Le: 24},
	}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("PrefixLists = %#v, want %#v", got, want)
	}
}

func TestParseCoreHZEgressRouteMapConfig(t *testing.T) {
	cfg, err := configparse.ParseConfig("frr", filepath.Join("..", "..", "..", "labs", "base-wan", "configs", "frr", "core-hz", "frr.conf"))
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if prefixListByName(cfg.PrefixLists, "HZ-LOCAL") == nil {
		t.Fatalf("HZ-LOCAL prefix-list not parsed: %#v", cfg.PrefixLists)
	}
	policy := routePolicyByName(cfg.RoutePolicies, "HZ-TRANSIT-OUT")
	if policy == nil || len(policy.Rules) != 2 || policy.Rules[0].SetMEDDelta == nil || *policy.Rules[0].SetMEDDelta != 7 || !reflect.DeepEqual(policy.Rules[0].SetASPathPrepend, []uint32{65100, 65100}) || policy.Rules[0].SetOriginCode != "incomplete" {
		t.Fatalf("HZ-TRANSIT-OUT = %#v", policy)
	}
	neighbor := neighborByAddress(cfg.Neighbors, "198.18.30.7")
	if neighbor == nil || neighbor.ExportPolicy != "HZ-TRANSIT-OUT" {
		t.Fatalf("neighbor 198.18.30.7 = %#v", neighbor)
	}
}

func TestParseCEOSConfig(t *testing.T) {
	cfg, err := configparse.ParseConfig("ceos", filepath.Join("..", "..", "..", "labs", "base-wan", "configs", "ceos", "core-sh.cfg"))
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if cfg.ASN != 65100 || cfg.RouterID != "10.255.100.2" {
		t.Fatalf("BGP = ASN %d router-id %s", cfg.ASN, cfg.RouterID)
	}
	if len(cfg.Neighbors) != 6 {
		t.Fatalf("neighbors = %d, want 6", len(cfg.Neighbors))
	}
	if prefixListByName(cfg.PrefixLists, "SH-LOCAL") == nil {
		t.Fatalf("SH-LOCAL prefix-list not parsed: %#v", cfg.PrefixLists)
	}
	policy := routePolicyByName(cfg.RoutePolicies, "PREFER-SH-LOCAL")
	if policy == nil || len(policy.Rules) != 2 || policy.Rules[0].MatchPrefixList != "SH-LOCAL" || policy.Rules[0].SetLocalPref == nil || *policy.Rules[0].SetLocalPref != 225 {
		t.Fatalf("PREFER-SH-LOCAL = %#v", policy)
	}
	policy = routePolicyByName(cfg.RoutePolicies, "SH-TRANSIT-OUT")
	if policy == nil || len(policy.Rules) != 2 || policy.Rules[0].MatchPrefixList != "SH-LOCAL" || policy.Rules[0].SetMED == nil || *policy.Rules[0].SetMED != 9 {
		t.Fatalf("SH-TRANSIT-OUT = %#v", policy)
	}
	neighbor := neighborByAddress(cfg.Neighbors, "198.18.10.4")
	if neighbor == nil || neighbor.ImportPolicy != "PREFER-SH-LOCAL" {
		t.Fatalf("neighbor 198.18.10.4 = %#v", neighbor)
	}
	neighbor = neighborByAddress(cfg.Neighbors, "198.18.30.3")
	if neighbor == nil || neighbor.ExportPolicy != "SH-TRANSIT-OUT" {
		t.Fatalf("neighbor 198.18.30.3 = %#v", neighbor)
	}
	var found bool
	for _, iface := range cfg.Interfaces {
		if iface.Name == "Ethernet1" && iface.Address == "198.18.10.5/31" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Ethernet1 address not parsed: %#v", cfg.Interfaces)
	}
}

func TestParseCEOSRouteMaps(t *testing.T) {
	config := `
hostname ceos1
ip prefix-list PL-IN seq 10 permit 10.0.0.0/24
ip prefix-list PL-IN seq 20 deny 10.0.1.0/24
ip prefix-list PL-OUT permit 10.0.2.0/24 ge 25 le 28
route-map RM-IN permit 10
   match ip address prefix-list PL-IN
   set local-preference 250
route-map RM-OUT permit 20
   match ip address prefix-list PL-OUT
   set metric 77
route-map RM-DENY deny 30
   match ip address prefix-list PL-IN
router bgp 65001
   router-id 10.255.0.1
   neighbor 192.0.2.1 remote-as 65002
   address-family ipv4
      neighbor 192.0.2.1 activate
      neighbor 192.0.2.1 route-map RM-IN in
      neighbor 192.0.2.1 route-map RM-OUT out
`
	cfg := parseCEOSConfigText(t, config)
	if got, want := prefixListsWithoutMatches(cfg.PrefixLists), []model.PrefixList{
		{Name: "PL-IN", Rules: []model.PrefixListRule{
			{Seq: 10, Action: "permit", Prefix: "10.0.0.0/24"},
			{Seq: 20, Action: "deny", Prefix: "10.0.1.0/24"},
		}},
		{Name: "PL-OUT", Rules: []model.PrefixListRule{{Seq: 0, Action: "permit", Prefix: "10.0.2.0/24", Ge: 25, Le: 28}}},
	}; !reflect.DeepEqual(got, want) {
		t.Fatalf("PrefixLists = %#v, want %#v", got, want)
	}
	rmIn := routePolicyByName(cfg.RoutePolicies, "RM-IN")
	if rmIn == nil || len(rmIn.Rules) != 1 || rmIn.Rules[0].MatchPrefixList != "PL-IN" || rmIn.Rules[0].SetLocalPref == nil || *rmIn.Rules[0].SetLocalPref != 250 {
		t.Fatalf("RM-IN = %#v", rmIn)
	}
	rmOut := routePolicyByName(cfg.RoutePolicies, "RM-OUT")
	if rmOut == nil || len(rmOut.Rules) != 1 || rmOut.Rules[0].MatchPrefixList != "PL-OUT" || rmOut.Rules[0].SetMED == nil || *rmOut.Rules[0].SetMED != 77 {
		t.Fatalf("RM-OUT = %#v", rmOut)
	}
	rmDeny := routePolicyByName(cfg.RoutePolicies, "RM-DENY")
	if rmDeny == nil || len(rmDeny.Rules) != 1 || rmDeny.Rules[0].Action != "deny" || rmDeny.Rules[0].MatchPrefixList != "PL-IN" {
		t.Fatalf("RM-DENY = %#v", rmDeny)
	}
	if len(cfg.Neighbors) != 1 || cfg.Neighbors[0].ImportPolicy != "RM-IN" || cfg.Neighbors[0].ExportPolicy != "RM-OUT" {
		t.Fatalf("Neighbors = %#v", cfg.Neighbors)
	}
}

func TestParseFRRStaticRoutesAndRedistribution(t *testing.T) {
	cfg := parseFRRConfigText(t, `
hostname r1
ip route 0.0.0.0/0 192.0.2.254
ip route 203.0.113.0/24 Null0
router bgp 65001
 address-family ipv4 unicast
  network 198.51.100.0/24
  redistribute static route-map STATIC-OUT
 exit-address-family
!
`)
	if got, want := len(cfg.Routes), 2; got != want {
		t.Fatalf("routes = %d, want %d: %#v", got, want, cfg.Routes)
	}
	if cfg.Routes[0].Prefix.String() != "0.0.0.0/0" || cfg.Routes[0].NextHop != "192.0.2.254" || cfg.Routes[0].Kind != model.RouteSourceStatic {
		t.Fatalf("default static route not parsed: %#v", cfg.Routes[0])
	}
	if cfg.Routes[1].Kind != model.RouteSourceBlackhole || cfg.Routes[1].Interface != "Null0" {
		t.Fatalf("blackhole route not parsed: %#v", cfg.Routes[1])
	}
	if len(cfg.Redistribute) != 1 || cfg.Redistribute[0].Kind != model.RouteSourceStatic || cfg.Redistribute[0].RouteMap != "STATIC-OUT" {
		t.Fatalf("redistribute static not parsed: %#v", cfg.Redistribute)
	}
	if len(cfg.Prefixes) != 1 || cfg.Prefixes[0] != "198.51.100.0/24" {
		t.Fatalf("BGP network prefixes = %#v", cfg.Prefixes)
	}
}

func TestParseFRRAggregateAddressSummaryOnly(t *testing.T) {
	cfg := parseFRRConfigText(t, `
hostname r1
router bgp 65001
 address-family ipv4 unicast
  aggregate-address 10.0.0.0/16 summary-only
 exit-address-family
!
`)
	if len(cfg.Routes) != 1 {
		t.Fatalf("routes = %#v, want one aggregate route", cfg.Routes)
	}
	route := cfg.Routes[0]
	if route.Kind != model.RouteSourceAggregate || route.Prefix.String() != "10.0.0.0/16" || !route.SummaryOnly || route.AdminDistance != 200 {
		t.Fatalf("aggregate route = %#v", route)
	}
	if len(cfg.Prefixes) != 0 {
		t.Fatalf("aggregate-address should not be parsed as unconditional BGP network prefix: %#v", cfg.Prefixes)
	}
}

func TestParseFRRAggregateAddressRejectsUnsupportedOptions(t *testing.T) {
	config := `
router bgp 65001
 address-family ipv4 unicast
  aggregate-address 10.0.0.0/16 as-set
 exit-address-family
`
	_, err := parseFRRConfigTextResult(t, config)
	if err == nil || !strings.Contains(err.Error(), `unsupported FRR aggregate-address option "as-set"`) {
		t.Fatalf("configparse.ParseConfig() error = %v, want unsupported aggregate-address option", err)
	}
	path := filepath.Join(t.TempDir(), "frr.conf")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	result, err := configparse.ParseConfigWithWarnings(model.KindFRR, path)
	if err != nil {
		t.Fatalf("configparse.ParseConfigWithWarnings() error = %v", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0].Reason, "unsupported FRR aggregate-address option") {
		t.Fatalf("warnings = %#v, want aggregate option warning", result.Warnings)
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

func TestParseUnsupportedStaticRouteWarningAndStrictError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frr.conf")
	if err := os.WriteFile(path, []byte("ip route 10.0.0.0/24 192.0.2.1 250\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := configparse.ParseConfigWithWarnings(model.KindFRR, path)
	if err != nil {
		t.Fatalf("configparse.ParseConfigWithWarnings() error = %v", err)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("warnings = %#v, want one unsupported static route warning", result.Warnings)
	}
	_, err = configparse.ParseConfig(model.KindFRR, path)
	if err == nil {
		t.Fatalf("configparse.ParseConfig() error = nil, want strict unsupported static route error")
	}
}

func TestParseFRRVRFInterfacesAndStaticRoutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frr.conf")
	config := strings.Join([]string{
		"interface eth1 vrf tenant-a",
		" ip address 192.0.2.1/30",
		"!",
		"interface eth2",
		" vrf forwarding tenant-b",
		" ip address 198.51.100.1/30",
		"!",
		"ip route 10.0.0.0/24 192.0.2.2 vrf tenant-a",
		"ip route vrf tenant-b 10.1.0.0/24 198.51.100.2",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := configparse.ParseConfig(model.KindFRR, path)
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if got := interfaceByName(cfg.Interfaces, "eth1").VRF; got != "tenant-a" {
		t.Fatalf("eth1 VRF = %q, want tenant-a", got)
	}
	if got := interfaceByName(cfg.Interfaces, "eth2").VRF; got != "tenant-b" {
		t.Fatalf("eth2 VRF = %q, want tenant-b", got)
	}
	if len(cfg.Routes) != 2 {
		t.Fatalf("routes = %#v, want 2", cfg.Routes)
	}
	if cfg.Routes[0].NetworkInstance != "tenant-a" || cfg.Routes[1].NetworkInstance != "tenant-b" {
		t.Fatalf("route VRFs = %q %q, want tenant-a tenant-b", cfg.Routes[0].NetworkInstance, cfg.Routes[1].NetworkInstance)
	}
}

func TestParseCEOSVRFInterfacesAndStaticRoutes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ceos.cfg")
	config := strings.Join([]string{
		"interface Ethernet1",
		"   vrf tenant-a",
		"   ip address 192.0.2.1/30",
		"!",
		"ip route vrf tenant-a 10.0.0.0/24 192.0.2.2",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := configparse.ParseConfig(model.KindCEOS, path)
	if err != nil {
		t.Fatalf("configparse.ParseConfig() error = %v", err)
	}
	if got := interfaceByName(cfg.Interfaces, "Ethernet1").VRF; got != "tenant-a" {
		t.Fatalf("Ethernet1 VRF = %q, want tenant-a", got)
	}
	if len(cfg.Routes) != 1 || cfg.Routes[0].NetworkInstance != "tenant-a" {
		t.Fatalf("routes = %#v, want one tenant-a route", cfg.Routes)
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

func TestParseFRRBGPVRF(t *testing.T) {
	cfg := parseFRRConfigText(t, `
hostname r1
interface eth1 vrf tenant-a
 ip address 192.0.2.1/30
!
router bgp 65001 vrf tenant-a
 neighbor 192.0.2.2 remote-as 65002
 !
 address-family ipv4 unicast
  network 10.255.0.1/32
  redistribute connected route-map CONNECTED-OUT
  neighbor 192.0.2.2 activate
  neighbor 192.0.2.2 route-map IMPORT-A in
 exit-address-family
exit
!
`)
	if cfg.ASN != 65001 {
		t.Fatalf("ASN = %d, want 65001", cfg.ASN)
	}
	if len(cfg.Neighbors) != 1 || cfg.Neighbors[0].NetworkInstance != "tenant-a" || cfg.Neighbors[0].Address != "192.0.2.2" || !cfg.Neighbors[0].Activated || cfg.Neighbors[0].ImportPolicy != "IMPORT-A" {
		t.Fatalf("Neighbors = %#v, want tenant-a neighbor", cfg.Neighbors)
	}
	if len(cfg.Routes) != 1 || cfg.Routes[0].Kind != model.RouteSourceBGP || cfg.Routes[0].NetworkInstance != "tenant-a" || cfg.Routes[0].Prefix.String() != "10.255.0.1/32" {
		t.Fatalf("Routes = %#v, want tenant-a BGP network", cfg.Routes)
	}
	if len(cfg.Redistribute) != 1 || cfg.Redistribute[0].NetworkInstance != "tenant-a" || cfg.Redistribute[0].Kind != model.RouteSourceConnected || cfg.Redistribute[0].RouteMap != "CONNECTED-OUT" {
		t.Fatalf("Redistribute = %#v, want tenant-a connected route-map", cfg.Redistribute)
	}
}

func TestParseCEOSRouteMapRejectsUnsupportedMatch(t *testing.T) {
	_, err := parseCEOSConfigTextResult(t, `
route-map RM permit 10
   match as-path ASPATH
   set local-preference 200
`)
	if err == nil || !strings.Contains(err.Error(), "unsupported cEOS route-map match statement") {
		t.Fatalf("configparse.ParseConfig() error = %v, want unsupported cEOS match", err)
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
