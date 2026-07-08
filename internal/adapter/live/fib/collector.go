package fib

import (
	"context"
	"fmt"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/adapter/live/device"
	"github.com/81ueman/hoyan-lab/internal/adapter/srlinuxjson"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type LiveCollector struct {
	runner           Runner
	containerNameFor func(string) string
}

func NewCollector(runner Runner) LiveCollector {
	return LiveCollector{runner: runner}
}

// NewCollectorWithResolver creates a LiveCollector that resolves container names
// via the given function. When the resolver is nil or returns empty, node names
// are used as container names directly.
func NewCollectorWithResolver(runner Runner, resolveContainerName func(string) string) LiveCollector {
	return LiveCollector{runner: runner, containerNameFor: resolveContainerName}
}

func CollectFIB(ctx context.Context, runner Runner, node model.Node, vrf model.NetworkInstanceID, opts Options) (FIB, error) {
	return NewCollector(runner).CollectFIB(ctx, node, vrf, opts)
}

func (c LiveCollector) CollectFIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID, opts Options) (FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	unsupported := c.unsupportedNodes([]model.Node{node})
	if len(unsupported) > 0 {
		return FIB{}, UnsupportedNodesError{Nodes: unsupported}
	}
	collectors := c.fibCollectorsByID()
	collectorID, ok := model.ProfileFor(node.Kind).LiveProfile().FIBCollector()
	if !ok || collectors[collectorID] == nil {
		return FIB{Node: model.NodeID(node.Name), VRF: vrf}, nil
	}
	fib, err := collectors[collectorID].CollectFIB(ctx, node, vrf)
	if err != nil {
		return FIB{}, err
	}
	return fib, nil
}

func (c LiveCollector) unsupportedNodes(nodes []model.Node) []string {
	var out []string
	collectors := c.fibCollectorsByID()
	for _, n := range nodes {
		collectorID, ok := model.ProfileFor(n.Kind).LiveProfile().FIBCollector()
		if !ok || collectors[collectorID] == nil {
			out = append(out, n.Name+"("+string(n.Kind)+")")
		}
	}
	sort.Strings(out)
	return out
}

type frrCollector struct{ device.VendorCollector }
type ceosCollector struct{ device.VendorCollector }
type srlinuxCollector struct{ device.VendorCollector }

type collector interface {
	CollectFIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID) (FIB, error)
}

func (c LiveCollector) fibCollectorsByID() map[model.LiveCollectorID]collector {
	return device.NewRegistryWithResolver[collector](c.runner, c.containerNameFor, func(id model.LiveCollectorID, base device.VendorCollector) collector {
		switch id {
		case model.LiveCollectorFRR:
			return frrCollector{VendorCollector: base}
		case model.LiveCollectorCEOS:
			return ceosCollector{VendorCollector: base}
		case model.LiveCollectorSRLinux:
			return srlinuxCollector{VendorCollector: base}
		default:
			return nil
		}
	})
}

func (c frrCollector) CollectFIB(ctx context.Context, n model.Node, vrf model.NetworkInstanceID) (FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	fib := FIB{Node: model.NodeID(n.Name), VRF: vrf}
	session := c.SessionForNode(n)
	if vrf == model.NetworkInstanceDefault {
		for _, table := range []string{"main", "local"} {
			data, err := session.Exec(ctx, "ip", "-j", "route", "show", "table", table)
			if err != nil {
				return FIB{}, fmt.Errorf("node %s ip -j route show table %s: %w", n.Name, table, err)
			}
			routes, err := ParseLinuxIPRoute(n.Name, data)
			if err != nil {
				return FIB{}, fmt.Errorf("%s Linux kernel FIB table %s: %w", n.Name, table, err)
			}
			fib.Entries = append(fib.Entries, routes...)
		}
		SortRoutes(fib.Entries)
		return fib, nil
	}
	data, err := session.Exec(ctx, "ip", "-j", "route", "show", "vrf", string(vrf))
	if err != nil {
		return FIB{}, fmt.Errorf("node %s ip -j route show vrf %s: %w", n.Name, vrf, err)
	}
	routes, err := ParseLinuxIPRouteVRF(n.Name, string(vrf), data)
	if err != nil {
		return FIB{}, fmt.Errorf("%s Linux kernel FIB vrf %s: %w", n.Name, vrf, err)
	}
	fib.Entries = routes
	SortRoutes(fib.Entries)
	return fib, nil
}

