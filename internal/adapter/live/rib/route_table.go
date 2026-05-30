package rib

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strings"
)

type routeTableNextHop struct {
	Address   string
	Interface string
}

func ParseFRRRouteTable(node string, data []byte) ([]NormalizedRoute, error) {
	return ParseFRRRouteTableWithOSPF(node, data, nil)
}

func ParseFRRRouteTableWithOSPF(node string, data, ospfData []byte) ([]NormalizedRoute, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if vrfs := asMap(raw["vrfs"]); len(vrfs) > 0 {
		var out []NormalizedRoute
		for vrf, value := range vrfs {
			vrfMap := asMap(value)
			routesMap := vrfMap
			if nested := asMap(vrfMap["routes"]); nested != nil {
				routesMap = nested
			}
			out = append(out, parseFRRRouteTableMap(node, vrf, routesMap, nil)...)
		}
		SortRoutes(out)
		return out, nil
	}
	if looksLikeFRRVRFRouteMap(raw) {
		var out []NormalizedRoute
		for vrf, value := range raw {
			routesMap := asMap(value)
			if routesMap == nil {
				continue
			}
			out = append(out, parseFRRRouteTableMap(node, vrf, routesMap, nil)...)
		}
		SortRoutes(out)
		return out, nil
	}
	routesMap := raw
	if nested := asMap(raw["routes"]); nested != nil {
		routesMap = nested
	}
	ospfRouteTypes, err := parseFRROSPFRouteTypes(ospfData)
	if err != nil {
		return nil, err
	}
	out := parseFRRRouteTableMap(node, "default", routesMap, ospfRouteTypes)
	SortRoutes(out)
	return out, nil
}

func looksLikeFRRVRFRouteMap(raw map[string]any) bool {
	if len(raw) == 0 {
		return false
	}
	for key, value := range raw {
		if _, err := netip.ParsePrefix(key); err == nil {
			return false
		}
		if asMap(value) == nil {
			return false
		}
	}
	return true
}

func parseFRRRouteTableMap(node, vrf string, routesMap map[string]any, ospfRouteTypes map[string]string) []NormalizedRoute {
	var out []NormalizedRoute
	for prefix, value := range routesMap {
		if _, err := netip.ParsePrefix(prefix); err != nil {
			continue
		}
		for _, item := range routeTableItems(value) {
			protocol := normalizedRouteTableProtocol(firstString(item, "protocol", "routeType", "type"))
			if protocol == "ospf" && frrRouteTableOSPFInterArea(item) {
				protocol = "ospf-ia"
			}
			if protocol == "ospf" && ospfRouteTypes[prefix] != "" {
				protocol = ospfRouteTypes[prefix]
			}
			if protocol == "" {
				continue
			}
			hops := frrRouteTableNextHops(item)
			if routeTableBlackholeItem(item) || discardRouteTableNextHops(hops) || normalizedRouteTableProtocol(firstString(item, "type")) == "blackhole" {
				protocol = "blackhole"
				hops = nil
			}
			route := nonBGPRoute(node, vrf, "ipv4", prefix, protocol, hops)
			if len(route.Paths) > 0 {
				out = append(out, route)
			}
		}
	}
	return out
}

