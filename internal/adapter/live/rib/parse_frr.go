package rib

import (
	"encoding/json"
	"strings"
)

func ParseFRR(node string, data []byte) ([]RIBRoute, error) {
	return ParseFRRVRF(node, "default", data)
}

func ParseFRRVRF(node, vrf string, data []byte) ([]RIBRoute, error) {
	type frrPath struct {
		Valid            bool     `json:"valid"`
		Best             bool     `json:"bestpath"`
		Multipath        bool     `json:"multipath"`
		Path             string   `json:"path"`
		Origin           string   `json:"origin"`
		LocalPref        int      `json:"locPrf"`
		MED              int      `json:"metric"`
		Weight           int      `json:"weight"`
		Peer             string   `json:"peerId"`
		Communities      []string `json:"community"`
		LargeCommunities []string `json:"largeCommunity"`
		Nexthops         []struct {
			IP string `json:"ip"`
		} `json:"nexthops"`
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if payload, ok := raw["routes"]; ok {
		if err := json.Unmarshal(payload, &raw); err != nil {
			return nil, err
		}
	}
	if vrfs, ok := raw["vrfs"]; ok {
		var byVRF map[string]json.RawMessage
		if err := json.Unmarshal(vrfs, &byVRF); err != nil {
			return nil, err
		}
		var out []RIBRoute
		for name, payload := range byVRF {
			routes, err := ParseFRRVRF(node, name, payload)
			if err != nil {
				return nil, err
			}
			out = append(out, routes...)
		}
		sortRoutes(out)
		return out, nil
	}
	var out []RIBRoute
	for prefix, payload := range raw {
		if !strings.Contains(prefix, "/") {
			continue
		}
		var paths []frrPath
		if err := json.Unmarshal(payload, &paths); err != nil {
			continue
		}
		route := RIBRoute{Node: node, NetworkInstance: vrf, AFI: "ipv4", Prefix: prefix}
		for _, p := range paths {
			nextHop := ""
			if len(p.Nexthops) > 0 {
				nextHop = p.Nexthops[0].IP
				if nextHop == "0.0.0.0" {
					nextHop = ""
				}
			}
			route.Paths = append(route.Paths, RIBPath{
				Best:             p.Best || p.Multipath,
				Valid:            p.Valid,
				NextHop:          nextHop,
				ASPath:           parseASPath(p.Path),
				Origin:           normalizeOrigin(p.Origin),
				LocalPref:        defaultLocalPref(p.LocalPref),
				MED:              p.MED,
				Weight:           p.Weight,
				Communities:      sortedStrings(p.Communities),
				LargeCommunities: sortedStrings(p.LargeCommunities),
				Peer:             p.Peer,
			})
		}
		if len(route.Paths) > 0 {
			sortPaths(route.Paths, DefaultCompareOptions())
			out = append(out, route)
		}
	}
	sortRoutes(out)
	return out, nil
}
