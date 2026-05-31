package observation

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	normalizedrib "github.com/81ueman/hoyan-lab/internal/domain/observation/rib"
)

type RIB struct {
	Node   NodeID     `json:"node"`
	VRF    VRFName    `json:"vrf"`
	Routes []RIBRoute `json:"routes"`
}

type RIBRoute struct {
	Common RIBRouteCommon `json:"common"`

	BGP       *BGPRIBRoute       `json:"bgp,omitempty"`
	OSPF      *OSPFRIBRoute      `json:"ospf,omitempty"`
	Static    *StaticRIBRoute    `json:"static,omitempty"`
	Connected *ConnectedRIBRoute `json:"connected,omitempty"`
	Blackhole *BlackholeRIBRoute `json:"blackhole,omitempty"`

	ModelInfo *ModelRouteInfo `json:"model_info,omitempty"`
}

type RIBRouteCommon struct {
	AFI      AddressFamily `json:"afi"`
	Prefix   string        `json:"prefix"`
	Protocol RouteProtocol `json:"protocol"`

	Preference int `json:"preference,omitempty"`
	Metric     int `json:"metric,omitempty"`

	Eligible bool `json:"eligible"`
	Best     bool `json:"best"`
}

type BGPRIBRoute struct {
	Paths []BGPPath `json:"paths"`
}

type BGPPath struct {
	NextHop          NextHop  `json:"next_hop,omitempty"`
	ASPath           []uint32 `json:"as_path,omitempty"`
	Origin           string   `json:"origin,omitempty"`
	LocalPref        int      `json:"local_pref,omitempty"`
	MED              int      `json:"med,omitempty"`
	Weight           int      `json:"weight,omitempty"`
	Communities      []string `json:"communities,omitempty"`
	LargeCommunities []string `json:"large_communities,omitempty"`
	OriginatorID     string   `json:"originator_id,omitempty"`
	ClusterList      []string `json:"cluster_list,omitempty"`
	Peer             string   `json:"peer,omitempty"`
	PeerAS           uint32   `json:"peer_as,omitempty"`
	Eligible         bool     `json:"eligible"`
	Best             bool     `json:"best"`
}

type OSPFRouteType string

const (
	OSPFRouteTypeIntraArea OSPFRouteType = "intra_area"
	OSPFRouteTypeInterArea OSPFRouteType = "inter_area"
	OSPFRouteTypeExternal1 OSPFRouteType = "external_type_1"
	OSPFRouteTypeExternal2 OSPFRouteType = "external_type_2"
	OSPFRouteTypeUnknown   OSPFRouteType = "unknown"
)

type OSPFRIBRoute struct {
	RouteType OSPFRouteType `json:"route_type"`
	Area      string        `json:"area,omitempty"`
	Paths     []OSPFPath    `json:"paths"`
}

type OSPFPath struct {
	NextHop NextHop `json:"next_hop,omitempty"`
	Cost    int     `json:"cost,omitempty"`
}

type StaticRIBRoute struct {
	NextHops []NextHop `json:"next_hops,omitempty"`
}

type ConnectedRIBRoute struct {
	Interface string `json:"interface,omitempty"`
}

type BlackholeRIBRoute struct {
	Reason string `json:"reason,omitempty"`
}

func (r RIBRoute) Validate() error {
	if r.Common.Prefix == "" {
		return errors.New("rib route prefix is required")
	}
	if NormalizeRouteProtocol(r.Common.Protocol) != r.Common.Protocol {
		return fmt.Errorf("rib route protocol %q is not normalized", r.Common.Protocol)
	}
	payloads := r.payloadCount()
	if payloads != 1 {
		return fmt.Errorf("rib route must have exactly one protocol payload, got %d", payloads)
	}
	if r.Common.Protocol != r.payloadProtocol() {
		return fmt.Errorf("rib route protocol %q does not match payload %q", r.Common.Protocol, r.payloadProtocol())
	}
	return nil
}

func (r RIB) Validate() error {
	if r.Node == "" {
		return errors.New("rib node is required")
	}
	if r.VRF == "" {
		return errors.New("rib vrf is required")
	}
	for i, route := range r.Routes {
		if err := route.Validate(); err != nil {
			return fmt.Errorf("rib route %d: %w", i, err)
		}
	}
	return nil
}

func (r RIB) Key() string {
	return string(r.Node) + "|" + string(r.VRF)
}

func (r RIBRoute) Key() string {
	return strings.Join([]string{
		string(NormalizeAddressFamily(r.Common.AFI)),
		string(NormalizeRouteProtocol(r.Common.Protocol)),
		r.Common.Prefix,
	}, "|")
}

func (r RIBRoute) payloadCount() int {
	count := 0
	if r.BGP != nil {
		count++
	}
	if r.OSPF != nil {
		count++
	}
	if r.Static != nil {
		count++
	}
	if r.Connected != nil {
		count++
	}
	if r.Blackhole != nil {
		count++
	}
	return count
}

