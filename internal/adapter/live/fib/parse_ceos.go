package fib

import (
	"encoding/json"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func ParseCEOSRoutes(node string, data []byte) ([]FIBEntry, error) {
	fibs, err := ParseCEOSFIBs(node, data)
	if err != nil {
		return nil, err
	}
	var out []FIBEntry
	for _, fib := range fibs {
		out = append(out, fib.Entries...)
	}
	sortRoutes(out)
	return out, nil
}

func ParseCEOSFIBs(node string, data []byte) ([]FIB, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	vrfs := mapValue(raw["vrfs"])
	if len(vrfs) == 0 {
		vrfs = map[string]any{"default": raw}
	}
	var out []FIB
	for ni, rawVRF := range vrfs {
		fib := FIB{Node: model.NodeID(node), VRF: model.NetworkInstanceID(ni)}
		routes := mapValue(mapValue(rawVRF)["routes"])
		for prefix, value := range routes {
			m := mapValue(value)
			if !boolValue(m["kernelProgrammed"]) && !boolValue(m["hardwareProgrammed"]) {
				continue
			}
			nextHops := ceosNextHops(m["vias"])
			protocol := ceosProtocol(stringValue(m["routeType"]))
			if discardNextHops(nextHops) {
				protocol = "blackhole"
				nextHops = nil
			}
			route := FIBEntry{
				AFI:        "ipv4",
				Prefix:     prefix,
				NextHops:   nextHops,
				Source:     canonicalRouteSource(protocol),
				Action:     forwardingAction(protocol, nextHops),
				Preference: intValue(m["preference"]),
				Metric:     intValue(m["metric"]),
			}
			route.NextHops = dedupeNextHops(route.NextHops)
			fib.Entries = append(fib.Entries, route)
		}
		sortRoutes(fib.Entries)
		out = append(out, fib)
	}
	out = sortedFIBs(out)
	return out, nil
}

func ceosProtocol(routeType string) string {
	return canonicalProtocol(routeType)
}

func ceosNextHops(raw any) []NextHop {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]NextHop, 0, len(items))
	for _, item := range items {
		m := mapValue(item)
		out = append(out, NextHop{
			Address:   stringValue(m["nexthopAddr"]),
			Interface: stringValue(m["interface"]),
			Weight:    intValue(m["weight"]),
		})
	}
	return out
}
