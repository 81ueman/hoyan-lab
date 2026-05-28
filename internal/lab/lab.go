package lab

import (
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/configparse"
	//lint:ignore ST1001 This package is a mechanical extraction of the legacy lab loader; issue #65 keeps behavior stable before follow-up cleanup.
	. "github.com/81ueman/hoyan-lab/internal/model"
	"gopkg.in/yaml.v3"
)

type clabFile struct {
	Name   string  `yaml:"name"`
	Prefix *string `yaml:"prefix"`
	Mgmt   struct {
		IPv4Subnet string `yaml:"ipv4-subnet"`
		Network    string `yaml:"network"`
	} `yaml:"mgmt"`
	Topology struct {
		Nodes map[string]clabNode `yaml:"nodes"`
		Links []struct {
			Endpoints []string `yaml:"endpoints"`
		} `yaml:"links"`
	} `yaml:"topology"`
}

type clabNode struct {
	Kind          string   `yaml:"kind"`
	Group         string   `yaml:"group"`
	NetworkMode   string   `yaml:"network-mode"`
	MgmtIPv4      string   `yaml:"mgmt-ipv4"`
	Binds         []string `yaml:"binds"`
	StartupConfig string   `yaml:"startup-config"`
}

type LoadOptions struct {
	CollectWarnings bool
	StrictConfig    bool
}

type clabTransitAttachment struct {
	Node string
	Intf string
}

func LoadTopology(clabPath string) (*Topology, error) {
	topo, _, err := LoadTopologyWithOptions(clabPath, LoadOptions{})
	return topo, err
}

func LoadTopologyWithWarnings(clabPath string) (*Topology, []configparse.UnsupportedStatement, error) {
	return LoadTopologyWithOptions(clabPath, LoadOptions{CollectWarnings: true})
}

