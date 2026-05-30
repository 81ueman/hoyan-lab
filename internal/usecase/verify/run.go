package verify

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/query"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

type VerifyOptions struct {
	CollapseEquivalentResults bool
	MaxPrefixClasses          int
}

func Run(topo *model.Topology, queries *query.Queries) Report {
	return RunWithOptions(topo, queries, VerifyOptions{})
}

func RunWithOptions(topo *model.Topology, queries *query.Queries, opts VerifyOptions) Report {
	return runPrefixClasses(topo, queries, opts)
}

func runPrefixClasses(topo *model.Topology, queries *query.Queries, opts VerifyOptions) Report {
	g := sim.NewGraph(topo)
	universe, err := prefixUniverseForGraph(topo, queries, g, nil)
	if err != nil {
		return Report{Results: []Result{NewSetupResult("prefix-universe", true, err.Error())}}
	}
	if err := checkPrefixClassLimit(universe, opts.MaxPrefixClasses); err != nil {
		stats := universe.Stats
		return Report{Stats: &stats, Results: []Result{NewSetupResult("prefix-universe", true, err.Error())}}
	}

	stats := universe.Stats
	report := Report{Stats: &stats}
	appendRouteCheckResults(&report, queries.RouteChecks, g, universe)
	appendPacketCheckResults(&report, topo, queries.PacketChecks, g, universe)
	appendFailureCheckResults(&report, topo, queries.FailureChecks, g, universe)
	if opts.CollapseEquivalentResults {
		report.Results = collapseResults(report.Results)
	}
	return report
}
