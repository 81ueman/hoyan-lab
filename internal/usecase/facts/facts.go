package facts

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	domainfacts "github.com/81ueman/hoyan-lab/internal/domain/facts"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

type RIBRow = domainfacts.RIBRow
type FIBRow = domainfacts.FIBRow
type CanonicalRIBRow = domainfacts.CanonicalRIBRow

var CanonicalRIBRows = domainfacts.CanonicalRIBRows

type Snapshot struct {
	Name     string
	LabPath  string
	Topology *model.Topology
	Graph    *sim.Graph
	RIB      []RIBRow
	FIB      []FIBRow
}

func Build(labPath, snapshotName string) (Snapshot, error) {
	if snapshotName == "" {
		snapshotName = "current"
	}
	labPath = resolveLabPath(labPath)
	topo, _, err := topology.LoadTopologyWithOptions(filepath.Join(labPath, "hoyan.clab.yml"), topology.LoadOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	graph := sim.NewGraph(topo)
	fibInstalled := map[string]map[string]bool{}
	var fibRows []FIBRow
	nodes := nodeNames(topo)
	for _, node := range nodes {
		for _, entry := range graph.FIB(node) {
			if fibInstalled[node] == nil {
				fibInstalled[node] = map[string]bool{}
			}
			fibInstalled[node][entry.Prefix.String()] = true
			fibRows = append(fibRows, FIBRow{
				Snapshot:  snapshotName,
				Device:    node,
				Prefix:    entry.Prefix.String(),
				NextHop:   firstNonEmpty(entry.NextHop, entry.NextHopAddress, entry.RawNextHop),
				Interface: entry.Interface,
				Installed: true,
			})
		}
	}
	sort.SliceStable(fibRows, func(i, j int) bool {
		return factKey(fibRows[i].Snapshot, fibRows[i].Device, fibRows[i].Prefix, fibRows[i].NextHop, fibRows[i].Interface) <
			factKey(fibRows[j].Snapshot, fibRows[j].Device, fibRows[j].Prefix, fibRows[j].NextHop, fibRows[j].Interface)
	})

	var ribRows []RIBRow
	for _, node := range nodes {
		table := graph.RIBTable(node)
		prefixes := make([]string, 0, len(table))
		for prefix := range table {
			prefixes = append(prefixes, prefix)
		}
		sort.Strings(prefixes)
		for _, prefix := range prefixes {
			for _, route := range table[prefix] {
				route = route.Normalize()
				ribRows = append(ribRows, RIBRow{
					Snapshot:  snapshotName,
					Device:    node,
					VRF:       string(route.RouteSource.NetworkInstance),
					Prefix:    route.NLRI.Prefix.String(),
					Protocol:  string(route.SourceKind),
					NextHop:   firstNonEmpty(route.ForwardingNextHop.Node, route.ForwardingNextHop.Addr),
					LocalPref: route.Attrs.LocalPref,
					MED:       route.Attrs.MED,
					Selected:  route.SelectedCond != nil,
					Installed: fibInstalled[node][route.NLRI.Prefix.String()],
				})
			}
		}
	}
	sort.SliceStable(ribRows, func(i, j int) bool {
		return factKey(ribRows[i].Snapshot, ribRows[i].Device, ribRows[i].Prefix, ribRows[i].Protocol, ribRows[i].NextHop) <
			factKey(ribRows[j].Snapshot, ribRows[j].Device, ribRows[j].Prefix, ribRows[j].Protocol, ribRows[j].NextHop)
	})
	return Snapshot{Name: snapshotName, LabPath: labPath, Topology: topo, Graph: graph, RIB: ribRows, FIB: fibRows}, nil
}

func resolveLabPath(raw string) string {
	if raw == "" || strings.ContainsRune(raw, filepath.Separator) || filepath.IsAbs(raw) {
		return raw
	}
	candidate := filepath.Join("labs", raw)
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return candidate
	}
	return raw
}

func nodeNames(topo *model.Topology) []string {
	nodes := make([]string, 0, len(topo.Nodes))
	for _, node := range topo.Nodes {
		nodes = append(nodes, node.Name)
	}
	sort.Strings(nodes)
	return nodes
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func factKey(parts ...string) string {
	key := ""
	for _, part := range parts {
		key += "\x00" + part
	}
	return key
}
