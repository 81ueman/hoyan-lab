package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/adapter/flowinput"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/engine/traffic"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
	"github.com/spf13/cobra"
)

func NewTrafficCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "traffic",
		Short:         "Traffic simulation and analysis commands",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(NewTrafficSimulateCommand())
	return cmd
}

func NewTrafficSimulateCommand() *cobra.Command {
	var opts trafficSimulateOptions
	cmd := &cobra.Command{
		Use:           "simulate",
		Short:         "Simulate traffic load on the lab topology",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return ExitError{Code: 2, Err: fmt.Errorf("unexpected arguments: %s", args)}
			}
			if err := opts.validate(); err != nil {
				return ExitError{Code: 2, Err: err}
			}
			if err := runTrafficSimulate(opts, cmd.OutOrStdout()); err != nil {
				return ExitError{Code: 2, Err: err}
			}
			return nil
		},
	}
	addLabFlag(cmd, &opts.labPath)
	cmd.Flags().StringVar(&opts.flowsPath, "flows", "", "path to JSON traffic flow file")
	cmd.Flags().StringVar(&opts.format, "format", "table", "output format: table, json")
	cmd.MarkFlagRequired("flows")
	return cmd
}

type trafficSimulateOptions struct {
	labPath   string
	flowsPath string
	format    string
}

func (o trafficSimulateOptions) validate() error {
	switch o.format {
	case "table", "json":
		return nil
	default:
		return fmt.Errorf("--format must be \"table\" or \"json\", got %q", o.format)
	}
}

func runTrafficSimulate(opts trafficSimulateOptions, out io.Writer) error {
	// 1. Resolve lab directory and topology path
	labDir := opts.labPath
	if labDir == "" {
		labDir = defaultLabDir
	} else {
		var err error
		labDir, err = resolveLabDir(labDir)
		if err != nil {
			return err
		}
	}
	topologyPath := filepath.Join(labDir, labTopologyFile)

	// 2. Load topology and build graph
	topo, err := topology.LoadTopology(topologyPath)
	if err != nil {
		return fmt.Errorf("loading topology: %w", err)
	}
	g, err := sim.NewGraph(topo)
	if err != nil {
		return fmt.Errorf("building simulation graph: %w", err)
	}

	// 3. Load flows from JSON
	flowsFile := opts.flowsPath
	if !filepath.IsAbs(flowsFile) {
		flowsFile = filepath.Join(labDir, flowsFile)
	}
	flows, err := flowinput.LoadJSONFile(flowsFile)
	if err != nil {
		return fmt.Errorf("loading flows: %w", err)
	}
	if len(flows) == 0 {
		fmt.Fprintln(out, "no flows loaded")
		return nil
	}

	// 4. Build prefix universe from topology
	universe, err := model.NewPrefixUniverse(topo)
	if err != nil {
		return fmt.Errorf("building prefix universe: %w", err)
	}

	// 5. Classify flows into equivalence classes
	classifier := traffic.NewFlowClassifier(universe)
	ecs := classifier.Classify(flows)

	// 6. Simulate traffic
	eng := g.DataplaneEngine()
	idx := g.TopoIndex()
	simulator := traffic.NewSimulator(eng, idx)
	result, err := simulator.Simulate(ecs)
	if err != nil {
		return fmt.Errorf("simulating traffic: %w", err)
	}

	// 7. Output result
	return outputTrafficResult(out, result, opts.format)
}

func outputTrafficResult(out io.Writer, result model.TrafficResult, format string) error {
	switch format {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	default:
		return writeTrafficTable(out, result)
	}
}

func writeTrafficTable(out io.Writer, result model.TrafficResult) error {
	if len(result.LinkLoads) == 0 {
		fmt.Fprintln(out, "no link loads")
		return nil
	}
	// Sort by link name
	names := make([]string, 0, len(result.LinkLoads))
	for name := range result.LinkLoads {
		names = append(names, name)
	}
	sort.Strings(names)

	fmt.Fprintf(out, "%-40s %12s %12s %8s\n", "Link", "Bytes", "Capacity", "Util%")
	fmt.Fprintln(out, "-------------------------------------------+------------+------------+--------")
	for _, name := range names {
		ll := result.LinkLoads[name]
		utilPct := 0.0
		if ll.Capacity > 0 {
			utilPct = float64(ll.Bytes) * 8 / float64(ll.Capacity) * 100 // bytes to bits
		}
		fmt.Fprintf(out, "%-40s %12d %12d %7.2f%%\n", name, ll.Bytes, ll.Capacity, utilPct)
	}
	return nil
}
