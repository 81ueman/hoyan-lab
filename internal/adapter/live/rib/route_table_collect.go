package rib

import (
	"context"
	"fmt"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func (c frrCollector) collectRouteTableRoutes(ctx context.Context, nodes []model.Node, afi model.AFI) ([]RIBRoute, error) {
	var out []RIBRoute
	routeCmd := "show ip route vrf all json"
	if afi == model.AFIIPv6 {
		routeCmd = "show ipv6 route vrf all json"
	}
	for _, n := range nodes {
		session := c.SessionForNode(n)
		data, err := session.Exec(ctx, "vtysh", "-c", routeCmd)
		if err != nil {
			return nil, fmt.Errorf("node %s vtysh -c %q: %w", n.RuntimeName(), routeCmd, err)
		}
		var ospfData []byte
		if afi != model.AFIIPv6 {
			var ospfErr error
			ospfData, ospfErr = session.Exec(ctx, "vtysh", "-c", "show ip ospf route json")
			if ospfErr != nil && strings.Contains(string(ospfData), "ospfd is not running") {
				ospfData = nil
			} else if ospfErr != nil {
				return nil, fmt.Errorf("node %s vtysh -c %q: %w", n.RuntimeName(), "show ip ospf route json", ospfErr)
			}
		}
		routes, err := ParseFRRRouteTableWithOSPF(n.Name, data, ospfData)
		if err != nil {
			return nil, fmt.Errorf("%s FRR route table: %w", n.Name, err)
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
}

func (c ceosCollector) collectRouteTableRoutes(ctx context.Context, nodes []model.Node, afi model.AFI) ([]RIBRoute, error) {
	var out []RIBRoute
	routeCmd := "show ip route vrf all | json"
	if afi == model.AFIIPv6 {
		routeCmd = "show ipv6 route vrf all | json"
	}
	for _, n := range nodes {
		session := c.SessionForNode(n)
		data, err := session.Exec(ctx, "Cli", "-p", "15", "-c", routeCmd)
		if err != nil {
			return nil, fmt.Errorf("node %s Cli -p 15 -c %q: %w", n.RuntimeName(), routeCmd, err)
		}
		routes, err := ParseCEOSRouteTable(n.Name, data)
		if err != nil {
			return nil, fmt.Errorf("%s cEOS route table: %w", n.Name, err)
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
}

func (c srlinuxCollector) collectRouteTableRoutes(ctx context.Context, nodes []model.Node, afi model.AFI) ([]RIBRoute, error) {
	var out []RIBRoute
	afiStr := "ipv4-unicast"
	if afi == model.AFIIPv6 {
		afiStr = "ipv6-unicast"
	}
	for _, n := range nodes {
		session := c.SessionForNode(n)
		for _, ni := range model.NetworkInstancesForNode(n) {
			data, err := RunSRLinuxJSON(ctx, session, "show", "network-instance", ni, "route-table", afiStr, "summary")
			if err != nil {
				return nil, fmt.Errorf("%s sr_cli network-instance %s route-table %s summary: %w", n.RuntimeName(), ni, afiStr, err)
			}
			routes, err := ParseSRLinuxRouteTableNetworkInstance(n.Name, ni, data)
			if err != nil {
				return nil, fmt.Errorf("%s SR Linux route table network-instance %s: %w", n.Name, ni, err)
			}
			normalizeSRLinuxStaticRouteNextHops(n, ni, routes)
			out = append(out, routes...)
		}
	}
	SortRoutes(out)
	return out, nil
}

func (c frrCollector) collectOSPFRoutes(ctx context.Context, nodes []model.Node, afi model.AFI) ([]RIBRoute, error) {
	var out []RIBRoute
	for _, n := range nodes {
		if afi == model.AFIIPv6 {
			return nil, fmt.Errorf("%s: OSPFv3 (IPv6) not supported; FRR OSPF collection only supports IPv4 (use the general IPv6 route-table collection for OSPFv3 routes)", n.Name)
		}
		session := c.SessionForNode(n)
		data, err := session.Exec(ctx, "vtysh", "-c", "show ip ospf route json")
		if err != nil {
			if strings.Contains(string(data), "ospfd is not running") {
				continue
			}
			return nil, fmt.Errorf("node %s vtysh -c %q: %w", n.RuntimeName(), "show ip ospf route json", err)
		}
		routes, err := ParseFRROSPFRouteTable(n.Name, data)
		if err != nil {
			return nil, fmt.Errorf("%s FRR OSPF routes: %w", n.Name, err)
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
}

func (c ceosCollector) collectOSPFRoutes(ctx context.Context, nodes []model.Node, afi model.AFI) ([]RIBRoute, error) {
	routes, err := c.collectRouteTableRoutes(ctx, nodes, afi)
	if err != nil {
		return nil, err
	}
	return ospfRoutes(routes), nil
}

func (c srlinuxCollector) collectOSPFRoutes(ctx context.Context, nodes []model.Node, afi model.AFI) ([]RIBRoute, error) {
	routes, err := c.collectRouteTableRoutes(ctx, nodes, afi)
	if err != nil {
		return nil, err
	}
	return ospfRoutes(routes), nil
}

func ospfRoutes(routes []RIBRoute) []RIBRoute {
	out := make([]RIBRoute, 0, len(routes))
	for _, route := range routes {
		switch route.Common.Protocol {
		case model.RouteSourceOSPF:
			out = append(out, route)
		}
	}
	SortRoutes(out)
	return out
}

func normalizeSRLinuxStaticRouteNextHops(node model.Node, networkInstance string, routes []RIBRoute) {
	configured := map[string]string{}
	for _, route := range node.Routes {
		if route.Kind != model.RouteSourceStatic || route.NextHop == "" {
			continue
		}
		vrf := string(model.NormalizeNetworkInstance(string(route.NetworkInstance)))
		configured[vrf+"|"+route.Prefix.String()] = route.NextHop
	}
	for ri := range routes {
		if routes[ri].Common.Protocol != model.RouteSourceStatic || routes[ri].Static == nil {
			continue
		}
		nh := configured[networkInstance+"|"+routes[ri].Common.Prefix]
		if nh == "" {
			continue
		}
		if len(routes[ri].Static.NextHops) == 0 {
			routes[ri].Static.NextHops = []observation.NextHop{{Address: nh}}
			continue
		}
		for pi := range routes[ri].Static.NextHops {
			routes[ri].Static.NextHops[pi].Address = nh
		}
	}
}
