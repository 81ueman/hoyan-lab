package fib

import (
	"encoding/json"
	"fmt"
	"net/netip"
	"strconv"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func ParseLinuxIPRoute(node string, data []byte) ([]FIBEntry, error) {
	return ParseLinuxIPRouteVRF(node, "default", data)
}

func ParseLinuxIPRouteVRF(node, vrf string, data []byte) ([]FIBEntry, error) {
	_, _ = node, vrf
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var out []FIBEntry
	for i, item := range raw {
		prefix, ok, err := routePrefix(item)
		if err != nil {
			return nil, fmt.Errorf("route[%d]: %w", i, err)
		}
		if !ok {
			continue
		}
		protocol := linuxRouteProtocol(item)
		nextHops := routeNextHops(item)
		if discardLinuxRoute(item, nextHops) {
			protocol = "blackhole"
			nextHops = nil
		}
		route := FIBEntry{
			AFI:        model.AFIFromPrefix(prefix),
			Prefix:     prefix,
			NextHops:   nextHops,
			Source:     canonicalRouteSource(protocol),
			Action:     forwardingAction(protocol, nextHops),
			Preference: intValue(item["pref"]),
			Metric:     intValue(item["metric"]),
		}
		route.NextHops = dedupeNextHops(route.NextHops)
		out = append(out, route)
	}
	sortRoutes(out)
	return out, nil
}

// routePrefix extracts the prefix from a Linux kernel route JSON item.
// It detects whether the route is IPv6 by checking the gateway/nexthop addresses
// so that "default" is correctly mapped to 0.0.0.0/0 or ::/0.
func routePrefix(item map[string]any) (string, bool, error) {
	dst := stringValue(item["dst"])
	if dst == "" {
		return "", false, nil
	}
	if dst == "default" {
		// Determine if this is an IPv6 default route by inspecting gateway/nexthop
		if isIPv6Route(item) {
			return "::/0", true, nil
		}
		return "0.0.0.0/0", true, nil
	}
	if dst == "::/0" {
		return "::/0", true, nil
	}
	if addr, err := netip.ParseAddr(dst); err == nil {
		if addr.Is4() {
			return netip.PrefixFrom(addr, 32).String(), true, nil
		}
		return netip.PrefixFrom(addr, 128).String(), true, nil
	}
	pfx, err := netip.ParsePrefix(dst)
	if err != nil {
		return "", false, err
	}
	return pfx.Masked().String(), true, nil
}

// isIPv6Route checks if the route item appears to be IPv6 by scanning gateway addresses.
func isIPv6Route(item map[string]any) bool {
	if gateway := stringValue(item["gateway"]); gateway != "" {
		if addr, err := netip.ParseAddr(gateway); err == nil && !addr.Is4() {
			return true
		}
	}
	if raw, ok := item["nexthops"].([]any); ok {
		for _, elem := range raw {
			if m, ok := elem.(map[string]any); ok {
				if gw := stringValue(m["gateway"]); gw != "" {
					if addr, err := netip.ParseAddr(gw); err == nil && !addr.Is4() {
						return true
					}
				}
			}
		}
	}
	return false
}

func linuxRouteProtocol(item map[string]any) string {
	if routeType := canonicalProtocol(stringValue(item["type"])); routeType == "blackhole" {
		return routeType
	}
	if protocol := canonicalProtocol(stringValue(item["protocol"])); protocol != "" {
		return protocol
	}
	return canonicalProtocol(stringValue(item["type"]))
}

func discardLinuxRoute(item map[string]any, hops []NextHop) bool {
	if canonicalProtocol(stringValue(item["type"])) == "blackhole" {
		return true
	}
	return discardNextHops(hops)
}

func discardNextHops(hops []NextHop) bool {
	if len(hops) == 0 {
		return false
	}
	for _, hop := range hops {
		if !discardNextHop(hop) {
			return false
		}
	}
	return true
}

func discardNextHop(hop NextHop) bool {
	if hop.Address != "" && !discardToken(hop.Address) {
		return false
	}
	return discardToken(hop.Interface)
}

func discardToken(raw string) bool {
	switch canonicalProtocol(raw) {
	case "blackhole":
		return true
	default:
		return false
	}
}

func routeNextHops(item map[string]any) []NextHop {
	if raw, ok := item["nexthops"].([]any); ok {
		out := make([]NextHop, 0, len(raw))
		for _, elem := range raw {
			m, ok := elem.(map[string]any)
			if !ok {
				continue
			}
			out = append(out, NextHop{
				Address:   stringValue(m["gateway"]),
				Interface: stringValue(m["dev"]),
				Weight:    intValue(m["weight"]),
			})
		}
		return out
	}
	if gateway := stringValue(item["gateway"]); gateway != "" || stringValue(item["dev"]) != "" {
		return []NextHop{{
			Address:   gateway,
			Interface: stringValue(item["dev"]),
			Weight:    intValue(item["weight"]),
		}}
	}
	return nil
}

func stringValue(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatInt(int64(x), 10)
	default:
		return ""
	}
}

func intValue(v any) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case json.Number:
		i, _ := strconv.Atoi(x.String())
		return i
	case string:
		i, _ := strconv.Atoi(x)
		return i
	default:
		return 0
	}
}