func parseFRROSPFRouteTypes(data []byte) (map[string]string, error) {
	out := map[string]string{}
	if len(data) == 0 {
		return out, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	for prefix, value := range raw {
		if _, err := netip.ParsePrefix(prefix); err != nil {
			continue
		}
		routeType := strings.ToLower(strings.TrimSpace(firstString(asMap(value), "routeType", "type")))
		switch {
		case strings.Contains(routeType, "ia"):
			out[prefix] = "ospf-ia"
		case strings.Contains(routeType, "n"):
			out[prefix] = "ospf"
		}
	}
	return out, nil
}

func ParseFRROSPFRouteTable(node string, data []byte) ([]NormalizedRoute, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var out []NormalizedRoute
	for prefix, value := range raw {
		if _, err := netip.ParsePrefix(prefix); err != nil {
			continue
		}
		m := asMap(value)
		protocol := "ospf"
		switch routeType := strings.ToLower(strings.TrimSpace(firstString(m, "routeType", "type"))); {
		case strings.Contains(routeType, "ia"):
			protocol = "ospf-ia"
		case strings.Contains(routeType, "n"), routeType == "":
			protocol = "ospf"
		default:
			if normalized := normalizedRouteTableProtocol(routeType); normalized != "" {
				protocol = normalized
			}
		}
		route := nonBGPRoute(node, "default", "ipv4", prefix, protocol, frrRouteTableNextHops(m))
		if len(route.Paths) > 0 {
			out = append(out, route)
		}
	}
	SortRoutes(out)
	return out, nil
}

func ParseCEOSRouteTable(node string, data []byte) ([]NormalizedRoute, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var out []NormalizedRoute
	vrfs := asMap(raw["vrfs"])
	if len(vrfs) == 0 {
		vrfs = map[string]any{"default": raw}
	}
	for ni, rawVRF := range vrfs {
		routes := asMap(asMap(rawVRF)["routes"])
		for prefix, value := range routes {
			m := asMap(value)
			protocol := normalizedRouteTableProtocol(firstString(m, "routeType", "sourceProtocol"))
			if protocol == "" {
				continue
			}
			hops := ceosRouteTableNextHops(m["vias"])
			if discardRouteTableNextHops(hops) {
				protocol = "blackhole"
				hops = nil
			}
			out = append(out, nonBGPRoute(node, ni, "ipv4", prefix, protocol, hops))
		}
	}
	SortRoutes(out)
	return out, nil
}

func ParseSRLinuxRouteTable(node string, data []byte) ([]NormalizedRoute, error) {
	return ParseSRLinuxRouteTableNetworkInstance(node, "default", data)
}

func ParseSRLinuxRouteTableNetworkInstance(node, networkInstance string, data []byte) ([]NormalizedRoute, error) {
	cleaned, err := jsonPayload(data)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := json.Unmarshal(cleaned, &raw); err != nil {
		return nil, err
	}
	var out []NormalizedRoute
	for _, inst := range asSlice(raw["instance"]) {
		for _, item := range asSlice(asMap(inst)["ip route"]) {
			m := asMap(item)
			if !routeTableActive(m) {
				continue
			}
			protocol := normalizedRouteTableProtocol(firstString(m, "Route Type", "route-type"))
			if protocol == "" {
				continue
			}
			prefix := firstString(m, "Prefix", "prefix")
			if prefix == "" {
				continue
			}
			hops := srlinuxRouteTableNextHops(m)
			if discardRouteTableNextHops(hops) {
				protocol = "blackhole"
				hops = nil
			}
			out = append(out, nonBGPRoute(node, networkInstance, "ipv4", prefix, protocol, hops))
		}
	}
	SortRoutes(out)
	return out, nil
}

func routeTableItems(value any) []map[string]any {
	switch x := value.(type) {
	case []any:
		out := make([]map[string]any, 0, len(x))
		for _, item := range x {
			if m := asMap(item); m != nil {
				out = append(out, m)
			}
		}
		return out
	case map[string]any:
		return []map[string]any{x}
	default:
		return nil
	}
}

func normalizedRouteTableProtocol(protocol string) string {
	normalized := strings.ToLower(strings.TrimSpace(protocol))
	if strings.Contains(normalized, "ospf") {
		return "ospf"
	}
	switch normalized {
	case "bgp", "ebgp", "ibgp", "":
		return ""
	case "kernel", "connected", "connect", "direct", "local", "host":
		return "connected"
	case "static":
		return "static"
	case "ospf ia", "ospfia", "ospf-ia", "o ia", "ia":
		return "ospf-ia"
	case "ospf", "ospf intra", "ospf-intra", "o":
		return "ospf"
	case "blackhole", "discard", "drop", "null0", "null":
		return "blackhole"
	default:
		return ""
	}
}

func frrRouteTableOSPFInterArea(m map[string]any) bool {
	for _, key := range []string{"routeType", "subType", "subtype", "ospfRouteType", "routeCode", "code"} {
		value := strings.ToLower(strings.TrimSpace(firstString(m, key)))
		if value == "" {
			continue
		}
		if strings.Contains(value, "inter") || strings.Contains(value, "ia") {
			return true
		}
	}
	return false
}

func nonBGPRoute(node, ni, afi, prefix, protocol string, hops []routeTableNextHop) NormalizedRoute {
	if ni == "" {
		ni = "default"
	}
	if afi == "" {
		afi = "ipv4"
	}
	return NormalizedRoute{
		Node:            node,
		NetworkInstance: ni,
		AFI:             afi,
		Prefix:          prefix,
		Protocol:        protocol,
		Paths:           []NormalizedPath{nonBGPPath(protocol, hops)},
	}
}

func nonBGPPath(protocol string, hops []routeTableNextHop) NormalizedPath {
	path := NormalizedPath{Best: true, Valid: true, Origin: "igp", LocalPref: 100}
	if protocol == "connected" || protocol == "blackhole" || len(hops) == 0 {
		return path
	}
	if hops[0].Address != "" && hops[0].Address != "0.0.0.0" {
		path.NextHop = hops[0].Address
	}
	return path
}

func discardRouteTableNextHops(hops []routeTableNextHop) bool {
	if len(hops) == 0 {
		return false
	}
	for _, hop := range hops {
		if !discardRouteTableNextHop(hop) {
			return false
		}
	}
	return true
}

func routeTableBlackholeItem(m map[string]any) bool {
	if boolValue(firstPresent(m, "blackhole", "discard")) {
		return true
	}
	for _, raw := range asSlice(firstPresent(m, "nexthops", "nextHops")) {
		hop := asMap(raw)
		if boolValue(firstPresent(hop, "blackhole", "discard")) {
			return true
		}
	}
	return false
}

func discardRouteTableNextHop(hop routeTableNextHop) bool {
	if hop.Address != "" && !discardRouteTableToken(hop.Address) {
		return false
	}
	return discardRouteTableToken(hop.Interface)
}

func discardRouteTableToken(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "null0", "null", "discard", "drop", "blackhole":
		return true
	default:
		return false
	}
}

