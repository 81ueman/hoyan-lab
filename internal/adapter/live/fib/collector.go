package fib

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/adapter/live/device"
	"github.com/81ueman/hoyan-lab/internal/adapter/srlinuxjson"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type LiveCollector struct {
	runner Runner
}

func NewCollector(runner Runner) LiveCollector {
	return LiveCollector{runner: runner}
}

func CollectFIB(ctx context.Context, runner Runner, node model.Node, vrf model.NetworkInstanceID, opts Options) (FIB, error) {
	return NewCollector(runner).CollectFIB(ctx, node, vrf, opts)
}

func (c LiveCollector) CollectFIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID, opts Options) (FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	unsupported := unsupportedNodes([]model.Node{node})
	if len(unsupported) > 0 {
		return FIB{}, UnsupportedNodesError{Nodes: unsupported}
	}
	collectors := fibCollectorsByID(c.runner)
	collectorID, ok := model.ProfileFor(node.Kind).LiveProfile().FIBCollector()
	if !ok || collectors[collectorID] == nil {
		return FIB{Node: model.NodeID(node.Name), VRF: vrf}, nil
	}
	afi := opts.AFI
	if afi == "" {
		afi = model.AFIIPv4
	}
	fib, err := collectors[collectorID].CollectFIB(ctx, node, vrf, afi)
	if err != nil {
		return FIB{}, err
	}
	return fib, nil
}

func unsupportedNodes(nodes []model.Node) []string {
	var out []string
	collectors := fibCollectorsByID(nil)
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
	CollectFIB(ctx context.Context, node model.Node, vrf model.NetworkInstanceID, afi model.AFI) (FIB, error)
}

func fibCollectorsByID(runner Runner) map[model.LiveCollectorID]collector {
	return device.NewRegistry[collector](runner, func(id model.LiveCollectorID, base device.VendorCollector) collector {
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

func (c frrCollector) CollectFIB(ctx context.Context, n model.Node, vrf model.NetworkInstanceID, afi model.AFI) (FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	fib := FIB{Node: model.NodeID(n.Name), VRF: vrf}
	session := c.SessionForNode(n)
	ipArgs := []string{"ip", "-j", "route"}
	if afi == model.AFIIPv6 {
		ipArgs = []string{"ip", "-j", "-6", "route"}
	}
	cmdStr := strings.Join(ipArgs, " ")
	if vrf == model.NetworkInstanceDefault {
		for _, table := range []string{"main", "local"} {
			args := append(append([]string(nil), ipArgs...), "show", "table", table)
			data, err := session.Exec(ctx, args...)
			if err != nil {
				return FIB{}, fmt.Errorf("node %s %s show table %s: %w", n.RuntimeName(), cmdStr, table, err)
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
	args := append(append([]string(nil), ipArgs...), "show", "vrf", string(vrf))
	data, err := session.Exec(ctx, args...)
	if err != nil {
		return FIB{}, fmt.Errorf("node %s %s show vrf %s: %w", n.RuntimeName(), cmdStr, vrf, err)
	}
	routes, err := ParseLinuxIPRouteVRF(n.Name, string(vrf), data)
	if err != nil {
		return FIB{}, fmt.Errorf("%s Linux kernel FIB vrf %s: %w", n.Name, vrf, err)
	}
	fib.Entries = routes
	SortRoutes(fib.Entries)
	return fib, nil
}

func (c ceosCollector) CollectFIB(ctx context.Context, n model.Node, vrf model.NetworkInstanceID, afi model.AFI) (FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	session := c.SessionForNode(n)
	fibCmd := "show ip route vrf all | json"
	if afi == model.AFIIPv6 {
		fibCmd = "show ipv6 route vrf all | json"
	}
	data, err := session.Exec(ctx, "Cli", "-p", "15", "-c", fibCmd)
	if err != nil {
		return FIB{}, fmt.Errorf("node %s Cli -p 15 -c %q: %w", n.RuntimeName(), fibCmd, err)
	}
	fibs, err := ParseCEOSFIBs(n.Name, data)
	if err != nil {
		return FIB{}, fmt.Errorf("%s cEOS installed FIB: %w", n.Name, err)
	}
	return fibByVRF(fibs, model.NodeID(n.Name), vrf), nil
}

func (c srlinuxCollector) CollectFIB(ctx context.Context, n model.Node, vrf model.NetworkInstanceID, afi model.AFI) (FIB, error) {
	vrf = model.NormalizeNetworkInstance(string(vrf))
	session := c.SessionForNode(n)
	ni := string(vrf)
	afiStr := "ipv4-unicast"
	if afi == model.AFIIPv6 {
		afiStr = "ipv6-unicast"
	}
	data, err := srlinuxjson.ExecJSON(ctx, session, "show", "network-instance", ni, "route-table", afiStr, "summary")
	if err != nil {
		return FIB{}, fmt.Errorf("%s sr_cli network-instance %s route-table %s summary: %w", n.RuntimeName(), ni, afiStr, err)
	}
	routes, err := ParseSRLinuxRoutesNetworkInstance(n.Name, ni, data)
	if err != nil {
		return FIB{}, fmt.Errorf("%s SR Linux installed FIB network-instance %s: %w", n.Name, ni, err)
	}
	for i := range routes {
		if !srlinuxNeedsRouteDetail(routes[i]) {
			continue
		}
		detail, err := srlinuxjson.ExecJSON(ctx, session, "show", "network-instance", ni, "route-table", afiStr, "prefix", routes[i].Prefix, "detail")
		if err != nil {
			return FIB{}, fmt.Errorf("%s sr_cli network-instance %s route-table %s prefix %s detail: %w", n.RuntimeName(), ni, afiStr, routes[i].Prefix, err)
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
