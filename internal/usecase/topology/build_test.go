package topology_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

func TestLoadLabTopology(t *testing.T) {
	topo, err := topology.LoadTopology(filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"))
	if err != nil {
		t.Fatalf("topology.LoadTopology() error = %v", err)
	}
	if len(topo.Nodes) != 18 {
		t.Fatalf("nodes = %d, want 18", len(topo.Nodes))
	}
	if len(topo.Links) < 25 {
		t.Fatalf("links = %d, want at least 25", len(topo.Links))
	}
	if _, ok := topo.Node("core-sh"); !ok {
		t.Fatalf("core-sh not found")
	}
	core, _ := topo.Node("core-bj")
	if core.ASN != 65100 {
		t.Fatalf("core-bj ASN = %d, want parsed 65100", core.ASN)
	}
	if len(core.Neighbors) == 0 {
		t.Fatalf("core-bj neighbors were not parsed from config")
	}
}

func TestBuilderUsesInjectedPorts(t *testing.T) {
	loader := &fakeLabFileLoader{
		file: topology.LabFile{
			Name: "ports-test",
			Nodes: map[string]topology.LabNode{
				"r1": {
					Kind:          "linux",
					MgmtIPv4:      "172.20.20.11",
					StartupConfig: "configs/r1/frr.conf",
				},
				"r2": {
					Kind:          "linux",
					MgmtIPv4:      "172.20.20.12",
					StartupConfig: "configs/r2/frr.conf",
				},
			},
			Links: []topology.LabLink{{Endpoints: []string{"r1:eth1", "r2:eth1"}}},
		},
	}
	parser := &fakeConfigParser{
		results: map[string]topology.ParseResult{
			filepath.Join("/virtual", "configs/r1/frr.conf"): {
				Config: topology.ParsedConfig{
					ASN: 65001,
					Interfaces: []model.Interface{{
						Name:    "eth1",
						Address: "192.0.2.1/30",
					}},
				},
			},
			filepath.Join("/virtual", "configs/r2/frr.conf"): {
				Config: topology.ParsedConfig{
					ASN: 65002,
					Interfaces: []model.Interface{{
						Name:    "eth1",
						Address: "192.0.2.2/30",
					}},
				},
			},
		},
	}

	topo, warnings, err := topology.NewBuilder(loader, parser).LoadTopologyWithOptions(filepath.Join("/virtual", "lab.clab.yml"), topology.LoadOptions{CollectWarnings: true})
	if err != nil {
		t.Fatalf("Builder.LoadTopologyWithOptions() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	if loader.loadedPath != filepath.Join("/virtual", "lab.clab.yml") {
		t.Fatalf("loader path = %q, want topology path", loader.loadedPath)
	}
	if parser.parseCalls != 2 {
		t.Fatalf("parser parse calls = %d, want 2", parser.parseCalls)
	}
	if len(topo.Nodes) != 2 || len(topo.Links) != 1 {
		t.Fatalf("topology nodes=%d links=%d, want 2 nodes and 1 link", len(topo.Nodes), len(topo.Links))
	}
	r1, ok := topo.Node("r1")
	if !ok || r1.ASN != 65001 || r1.ConfigPath != "configs/r1/frr.conf" {
		t.Fatalf("r1 = %#v, want injected config data", r1)
	}
	if topo.Links[0].Subnet != "192.0.2.0/30" {
		t.Fatalf("link subnet = %q, want 192.0.2.0/30", topo.Links[0].Subnet)
	}
}

func TestLoadDomainTopologyWithRuntimeSeparatesRuntimeMetadata(t *testing.T) {
	loader := &fakeLabFileLoader{
		file: topology.LabFile{
			Name:             "ports-test",
			ManagementSubnet: "172.20.20.0/24",
			Nodes: map[string]topology.LabNode{
				"r1": {Kind: "linux", MgmtIPv4: "172.20.20.11", StartupConfig: "configs/r1/frr.conf"},
				"r2": {Kind: "linux", MgmtIPv4: "172.20.20.12", StartupConfig: "configs/r2/frr.conf"},
			},
			Links: []topology.LabLink{{Endpoints: []string{"r1:eth1", "r2:eth1"}}},
		},
	}
	parser := &fakeConfigParser{results: map[string]topology.ParseResult{
		filepath.Join("/virtual", "configs/r1/frr.conf"): {Config: topology.ParsedConfig{Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.1/30"}}}},
		filepath.Join("/virtual", "configs/r2/frr.conf"): {Config: topology.ParsedConfig{Interfaces: []model.Interface{{Name: "eth1", Address: "192.0.2.2/30"}}}},
	}}

	topo, runtime, warnings, err := topology.NewBuilder(loader, parser).LoadDomainTopologyWithRuntime(filepath.Join("/virtual", "lab.clab.yml"), topology.LoadOptions{CollectWarnings: true})
	if err != nil {
		t.Fatalf("LoadDomainTopologyWithRuntime() error = %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}
	r1, ok := topo.Node("r1")
	if !ok {
		t.Fatalf("r1 not found")
	}
	if r1.ContainerName != "" || r1.MgmtIPv4 != "" || r1.ConfigPath != "" {
		t.Fatalf("domain node contains runtime metadata: %#v", r1)
	}
	if got := runtime.Nodes["r1"].ConfigPath; got != "configs/r1/frr.conf" {
		t.Fatalf("runtime r1 config path = %q", got)
	}
	if got := runtime.RuntimeName("r1"); got != "clab-ports-test-r1" {
		t.Fatalf("runtime r1 name = %q", got)
	}
}

func TestLoadLabTopologyIncludesRouteMaps(t *testing.T) {
	topo, err := topology.LoadTopology(filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"))
	if err != nil {
		t.Fatalf("topology.LoadTopology() error = %v", err)
	}
	coreBJ, ok := topo.Node("core-bj")
	if !ok {
		t.Fatalf("core-bj not found")
	}
	if prefixListByName(coreBJ.PrefixLists, "BJ-LOCAL") == nil {
		t.Fatalf("core-bj BJ-LOCAL prefix-list not loaded: %#v", coreBJ.PrefixLists)
	}
	if routePolicyByName(coreBJ.RoutePolicies, "PREFER-BJ-LOCAL") == nil {
		t.Fatalf("core-bj PREFER-BJ-LOCAL route policy not loaded: %#v", coreBJ.RoutePolicies)
	}
	for _, addr := range []string{"198.18.10.0", "198.18.10.2"} {
		neighbor := neighborByAddress(coreBJ.Neighbors, addr)
		if neighbor == nil || neighbor.ImportPolicy != "PREFER-BJ-LOCAL" {
			t.Fatalf("core-bj neighbor %s = %#v, want import policy PREFER-BJ-LOCAL", addr, neighbor)
		}
	}
	coreHZ, ok := topo.Node("core-hz")
	if !ok {
		t.Fatalf("core-hz not found")
	}
	if prefixListByName(coreHZ.PrefixLists, "HZ-LOCAL") == nil {
		t.Fatalf("core-hz HZ-LOCAL prefix-list not loaded: %#v", coreHZ.PrefixLists)
	}
	if routePolicyByName(coreHZ.RoutePolicies, "HZ-TRANSIT-OUT") == nil {
		t.Fatalf("core-hz HZ-TRANSIT-OUT route policy not loaded: %#v", coreHZ.RoutePolicies)
	}
	neighbor := neighborByAddress(coreHZ.Neighbors, "198.18.30.7")
	if neighbor == nil || neighbor.ExportPolicy != "HZ-TRANSIT-OUT" {
		t.Fatalf("core-hz neighbor 198.18.30.7 = %#v, want export policy HZ-TRANSIT-OUT", neighbor)
	}
	coreSH, ok := topo.Node("core-sh")
	if !ok {
		t.Fatalf("core-sh not found")
	}
	if prefixListByName(coreSH.PrefixLists, "SH-LOCAL") == nil {
		t.Fatalf("core-sh SH-LOCAL prefix-list not loaded: %#v", coreSH.PrefixLists)
	}
	if routePolicyByName(coreSH.RoutePolicies, "PREFER-SH-LOCAL") == nil || routePolicyByName(coreSH.RoutePolicies, "SH-TRANSIT-OUT") == nil {
		t.Fatalf("core-sh route policies not loaded: %#v", coreSH.RoutePolicies)
	}
	for _, addr := range []string{"198.18.10.4", "198.18.10.6"} {
		neighbor := neighborByAddress(coreSH.Neighbors, addr)
		if neighbor == nil || neighbor.ImportPolicy != "PREFER-SH-LOCAL" {
			t.Fatalf("core-sh neighbor %s = %#v, want import policy PREFER-SH-LOCAL", addr, neighbor)
		}
	}
	neighbor = neighborByAddress(coreSH.Neighbors, "198.18.30.3")
	if neighbor == nil || neighbor.ExportPolicy != "SH-TRANSIT-OUT" {
		t.Fatalf("core-sh neighbor 198.18.30.3 = %#v, want export policy SH-TRANSIT-OUT", neighbor)
	}
}

func TestLoadLabTopologyIncludesACLPoliciesWithoutPolicyFile(t *testing.T) {
	topo, err := topology.LoadTopology(filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"))
	if err != nil {
		t.Fatalf("topology.LoadTopology() error = %v", err)
	}
	for _, tt := range []struct {
		node  string
		iface string
	}{
		{node: "core-hz", iface: "eth1"},
		{node: "core-hz", iface: "eth2"},
		{node: "core-sh", iface: "Ethernet5"},
		{node: "core-gz", iface: "ethernet-1/4.0"},
	} {
		acl, binding := aclByNodeInterface(topo, tt.node, tt.iface)
		if acl == nil || binding == nil {
			t.Fatalf("acl for %s %s not found in ACLs=%#v bindings=%#v", tt.node, tt.iface, topo.ACLs, topo.ACLBindings)
		}
		if acl.Name != "BLOCK-HTTP-TO-HZ" || binding.Direction != "egress" || len(acl.Rules) == 0 || acl.Rules[0].Action != model.ACLDeny || acl.Rules[0].Match.Protocol != "tcp" {
			t.Fatalf("acl for %s %s = %#v binding=%#v", tt.node, tt.iface, acl, binding)
		}
		if acl.Rules[0].Match.DstSet == nil || !acl.Rules[0].Match.DstPort.Contains(80) {
			t.Fatalf("acl first rule match = %#v, want dst 10.4.0.0/16 tcp/80", acl.Rules[0].Match)
		}
		if acl.Rules[0].Source.File == "" || acl.Rules[0].Source.Line == 0 || acl.Rules[0].Source.Raw == "" {
			t.Fatalf("acl rule source not populated: %#v", acl.Rules[0].Source)
		}
		if tt.node == "core-hz" && acl.Source.Vendor != "nftables" {
			t.Fatalf("core-hz acl source vendor = %q, want nftables", acl.Source.Vendor)
		}
	}
}

func TestOriginLookups(t *testing.T) {
	topo, err := topology.LoadTopology(filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"))
	if err != nil {
		t.Fatalf("topology.LoadTopology() error = %v", err)
	}
	node, ok := topo.OriginForPrefix("10.4.0.0/16")
	if !ok || node != "hz-edge1" {
		t.Fatalf("OriginForPrefix() = %q, %v", node, ok)
	}
	node, pfx, ok := topo.OriginForIP("10.4.1.10")
	if !ok || node != "cust-hz" || pfx.String() != "10.4.1.10/32" {
		t.Fatalf("OriginForIP() = %q %s %v", node, pfx, ok)
	}
}

func TestLoadLabTopologyExpandsL2TransitNode(t *testing.T) {
	dir := t.TempDir()
	mkdirAll(t, filepath.Join(dir, "configs", "r1"))
	mkdirAll(t, filepath.Join(dir, "configs", "r2"))
	mkdirAll(t, filepath.Join(dir, "configs", "r3"))
	writeFile(t, filepath.Join(dir, "configs", "r1", "frr.conf"), `hostname r1
interface lo
 ip address 10.255.1.1/32
interface eth1
 ip address 198.51.100.1/29
 ip ospf area 0
 ip ospf network broadcast
router ospf
 network 10.255.1.1/32 area 0
 network 198.51.100.0/29 area 0
`)
	writeFile(t, filepath.Join(dir, "configs", "r2", "frr.conf"), strings.ReplaceAll(mustReadFileString(t, filepath.Join(dir, "configs", "r1", "frr.conf")), "r1", "r2"))
	writeFile(t, filepath.Join(dir, "configs", "r2", "frr.conf"), strings.ReplaceAll(mustReadFileString(t, filepath.Join(dir, "configs", "r2", "frr.conf")), "10.255.1.1", "10.255.2.2"))
	writeFile(t, filepath.Join(dir, "configs", "r2", "frr.conf"), strings.ReplaceAll(mustReadFileString(t, filepath.Join(dir, "configs", "r2", "frr.conf")), "198.51.100.1", "198.51.100.2"))
	writeFile(t, filepath.Join(dir, "configs", "r3", "frr.conf"), strings.ReplaceAll(mustReadFileString(t, filepath.Join(dir, "configs", "r1", "frr.conf")), "r1", "r3"))
	writeFile(t, filepath.Join(dir, "configs", "r3", "frr.conf"), strings.ReplaceAll(mustReadFileString(t, filepath.Join(dir, "configs", "r3", "frr.conf")), "10.255.1.1", "10.255.3.3"))
	writeFile(t, filepath.Join(dir, "configs", "r3", "frr.conf"), strings.ReplaceAll(mustReadFileString(t, filepath.Join(dir, "configs", "r3", "frr.conf")), "198.51.100.1", "198.51.100.3"))
	topologyPath := filepath.Join(dir, "lab.clab.yml")
	writeFile(t, topologyPath, `name: shared
topology:
  nodes:
    r1:
      kind: linux
      group: router
      binds: ["configs/r1:/etc/frr:ro"]
    r2:
      kind: linux
      group: router
      binds: ["configs/r2:/etc/frr:ro"]
    r3:
      kind: linux
      group: router
      binds: ["configs/r3:/etc/frr:ro"]
    sw1:
      kind: linux
      group: switch
  links:
    - endpoints: ["r1:eth1", "sw1:eth1"]
    - endpoints: ["r2:eth1", "sw1:eth2"]
    - endpoints: ["r3:eth1", "sw1:eth3"]
`)
	topo, err := topology.LoadTopology(topologyPath)
	if err != nil {
		t.Fatalf("topology.LoadTopology() error = %v", err)
	}
	if len(topo.Nodes) != 3 {
		t.Fatalf("nodes = %d, want 3 routers and no transit node", len(topo.Nodes))
	}
	if len(topo.Links) != 3 {
		t.Fatalf("links = %#v, want complete graph across shared segment", topo.Links)
	}
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		t.Fatalf("BuildTopologyIndex() error = %v", err)
	}
	if _, ok := idx.LinkBetween("r1", "r3"); !ok {
		t.Fatalf("r1-r3 shared segment link missing: %#v", topo.Links)
	}
}

func TestLoadLabTopologyStrictConfigRejectsUnsupportedStatements(t *testing.T) {
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

	topo, warnings, err := topology.LoadTopologyWithOptions(topologyPath, topology.LoadOptions{CollectWarnings: true})
	if err != nil {
		t.Fatalf("topology.LoadTopologyWithOptions(non-strict) error = %v", err)
	}
	if topo == nil || len(warnings) != 1 {
		t.Fatalf("non-strict topology=%#v warnings=%#v, want topology and one warning", topo, warnings)
	}

	_, warnings, err = topology.LoadTopologyWithOptions(topologyPath, topology.LoadOptions{StrictConfig: true})
	if err == nil {
		t.Fatalf("topology.LoadTopologyWithOptions(strict) error = nil")
	}
	if len(warnings) != 1 {
		t.Fatalf("strict warnings = %#v, want one", warnings)
	}
	msg := err.Error()
	for _, want := range []string{"vendor=frr", "file=" + configPath, "line=4", `raw="match source-protocol bgp"`, "reason=unsupported FRR route-map match statement"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("strict error missing %q:\n%s", want, msg)
		}
	}
}

func TestLoadOSPFBasicLabIncludesNonFRRNodes(t *testing.T) {
	topo, err := topology.LoadTopology(filepath.Join("..", "..", "..", "labs", "ospf-basic", "hoyan.clab.yml"))
	if err != nil {
		t.Fatalf("topology.LoadTopology() error = %v", err)
	}
	kinds := map[string]model.DeviceKind{}
	for _, node := range topo.Nodes {
		kinds[node.Name] = node.Kind
		if !node.OSPF.Enabled {
			t.Fatalf("%s OSPF disabled: %#v", node.Name, node.OSPF)
		}
	}
	if kinds["r2"] != model.KindCEOS || kinds["r3"] != model.KindSRLinux {
		t.Fatalf("node kinds = %#v, want r2 cEOS and r3 SR Linux", kinds)
	}
}

func TestLoadOSPFVRFLabIncludesScopedProcesses(t *testing.T) {
	topo, err := topology.LoadTopology(filepath.Join("..", "..", "..", "labs", "ospf-vrf", "hoyan.clab.yml"))
	if err != nil {
		t.Fatalf("topology.LoadTopology() error = %v", err)
	}
	r1, ok := topo.Node("r1")
	if !ok {
		t.Fatalf("r1 not found")
	}
	vrfs := map[model.NetworkInstanceID]bool{}
	for _, process := range r1.OSPFProcesses {
		vrfs[process.NetworkInstance] = process.Enabled
	}
	if !vrfs["tenant-a"] || !vrfs["tenant-b"] {
		t.Fatalf("r1 OSPFProcesses = %#v, want tenant-a and tenant-b", r1.OSPFProcesses)
	}
	for _, iface := range r1.Interfaces {
		if iface.Name == "eth1" && iface.VRF != "tenant-a" {
			t.Fatalf("r1 eth1 VRF = %q, want tenant-a", iface.VRF)
		}
		if iface.Name == "eth2" && iface.VRF != "tenant-b" {
			t.Fatalf("r1 eth2 VRF = %q, want tenant-b", iface.VRF)
		}
	}
}

func TestLoadLabTopologyIncludesCEOSRouteMaps(t *testing.T) {
	dir := t.TempDir()
	config := `
hostname ceos1
ip prefix-list PL seq 10 permit 10.0.0.0/24
route-map RM-IN permit 10
   match ip address prefix-list PL
   set local-preference 250
route-map RM-OUT permit 20
   set metric 77
router bgp 65001
   router-id 10.255.0.1
   neighbor 192.0.2.1 remote-as 65002
   address-family ipv4
      neighbor 192.0.2.1 activate
      neighbor 192.0.2.1 route-map RM-IN in
      neighbor 192.0.2.1 route-map RM-OUT out
`
	if err := os.WriteFile(filepath.Join(dir, "ceos.cfg"), []byte(config), 0o644); err != nil {
		t.Fatalf("WriteFile(config) error = %v", err)
	}
	topologyYAML := `
name: ceos-policy
topology:
  nodes:
    ceos1:
      kind: arista_ceos
      startup-config: ceos.cfg
`
	topologyPath := filepath.Join(dir, "lab.clab.yml")
	if err := os.WriteFile(topologyPath, []byte(topologyYAML), 0o644); err != nil {
		t.Fatalf("WriteFile(topology) error = %v", err)
	}
	topo, err := topology.LoadTopology(topologyPath)
	if err != nil {
		t.Fatalf("topology.LoadTopology() error = %v", err)
	}
	node, ok := topo.Node("ceos1")
	if !ok {
		t.Fatalf("ceos1 not found")
	}
	if prefixListByName(node.PrefixLists, "PL") == nil {
		t.Fatalf("PL prefix-list not propagated: %#v", node.PrefixLists)
	}
	if routePolicyByName(node.RoutePolicies, "RM-IN") == nil || routePolicyByName(node.RoutePolicies, "RM-OUT") == nil {
		t.Fatalf("route policies not propagated: %#v", node.RoutePolicies)
	}
	neighbor := neighborByAddress(node.Neighbors, "192.0.2.1")
	if neighbor == nil || neighbor.ImportPolicy != "RM-IN" || neighbor.ExportPolicy != "RM-OUT" {
		t.Fatalf("neighbor = %#v, want route-map bindings", neighbor)
	}
}

func TestLoadVendorVRFLabs(t *testing.T) {
	for _, labName := range []string{"vrf-ceos-basic", "vrf-srlinux-basic"} {
		t.Run(labName, func(t *testing.T) {
			topo, err := topology.LoadTopology(filepath.Join("..", "..", "..", "labs", labName, "hoyan.clab.yml"))
			if err != nil {
				t.Fatalf("topology.LoadTopology() error = %v", err)
			}
			r1, ok := topo.Node("r1")
			if !ok {
				t.Fatalf("r1 missing")
			}
			if got := interfaceByName(r1.Interfaces, vendorVRFInterfaceName(labName, "tenant-a")).VRF; got != "tenant-a" {
				t.Fatalf("tenant-a interface VRF = %q", got)
			}
			if len(r1.Routes) != 2 {
				t.Fatalf("r1 routes = %#v, want 2", r1.Routes)
			}
		})
	}
}

func TestLoadLabTopologyContainerNames(t *testing.T) {
	topo, err := topology.LoadTopology(filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml"))
	if err != nil {
		t.Fatalf("topology.LoadTopology() error = %v", err)
	}
	node, ok := topo.Node("bj-edge1")
	if !ok {
		t.Fatalf("bj-edge1 not found")
	}
	if node.ContainerName != "clab-hoyan-base-wan-bj-edge1" {
		t.Fatalf("original topology container name = %q, want containerlab default name", node.ContainerName)
	}

	sourceDir := absPath(t, filepath.Join("..", "..", "..", "labs", "base-wan"))
	data, err := model.RenderIsolatedTopology(mustReadFile(t, filepath.Join("..", "..", "..", "labs", "base-wan", "hoyan.clab.yml")), model.TopologyRenderOptions{Suffix: "issue-21", SourceDir: sourceDir})
	if err != nil {
		t.Fatalf("RenderIsolatedTopology() error = %v", err)
	}
	if !strings.Contains(string(data), filepath.Join(sourceDir, "configs", "frr", "bj-edge1", "frr.conf")) {
		t.Fatalf("rendered topology did not absolute config paths")
	}
	path := writeTempTopology(t, data)
	topo, err = topology.LoadTopology(path)
	if err != nil {
		t.Fatalf("topology.LoadTopology(rendered) error = %v", err)
	}
	node, ok = topo.Node("bj-edge1")
	if !ok {
		t.Fatalf("bj-edge1 not found in rendered topology")
	}
	if node.ContainerName != "clab-hoyan-base-wan-issue-21-bj-edge1" {
		t.Fatalf("rendered topology container name = %q", node.ContainerName)
	}
	if node.MgmtIPv4 != "172.86.21.11" {
		t.Fatalf("rendered topology management IP = %q", node.MgmtIPv4)
	}
}

func vendorVRFInterfaceName(lab, vrf string) string {
	if lab == "vrf-ceos-basic" {
		if vrf == "tenant-a" {
			return "Ethernet1"
		}
		return "Ethernet2"
	}
	if vrf == "tenant-a" {
		return "ethernet-1/1"
	}
	return "ethernet-1/2"
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

func aclByNodeInterface(topo *model.Topology, node, iface string) (*model.ACL, *model.ACLBinding) {
	for i := range topo.ACLBindings {
		binding := &topo.ACLBindings[i]
		if binding.Node != node || binding.Interface != iface {
			continue
		}
		for j := range topo.ACLs {
			if topo.ACLs[j].Node == node && topo.ACLs[j].Name == binding.ACLName {
				return &topo.ACLs[j], binding
			}
		}
	}
	return nil, nil
}

func neighborByAddress(neighbors []model.BGPNeighbor, addr string) *model.BGPNeighbor {
	for i := range neighbors {
		if neighbors[i].Address == addr {
			return &neighbors[i]
		}
	}
	return nil
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
}

type fakeLabFileLoader struct {
	file       topology.LabFile
	loadedPath string
}

func (l *fakeLabFileLoader) Load(path string) (topology.LabFile, error) {
	l.loadedPath = path
	return l.file, nil
}

type fakeConfigParser struct {
	results    map[string]topology.ParseResult
	parseCalls int
}

func (p *fakeConfigParser) Parse(_ model.DeviceKind, path string, _ topology.ParseOptions) (topology.ParseResult, error) {
	p.parseCalls++
	return p.results[path], nil
}

func (*fakeConfigParser) ParseNftablesACL(string) ([]model.ACL, []model.ACLBinding, error) {
	return nil, nil, nil
}

func mustReadFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	return string(data)
}

func mustReadFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	return data
}

func writeTempTopology(t *testing.T, data []byte) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "generated.clab.yml")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("WriteFile(%s) error = %v", path, err)
	}
	return path
}

func absPath(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("Abs(%s) error = %v", path, err)
	}
	return abs
}
