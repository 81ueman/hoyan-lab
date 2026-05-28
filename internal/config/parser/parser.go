package parser

import (
	"github.com/81ueman/hoyan-lab/internal/config/routing"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

type LoadLabTopologyOptions = topology.LoadLabTopologyOptions
type ParsedConfig = topology.ParsedConfig
type ParseResult = topology.ParseResult
type UnsupportedStatement = topology.UnsupportedStatement
type UnsupportedConfigError = topology.UnsupportedConfigError
type FRRParser = topology.FRRParser
type CEOSParser = topology.CEOSParser
type SRLinuxParser = topology.SRLinuxParser

var (
	ParseConfig             = topology.ParseConfig
	ParseConfigWithWarnings = topology.ParseConfigWithWarnings
	ParseNftablesACLConfig  = topology.ParseNftablesACLConfig
)

type Lab struct {
	Topology *topology.Topology
	Routing  routing.TopologyRouting
}

func LoadLabTopology(clabPath string) (Lab, error) {
	lab, _, err := LoadLabTopologyWithOptions(clabPath, LoadLabTopologyOptions{})
	return lab, err
}

func LoadLabTopologyWithWarnings(clabPath string) (Lab, []UnsupportedStatement, error) {
	return LoadLabTopologyWithOptions(clabPath, LoadLabTopologyOptions{CollectWarnings: true})
}

func LoadLabTopologyWithOptions(clabPath string, opts LoadLabTopologyOptions) (Lab, []UnsupportedStatement, error) {
	topo, warnings, err := topology.LoadLabTopologyWithOptions(clabPath, opts)
	if err != nil {
		return Lab{}, warnings, err
	}
	return Lab{Topology: routing.StripTopology(topo), Routing: routing.FromTopology(topo)}, warnings, nil
}
