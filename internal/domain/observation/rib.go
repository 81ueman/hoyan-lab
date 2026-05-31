package observation

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type RIB struct {
	Node   model.NodeID            `json:"node"`
	VRF    model.NetworkInstanceID `json:"vrf"`
	Routes []RIBRoute              `json:"routes"`
}

type RIBRoute struct {
	Node            string                    `json:"node,omitempty"`
	NetworkInstance string                    `json:"network_instance,omitempty"`
	AFI             string                    `json:"afi,omitempty"`
	Prefix          string                    `json:"prefix,omitempty"`
	Protocol        string                    `json:"protocol,omitempty"`
	ConnectedClass  model.ConnectedRouteClass `json:"connected_class,omitempty"`
	Paths           []RIBPath                 `json:"paths,omitempty"`

	Common RIBRouteCommon `json:"common"`

	BGP       *BGPRIBRoute       `json:"bgp,omitempty"`
	OSPF      *OSPFRIBRoute      `json:"ospf,omitempty"`
	Static    *StaticRIBRoute    `json:"static,omitempty"`
	Connected *ConnectedRIBRoute `json:"connected,omitempty"`
	Blackhole *BlackholeRIBRoute `json:"blackhole,omitempty"`

	ModelInfo *ModelRouteInfo `json:"model_info,omitempty"`
}

type RIBRouteCommon struct {
	AFI      model.AFI             `json:"afi"`
	Prefix   string                `json:"prefix"`
	Protocol model.RouteSourceKind `json:"protocol"`

	Preference int `json:"preference,omitempty"`
	Metric     int `json:"metric,omitempty"`

	Eligible bool `json:"eligible"`
	Best     bool `json:"best"`
}

type BGPRIBRoute struct {
	Paths []BGPPath `json:"paths"`
}

type BGPPath struct {
	NextHop          NextHop             `json:"next_hop,omitempty"`
	ASPath           []uint32            `json:"as_path,omitempty"`
	Origin           model.BGPOriginCode `json:"origin,omitempty"`
	LocalPref        int                 `json:"local_pref,omitempty"`
	MED              int                 `json:"med,omitempty"`
	Weight           int                 `json:"weight,omitempty"`
	Communities      []string            `json:"communities,omitempty"`
	LargeCommunities []string            `json:"large_communities,omitempty"`
	OriginatorID     string              `json:"originator_id,omitempty"`
	ClusterList      []string            `json:"cluster_list,omitempty"`
	Peer             string              `json:"peer,omitempty"`
	PeerAS           uint32              `json:"peer_as,omitempty"`
	Eligible         bool                `json:"eligible"`
	Best             bool                `json:"best"`
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
	if model.NormalizeRouteSourceKind(r.Common.Protocol) != r.Common.Protocol {
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
		string(model.NormalizeAFI(r.Common.AFI)),
		string(model.NormalizeRouteSourceKind(r.Common.Protocol)),
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

func (r RIBRoute) payloadProtocol() model.RouteSourceKind {
	switch {
	case r.BGP != nil:
		return model.RouteSourceBGP
	case r.OSPF != nil:
		return model.RouteSourceOSPF
	case r.Static != nil:
		return model.RouteSourceStatic
	case r.Connected != nil:
		return model.RouteSourceConnected
	case r.Blackhole != nil:
		return model.RouteSourceBlackhole
	default:
		return model.RouteSourceUnknown
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

func RIBFromRouteRecords(node model.NodeID, vrf model.NetworkInstanceID, routes []RIBRoute) RIB {
	requestedVRF := vrf
	out := RIB{Node: node, VRF: model.NormalizeNetworkInstance(string(vrf))}
	for _, route := range routes {
		route = NormalizeRIBRouteRecord(route)
		if node == "" {
			out.Node = model.NodeID(route.Node)
		}
		if requestedVRF == "" {
			out.VRF = model.NormalizeNetworkInstance(route.NetworkInstance)
		}
		out.Routes = append(out.Routes, RIBRouteFromRouteRecord(route))
	}
	SortRIBRoutes(out.Routes)
	return out
}

func RIBsFromRouteRecords(routes []RIBRoute) []RIB {
	byKey := map[string]*RIB{}
	for _, route := range routes {
		route = NormalizeRIBRouteRecord(route)
		node := model.NodeID(route.Node)
		vrf := model.NormalizeNetworkInstance(route.NetworkInstance)
		key := string(node) + "|" + string(vrf)
		if byKey[key] == nil {
			byKey[key] = &RIB{Node: node, VRF: vrf}
		}
		byKey[key].Routes = append(byKey[key].Routes, RIBRouteFromRouteRecord(route))
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

func RIBRouteFromRouteRecord(route RIBRoute) RIBRoute {
	route = NormalizeRIBRouteRecord(route)
	protocol := model.NormalizeRouteSourceKind(model.RouteSourceKind(route.Protocol))
	common := RIBRouteCommon{
		AFI:      model.NormalizeAFI(model.AFI(route.AFI)),
		Prefix:   route.Prefix,
		Protocol: protocol,
		Eligible: routeHasEligiblePath(route.Paths),
		Best:     routeHasBestPath(route.Paths),
	}
	out := RIBRoute{Common: common}
	switch protocol {
	case model.RouteSourceBGP:
		out.BGP = &BGPRIBRoute{Paths: bgpPathsFromRouteRecord(route.Paths)}
	case model.RouteSourceOSPF:
		out.OSPF = &OSPFRIBRoute{RouteType: OSPFRouteTypeUnknown, Paths: ospfPathsFromRouteRecord(route.Paths)}
	case model.RouteSourceStatic:
		out.Static = &StaticRIBRoute{NextHops: nextHopsFromRouteRecordRIBPaths(route.Paths)}
	case model.RouteSourceConnected:
		out.Connected = &ConnectedRIBRoute{}
	case model.RouteSourceBlackhole:
		out.Blackhole = &BlackholeRIBRoute{}
	default:
		out.Common.Protocol = model.RouteSourceUnknown
	}
	return out
}

func bgpPathsFromRouteRecord(paths []RIBPath) []BGPPath {
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

func ospfPathsFromRouteRecord(paths []RIBPath) []OSPFPath {
	out := make([]OSPFPath, 0, len(paths))
	for _, path := range paths {
		out = append(out, OSPFPath{NextHop: NextHop{Address: path.NextHop}, Cost: path.MED})
	}
	return out
}

func nextHopsFromRouteRecordRIBPaths(paths []RIBPath) []NextHop {
	out := make([]NextHop, 0, len(paths))
	for _, path := range paths {
		if path.NextHop == "" {
			continue
		}
		out = append(out, NextHop{Address: path.NextHop, Weight: path.Weight})
	}
	return out
}

func routeHasEligiblePath(paths []RIBPath) bool {
	for _, path := range paths {
		if path.Valid {
			return true
		}
	}
	return len(paths) == 0
}

func routeHasBestPath(paths []RIBPath) bool {
	for _, path := range paths {
		if path.Best {
			return true
		}
	}
	return false
}
