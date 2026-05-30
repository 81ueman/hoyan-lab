package fib

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/adapter/srlinuxjson"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type LiveCollector struct {
	runner Runner
}

func NewCollector(runner Runner) LiveCollector {
	return LiveCollector{runner: runner}
}

func Collect(ctx context.Context, runner Runner, nodes []model.Node, opts Options) ([]NormalizedFIBRoute, error) {
	return NewCollector(runner).Collect(ctx, nodes, opts)
}

func (c LiveCollector) Collect(ctx context.Context, nodes []model.Node, opts Options) ([]NormalizedFIBRoute, error) {
	var out []NormalizedFIBRoute
	unsupported := unsupportedNodes(nodes)
	if len(unsupported) > 0 && !opts.AllowUnsupported {
		return nil, UnsupportedNodesError{Nodes: unsupported}
	}
	collectors := fibCollectorsByID()
	for _, kind := range model.RegisteredDeviceKinds() {
		collectorID, ok := model.ProfileFor(kind).LiveProfile().FIBCollector()
		if !ok {
			continue
		}
		collector := collectors[collectorID]
		if collector == nil {
			continue
		}
		selected := nodesByKind(nodes, kind)
		if len(selected) == 0 {
			continue
		}
		routes, err := collector.Collect(ctx, c.runner, selected)
		if err != nil {
			return nil, err
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
}

func SupportedNodes(nodes []model.Node) []model.Node {
	return NewCollector(nil).SupportedNodes(nodes)
}

func (c LiveCollector) SupportedNodes(nodes []model.Node) []model.Node {
	var out []model.Node
	collectors := fibCollectorsByID()
	for _, n := range nodes {
		collectorID, ok := model.ProfileFor(n.Kind).LiveProfile().FIBCollector()
		if ok && collectors[collectorID] != nil {
			out = append(out, n)
		}
	}
	return out
}

func unsupportedNodes(nodes []model.Node) []string {
	var out []string
	collectors := fibCollectorsByID()
	for _, n := range nodes {
		collectorID, ok := model.ProfileFor(n.Kind).LiveProfile().FIBCollector()
		if !ok || collectors[collectorID] == nil {
			out = append(out, n.Name+"("+string(n.Kind)+")")
		}
	}
	sort.Strings(out)
	return out
}

func nodesByKind(nodes []model.Node, kind model.DeviceKind) []model.Node {
	var out []model.Node
	for _, n := range nodes {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

type frrCollector struct{}
type ceosCollector struct{}
type srlinuxCollector struct{}

type collector interface {
	Collect(ctx context.Context, runner Runner, nodes []model.Node) ([]NormalizedFIBRoute, error)
}

func fibCollectorsByID() map[model.LiveCollectorID]collector {
	return map[model.LiveCollectorID]collector{
		model.LiveCollectorFRR:     frrCollector{},
		model.LiveCollectorCEOS:    ceosCollector{},
		model.LiveCollectorSRLinux: srlinuxCollector{},
	}
}

func (frrCollector) Collect(ctx context.Context, runner Runner, nodes []model.Node) ([]NormalizedFIBRoute, error) {
	var out []NormalizedFIBRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		vrfs, err := collectLinuxVRFs(ctx, runner, containerName)
		if err != nil {
			return nil, fmt.Errorf("docker exec -i %s ip -j link show type vrf: %w", containerName, err)
		}
		for _, table := range []string{"main", "local"} {
			data, err := runner.Run(ctx, "docker", "exec", "-i", containerName, "ip", "-j", "route", "show", "table", table)
			if err != nil {
				return nil, fmt.Errorf("docker exec -i %s ip -j route show table %s: %w", containerName, table, err)
			}
			routes, err := ParseLinuxIPRoute(n.Name, data)
			if err != nil {
				return nil, fmt.Errorf("%s Linux kernel FIB table %s: %w", n.Name, table, err)
			}
			out = append(out, routes...)
		}
		for _, vrf := range vrfs {
			data, err := runner.Run(ctx, "docker", "exec", "-i", containerName, "ip", "-j", "route", "show", "vrf", vrf)
			if err != nil {
				return nil, fmt.Errorf("docker exec -i %s ip -j route show vrf %s: %w", containerName, vrf, err)
			}
			routes, err := ParseLinuxIPRouteVRF(n.Name, vrf, data)
			if err != nil {
				return nil, fmt.Errorf("%s Linux kernel FIB vrf %s: %w", n.Name, vrf, err)
			}
			out = append(out, routes...)
		}
	}
	SortRoutes(out)
	return out, nil
}

func collectLinuxVRFs(ctx context.Context, runner Runner, containerName string) ([]string, error) {
	data, err := runner.Run(ctx, "docker", "exec", "-i", containerName, "ip", "-j", "link", "show", "type", "vrf")
	if err != nil {
		return nil, err
	}
	var raw []map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var out []string
	for _, item := range raw {
		name, _ := item["ifname"].(string)
		if name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (ceosCollector) Collect(ctx context.Context, runner Runner, nodes []model.Node) ([]NormalizedFIBRoute, error) {
	var out []NormalizedFIBRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		data, err := runner.Run(ctx, "docker", "exec", "-i", containerName, "Cli", "-p", "15", "-c", "show ip route vrf all | json")
		if err != nil {
			return nil, fmt.Errorf("docker exec -i %s Cli -p 15 -c %q: %w", containerName, "show ip route vrf all | json", err)
		}
		routes, err := ParseCEOSRoutes(n.Name, data)
		if err != nil {
			return nil, fmt.Errorf("%s cEOS installed FIB: %w", n.Name, err)
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
}

func (srlinuxCollector) Collect(ctx context.Context, runner Runner, nodes []model.Node) ([]NormalizedFIBRoute, error) {
	var out []NormalizedFIBRoute
	for _, n := range nodes {
		containerName := n.RuntimeName()
		for _, ni := range model.NetworkInstancesForNode(n) {
			data, err := srlinuxjson.ExecJSON(ctx, runner, containerName, "show", "network-instance", ni, "route-table", "ipv4-unicast", "summary")
			if err != nil {
				return nil, fmt.Errorf("%s sr_cli network-instance %s route-table ipv4-unicast summary: %w", containerName, ni, err)
			}
			routes, err := ParseSRLinuxRoutesNetworkInstance(n.Name, ni, data)
			if err != nil {
				return nil, fmt.Errorf("%s SR Linux installed FIB network-instance %s: %w", n.Name, ni, err)
			}
			for i := range routes {
				if !srlinuxNeedsRouteDetail(routes[i]) {
					continue
				}
				detail, err := srlinuxjson.ExecJSON(ctx, runner, containerName, "show", "network-instance", ni, "route-table", "ipv4-unicast", "prefix", routes[i].Prefix, "detail")
				if err != nil {
					return nil, fmt.Errorf("%s sr_cli network-instance %s route-table ipv4-unicast prefix %s detail: %w", containerName, ni, routes[i].Prefix, err)
				}
				detailRoutes, err := ParseSRLinuxRouteDetailsNetworkInstance(n.Name, ni, detail)
				if err != nil {
					return nil, fmt.Errorf("%s SR Linux installed FIB network-instance %s prefix %s detail: %w", n.Name, ni, routes[i].Prefix, err)
				}
				if detailRoute, ok := srlinuxRouteDetailFor(routes[i], detailRoutes); ok && len(detailRoute.NextHops) > 0 {
					routes[i].NextHops = detailRoute.NextHops
				}
			}
			out = append(out, routes...)
		}
	}
	SortRoutes(out)
	return out, nil
}

func srlinuxNeedsRouteDetail(route NormalizedFIBRoute) bool {
	switch CanonicalProtocol(route.Protocol) {
	case "bgp", "static":
		return true
	default:
		return false
	}
}

func srlinuxRouteDetailFor(summary NormalizedFIBRoute, details []NormalizedFIBRoute) (NormalizedFIBRoute, bool) {
	for _, detail := range details {
		if detail.Node == summary.Node && detail.VRF == summary.VRF && detail.AFI == summary.AFI && detail.Prefix == summary.Prefix && CanonicalProtocol(detail.Protocol) == CanonicalProtocol(summary.Protocol) {
			return detail, true
		}
	}
	return NormalizedFIBRoute{}, false
}
