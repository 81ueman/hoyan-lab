package configparse

import (
	"fmt"
	"net/netip"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func parseFRRLikeStaticRoute(kind model.DeviceKind, path string, lineNo int, raw string, fields []string) (model.ConfiguredRoute, error) {
	vrf := model.NetworkInstanceDefault
	if len(fields) >= 5 && fields[2] == "vrf" {
		vrf = model.NormalizeNetworkInstance(fields[3])
		fields = append([]string{fields[0], fields[1]}, fields[4:]...)
	}
	if len(fields) == 6 && fields[4] == "vrf" {
		vrf = model.NormalizeNetworkInstance(fields[5])
		fields = fields[:4]
	}
	if len(fields) != 4 {
		return model.ConfiguredRoute{}, fmt.Errorf("unsupported %s static route statement", routeMapVendorName(kind))
	}
	prefix, err := model.ParsePrefix(fields[2])
	if err != nil {
		return model.ConfiguredRoute{}, err
	}
	route := model.ConfiguredRoute{
		NetworkInstance: vrf,
		AFI:             model.AFIIPv4,
		Prefix:          prefix,
		Kind:            model.RouteSourceStatic,
		AdminDistance:   model.AdminDistanceStatic,
		Source:          model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: raw},
	}
	target := fields[3]
	if strings.EqualFold(target, "Null0") {
		route.Kind = model.RouteSourceBlackhole
		route.Interface = target
		return route, nil
	}
	if _, err := netip.ParseAddr(target); err == nil {
		route.NextHop = target
		return route, nil
	}
	route.Interface = target
	return route, nil
}

func parseAggregateRoute(kind model.DeviceKind, path string, lineNo int, raw string, fields []string) (model.ConfiguredRoute, error) {
	if len(fields) < 2 {
		return model.ConfiguredRoute{}, fmt.Errorf("unsupported %s aggregate-address statement", routeMapVendorName(kind))
	}
	prefixText := fields[1]
	prefix, err := model.ParsePrefix(prefixText)
	if err != nil {
		return model.ConfiguredRoute{}, err
	}
	route := model.ConfiguredRoute{
		NetworkInstance: model.NetworkInstanceDefault,
		AFI:             model.AFIIPv4,
		Prefix:          prefix,
		Kind:            model.RouteSourceAggregate,
		AdminDistance:   model.AdminDistanceAggregate,
		Source:          model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: raw},
	}
	for _, opt := range fields[2:] {
		switch opt {
		case "summary-only":
			route.SummaryOnly = true
		default:
			return model.ConfiguredRoute{}, fmt.Errorf("unsupported %s aggregate-address option %q", routeMapVendorName(kind), opt)
		}
	}
	return route, nil
}

func parseFRRLikeRedistribution(kind model.DeviceKind, path string, lineNo int, raw string, fields []string) (model.BGPRedistribution, error) {
	redist := model.BGPRedistribution{Source: model.ConfigSource{Vendor: string(kind), File: path, Line: lineNo, Raw: raw}}
	switch fields[1] {
	case "connected":
		redist.Kind = model.RouteSourceConnected
	case "static":
		redist.Kind = model.RouteSourceStatic
	default:
		return model.BGPRedistribution{}, fmt.Errorf("unsupported %s redistribute source %q", routeMapVendorName(kind), fields[1])
	}
	if len(fields) == 2 {
		return redist, nil
	}
	if len(fields) == 4 && fields[2] == "route-map" {
		redist.RouteMap = fields[3]
		return redist, nil
	}
	return model.BGPRedistribution{}, fmt.Errorf("unsupported %s redistribute statement", routeMapVendorName(kind))
}

func parseSRLinuxStaticRoute(path string, lineNo int, raw string, fields []string, nextHopGroups map[string]string) (model.ConfiguredRoute, error) {
	prefixText := fieldAfter(fields, "route")
	if prefixText == "" {
		return model.ConfiguredRoute{}, fmt.Errorf("unsupported SR Linux static route statement")
	}
	prefix, err := model.ParsePrefix(prefixText)
	if err != nil {
		return model.ConfiguredRoute{}, err
	}
	route := model.ConfiguredRoute{
		NetworkInstance: model.NormalizeNetworkInstance(fieldAfter(fields, "network-instance")),
		AFI:             model.AFIIPv4,
		Prefix:          prefix,
		Kind:            model.RouteSourceStatic,
		AdminDistance:   model.AdminDistanceSRLinuxStatic,
		Source:          model.ConfigSource{Vendor: "srlinux", File: path, Line: lineNo, Raw: raw},
	}
	if nh := fieldAfter(fields, "next-hop"); nh != "" {
		if _, err := netip.ParseAddr(nh); err == nil {
			route.NextHop = nh
			return route, nil
		}
	}
	if group := fieldAfter(fields, "next-hop-group"); group != "" {
		nh := nextHopGroups[srlinuxNextHopGroupKey(string(route.NetworkInstance), group)]
		if _, err := netip.ParseAddr(nh); err == nil {
			route.NextHop = nh
			return route, nil
		}
		return model.ConfiguredRoute{}, fmt.Errorf("unsupported SR Linux static route next-hop-group")
	}
	if iface := fieldAfter(fields, "interface"); iface != "" {
		route.Interface = iface
		return route, nil
	}
	if containsAnyField(fields, "blackhole") || containsAnyField(fields, "discard") {
		route.Kind = model.RouteSourceBlackhole
		return route, nil
	}
	return model.ConfiguredRoute{}, fmt.Errorf("unsupported SR Linux static route next-hop")
}

func srlinuxNextHopGroupKey(networkInstance, group string) string {
	return string(model.NormalizeNetworkInstance(networkInstance)) + "\x00" + group
}

func srlinuxConfigInterfaceName(iface string) string {
	if base, ok := strings.CutSuffix(iface, ".0"); ok && (strings.HasPrefix(base, "ethernet-1/") || strings.HasPrefix(base, "lo")) {
		return base
	}
	return iface
}
