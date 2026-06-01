package facts

import (
	"context"
	"path/filepath"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/collect"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

type Snapshot struct {
	Name     string
	LabPath  string
	Topology *model.Topology
	Graph    *sim.Graph
	RIB      []observation.RIB
	FIB      []observation.FIB
}

func Build(labPath, snapshotName string) (Snapshot, error) {
	if snapshotName == "" {
		snapshotName = "current"
	}
	topo, _, err := topology.LoadTopologyWithOptions(filepath.Join(labPath, "hoyan.clab.yml"), topology.LoadOptions{})
	if err != nil {
		return Snapshot{}, err
	}
	simulator, err := collect.NewSimulator(topo)
	if err != nil {
		return Snapshot{}, err
	}
	graph := simulator.Graph()
	networkSnapshot, err := observation.CollectSnapshot(context.Background(), simulator, observation.CollectOptions{IncludeInactive: true, IncludeModelInfo: true})
	if err != nil {
		return Snapshot{}, err
	}
	rib := snapshotRIBs(networkSnapshot)
	fib := snapshotFIBs(networkSnapshot)
	return Snapshot{Name: snapshotName, LabPath: labPath, Topology: topo, Graph: graph, RIB: rib, FIB: fib}, nil
}

func snapshotRIBs(snapshot observation.NetworkSnapshot) []observation.RIB {
	var out []observation.RIB
	for _, node := range snapshot.Nodes {
		for _, vrf := range node.VRFs {
			out = append(out, vrf.RIB)
		}
	}
	return out
}

func snapshotFIBs(snapshot observation.NetworkSnapshot) []observation.FIB {
	var out []observation.FIB
	for _, node := range snapshot.Nodes {
		for _, vrf := range node.VRFs {
			out = append(out, vrf.FIB)
		}
	}
	return out
}
