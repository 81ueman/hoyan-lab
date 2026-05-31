package rib

import (
	"encoding/json"

	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func ParseCEOS(node string, data []byte) ([]RIBRoute, error) {
	_ = node
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	vrfs := asMap(root["vrfs"])
	if len(vrfs) == 0 {
		vrfs = map[string]any{"default": root}
	}
	var out []RIBRoute
	for ni, rawVRF := range vrfs {
		_ = ni
		vrf := asMap(rawVRF)
		entries := asMap(vrf["bgpRouteEntries"])
		for prefix, rawEntry := range entries {
			entry := asMap(rawEntry)
			var paths []observation.BGPPath
			for _, rawPath := range asSlice(entry["bgpRoutePaths"]) {
				p := asMap(rawPath)
				routeType := asMap(p["routeType"])
				peer := asMap(p["peerEntry"])
				asPathEntry := asMap(p["asPathEntry"])
				weight := intValue(p["weight"])
				paths = append(paths, observation.BGPPath{
					Best:      boolValue(routeType["active"]),
					Eligible:  boolValue(routeType["valid"]),
					NextHop:   observation.NextHop{Address: normalizeLocalNextHop(stringValue(p["nextHop"])), Weight: weight},
					ASPath:    parseASPath(stringValue(asPathEntry["asPath"])),
					Origin:    normalizeOrigin(firstString(p, "routeOrigin", "origin")),
					LocalPref: defaultLocalPref(intValue(p["localPreference"])),
					MED:       intValue(p["med"]),
					Weight:    weight,
					Communities: sortedStrings(appendCommunities(nil,
						firstPresent(p, "community", "communities", "communityList"),
						firstPresent(asPathEntry, "community", "communities", "communityList"),
					)),
					LargeCommunities: sortedStrings(appendCommunities(nil,
						firstPresent(p, "largeCommunity", "largeCommunities", "largeCommunityList"),
						firstPresent(asPathEntry, "largeCommunity", "largeCommunities", "largeCommunityList"),
					)),
					Peer:   stringValue(peer["peerAddr"]),
					PeerAS: uint32(intValue(peer["peerAS"])),
				})
			}
			if len(paths) > 0 {
				out = append(out, bgpRoute(prefix, paths))
			}
		}
	}
	sortRoutes(out)
	return out, nil
}
