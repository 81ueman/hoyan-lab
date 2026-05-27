package model

import (
	"net/netip"
	"sort"
)

type NodeID string
type LinkID string
type PolicyID string
type RoutePolicyID string
type PrefixListID string
type DeviceKind string
type NetworkInstanceID string
type AFI string

const (
	KindFRR     DeviceKind = "frr"
	KindCEOS    DeviceKind = "ceos"
	KindSRLinux DeviceKind = "srlinux"

	NetworkInstanceDefault NetworkInstanceID = "default"
	AFIIPv4                AFI               = "ipv4"
)

func NormalizeNetworkInstance(vrf string) NetworkInstanceID {
	if vrf == "" {
		return NetworkInstanceDefault
	}
	return NetworkInstanceID(vrf)
}

func NetworkInstancesForNode(n Node) []string {
	seen := map[string]bool{string(NetworkInstanceDefault): true}
	for _, iface := range n.Interfaces {
		seen[string(NormalizeNetworkInstance(string(iface.VRF)))] = true
	}
	for _, route := range n.Routes {
		seen[string(NormalizeNetworkInstance(string(route.NetworkInstance)))] = true
	}
	out := make([]string, 0, len(seen))
	for ni := range seen {
		out = append(out, ni)
	}
	sort.Strings(out)
	return out
}

type NextHop struct {
	Node NodeID
	Addr netip.Addr
}

func NodeNextHop(node NodeID) NextHop {
	return NextHop{Node: node}
}

func AddrNextHop(addr netip.Addr) NextHop {
	return NextHop{Addr: addr}
}

func (h NextHop) IsZero() bool {
	return h.Node == "" && !h.Addr.IsValid()
}

func (h NextHop) String() string {
	if h.Addr.IsValid() {
		return h.Addr.String()
	}
	return string(h.Node)
}