func LoadTopologyWithOptions(clabPath string, opts LoadOptions) (*Topology, []configparse.UnsupportedStatement, error) {
	data, err := os.ReadFile(clabPath)
	if err != nil {
		return nil, nil, err
	}
	var raw clabFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, nil, err
	}
	root := filepath.Dir(clabPath)
	topo := &Topology{Name: raw.Name, ManagementSubnet: raw.Mgmt.IPv4Subnet}
	var warnings []configparse.UnsupportedStatement
	collectWarnings := opts.CollectWarnings || opts.StrictConfig
	names := make([]string, 0, len(raw.Topology.Nodes))
	for name := range raw.Topology.Nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	transitNodes := map[string]bool{}
	for _, name := range names {
		cnode := raw.Topology.Nodes[name]
		if isL2TransitNode(cnode) {
			transitNodes[name] = true
			continue
		}
		kind := normalizeKind(cnode.Kind)
		configPath := resolveConfigPath(cnode)
		if configPath == "" {
			return nil, nil, fmt.Errorf("node %s has no startup config or frr.conf bind", name)
		}
		fullConfigPath := configPath
		if !filepath.IsAbs(fullConfigPath) {
			fullConfigPath = filepath.Join(root, configPath)
		}
		result, err := configparse.ParseConfigWithOptions(kind, fullConfigPath, configparse.ParseOptions{CollectWarnings: collectWarnings})
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		parsed := result.Config
		warnings = append(warnings, result.Warnings...)
		prefixes, err := parsePrefixes(parsed.Prefixes)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", name, err)
		}
		if parsedOSPFEnabled(parsed) && parsed.Loopback != "" {
			loopbackPrefix, err := ParsePrefix(parsed.Loopback)
			if err != nil {
				return nil, nil, fmt.Errorf("%s loopback: %w", name, err)
			}
			prefixes = appendUniquePrefix(prefixes, loopbackPrefix)
		}
		node := Node{
			Name:           name,
			ContainerName:  containerlabContainerName(raw.Prefix, raw.Name, name),
			Kind:           kind,
			Role:           cnode.Group,
			ASN:            parsed.ASN,
			MgmtIPv4:       cnode.MgmtIPv4,
			Loopback:       parsed.Loopback,
			ConfigPath:     configPath,
			Prefixes:       prefixes,
			Routes:         parsed.Routes,
			Interfaces:     parsed.Interfaces,
			Neighbors:      parsed.Neighbors,
			Redistribute:   parsed.Redistribute,
			OSPF:           parsed.OSPF,
			OSPFProcesses:  parsed.OSPFProcesses,
			PrefixLists:    parsed.PrefixLists,
			ASPathLists:    parsed.ASPathLists,
			CommunityLists: parsed.CommunityLists,
			RoutePolicies:  parsed.RoutePolicies,
		}
		for ri := range node.Routes {
			node.Routes[ri].Node = name
			if node.Routes[ri].NetworkInstance == "" {
				node.Routes[ri].NetworkInstance = NetworkInstanceDefault
			} else {
				node.Routes[ri].NetworkInstance = NormalizeNetworkInstance(string(node.Routes[ri].NetworkInstance))
			}
			if node.Routes[ri].AFI == "" {
				node.Routes[ri].AFI = AFIIPv4
			}
		}
		for ni := range node.Neighbors {
			node.Neighbors[ni].NetworkInstance = NormalizeNetworkInstance(string(node.Neighbors[ni].NetworkInstance))
		}
		for ri := range node.Redistribute {
			node.Redistribute[ri].NetworkInstance = NormalizeNetworkInstance(string(node.Redistribute[ri].NetworkInstance))
		}
		topo.Nodes = append(topo.Nodes, node)
		for _, acl := range parsed.ACLs {
			acl.Node = name
			topo.ACLs = append(topo.ACLs, acl)
		}
		for _, binding := range parsed.ACLBindings {
			binding.Node = name
			topo.ACLBindings = append(topo.ACLBindings, binding)
		}
		nftPath := resolveNftablesConfigPath(cnode)
		if nftPath != "" {
			fullNftPath := nftPath
			if !filepath.IsAbs(fullNftPath) {
				fullNftPath = filepath.Join(root, nftPath)
			}
			acls, bindings, err := configparse.ParseNftablesACLConfig(fullNftPath)
			if err != nil {
				return nil, nil, fmt.Errorf("%s nftables: %w", name, err)
			}
			for _, acl := range acls {
				acl.Node = name
				topo.ACLs = append(topo.ACLs, acl)
			}
			for _, binding := range bindings {
				binding.Node = name
				topo.ACLBindings = append(topo.ACLBindings, binding)
			}
		}
	}
	if opts.StrictConfig && len(warnings) > 0 {
		return nil, warnings, configparse.UnsupportedConfigError{Warnings: warnings}
	}
	transitAttachments := map[string][]clabTransitAttachment{}
	for i, link := range raw.Topology.Links {
		if len(link.Endpoints) != 2 {
			return nil, nil, fmt.Errorf("link %d must have two endpoints", i)
		}
		aNode, aIntf, err := splitEndpoint(link.Endpoints[0])
		if err != nil {
			return nil, nil, err
		}
		bNode, bIntf, err := splitEndpoint(link.Endpoints[1])
		if err != nil {
			return nil, nil, err
		}
		aTransit := transitNodes[aNode]
		bTransit := transitNodes[bNode]
		switch {
		case aTransit && bTransit:
			return nil, nil, fmt.Errorf("link %s-%s connects two L2 transit nodes", link.Endpoints[0], link.Endpoints[1])
		case aTransit:
			transitAttachments[aNode] = append(transitAttachments[aNode], clabTransitAttachment{Node: bNode, Intf: bIntf})
			continue
		case bTransit:
			transitAttachments[bNode] = append(transitAttachments[bNode], clabTransitAttachment{Node: aNode, Intf: aIntf})
			continue
		}
		a, _ := topo.Node(aNode)
		b, _ := topo.Node(bNode)
		subnet, err := linkSubnet(a, aIntf, b, bIntf)
		if err != nil {
			return nil, nil, fmt.Errorf("%s-%s: %w", link.Endpoints[0], link.Endpoints[1], err)
		}
		topo.Links = append(topo.Links, Link{
			Name:   linkName(aNode, aIntf, bNode, bIntf),
			A:      aNode,
			B:      bNode,
			AIntf:  aIntf,
			BIntf:  bIntf,
			Cost:   1,
			Subnet: subnet.String(),
		})
	}
	segments := make([]string, 0, len(transitAttachments))
	for segment := range transitAttachments {
		segments = append(segments, segment)
	}
	sort.Strings(segments)
	for _, segment := range segments {
		attachments := transitAttachments[segment]
		if len(attachments) < 2 {
			return nil, nil, fmt.Errorf("L2 transit node %s has fewer than two router attachments", segment)
		}
		sort.Slice(attachments, func(i, j int) bool {
			if attachments[i].Node == attachments[j].Node {
				return attachments[i].Intf < attachments[j].Intf
			}
			return attachments[i].Node < attachments[j].Node
		})
		for i := 0; i < len(attachments); i++ {
			for j := i + 1; j < len(attachments); j++ {
				aRef := attachments[i]
				bRef := attachments[j]
				a, _ := topo.Node(aRef.Node)
				b, _ := topo.Node(bRef.Node)
				subnet, err := linkSubnet(a, aRef.Intf, b, bRef.Intf)
				if err != nil {
					return nil, nil, fmt.Errorf("%s:%s-%s:%s via %s: %w", aRef.Node, aRef.Intf, bRef.Node, bRef.Intf, segment, err)
				}
				topo.Links = append(topo.Links, Link{
					Name:   linkName(segment, aRef.Node+"-"+aRef.Intf, bRef.Node, bRef.Intf),
					A:      aRef.Node,
					B:      bRef.Node,
					AIntf:  aRef.Intf,
					BIntf:  bRef.Intf,
					Cost:   1,
					Subnet: subnet.String(),
				})
			}
		}
	}
	resolveNeighborNodes(topo)
	if err := topo.Validate(); err != nil {
		return nil, nil, err
	}
	return topo, warnings, nil
}