func (c ceosCollector) CollectFIB(ctx context.Context, n model.Node, vrf model.NetworkInstanceID) (FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	session := c.SessionForNode(n)
	data, err := session.Exec(ctx, "Cli", "-p", "15", "-c", "show ip route vrf all | json")
	if err != nil {
		return FIB{}, fmt.Errorf("node %s Cli -p 15 -c %q: %w", n.Name, "show ip route vrf all | json", err)
	}
	fibs, err := ParseCEOSFIBs(n.Name, data)
	if err != nil {
		return FIB{}, fmt.Errorf("%s cEOS installed FIB: %w", n.Name, err)
	}
	return fibByVRF(fibs, model.NodeID(n.Name), vrf), nil
}

func (c srlinuxCollector) CollectFIB(ctx context.Context, n model.Node, vrf model.NetworkInstanceID) (FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	session := c.SessionForNode(n)
	ni := string(vrf)
	data, err := srlinuxjson.ExecJSON(ctx, session, "show", "network-instance", ni, "route-table", "ipv4-unicast", "summary")
	if err != nil {
		return FIB{}, fmt.Errorf("%s sr_cli network-instance %s route-table ipv4-unicast summary: %w", n.Name, ni, err)
	}
	routes, err := ParseSRLinuxRoutesNetworkInstance(n.Name, ni, data)
	if err != nil {
		return FIB{}, fmt.Errorf("%s SR Linux installed FIB network-instance %s: %w", n.Name, ni, err)
	}
	for i := range routes {
		if !srlinuxNeedsRouteDetail(routes[i]) {
			continue
		}
		detail, err := srlinuxjson.ExecJSON(ctx, session, "show", "network-instance", ni, "route-table", "ipv4-unicast", "prefix", routes[i].Prefix, "detail")
		if err != nil {
			return FIB{}, fmt.Errorf("%s sr_cli network-instance %s route-table ipv4-unicast prefix %s detail: %w", n.Name, ni, routes[i].Prefix, err)
		}
		detailRoutes, err := ParseSRLinuxRouteDetailsNetworkInstance(n.Name, ni, detail)
		if err != nil {
			return FIB{}, fmt.Errorf("%s SR Linux installed FIB network-instance %s prefix %s detail: %w", n.Name, ni, routes[i].Prefix, err)
		}
		if detailRoute, ok := srlinuxRouteDetailFor(routes[i], detailRoutes); ok && len(detailRoute.NextHops) > 0 {
			routes[i].NextHops = detailRoute.NextHops
		}
	}
	fib := FIB{Node: model.NodeID(n.Name), VRF: model.NetworkInstanceID(model.NormalizeNetworkInstance(ni)), Entries: routes}
	SortRoutes(fib.Entries)
	return fib, nil
}

func fibByVRF(fibs []FIB, node model.NodeID, vrf model.NetworkInstanceID) FIB {
	for _, fib := range fibs {
		if fib.VRF == vrf {
			if fib.Node == "" {
				fib.Node = node
			}
			return fib
		}
	}
	return FIB{Node: node, VRF: vrf}
}

func srlinuxNeedsRouteDetail(route FIBEntry) bool {
	switch route.Source.Protocol {
	case "bgp", "static":
		return true
	default:
		return false
	}
}

func srlinuxRouteDetailFor(summary FIBEntry, details []FIBEntry) (FIBEntry, bool) {
	for _, detail := range details {
		if detail.AFI == summary.AFI && detail.Prefix == summary.Prefix && detail.Source.Protocol == summary.Source.Protocol {
			return detail, true
		}
	}
	return FIBEntry{}, false
}
