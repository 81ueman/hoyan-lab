package topology

import (
	"net/netip"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func normalizeKind(kind string) model.DeviceKind {
	switch kind {
	case "linux":
		return model.KindFRR
	case "arista_ceos":
		return model.KindCEOS
	case "nokia_srlinux":
		return model.KindSRLinux
	default:
		return model.DeviceKind(kind)
	}
}

func normalizeNodeRouting(node *model.Node) {
	for ri := range node.Routes {
		node.Routes[ri].Node = node.Name
		if node.Routes[ri].NetworkInstance == "" {
			node.Routes[ri].NetworkInstance = model.NetworkInstanceDefault
		} else {
			node.Routes[ri].NetworkInstance = model.NormalizeNetworkInstance(string(node.Routes[ri].NetworkInstance))
		}
		if node.Routes[ri].AFI == "" {
			node.Routes[ri].AFI = model.AFIIPv4
		}
	}
	for ni := range node.Neighbors {
		node.Neighbors[ni].NetworkInstance = model.NormalizeNetworkInstance(string(node.Neighbors[ni].NetworkInstance))
	}
	for ri := range node.Redistribute {
		node.Redistribute[ri].NetworkInstance = model.NormalizeNetworkInstance(string(node.Redistribute[ri].NetworkInstance))
	}
}

func parsedOSPFEnabled(parsed ParsedConfig) bool {
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

func appendUniquePrefix(prefixes []model.Prefix, prefix model.Prefix) []model.Prefix {
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

func resolveNeighborNodes(topo *model.Topology) {
	addrToNode := map[string]string{}
	for _, n := range topo.Nodes {
		for _, iface := range n.Interfaces {
			pfx, err := netip.ParsePrefix(iface.Address)
			if err == nil {
				vrf := model.NormalizeNetworkInstance(string(iface.VRF))
				addrToNode[string(vrf)+"|"+pfx.Addr().String()] = n.Name
			}
		}
	}
	for ni := range topo.Nodes {
		for pi := range topo.Nodes[ni].Neighbors {
			neighbor := topo.Nodes[ni].Neighbors[pi]
			vrf := model.NormalizeNetworkInstance(string(neighbor.NetworkInstance))
			peer := addrToNode[string(vrf)+"|"+neighbor.Address]
			topo.Nodes[ni].Neighbors[pi].PeerNode = peer
		}
	}
}