func parsedOSPFEnabled(parsed configparse.ParsedConfig) bool {
	if parsed.OSPF.Enabled {
		return true
	}
	for _, process := range parsed.OSPFProcesses {
		if process.Enabled {
			return true
		}
	}
	return false
}

func appendUniquePrefix(prefixes []Prefix, prefix Prefix) []Prefix {
	if prefix.IsZero() {
		return prefixes
	}
	for _, existing := range prefixes {
		if existing.Equal(prefix) {
			return prefixes
		}
	}
	return append(prefixes, prefix)
}

func containerlabContainerName(prefix *string, labName, nodeName string) string {
	if prefix != nil && *prefix == "" {
		return nodeName
	}
	effectivePrefix := "clab"
	if prefix != nil {
		effectivePrefix = *prefix
	}
	return effectivePrefix + "-" + labName + "-" + nodeName
}

func parsePrefixes(raw []string) ([]Prefix, error) {
	out := make([]Prefix, 0, len(raw))
	for _, p := range raw {
		parsed, err := ParsePrefix(p)
		if err != nil {
			return nil, fmt.Errorf("prefix %s: %w", p, err)
		}
		out = append(out, parsed)
	}
	return out, nil
}

func normalizeKind(kind string) DeviceKind {
	switch kind {
	case "linux":
		return KindFRR
	case "arista_ceos":
		return KindCEOS
	case "nokia_srlinux":
		return KindSRLinux
	default:
		return DeviceKind(kind)
	}
}

func isL2TransitNode(n clabNode) bool {
	group := strings.ToLower(strings.TrimSpace(n.Group))
	kind := strings.ToLower(strings.TrimSpace(n.Kind))
	mode := strings.ToLower(strings.TrimSpace(n.NetworkMode))
	return group == "switch" || group == "l2" || kind == "bridge" || mode == "bridge"
}

func resolveConfigPath(n clabNode) string {
	if n.StartupConfig != "" {
		return n.StartupConfig
	}
	for _, bind := range n.Binds {
		parts := strings.Split(bind, ":")
		if len(parts) >= 2 && parts[1] == "/etc/frr/frr.conf" {
			return parts[0]
		}
		if len(parts) >= 2 && parts[1] == "/etc/frr" {
			return filepath.Join(parts[0], "frr.conf")
		}
	}
	return ""
}

func resolveNftablesConfigPath(n clabNode) string {
	for _, bind := range n.Binds {
		parts := strings.Split(bind, ":")
		if len(parts) >= 2 && parts[1] == "/etc/hoyan/nftables.conf" {
			return parts[0]
		}
	}
	return ""
}

func splitEndpoint(endpoint string) (string, string, error) {
	parts := strings.Split(endpoint, ":")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid endpoint %q", endpoint)
	}
	return parts[0], parts[1], nil
}

func linkSubnet(a Node, aIntf string, b Node, bIntf string) (netip.Prefix, error) {
	ap, aok := InterfaceAddress(a.Kind, a.Interfaces, aIntf)
	bp, bok := InterfaceAddress(b.Kind, b.Interfaces, bIntf)
	switch {
	case aok && bok && ap.Masked() == bp.Masked():
		return ap.Masked(), nil
	case aok && !bok:
		return ap.Masked(), nil
	case !aok && bok:
		return bp.Masked(), nil
	case aok && bok:
		return netip.Prefix{}, fmt.Errorf("interface subnets differ: %s and %s", ap, bp)
	default:
		return netip.Prefix{}, fmt.Errorf("missing interface addresses")
	}
}

func linkName(aNode, aIntf, bNode, bIntf string) string {
	return strings.NewReplacer(":", "-", "_", "-").Replace(aNode + "-" + aIntf + "__" + bNode + "-" + bIntf)
}

func resolveNeighborNodes(topo *Topology) {
	addrToNode := map[string]string{}
	for _, n := range topo.Nodes {
		for _, iface := range n.Interfaces {
			pfx, err := netip.ParsePrefix(iface.Address)
			if err == nil {
				vrf := NormalizeNetworkInstance(string(iface.VRF))
				addrToNode[string(vrf)+"|"+pfx.Addr().String()] = n.Name
			}
		}
	}
	for ni := range topo.Nodes {
		for pi := range topo.Nodes[ni].Neighbors {
			neighbor := topo.Nodes[ni].Neighbors[pi]
			vrf := NormalizeNetworkInstance(string(neighbor.NetworkInstance))
			peer := addrToNode[string(vrf)+"|"+neighbor.Address]
			topo.Nodes[ni].Neighbors[pi].PeerNode = peer
		}
	}
}
