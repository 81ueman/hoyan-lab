package rib

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func bgpRoute(prefix string, paths []observation.BGPPath) RIBRoute {
	observation.SortBGPPaths(paths, observation.DefaultCompareOptions())
	return RIBRoute{
		Common: observation.RIBRouteCommon{
			AFI:      model.AFIFromPrefix(prefix),
			Prefix:   prefix,
			Protocol: model.RouteSourceBGP,
			Eligible: bgpHasEligiblePath(paths),
			Best:     bgpHasBestPath(paths),
		},
		BGP: &observation.BGPRIBRoute{Paths: paths},
	}
}

func nonBGPRoute(prefix, protocol string, hops []routeTableNextHop) RIBRoute {
	routeProtocol := model.NormalizeRouteSourceKind(model.RouteSourceKind(protocol))
	common := observation.RIBRouteCommon{
		AFI:      model.AFIFromPrefix(prefix),
		Prefix:   prefix,
		Protocol: routeProtocol,
		Eligible: true,
		Best:     true,
	}
	route := RIBRoute{Common: common}
	switch routeProtocol {
	case model.RouteSourceOSPF:
		paths := ospfPathsFromRouteTableHops(hops)
		observation.SortOSPFPaths(paths, observation.DefaultCompareOptions())
		route.OSPF = &observation.OSPFRIBRoute{RouteType: ospfRouteType(protocol), Paths: paths}
	case model.RouteSourceStatic:
		route.Static = &observation.StaticRIBRoute{NextHops: nextHopsFromRouteTableHops(hops)}
	case model.RouteSourceConnected:
		route.Connected = &observation.ConnectedRIBRoute{}
	case model.RouteSourceBlackhole:
		route.Blackhole = &observation.BlackholeRIBRoute{}
	default:
		route.Common.Protocol = model.RouteSourceUnknown
	}
	return route
}

func bgpHasEligiblePath(paths []observation.BGPPath) bool {
	for _, path := range paths {
		if path.Eligible {
			return true
		}
	}
	return len(paths) == 0
}

func bgpHasBestPath(paths []observation.BGPPath) bool {
	for _, path := range paths {
		if path.Best {
			return true
		}
	}
	return false
}

func ospfPathsFromRouteTableHops(hops []routeTableNextHop) []observation.OSPFPath {
	if len(hops) == 0 {
		return []observation.OSPFPath{{}}
	}
	out := make([]observation.OSPFPath, 0, len(hops))
	for _, hop := range hops {
		out = append(out, observation.OSPFPath{NextHop: nextHopFromRouteTableHop(hop)})
	}
	return out
}

func nextHopsFromRouteTableHops(hops []routeTableNextHop) []observation.NextHop {
	out := make([]observation.NextHop, 0, len(hops))
	for _, hop := range hops {
		nh := nextHopFromRouteTableHop(hop)
		if nh.Address == "" && nh.Interface == "" {
			continue
		}
		out = append(out, nh)
	}
	return out
}

func nextHopFromRouteTableHop(hop routeTableNextHop) observation.NextHop {
	if hop.Address == "0.0.0.0" || hop.Address == "::" {
		hop.Address = ""
	}
	return observation.NextHop{Address: hop.Address, Interface: hop.Interface}
}

func ospfRouteType(protocol string) observation.OSPFRouteType {
	switch protocol {
	case "ospf-ia":
		return observation.OSPFRouteTypeInterArea
	case "ospf":
		return observation.OSPFRouteTypeIntraArea
	default:
		return observation.OSPFRouteTypeUnknown
	}
}
