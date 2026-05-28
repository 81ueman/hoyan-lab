package ribcompare

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/core/topology"
	"github.com/81ueman/hoyan-lab/internal/srlinuxjson"
)

func Collect(ctx context.Context, runner Runner, nodes []topology.Node) ([]NormalizedBgpRoute, error) {
	out, err := collectBGP(ctx, runner, nodes)
	if err != nil {
		return nil, err
	}
	nonBGP, err := collectNonBGPRoutes(ctx, runner, nodes)
	if err != nil {
		return nil, err
	}
	out = append(out, nonBGP...)
	sortRoutes(out)
	return out, nil
}

func CollectBGPOnlyWithRunner(ctx context.Context, runner Runner, nodes []topology.Node) ([]NormalizedBgpRoute, error) {
	return collectBGP(ctx, runner, nodes)
}

func collectBGP(ctx context.Context, runner Runner, nodes []topology.Node) ([]NormalizedBgpRoute, error) {
	var out []NormalizedBgpRoute
	collectors := bgpCollectorsByID()
	for _, kind := range topology.RegisteredDeviceKinds() {
		profile := topology.ProfileFor(kind).LiveProfile()
		collectorID, ok := profile.BGPRIBCollector()
		if !ok {
			continue
		}
		collector := collectors[collectorID]
		if collector == nil {
			continue
		}
		selected := bgpCollectionNodes(profile, NodesByKind(nodes, kind))
		if len(selected) == 0 {
			continue
		}
		routes, err := collector.Collect(ctx, runner, selected)
		if err != nil {
			return nil, err
		}
		out = append(out, routes...)
	}
	sortRoutes(out)
	return out, nil
}

func CollectWithRunner(ctx context.Context, runner Runner, nodes []topology.Node) ([]NormalizedBgpRoute, error) {
	return Collect(ctx, runner, nodes)
}

func CollectFRR(nodes []topology.Node) ([]NormalizedBgpRoute, error) {
	return CollectFRRWithRunner(context.Background(), ExecRunner{}, nodes)
}

func CollectFRRWithRunner(ctx context.Context, runner Runner, nodes []topology.Node) ([]NormalizedBgpRoute, error) {
	return frrCollector{}.Collect(ctx, runner, nodes)
}

func SupportedNodes(nodes []topology.Node) []topology.Node {
	var out []topology.Node
	collectors := bgpCollectorsByID()
	for _, n := range nodes {
		collectorID, ok := topology.ProfileFor(n.Kind).LiveProfile().BGPRIBCollector()
		if ok && collectors[collectorID] != nil {
			out = append(out, n)
		}
	}
	return out
}

func FRRNodes(nodes []topology.Node) []topology.Node {
	return NodesByKind(nodes, topology.KindFRR)
}

func NodesByKind(nodes []topology.Node, kind topology.DeviceKind) []topology.Node {
	var out []topology.Node
	for _, n := range nodes {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

func bgpCollectionNodes(profile topology.LiveProfile, nodes []topology.Node) []topology.Node {
	var out []topology.Node
	for _, n := range nodes {
		if profile.ShouldCollectBGP(n) {
			out = append(out, n)
		}
	}
	return out
}

type frrCollector struct{}
type ceosCollector struct{}
type srlinuxCollector struct{}

func bgpCollectorsByID() map[topology.LiveCollectorID]BgpRibCollector {
	return map[topology.LiveCollectorID]BgpRibCollector{
		topology.LiveCollectorFRR:     frrCollector{},
		topology.LiveCollectorCEOS:    ceosCollector{},
		topology.LiveCollectorSRLinux: srlinuxCollector{},
	}
}

func routeTableCollectorsByID() map[topology.LiveCollectorID]RouteTableCollector {
	return map[topology.LiveCollectorID]RouteTableCollector{
		topology.LiveCollectorFRR:     frrCollector{},
		topology.LiveCollectorCEOS:    ceosCollector{},
		topology.LiveCollectorSRLinux: srlinuxCollector{},
	}
}

func (frrCollector) Collect(ctx context.Context, runner Runner, nodes []topology.Node) ([]NormalizedBgpRoute, error) {
	var out []NormalizedBgpRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		data, err := runner.Run(ctx, "docker", "exec", "-i", containerName, "vtysh", "-c", "show ip bgp json")
		if err != nil {
			if strings.Contains(string(data), "bgpd is not running") {
				continue
			}
			return nil, fmt.Errorf("docker exec -i %s vtysh -c %q: %w", containerName, "show ip bgp json", err)
		}
		routes, err := ParseFRR(n.Name, data)
		if err != nil {
			return nil, fmt.Errorf("%s FRR BGP RIB: %w", n.Name, err)
		}
		out = append(out, routes...)
		for _, vrf := range frrVRFsFromNode(n) {
			cmd := fmt.Sprintf("show bgp vrf %s ipv4 unicast json", vrf)
			data, err := runner.Run(ctx, "docker", "exec", "-i", containerName, "vtysh", "-c", cmd)
			if err != nil {
				if strings.Contains(string(data), "bgpd is not running") {
					continue
				}
				return nil, fmt.Errorf("docker exec -i %s vtysh -c %q: %w", containerName, cmd, err)
			}
			routes, err := ParseFRRVRF(n.Name, vrf, data)
			if err != nil {
				return nil, fmt.Errorf("%s FRR BGP RIB vrf %s: %w", n.Name, vrf, err)
			}
			out = append(out, routes...)
		}
	}
	sortRoutes(out)
	return out, nil
}

func frrVRFsFromNode(n topology.Node) []string {
	seen := map[string]bool{}
	var vrfs []string
	for _, iface := range n.Interfaces {
		vrf := string(topology.NormalizeNetworkInstance(string(iface.VRF)))
		if vrf == "" || vrf == string(topology.NetworkInstanceDefault) || seen[vrf] {
			continue
		}
		seen[vrf] = true
		vrfs = append(vrfs, vrf)
	}
	sort.Strings(vrfs)
	return vrfs
}

func (ceosCollector) Collect(ctx context.Context, runner Runner, nodes []topology.Node) ([]NormalizedBgpRoute, error) {
	var out []NormalizedBgpRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		data, err := runner.Run(ctx, "docker", "exec", "-i", containerName, "Cli", "-p", "15", "-c", "show ip bgp vrf all | json")
		if err != nil {
			return nil, fmt.Errorf("docker exec -i %s Cli -p 15 -c %q: %w", containerName, "show ip bgp vrf all | json", err)
		}
		routes, err := ParseCEOS(n.Name, data)
		if err != nil {
			return nil, fmt.Errorf("%s cEOS BGP RIB: %w", n.Name, err)
		}
		out = append(out, routes...)
	}
	sortRoutes(out)
	return out, nil
}

