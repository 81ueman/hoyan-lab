package fib

import "encoding/json"

func ParseCEOSRoutes(node string, data []byte) ([]FIBEntry, error) {
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	vrfs := mapValue(raw["vrfs"])
	if len(vrfs) == 0 {
		vrfs = map[string]any{"default": raw}
	}
	var out []FIBEntry
	for ni, rawVRF := range vrfs {
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
				Node:       node,
				VRF:        ni,
				AFI:        "ipv4",
				Prefix:     prefix,
				NextHops:   nextHops,
				Protocol:   protocol,
				Preference: intValue(m["preference"]),
				Metric:     intValue(m["metric"]),
				Installed:  true,
			}
			route.NextHops = dedupeNextHops(route.NextHops)
			out = append(out, route)
		}
	}
	sortRoutes(out)
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
