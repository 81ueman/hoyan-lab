package rib

import (
	"context"
	"fmt"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func (c frrCollector) CollectRouteTableRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	var out []NormalizedRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		data, err := c.runner.Run(ctx, "docker", "exec", "-i", containerName, "vtysh", "-c", "show ip route vrf all json")
		if err != nil {
			return nil, fmt.Errorf("docker exec -i %s vtysh -c %q: %w", containerName, "show ip route vrf all json", err)
		}
		ospfData, ospfErr := c.runner.Run(ctx, "docker", "exec", "-i", containerName, "vtysh", "-c", "show ip ospf route json")
		if ospfErr != nil && strings.Contains(string(ospfData), "ospfd is not running") {
			ospfData = nil
		} else if ospfErr != nil {
			return nil, fmt.Errorf("docker exec -i %s vtysh -c %q: %w", containerName, "show ip ospf route json", ospfErr)
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

func (c ceosCollector) CollectRouteTableRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	var out []NormalizedRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		data, err := c.runner.Run(ctx, "docker", "exec", "-i", containerName, "Cli", "-p", "15", "-c", "show ip route vrf all | json")
		if err != nil {
			return nil, fmt.Errorf("docker exec -i %s Cli -p 15 -c %q: %w", containerName, "show ip route vrf all | json", err)
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

func (c srlinuxCollector) CollectRouteTableRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	var out []NormalizedRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		for _, ni := range model.NetworkInstancesForNode(n) {
			data, err := RunSRLinuxJSON(ctx, c.runner, containerName, "show", "network-instance", ni, "route-table", "ipv4-unicast", "summary")
			if err != nil {
				return nil, fmt.Errorf("%s sr_cli network-instance %s route-table ipv4-unicast summary: %w", containerName, ni, err)
			}
			routes, err := ParseSRLinuxRouteTableNetworkInstance(n.Name, ni, data)
			if err != nil {
				return nil, fmt.Errorf("%s SR Linux route table network-instance %s: %w", n.Name, ni, err)
			}
			normalizeSRLinuxStaticRouteNextHops(n, routes)
			out = append(out, routes...)
		}
	}
	SortRoutes(out)
	return out, nil
}

func (c frrCollector) CollectOSPFRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	var out []NormalizedRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		data, err := c.runner.Run(ctx, "docker", "exec", "-i", containerName, "vtysh", "-c", "show ip ospf route json")
		if err != nil {
			if strings.Contains(string(data), "ospfd is not running") {
				continue
			}
			return nil, fmt.Errorf("docker exec -i %s vtysh -c %q: %w", containerName, "show ip ospf route json", err)
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

func (c ceosCollector) CollectOSPFRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	routes, err := c.CollectRouteTableRoutes(ctx, nodes)
	if err != nil {
		return nil, err
	}
	return ospfRoutes(routes), nil
}

func (c srlinuxCollector) CollectOSPFRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	routes, err := c.CollectRouteTableRoutes(ctx, nodes)
	if err != nil {
		return nil, err
	}
	return ospfRoutes(routes), nil
}

func ospfRoutes(routes []NormalizedRoute) []NormalizedRoute {
	out := make([]NormalizedRoute, 0, len(routes))
	for _, route := range routes {
		switch route.Protocol {
		case "ospf", "ospf-ia":
			out = append(out, route)
		}
	}
	SortRoutes(out)
	return out
}

func normalizeSRLinuxStaticRouteNextHops(node model.Node, routes []NormalizedRoute) {
	configured := map[string]string{}
	for _, route := range node.Routes {
		if route.Kind != model.RouteSourceStatic || route.NextHop == "" {
			continue
		}
		vrf := string(model.NormalizeNetworkInstance(string(route.NetworkInstance)))
		configured[vrf+"|"+route.Prefix.String()] = route.NextHop
	}
	for ri := range routes {
		route := normalizeRoute(routes[ri])
		if route.Protocol != "static" {
			continue
		}
		nh := configured[routes[ri].NetworkInstance+"|"+routes[ri].Prefix]
		if nh == "" {
			continue
		}
		for pi := range routes[ri].Paths {
			routes[ri].Paths[pi].NextHop = nh
		}
	}
}
