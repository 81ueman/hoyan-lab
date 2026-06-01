package model

import (
	"sort"
	"strings"
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
	AFIIPv6                AFI               = "ipv6"
)

func NormalizeNetworkInstance(vrf string) NetworkInstanceID {
	if vrf == "" {
		return NetworkInstanceDefault
	}
	return NetworkInstanceID(vrf)
}

func NormalizeAFI(afi AFI) AFI {
	switch AFI(strings.ToLower(strings.TrimSpace(string(afi)))) {
	case "", AFIIPv4:
		return AFIIPv4
	case AFIIPv6:
		return AFIIPv6
	default:
		return AFI(strings.ToLower(strings.TrimSpace(string(afi))))
	}
}

func NetworkInstancesForNode(n Node) []string {
	out := make([]string, 0)
	for _, ni := range networkInstanceIDsForNode(n) {
		out = append(out, string(ni))
	}
	return out
}

func networkInstanceIDsForNode(n Node) []NetworkInstanceID {
	seen := map[string]bool{string(NetworkInstanceDefault): true}
	for _, iface := range n.Interfaces {
		seen[string(NormalizeNetworkInstance(string(iface.VRF)))] = true
	}
	for _, route := range n.Routes {
		seen[string(NormalizeNetworkInstance(string(route.NetworkInstance)))] = true
	}
	out := make([]NetworkInstanceID, 0, len(seen))
	for ni := range seen {
		out = append(out, NetworkInstanceID(ni))
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}
