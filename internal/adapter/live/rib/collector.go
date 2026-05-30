package rib

import (
	"context"
	"fmt"
	"sort"
	"strings"

	liveexec "github.com/81ueman/hoyan-lab/internal/adapter/live"
	"github.com/81ueman/hoyan-lab/internal/adapter/srlinuxjson"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type LiveCollector struct {
	runner Runner
}

func NewCollector(runner Runner) LiveCollector {
	return LiveCollector{runner: runner}
}

func Collect(ctx context.Context, runner Runner, nodes []model.Node) ([]NormalizedRoute, error) {
	return NewCollector(runner).Collect(ctx, nodes)
}

func (c LiveCollector) Collect(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	out, err := c.CollectBGPRoutes(ctx, nodes)
	if err != nil {
		return nil, err
	}
	nonBGP, err := c.CollectRouteTableRoutes(ctx, nodes)
	if err != nil {
		return nil, err
	}
	out = append(out, nonBGP...)
	SortRoutes(out)
	return out, nil
}

func CollectBGPRoutesWithRunner(ctx context.Context, runner Runner, nodes []model.Node) ([]NormalizedRoute, error) {
	return NewCollector(runner).CollectBGPRoutes(ctx, nodes)
}

func (c LiveCollector) CollectBGPRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	var out []NormalizedRoute
	collectors := collectorsByID(c.runner)
	for _, kind := range model.RegisteredDeviceKinds() {
		profile := model.ProfileFor(kind).LiveProfile()
		collectorID, ok := profile.BGPRIBCollector()
		if !ok {
			continue
		}
		collector := collectors[collectorID]
		if collector == nil {
			continue
		}
		selected := bgpRouteCollectionNodes(profile, NodesByKind(nodes, kind))
		if len(selected) == 0 {
			continue
		}
		routes, err := collector.CollectBGPRoutes(ctx, selected)
		if err != nil {
			return nil, err
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
}

func CollectWithRunner(ctx context.Context, runner Runner, nodes []model.Node) ([]NormalizedRoute, error) {
	return Collect(ctx, runner, nodes)
}

func CollectFRR(nodes []model.Node) ([]NormalizedRoute, error) {
	return CollectFRRWithRunner(context.Background(), liveexec.ExecRunner{}, nodes)
}

func CollectFRRWithRunner(ctx context.Context, runner Runner, nodes []model.Node) ([]NormalizedRoute, error) {
	return frrCollector{runner: runner}.CollectBGPRoutes(ctx, nodes)
}

func SupportedNodes(nodes []model.Node) []model.Node {
	var out []model.Node
	collectors := collectorsByID(nil)
	for _, n := range nodes {
		collectorID, ok := model.ProfileFor(n.Kind).LiveProfile().BGPRIBCollector()
		if ok && collectors[collectorID] != nil {
			out = append(out, n)
		}
	}
	return out
}

func FRRNodes(nodes []model.Node) []model.Node {
	return NodesByKind(nodes, model.KindFRR)
}

func NodesByKind(nodes []model.Node, kind model.DeviceKind) []model.Node {
	var out []model.Node
	for _, n := range nodes {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

func bgpRouteCollectionNodes(profile model.LiveProfile, nodes []model.Node) []model.Node {
	var out []model.Node
	for _, n := range nodes {
		if profile.ShouldCollectBGP(n) {
			out = append(out, n)
		}
	}
	return out
}

type frrCollector struct{ runner Runner }
type ceosCollector struct{ runner Runner }
type srlinuxCollector struct{ runner Runner }

func collectorsByID(runner Runner) map[model.LiveCollectorID]Collector {
	return map[model.LiveCollectorID]Collector{
		model.LiveCollectorFRR:     frrCollector{runner: runner},
		model.LiveCollectorCEOS:    ceosCollector{runner: runner},
		model.LiveCollectorSRLinux: srlinuxCollector{runner: runner},
	}
}

func (c frrCollector) CollectBGPRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	var out []NormalizedRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		data, err := c.runner.Run(ctx, "docker", "exec", "-i", containerName, "vtysh", "-c", "show ip bgp json")
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
			data, err := c.runner.Run(ctx, "docker", "exec", "-i", containerName, "vtysh", "-c", cmd)
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
	SortRoutes(out)
	return out, nil
}

func frrVRFsFromNode(n model.Node) []string {
	seen := map[string]bool{}
	var vrfs []string
	for _, iface := range n.Interfaces {
		vrf := string(model.NormalizeNetworkInstance(string(iface.VRF)))
		if vrf == "" || vrf == string(model.NetworkInstanceDefault) || seen[vrf] {
			continue
		}
		seen[vrf] = true
		vrfs = append(vrfs, vrf)
	}
	sort.Strings(vrfs)
	return vrfs
}

func (c ceosCollector) CollectBGPRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	var out []NormalizedRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		data, err := c.runner.Run(ctx, "docker", "exec", "-i", containerName, "Cli", "-p", "15", "-c", "show ip bgp vrf all | json")
		if err != nil {
			return nil, fmt.Errorf("docker exec -i %s Cli -p 15 -c %q: %w", containerName, "show ip bgp vrf all | json", err)
		}
		routes, err := ParseCEOS(n.Name, data)
		if err != nil {
			return nil, fmt.Errorf("%s cEOS BGP RIB: %w", n.Name, err)
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
}

func CollectOSPFRoutesWithRunner(ctx context.Context, runner Runner, nodes []model.Node) ([]NormalizedRoute, error) {
	return NewCollector(runner).CollectOSPFRoutes(ctx, nodes)
}

func (c LiveCollector) CollectOSPFRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	var out []NormalizedRoute
	collectors := collectorsByID(c.runner)
	for _, kind := range model.RegisteredDeviceKinds() {
		collectorID, ok := model.ProfileFor(kind).LiveProfile().RouteTableCollector()
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
		routes, err := collector.CollectOSPFRoutes(ctx, selected)
		if err != nil {
			return nil, err
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
}

func CollectRouteTableRoutesWithRunner(ctx context.Context, runner Runner, nodes []model.Node) ([]NormalizedRoute, error) {
	return NewCollector(runner).CollectRouteTableRoutes(ctx, nodes)
}

func (c LiveCollector) CollectRouteTableRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	var out []NormalizedRoute
	collectors := collectorsByID(c.runner)
	for _, kind := range model.RegisteredDeviceKinds() {
		collectorID, ok := model.ProfileFor(kind).LiveProfile().RouteTableCollector()
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
		routes, err := collector.CollectRouteTableRoutes(ctx, selected)
		if err != nil {
			return nil, err
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
}

func (c srlinuxCollector) CollectBGPRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error) {
	var out []NormalizedRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		for _, ni := range model.NetworkInstancesForNode(n) {
			summary, err := RunSRLinuxJSON(ctx, c.runner, containerName, "show", "network-instance", ni, "protocols", "bgp", "routes", "ipv4", "summary")
			if err != nil {
				return nil, fmt.Errorf("%s SR Linux BGP RIB summary collection network-instance %s: %w", n.Name, ni, err)
			}
			prefixes, err := ParseSRLinuxSummary(summary)
			if err != nil {
				return nil, fmt.Errorf("%s SR Linux BGP RIB summary network-instance %s: %w", n.Name, ni, err)
			}
			for _, prefix := range prefixes {
				detail, err := RunSRLinuxJSON(ctx, c.runner, containerName, "show", "network-instance", ni, "protocols", "bgp", "routes", "ipv4", "prefix", prefix, "detail")
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
	SortRoutes(out)
	return out, nil
}

func RunSRLinuxJSON(ctx context.Context, runner Runner, containerName string, showArgs ...string) ([]byte, error) {
	return srlinuxjson.ExecJSON(ctx, runner, containerName, showArgs...)
}