func frrRouteTableNextHops(m map[string]any) []routeTableNextHop {
	if hops := routeTableNextHops(firstPresent(m, "nexthops", "nextHops")); len(hops) > 0 {
		return hops
	}
	hop := routeTableNextHop{
		Address:   firstString(m, "nexthop", "nextHop", "gateway", "via"),
		Interface: firstString(m, "interfaceName", "interface", "dev"),
	}
	if hop.Address != "" || hop.Interface != "" {
		return []routeTableNextHop{hop}
	}
	return nil
}

func ceosRouteTableNextHops(raw any) []routeTableNextHop {
	var out []routeTableNextHop
	for _, item := range asSlice(raw) {
		m := asMap(item)
		hop := routeTableNextHop{
			Address:   firstString(m, "nexthopAddr", "nextHop", "gateway"),
			Interface: firstString(m, "interface", "interfaceName"),
		}
		if hop.Address != "" || hop.Interface != "" {
			out = append(out, hop)
		}
	}
	return out
}

func srlinuxRouteTableNextHops(m map[string]any) []routeTableNextHop {
	hop := routeTableNextHop{
		Address:   srlinuxNextHopAddress(firstString(m, "Next-hop (Type)", "Next-hop", "next-hop")),
		Interface: firstString(m, "Next-hop Interface", "next-hop-interface"),
	}
	if hop.Address == "" && hop.Interface == "" {
		return nil
	}
	return []routeTableNextHop{hop}
}

func routeTableNextHops(raw any) []routeTableNextHop {
	var out []routeTableNextHop
	for _, item := range asSlice(raw) {
		m := asMap(item)
		hop := routeTableNextHop{
			Address:   firstString(m, "ip", "gateway", "nexthop", "nextHop"),
			Interface: firstString(m, "interfaceName", "interface", "dev", "via"),
		}
		if hop.Address != "" || hop.Interface != "" {
			out = append(out, hop)
		}
	}
	return out
}

func routeTableActive(m map[string]any) bool {
	if v := firstPresent(m, "Active", "active"); v != nil {
		return boolValue(v)
	}
	if v := firstPresent(m, "selected", "installed", "fib"); v != nil {
		return boolValue(v)
	}
	return true
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
