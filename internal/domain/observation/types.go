package observation

import (
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type AddressFamily string
type RouteProtocol string

const (
	AFIIPv4 AddressFamily = "ipv4"
	AFIIPv6 AddressFamily = "ipv6"
)

const (
	ProtocolBGP       RouteProtocol = "bgp"
	ProtocolOSPF      RouteProtocol = "ospf"
	ProtocolStatic    RouteProtocol = "static"
	ProtocolConnected RouteProtocol = "connected"
	ProtocolBlackhole RouteProtocol = "blackhole"
	ProtocolAggregate RouteProtocol = "aggregate"
	ProtocolUnknown   RouteProtocol = "unknown"
)

type RouteSource struct {
	Protocol RouteProtocol `json:"protocol"`
	Origin   string        `json:"origin,omitempty"`
}

type NextHop struct {
	Address   string       `json:"address,omitempty"`
	Interface string       `json:"interface,omitempty"`
	Node      model.NodeID `json:"node,omitempty"`
	Weight    int          `json:"weight,omitempty"`

	Resolution *NextHopResolution `json:"resolution,omitempty"`
}

type NextHopResolution struct {
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}

type ModelRouteInfo struct {
	Conditions ModelConditions `json:"conditions,omitempty"`
	Provenance RouteProvenance `json:"provenance,omitempty"`
	Decision   *DecisionInfo   `json:"decision,omitempty"`
	Resolution *ResolutionInfo `json:"resolution,omitempty"`
}

type ModelConditions struct {
	Base      string `json:"base,omitempty"`
	Eligible  string `json:"eligible,omitempty"`
	Selected  string `json:"selected,omitempty"`
	Installed string `json:"installed,omitempty"`
}

type RouteProvenance struct {
	OriginNode model.NodeID   `json:"origin_node,omitempty"`
	FromNode   model.NodeID   `json:"from_node,omitempty"`
	PathNodes  []model.NodeID `json:"path_nodes,omitempty"`
	PathLinks  []string       `json:"path_links,omitempty"`
	Source     string         `json:"source,omitempty"`
	Inputs     []string       `json:"inputs,omitempty"`
}

type DecisionInfo struct {
	Rank       int    `json:"rank,omitempty"`
	GroupID    string `json:"group_id,omitempty"`
	Equivalent bool   `json:"equivalent,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type ResolutionInfo struct {
	Status string `json:"status,omitempty"`
	Reason string `json:"reason,omitempty"`
}

func NormalizeAddressFamily(afi AddressFamily) AddressFamily {
	switch AddressFamily(strings.ToLower(strings.TrimSpace(string(afi)))) {
	case "", AFIIPv4:
		return AFIIPv4
	case AFIIPv6:
		return AFIIPv6
	default:
		return AddressFamily(strings.ToLower(strings.TrimSpace(string(afi))))
	}
}

func NormalizeRouteProtocol(protocol RouteProtocol) RouteProtocol {
	switch RouteProtocol(strings.ToLower(strings.TrimSpace(string(protocol)))) {
	case ProtocolBGP:
		return ProtocolBGP
	case ProtocolOSPF:
		return ProtocolOSPF
	case "ospf-ia", "ospf-ia-routes", "ospf-inter-area":
		return ProtocolOSPF
	case ProtocolStatic:
		return ProtocolStatic
	case ProtocolConnected:
		return ProtocolConnected
	case ProtocolBlackhole:
		return ProtocolBlackhole
	case ProtocolAggregate:
		return ProtocolAggregate
	case "", ProtocolUnknown:
		return ProtocolUnknown
	default:
		return RouteProtocol(strings.ToLower(strings.TrimSpace(string(protocol))))
	}
}
