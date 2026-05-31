package rib

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/adapter/srlinuxjson"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type LiveCollector struct {
	runner Runner
}

func NewCollector(runner Runner) LiveCollector {
	return LiveCollector{runner: runner}
}

func (c LiveCollector) collectRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error) {
	out, err := c.collectBGPRoutes(ctx, nodes)
	if err != nil {
		return nil, err
	}
	nonBGP, err := c.collectRouteTableRoutes(ctx, nodes)
	if err != nil {
		return nil, err
	}
	out = append(out, nonBGP...)
	SortRoutes(out)
	return out, nil
}

func (c LiveCollector) CollectRIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID, opts observation.CollectOptions) (observation.RIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	routes, err := c.collectRoutes(ctx, []model.Node{node})
	if err != nil {
		return observation.RIB{}, err
	}
	return observation.FilterRIB(observation.RIB{Node: model.NodeID(node.Name), VRF: vrf, Routes: routes}, opts), nil
}

func (c LiveCollector) collectBGPRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error) {
	var out []RIBRoute
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
		routes, err := collector.collectBGPRoutes(ctx, selected)
		if err != nil {
			return nil, err
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
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

type routeCollector interface {
	collectBGPRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error)
	collectOSPFRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error)
	collectRouteTableRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error)
}

func collectorsByID(runner Runner) map[model.LiveCollectorID]routeCollector {
	return map[model.LiveCollectorID]routeCollector{
		model.LiveCollectorFRR:     frrCollector{runner: runner},
		model.LiveCollectorCEOS:    ceosCollector{runner: runner},
		model.LiveCollectorSRLinux: srlinuxCollector{runner: runner},
	}
}

func (c frrCollector) collectBGPRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error) {
	var out []RIBRoute
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

func (c ceosCollector) collectBGPRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error) {
	var out []RIBRoute
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

func (c LiveCollector) collectOSPFRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error) {
	var out []RIBRoute
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
		routes, err := collector.collectOSPFRoutes(ctx, selected)
		if err != nil {
			return nil, err
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
}

func (c LiveCollector) collectRouteTableRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error) {
	var out []RIBRoute
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
		routes, err := collector.collectRouteTableRoutes(ctx, selected)
		if err != nil {
			return nil, err
		}
		out = append(out, routes...)
	}
	SortRoutes(out)
	return out, nil
}

func (c srlinuxCollector) collectBGPRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error) {
	var out []RIBRoute
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
