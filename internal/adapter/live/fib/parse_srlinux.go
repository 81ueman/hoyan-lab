package fib

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"regexp"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

var srlinuxDetailNextHopRE = regexp.MustCompile(`(?m)(?:^|\s)(?:via\s+)?([0-9A-Fa-f:.]+)\s+\([^)]*\)\s+via\s+\[([^\]]+)\]`)

func ParseSRLinuxRoutes(node string, data []byte) ([]FIBEntry, error) {
	return ParseSRLinuxRoutesNetworkInstance(node, "default", data)
}

func ParseSRLinuxRoutesNetworkInstance(node, networkInstance string, data []byte) ([]FIBEntry, error) {
	cleaned, err := jsonPayload(data)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(cleaned, &raw); err != nil {
		return nil, err
	}
	var out []FIBEntry
	for _, inst := range sliceValue(raw["instance"]) {
		m := mapValue(inst)
		for _, item := range sliceValue(m["ip route"]) {
			route, ok := srlinuxRoute(node, networkInstance, mapValue(item))
			if ok {
				out = append(out, route)
			}
		}
	}
	sortRoutes(out)
	return out, nil
}

func ParseSRLinuxRouteDetails(node string, data []byte) ([]FIBEntry, error) {
	return ParseSRLinuxRouteDetailsNetworkInstance(node, "default", data)
}

func ParseSRLinuxRouteDetailsNetworkInstance(node, networkInstance string, data []byte) ([]FIBEntry, error) {
	cleaned, err := jsonPayload(data)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(cleaned, &raw); err != nil {
		return nil, err
	}
	var out []FIBEntry
	for _, inst := range sliceValue(raw["instance"]) {
		m := mapValue(inst)
		for _, item := range sliceValue(m["ip route"]) {
			route, ok := srlinuxDetailRoute(node, networkInstance, mapValue(item))
			if ok {
				out = append(out, route)
			}
		}
	}
	sortRoutes(out)
	return out, nil
}

func jsonPayload(data []byte) ([]byte, error) {
	s := string(data)
	start := strings.IndexByte(s, '{')
	end := strings.LastIndexByte(s, '}')
	if start < 0 || end < start {
		return nil, fmt.Errorf("no JSON object found")
	}
	return []byte(s[start : end+1]), nil
}

func srlinuxRoute(node, networkInstance string, m map[string]any) (FIBEntry, bool) {
	_, _ = node, networkInstance
	if !boolString(m["Active"]) {
		return FIBEntry{}, false
	}
	prefix := stringValue(m["Prefix"])
	if prefix == "" {
		return FIBEntry{}, false
	}
	protocol := canonicalProtocol(stringValue(m["Route Type"]))
	route := FIBEntry{
		AFI:        model.AFIFromPrefix(prefix),
		Prefix:     prefix,
		Source:     canonicalRouteSource(protocol),
		Action:     forwardingAction(protocol, nil),
		Preference: intValue(m["Pref"]),
		Metric:     intValue(m["Metric"]),
	}
	hop := NextHop{
		Address:   srlinuxNextHopAddress(stringValue(m["Next-hop (Type)"])),
		Interface: strings.TrimSpace(stringValue(m["Next-hop Interface"])),
	}
	if hop.Address != "" || hop.Interface != "" {
		route.NextHops = append(route.NextHops, hop)
	}
	if discardNextHops(route.NextHops) {
		route.Source = canonicalRouteSource("blackhole")
		route.NextHops = nil
	}
	route.Action = forwardingAction(string(route.Source.Protocol), route.NextHops)
	route.NextHops = dedupeNextHops(route.NextHops)
	return route, true
}

func srlinuxDetailRoute(node, networkInstance string, m map[string]any) (FIBEntry, bool) {
	_, _ = node, networkInstance
	if !boolValue(m["Active"]) {
		return FIBEntry{}, false
	}
	prefix := stringValue(m["Destination"])
	if prefix == "" {
		prefix = stringValue(m["Prefix"])
	}
	if prefix == "" {
		return FIBEntry{}, false
	}
	route := FIBEntry{
		AFI:        model.AFIFromPrefix(prefix),
		Prefix:     prefix,
		Source:     canonicalRouteSource(stringValue(m["Route Type"])),
		Preference: firstIntValue(m["Preference"], m["Pref"]),
		Metric:     intValue(m["Metric"]),
	}
	route.NextHops = append(route.NextHops, srlinuxDetailNextHops(mapValue(m["ip route nexthop"]), "Next hops")...)
	route.NextHops = dedupeNextHops(route.NextHops)
	route.Action = forwardingAction(string(route.Source.Protocol), route.NextHops)
	return route, true
}

func srlinuxDetailNextHops(m map[string]any, key string) []NextHop {
	raw := stringValue(m[key])
	if raw == "" {
		return nil
	}
	matches := srlinuxDetailNextHopRE.FindAllStringSubmatch(raw, -1)
	out := make([]NextHop, 0, len(matches))
	for _, match := range matches {
		addr := srlinuxNextHopAddress(match[1])
		iface := strings.TrimSpace(match[2])
		if addr == "" && iface == "" {
			continue
		}
		out = append(out, NextHop{Address: addr, Interface: iface})
	}
	return out
}

func srlinuxNextHopAddress(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "None" {
		return ""
	}
	fields := strings.Fields(raw)
	if len(fields) == 0 {
		return ""
	}
	addr := fields[0]
	if pfx, err := netip.ParsePrefix(addr); err == nil {
		return pfx.Addr().String()
	}
	if ip, err := netip.ParseAddr(addr); err == nil {
		return ip.String()
	}
	return addr
}

func firstIntValue(values ...any) int {
	for _, v := range values {
		if got := intValue(v); got != 0 {
			return got
		}
	}
	return 0
}
