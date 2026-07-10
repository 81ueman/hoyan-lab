package intent

import (
	"fmt"
	"path/filepath"

	"github.com/81ueman/hoyan-lab/internal/domain/solver"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

// RacingReport contains the results of racing detection for all prefixes.
type RacingReport struct {
	LabPath            string             `json:"lab_path"`
	Prefixes           []sim.RacingResult `json:"prefixes"`
	Racing             bool               `json:"racing"`
	PrefixesWithRacing int                `json:"prefixes_with_racing"`
}

// DetectRacing loads a lab topology from the given directory, runs the simulation
// with racing propagation, and detects BGP route update racing for all prefixes
// with multiple origins. The labPath must be an absolute path to a lab directory
// containing hoyan.clab.yml. The solverBackend is used for Z3 satisfiability checks.
func DetectRacing(labPath string, solverBackend solver.Backend) (*RacingReport, error) {
	topo, _, err := topology.LoadTopologyWithOptions(
		filepath.Join(labPath, "hoyan.clab.yml"),
		topology.LoadOptions{},
	)
	if err != nil {
		return nil, fmt.Errorf("load topology: %w", err)
	}

	graph, err := sim.NewGraph(topo, sim.WithSolverBackend(solverBackend))
	if err != nil {
		return nil, fmt.Errorf("build graph: %w", err)
	}

	prefixResults := graph.DetectAllRacing()

	racingCount := 0
	for _, pr := range prefixResults {
		if pr.Racing {
			racingCount++
		}
	}

	return &RacingReport{
		LabPath:            labPath,
		Prefixes:           prefixResults,
		Racing:             racingCount > 0,
		PrefixesWithRacing: racingCount,
	}, nil
}
