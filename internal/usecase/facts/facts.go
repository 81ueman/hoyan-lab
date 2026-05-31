package facts

import (
	"path/filepath"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	fibusecase "github.com/81ueman/hoyan-lab/internal/usecase/fib"
	ribusecase "github.com/81ueman/hoyan-lab/internal/usecase/rib"
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
	graph := sim.NewGraph(topo)
	rib := groupRIBRoutes((ribusecase.ExpectedBuilder{}).Build(topo))
	fib := fibusecase.NewExpectedBuilder().ExpectedFIBs(topo)
	return Snapshot{Name: snapshotName, LabPath: labPath, Topology: topo, Graph: graph, RIB: rib, FIB: fib}, nil
}

func groupRIBRoutes(routes []observation.RIBRoute) []observation.RIB {
	byKey := map[string]observation.RIB{}
	for _, route := range routes {
		node := route.ModelInfo.Provenance.FromNode
		vrf := model.NetworkInstanceDefault
		key := string(node) + "|" + string(vrf)
		rib := byKey[key]
		if rib.Node == "" {
			rib.Node = node
			rib.VRF = vrf
		}
		rib.Routes = append(rib.Routes, route)
		byKey[key] = rib
	}
	out := make([]observation.RIB, 0, len(byKey))
	for _, rib := range byKey {
		observation.SortRIBRoutes(rib.Routes)
		out = append(out, rib)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key() < out[j].Key() })
	return out
}
