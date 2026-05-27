package model

import (
	"net/netip"
)

type InterfaceRef struct {
	Node       NodeID
	Link       LinkID
	ClabName   string
	ConfigName string
	Address    netip.Prefix
}

type InterfaceResolver struct {
	Index *TopologyIndex
}

func InterfaceAliases(kind DeviceKind, clabName string) []string {
	return ProfileFor(kind).InterfaceProfile().InterfaceAliases(clabName)
}

func CanonicalInterfaceName(kind DeviceKind, name string) string {
	return ProfileFor(kind).InterfaceProfile().CanonicalInterfaceName(name)
}

func ResolveInterface(node Node, clabName string) (InterfaceRef, bool) {
	return resolveInterface(node, "", clabName)
}

func EquivalentInterfaceName(kind DeviceKind, a, b string) bool {
	return ProfileFor(kind).InterfaceProfile().EquivalentInterfaceName(a, b)
}

func InterfaceAddress(kind DeviceKind, interfaces []Interface, name string) (netip.Prefix, bool) {
	for _, alias := range InterfaceAliases(kind, name) {
		for _, iface := range interfaces {
			if iface.Name != alias {
				continue
			}
			pfx, err := netip.ParsePrefix(iface.Address)
			return pfx, err == nil
		}
	}
	return netip.Prefix{}, false
}

func (r InterfaceResolver) ResolveInterface(node Node, link LinkID, clabName string) (InterfaceRef, bool) {
	return resolveInterface(node, link, clabName)
}

func resolveInterface(node Node, link LinkID, clabName string) (InterfaceRef, bool) {
	for _, alias := range InterfaceAliases(node.Kind, clabName) {
		for _, iface := range node.Interfaces {
			if iface.Name != alias {
				continue
			}
			pfx, err := netip.ParsePrefix(iface.Address)
			if err != nil {
				return InterfaceRef{}, false
			}
			return InterfaceRef{
				Node:       NodeID(node.Name),
				Link:       link,
				ClabName:   clabName,
				ConfigName: iface.Name,
				Address:    pfx,
			}, true
		}
	}
	return InterfaceRef{}, false
}

func uniqueStrings(xs ...string) []string {
	out := make([]string, 0, len(xs))
	seen := map[string]bool{}
	for _, x := range xs {
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
	}
	return out
}