func collectNonBGPRoutes(ctx context.Context, runner Runner, nodes []topology.Node) ([]NormalizedBgpRoute, error) {
	return CollectRouteTablesWithRunner(ctx, runner, nodes)
}

func CollectRouteTablesWithRunner(ctx context.Context, runner Runner, nodes []topology.Node) ([]NormalizedBgpRoute, error) {
	var out []NormalizedBgpRoute
	collectors := routeTableCollectorsByID()
	for _, kind := range topology.RegisteredDeviceKinds() {
		collectorID, ok := topology.ProfileFor(kind).LiveProfile().RouteTableCollector()
		if !ok {
			continue
		}
		collector := collectors[collectorID]
		if collector == nil {
			continue
		}
		selected := NodesByKind(nodes, kind)
		if len(selected) == 0 {
			continue
		}
		routes, err := collector.CollectRouteTables(ctx, runner, selected)
		if err != nil {
			return nil, err
		}
		out = append(out, routes...)
	}
	sortRoutes(out)
	return out, nil
}

func (srlinuxCollector) Collect(ctx context.Context, runner Runner, nodes []topology.Node) ([]NormalizedBgpRoute, error) {
	var out []NormalizedBgpRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		for _, ni := range topology.NetworkInstancesForNode(n) {
			summary, err := RunSRLinuxJSON(ctx, runner, containerName, "show", "network-instance", ni, "protocols", "bgp", "routes", "ipv4", "summary")
			if err != nil {
				return nil, fmt.Errorf("%s SR Linux BGP RIB summary collection network-instance %s: %w", n.Name, ni, err)
			}
			prefixes, err := ParseSRLinuxSummary(summary)
			if err != nil {
				return nil, fmt.Errorf("%s SR Linux BGP RIB summary network-instance %s: %w", n.Name, ni, err)
			}
			for _, prefix := range prefixes {
				detail, err := RunSRLinuxJSON(ctx, runner, containerName, "show", "network-instance", ni, "protocols", "bgp", "routes", "ipv4", "prefix", prefix, "detail")
				if err != nil {
					return nil, fmt.Errorf("%s SR Linux BGP RIB network-instance %s prefix %s detail collection: %w", n.Name, ni, prefix, err)
				}
				routes, err := ParseSRLinuxDetailNetworkInstance(n.Name, ni, prefix, detail)
				if err != nil {
					return nil, fmt.Errorf("%s SR Linux BGP RIB network-instance %s prefix %s detail: %w", n.Name, ni, prefix, err)
				}
				out = append(out, routes...)
			}
		}
	}
	sortRoutes(out)
	return out, nil
}

func RunSRLinuxJSON(ctx context.Context, runner Runner, containerName string, showArgs ...string) ([]byte, error) {
	return srlinuxjson.ExecJSON(ctx, runner, containerName, showArgs...)
}
