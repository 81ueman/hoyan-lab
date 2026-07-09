package modelinspect

import (
	"fmt"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

func InspectRIB(req Request) (RIBResult, error) {
	protocol, err := CanonicalRouteProtocol(req.Protocol)
	if err != nil {
		return RIBResult{}, err
	}
	topo, graph, err := loadGraph(req.TopologyPath, req.StrictConfig)
	if err != nil {
		return RIBResult{}, err
	}
	nodes, err := inspectNodes(topo, req.Node)
	if err != nil {
		return RIBResult{}, err
	}
	prefix, err := CanonicalPrefix(req.Prefix)
	if err != nil {
		return RIBResult{}, err
	}
	return RIBResult{Protocol: protocol, Rows: collectRIBRows(graph, nodes, prefix, protocol)}, nil
}

func collectRIBRows(graph *sim.Graph, nodes []string, prefix string, protocol model.RouteSourceKind) []RIBRow {
	var rows []RIBRow
	for _, node := range nodes {
		nodeID := model.NodeID(node)
		if prefix != "" {
			pfx, err := model.ParsePrefix(prefix)
			if err != nil {
				continue
			}
			rows = append(rows, ribRowsForRoutes(node, graph.RIB(nodeID, pfx), protocol)...)
			continue
		}
		table := graph.RIBTable(nodeID)
		prefixes := make([]model.Prefix, 0, len(table))
		for p := range table {
			prefixes = append(prefixes, p)
		}
		sort.Slice(prefixes, func(i, j int) bool { return prefixes[i].String() < prefixes[j].String() })
		for _, p := range prefixes {
			rows = append(rows, ribRowsForRoutes(node, table[p], protocol)...)
		}
	}
	return rows
}

func ribRowsForRoutes(node string, routes []sim.RIBEntry, protocol model.RouteSourceKind) []RIBRow {
	rows := make([]RIBRow, 0, len(routes))
	for _, route := range routes {
		route = route.Normalize()
		if protocol != "" && route.SourceKind != protocol {
			continue
		}
		rows = append(rows, RIBRow{
			Node:                  node,
			Prefix:                route.NLRI.Prefix.String(),
			SourceKind:            string(route.SourceKind),
			ConnectedClass:        string(route.RouteSource.ConnectedClass),
			OSPFRouteType:         route.RouteSource.OSPFRouteType,
			RouteInterface:        route.RouteSource.Interface,
			NextHopNode:           route.ForwardingNextHop.Node,
			NextHopAddr:           route.ForwardingNextHop.Addr,
			OriginNode:            route.Provenance.OriginNode,
			FromNode:              route.Provenance.FromNode,
			PathNodes:             append([]string(nil), route.Provenance.PathNodes...),
			PathLinks:             append([]string(nil), route.Provenance.PathLinks...),
			AggregateContributors: append([]string(nil), route.AggregateContributors...),
			Condition:             condString(route.Condition),
			SelectedCondition:     condString(route.SelectedCond),
			BaseCondition:         condString(route.BaseCond),
		})
		if route.SourceKind == model.RouteSourceBGP {
			last := &rows[len(rows)-1]
			last.ASPath = append([]uint32(nil), route.Attrs.ASPath...)
			last.Communities = append([]string(nil), route.Attrs.Communities...)
			last.OriginCode = ptr(string(route.Attrs.OriginCode))
			last.LocalPref = ptr(route.Attrs.LocalPref)
			last.MED = ptr(route.Attrs.MED)
			last.LearnedIBGP = ptr(route.Attrs.LearnedIBGP)
			last.Invalid = ptr(route.Attrs.Invalid)
		}
		if route.SourceKind == model.RouteSourceOSPF {
			rows[len(rows)-1].Metric = ptr(route.RouteSource.Metric)
		}
	}
	return rows
}

func loadGraph(topologyPath string, strictConfig bool) (*model.Topology, *sim.Graph, error) {
	topo, _, err := topology.LoadTopologyWithOptions(topologyPath, topology.LoadOptions{StrictConfig: strictConfig})
	if err != nil {
		return nil, nil, err
	}
	return topo, sim.NewGraph(topo), nil
}

func inspectNodes(topo *model.Topology, node string) ([]string, error) {
	if node != "" {
		if _, ok := topo.Node(node); !ok {
			return nil, fmt.Errorf("unknown node %q", node)
		}
		return []string{node}, nil
	}
	nodes := make([]string, 0, len(topo.Nodes))
	for _, n := range topo.Nodes {
		nodes = append(nodes, n.Name)
	}
	sort.Strings(nodes)
	return nodes, nil
}

func CanonicalPrefix(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	prefix, err := model.ParsePrefix(raw)
	if err != nil {
		return "", fmt.Errorf("--prefix %q: %w", raw, err)
	}
	return prefix.String(), nil
}

func CanonicalRouteProtocol(raw string) (model.RouteSourceKind, error) {
	protocol := model.RouteSourceKind(strings.ToLower(strings.TrimSpace(raw)))
	switch protocol {
	case "":
		return "", nil
	case model.RouteSourceBGP, model.RouteSourceConnected, model.RouteSourceStatic, model.RouteSourceOSPF, model.RouteSourceAggregate, model.RouteSourceBlackhole:
		return protocol, nil
	default:
		return "", fmt.Errorf("protocol must be one of bgp, connected, static, ospf, aggregate, or blackhole")
	}
}