func (r RIBRoute) payloadProtocol() RouteProtocol {
	switch {
	case r.BGP != nil:
		return ProtocolBGP
	case r.OSPF != nil:
		return ProtocolOSPF
	case r.Static != nil:
		return ProtocolStatic
	case r.Connected != nil:
		return ProtocolConnected
	case r.Blackhole != nil:
		return ProtocolBlackhole
	default:
		return ProtocolUnknown
	}
}

func SortRIBRoutes(routes []RIBRoute) {
	sort.SliceStable(routes, func(i, j int) bool {
		return routes[i].Key() < routes[j].Key()
	})
}

func FilterRIBRoutes(routes []RIBRoute, pred func(RIBRoute) bool) []RIBRoute {
	out := make([]RIBRoute, 0, len(routes))
	for _, route := range routes {
		if pred(route) {
			out = append(out, route)
		}
	}
	return out
}

func RIBFromNormalizedRoutes(node NodeID, vrf VRFName, routes []normalizedrib.NormalizedRoute) RIB {
	out := RIB{Node: node, VRF: vrf}
	for _, route := range routes {
		route = normalizedrib.NormalizeRoute(route)
		if node == "" {
			out.Node = NodeID(route.Node)
		}
		if vrf == "" {
			out.VRF = VRFName(route.NetworkInstance)
		}
		out.Routes = append(out.Routes, RIBRouteFromNormalizedRoute(route))
	}
	SortRIBRoutes(out.Routes)
	return out
}

func RIBsFromNormalizedRoutes(routes []normalizedrib.NormalizedRoute) []RIB {
	byKey := map[string]*RIB{}
	for _, route := range routes {
		route = normalizedrib.NormalizeRoute(route)
		node := NodeID(route.Node)
		vrf := VRFName(route.NetworkInstance)
		key := string(node) + "|" + string(vrf)
		if byKey[key] == nil {
			byKey[key] = &RIB{Node: node, VRF: vrf}
		}
		byKey[key].Routes = append(byKey[key].Routes, RIBRouteFromNormalizedRoute(route))
	}
	out := make([]RIB, 0, len(byKey))
	for _, rib := range byKey {
		SortRIBRoutes(rib.Routes)
		out = append(out, *rib)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Key() < out[j].Key()
	})
	return out
}

func RIBRouteFromNormalizedRoute(route normalizedrib.NormalizedRoute) RIBRoute {
	route = normalizedrib.NormalizeRoute(route)
	protocol := NormalizeRouteProtocol(RouteProtocol(route.Protocol))
	common := RIBRouteCommon{
		AFI:      NormalizeAddressFamily(AddressFamily(route.AFI)),
		Prefix:   route.Prefix,
		Protocol: protocol,
		Eligible: routeHasEligiblePath(route.Paths),
		Best:     routeHasBestPath(route.Paths),
	}
	out := RIBRoute{Common: common}
	switch protocol {
	case ProtocolBGP:
		out.BGP = &BGPRIBRoute{Paths: bgpPathsFromNormalized(route.Paths)}
	case ProtocolOSPF:
		out.OSPF = &OSPFRIBRoute{RouteType: OSPFRouteTypeUnknown, Paths: ospfPathsFromNormalized(route.Paths)}
	case ProtocolStatic:
		out.Static = &StaticRIBRoute{NextHops: nextHopsFromNormalizedRIBPaths(route.Paths)}
	case ProtocolConnected:
		out.Connected = &ConnectedRIBRoute{}
	case ProtocolBlackhole:
		out.Blackhole = &BlackholeRIBRoute{}
	default:
		out.Common.Protocol = ProtocolUnknown
	}
	return out
}

func bgpPathsFromNormalized(paths []normalizedrib.NormalizedPath) []BGPPath {
	out := make([]BGPPath, 0, len(paths))
	for _, path := range paths {
		out = append(out, BGPPath{
			NextHop:          NextHop{Address: path.NextHop, Weight: path.Weight},
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
			Eligible:         path.Valid,
			Best:             path.Best,
		})
	}
	return out
}

func ospfPathsFromNormalized(paths []normalizedrib.NormalizedPath) []OSPFPath {
	out := make([]OSPFPath, 0, len(paths))
	for _, path := range paths {
		out = append(out, OSPFPath{NextHop: NextHop{Address: path.NextHop}, Cost: path.MED})
	}
	return out
}

func nextHopsFromNormalizedRIBPaths(paths []normalizedrib.NormalizedPath) []NextHop {
	out := make([]NextHop, 0, len(paths))
	for _, path := range paths {
		if path.NextHop == "" {
			continue
		}
		out = append(out, NextHop{Address: path.NextHop, Weight: path.Weight})
	}
	return out
}

func routeHasEligiblePath(paths []normalizedrib.NormalizedPath) bool {
	for _, path := range paths {
		if path.Valid {
			return true
		}
	}
	return len(paths) == 0
}

func routeHasBestPath(paths []normalizedrib.NormalizedPath) bool {
	for _, path := range paths {
		if path.Best {
			return true
		}
	}
	return false
}
