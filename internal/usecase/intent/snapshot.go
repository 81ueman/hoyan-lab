package intent

import (
	"context"
	"path/filepath"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/collect"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

type SnapshotContext struct {
	Name     string
	LabPath  string
	Topology *model.Topology
	Graph    *sim.Graph
	Network  observation.NetworkSnapshot
}

func BuildSnapshot(labPath, snapshotName string) (SnapshotContext, error) {
	if snapshotName == "" {
		snapshotName = "current"
	}
	topo, _, err := topology.LoadTopologyWithOptions(filepath.Join(labPath, "hoyan.clab.yml"), topology.LoadOptions{})
	if err != nil {
		return SnapshotContext{}, err
	}
	simulator, err := collect.NewSimulator(topo)
	if err != nil {
		return SnapshotContext{}, err
	}
	network, err := collect.CollectSnapshot(context.Background(), simulator, observation.CollectOptions{IncludeInactive: true, IncludeModelInfo: true})
	if err != nil {
		return SnapshotContext{}, err
	}
	return SnapshotContext{Name: snapshotName, LabPath: labPath, Topology: topo, Graph: simulator.Graph(), Network: network}, nil
}

func RIBs(snapshot observation.NetworkSnapshot) []observation.RIB {
	var out []observation.RIB
	for _, node := range snapshot.Nodes {
		for _, vrf := range node.VRFs {
			out = append(out, vrf.RIB)
		}
	}
	return out
}

func FIBs(snapshot observation.NetworkSnapshot) []observation.FIB {
	var out []observation.FIB
	for _, node := range snapshot.Nodes {
		for _, vrf := range node.VRFs {
			out = append(out, vrf.FIB)
		}
	}
	return out
}
