package model

import "strings"

const DefaultLocalPreference = 100

type RouteSourceKind string

const (
	RouteSourceConnected RouteSourceKind = "connected"
	RouteSourceStatic    RouteSourceKind = "static"
	RouteSourceBGP       RouteSourceKind = "bgp"
	RouteSourceOSPF      RouteSourceKind = "ospf"
	RouteSourceAggregate RouteSourceKind = "aggregate"
	RouteSourceBlackhole RouteSourceKind = "blackhole"
	RouteSourceUnknown   RouteSourceKind = "unknown"
)

func NormalizeRouteSourceKind(kind RouteSourceKind) RouteSourceKind {
	normalized := strings.ToLower(strings.TrimSpace(string(kind)))
	if strings.Contains(normalized, "ospf") {
		return RouteSourceOSPF
	}
	switch normalized {
	case "ebgp", "ibgp", string(RouteSourceBGP):
		return RouteSourceBGP
	case string(RouteSourceOSPF), "188":
		return RouteSourceOSPF
	case "kernel", string(RouteSourceConnected), "direct", "local", "host":
		return RouteSourceConnected
	case string(RouteSourceStatic), "196":
		return RouteSourceStatic
	case string(RouteSourceBlackhole), "discard", "drop", "null0", "null":
		return RouteSourceBlackhole
	case string(RouteSourceAggregate):
		return RouteSourceAggregate
	case "", string(RouteSourceUnknown):
		return RouteSourceUnknown
	default:
		return RouteSourceKind(normalized)
	}
}

type ConnectedRouteClass string

const (
	ConnectedRouteClassLink     ConnectedRouteClass = "link"
	ConnectedRouteClassLoopback ConnectedRouteClass = "loopback"
	ConnectedRouteClassService  ConnectedRouteClass = "service"
	ConnectedRouteClassHost     ConnectedRouteClass = "host"
)

type ConfiguredRoute struct {
	Node            string              `yaml:"node,omitempty" json:"node,omitempty"`
	NetworkInstance NetworkInstanceID   `yaml:"network_instance,omitempty" json:"network_instance,omitempty"`
	AFI             AFI                 `yaml:"afi,omitempty" json:"afi,omitempty"`
	Prefix          Prefix              `yaml:"prefix" json:"prefix"`
	NextHop         string              `yaml:"next_hop,omitempty" json:"next_hop,omitempty"`
	Interface       string              `yaml:"interface,omitempty" json:"interface,omitempty"`
	Kind            RouteSourceKind     `yaml:"kind" json:"kind"`
	ConnectedClass  ConnectedRouteClass `yaml:"connected_class,omitempty" json:"connected_class,omitempty"`
	AdminDistance   int                 `yaml:"admin_distance,omitempty" json:"admin_distance,omitempty"`
	Metric          int                 `yaml:"metric,omitempty" json:"metric,omitempty"`
	MetricType      int                 `yaml:"metric_type,omitempty" json:"metric_type,omitempty"`
	OSPFRouteType   string              `yaml:"ospf_route_type,omitempty" json:"ospf_route_type,omitempty"`
	SummaryOnly     bool                `yaml:"summary_only,omitempty" json:"summary_only,omitempty"`
	Source          ConfigSource        `yaml:"source,omitempty" json:"source,omitempty"`
}

type BGPRedistribution struct {
	NetworkInstance NetworkInstanceID `yaml:"network_instance,omitempty" json:"network_instance,omitempty"`
	Kind            RouteSourceKind   `yaml:"kind" json:"kind"`
	RouteMap        string            `yaml:"route_map,omitempty" json:"route_map,omitempty"`
	Source          ConfigSource      `yaml:"source,omitempty" json:"source,omitempty"`
}
