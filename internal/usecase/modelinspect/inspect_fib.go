package modelinspect

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

func InspectFIB(req Request) (FIBResult, error) {
	topo, graph, err := loadGraph(req.TopologyPath, req.StrictConfig)
	if err != nil {
		return FIBResult{}, err
	}
	nodes, err := inspectNodes(topo, req.Node)
	if err != nil {
		return FIBResult{}, err
	}
	prefix, err := CanonicalPrefix(req.Prefix)
	if err != nil {
		return FIBResult{}, err
	}
	return FIBResult{Rows: collectFIBRows(graph, nodes, prefix)}, nil
}

func collectFIBRows(graph *sim.Graph, nodes []string, prefix string) []FIBRow {
	var rows []FIBRow
	for _, node := range nodes {
		for _, entry := range graph.FIB(model.NodeID(node)) {
			if prefix != "" && entry.Prefix.String() != prefix {
				continue
			}
			rows = append(rows, FIBRow{
				Node:             node,
				Prefix:           entry.Prefix.String(),
				SourceKind:       string(entry.SourceKind),
				Discard:          entry.Discard,
				ConnectedClass:   string(entry.ConnectedClass),
				Interface:        entry.Interface,
				NextHop:          entry.NextHop,
				RawNextHop:       entry.RawNextHop,
				NextHopAddress:   entry.NextHopAddress,
				ResolutionStatus: string(entry.ResolutionStatus),
				ResolutionReason: entry.ResolutionReason,
				Rank:             entry.Rank,
				GroupID:          entry.GroupID,
				Equivalent:       entry.Equivalent,
				PathNodes:        append([]string(nil), entry.Path.Nodes...),
				PathLinks:        append([]string(nil), entry.Path.Links...),
				Cost:             entry.Path.Cost,
				Condition:        condString(entry.Condition),
			})
		}
	}
	return rows
}
