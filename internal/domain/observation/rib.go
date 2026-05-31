package observation

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type RIB struct {
	Node   NodeID     `json:"node"`
	VRF    VRFName    `json:"vrf"`
	Routes []RIBRoute `json:"routes"`
}

type RIBCollector interface {
	CollectBGPRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error)
	CollectOSPFRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error)
	CollectRouteTableRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error)
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
