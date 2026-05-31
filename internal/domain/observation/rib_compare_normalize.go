package observation

import (
	"fmt"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func normalizeRoute(r RIBRoute) RIBRoute {
	if r.NetworkInstance == "" {
		r.NetworkInstance = "default"
	}
	if r.AFI == "" {
		r.AFI = "ipv4"
	}
	if r.Protocol == "" {
		r.Protocol = "bgp"
	}
	for i := range r.Paths {
		r.Paths[i] = normalizePath(r.Paths[i])
	}
	return r
}

func comparableRIBRoutes(routes []RIBRoute) []RIBRoute {
	out := make([]RIBRoute, 0, len(routes))
	for _, route := range routes {
		out = append(out, comparableRIBRoute(route))
	}
	return out
}

func comparableRIBRoute(route RIBRoute) RIBRoute {
	if route.Common.Prefix == "" {
		return route
	}
	out := RIBRoute{
		AFI:      string(model.NormalizeAFI(route.Common.AFI)),
		Prefix:   route.Common.Prefix,
		Protocol: string(model.NormalizeRouteSourceKind(route.Common.Protocol)),
	}
	if route.BGP != nil {
		out.Paths = ribPathsFromBGPPaths(route.BGP.Paths)
	}
	return out
}

func ribPathsFromBGPPaths(paths []BGPPath) []RIBPath {
	out := make([]RIBPath, 0, len(paths))
	for _, path := range paths {
		out = append(out, RIBPath{
			NextHop:          path.NextHop.Address,
			ASPath:           append([]uint32(nil), path.ASPath...),
			Origin:           path.Origin,
			LocalPref:        path.LocalPref,
			MED:              path.MED,
			Weight:           path.Weight,
			Communities:      append([]string(nil), path.Communities...),
			LargeCommunities: append([]string(nil), path.LargeCommunities...),
			OriginatorID:     path.OriginatorID,
			ClusterList:      append([]string(nil), path.ClusterList...),
			Peer:             path.Peer,
			PeerAS:           path.PeerAS,
			Valid:            path.Eligible,
			Best:             path.Best,
		})
	}
	return out
}

func NormalizeRIBRouteRecord(r RIBRoute) RIBRoute {
	return normalizeRoute(r)
}

func normalizePath(p RIBPath) RIBPath {
	p.Origin = normalizeOrigin(p.Origin)
	p.Communities = sortedStrings(p.Communities)
	p.LargeCommunities = sortedStrings(p.LargeCommunities)
	p.ClusterList = sortedStrings(p.ClusterList)
	return p
}

func routeKey(r RIBRoute) string {
	r = normalizeRoute(r)
	if r.Protocol != "" && r.Protocol != "bgp" {
		return r.Node + "|" + r.NetworkInstance + "|" + r.AFI + "|" + r.Protocol + "|" + r.Prefix
	}
	return r.Node + "|" + r.NetworkInstance + "|" + r.AFI + "|" + r.Prefix
}

func ribTableRouteKey(r RIBRoute) string {
	r = normalizeRoute(r)
	return r.AFI + "|" + r.Protocol + "|" + r.Prefix
}

func pathKey(p RIBPath, opts CompareOptions) string {
	// Path identity is deliberately narrower than full path equality. The
	// default identity is next-hop plus AS path; attributes such as best, valid,
	// origin, local-pref, MED, weight, communities, originator ID, and cluster
	// list are compared after identity matching so attribute mismatches stay
	// distinct from missing/unexpected paths. ComparePeer and ComparePeerAS are
	// the only options that extend identity, letting callers distinguish
	// otherwise identical multipath entries learned from different peers.
	parts := []string{"nh=" + p.NextHop, "as=" + formatASPath(p.ASPath)}
	if opts.ComparePeer && p.Peer != "" {
		parts = append(parts, "peer="+p.Peer)
	}
	if opts.ComparePeerAS && p.PeerAS != 0 {
		parts = append(parts, fmt.Sprintf("peer_as=%d", p.PeerAS))
	}
	return strings.Join(parts, "|")
}

func formatASPath(path []uint32) string {
	parts := make([]string, 0, len(path))
	for _, asn := range path {
		parts = append(parts, fmt.Sprint(asn))
	}
	return strings.Join(parts, " ")
}

func normalizeOrigin(origin model.BGPOriginCode) model.BGPOriginCode {
	switch strings.ToLower(strings.TrimSpace(string(origin))) {
	case "", "i", "igp":
		return model.BGPOriginIGP
	case "e", "egp":
		return model.BGPOriginEGP
	case "?", "incomplete":
		return model.BGPOriginIncomplete
	default:
		return model.NormalizeBGPOriginCode(model.BGPOriginCode(origin))
	}
}

func defaultLocalPref(v int) int {
	if v == 0 {
		return 100
	}
	return v
}

func DefaultLocalPref(v int) int {
	return defaultLocalPref(v)
}
