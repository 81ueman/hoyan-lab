package model

import (
	"sort"
)

type NodeID string
type LinkID string
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
