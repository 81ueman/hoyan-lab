package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	trafficengine "github.com/81ueman/hoyan-lab/internal/engine/traffic"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
	"github.com/spf13/cobra"
)

func NewTrafficCommand() *cobra.Command {
	var opts trafficOptions
	cmd := &cobra.Command{
		Use:          "traffic <topology-path>",
		Short:        "Simulate traffic distribution through the network",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.topologyPath = args[0]
			return runTraffic(cmd, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.ecmpMode, "ecmp-mode", "uniform", "ECMP mode: uniform or hash")
	cmd.Flags().StringVar(&opts.snapshotsPath, "snapshots", "", "path to multi-snapshot JSON file")
	cmd.Flags().IntVar(&opts.workers, "workers", runtime.GOMAXPROCS(0), "parallelism for simulation")
	cmd.Flags().Float64Var(&opts.sampleRate, "sample-rate", 1.0, "flow sampling rate (0.0-1.0)")
	cmd.Flags().StringVar(&opts.outputPath, "output", "", "output file path (default: stdout)")
	return cmd
}

type trafficOptions struct {
	topologyPath  string
	ecmpMode      string
	snapshotsPath string
	workers       int
	sampleRate    float64
	outputPath    string
}

func runTraffic(cmd *cobra.Command, opts trafficOptions, out io.Writer) error {
	ecmpMode, err := parseECMPMode(opts.ecmpMode)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}

	if opts.sampleRate < 0.0 || opts.sampleRate > 1.0 {
		return ExitError{Code: 2, Err: fmt.Errorf("--sample-rate must be between 0.0 and 1.0, got %f", opts.sampleRate)}
	}

	simConfig := trafficengine.SimulatorConfig{ECMPMode: ecmpMode}
	sim := trafficengine.NewTrafficSimulator(simConfig)

	if opts.snapshotsPath != "" {
		return runMultiSnapshot(sim, opts, out)
	}
	return runSingleSnapshot(sim, opts, out)
}

func parseECMPMode(raw string) (trafficengine.ECMPMode, error) {
	switch strings.ToLower(raw) {
	case "uniform":
		return trafficengine.ECMPModeUniform, nil
	case "hash":
		return trafficengine.ECMPModeHash, nil
	default:
		return 0, fmt.Errorf("--ecmp-mode must be 'uniform' or 'hash', got %q", raw)
	}
}

// runSingleSnapshot runs a single traffic simulation using a topology file.
func runSingleSnapshot(sim *trafficengine.TrafficSimulator, opts trafficOptions, out io.Writer) error {
	topo, _, _, err := topology.LoadDomainTopologyWithRuntime(opts.topologyPath, topology.LoadOptions{})
	if err != nil {
		return fmt.Errorf("loading topology: %w", err)
	}

	if len(topo.Nodes) == 0 {
		return fmt.Errorf("topology has no nodes")
	}
	rootNode := topo.Nodes[0].Name

	// Build FIB table from topology destinations
	fibs := buildFIBFromTopoNodes(topo.Nodes)

	// Derive packet classes from topology
	packetClasses := derivePacketClasses(topo.Nodes)

	classifier := trafficengine.NewClassifier(trafficengine.SamplingConfig{
		Rate:     opts.sampleRate,
		Strategy: trafficengine.SamplingRandom,
	})

	allResults := make(map[string]map[string]uint64)
	for _, pc := range packetClasses {
		// Classify flows for this packet class
		ecs := classifier.ClassifyFlowsFromPacketClass(pc, nil)
		_ = ecs

		var linkLoads map[string]uint64
		if opts.workers > 1 && len(packetClasses) > 1 {
			ecList := make([]trafficengine.FlowEquivalenceClass, 0, len(packetClasses))
			for _, pc2 := range packetClasses {
				ecList = append(ecList, trafficengine.FlowEquivalenceClass{
					Key:    trafficengine.FlowEquivalenceClassKeyFromPacketClass(pc2, trafficengine.DSCPDefault),
					DstSet: pc2.DstSet,
				})
			}
			linkLoads = sim.SimulateParallel(rootNode, ecList, fibs, opts.workers)
		} else {
			linkLoads = sim.SimulateClass(rootNode, pc, fibs, 1000000)
		}
		allResults[fmt.Sprintf("class_%d", pc.ID)] = linkLoads
	}

	output := map[string]interface{}{
		"root_node": rootNode,
		"results":   allResults,
		"config": map[string]interface{}{
			"ecmp_mode":   opts.ecmpMode,
			"workers":     opts.workers,
			"sample_rate": opts.sampleRate,
		},
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(output)
}

// runMultiSnapshot runs multi-snapshot traffic simulation.
func runMultiSnapshot(sim *trafficengine.TrafficSimulator, opts trafficOptions, out io.Writer) error {
	topo, _, _, err := topology.LoadDomainTopologyWithRuntime(opts.topologyPath, topology.LoadOptions{})
	if err != nil {
		return fmt.Errorf("loading topology: %w", err)
	}

	if len(topo.Nodes) == 0 {
		return fmt.Errorf("topology has no nodes")
	}
	rootNode := topo.Nodes[0].Name
	fibs := buildFIBFromTopoNodes(topo.Nodes)
	packetClasses := derivePacketClasses(topo.Nodes)

	// Build snapshot definitions from the topology FIB
	snapshotDefs := []trafficengine.SnapshotDef{
		{
			Label:      "baseline",
			FIBs:       fibs,
			TotalBytes: 1000000,
		},
	}

	classifier := trafficengine.NewClassifier(trafficengine.SamplingConfig{
		Rate:     opts.sampleRate,
		Strategy: trafficengine.SamplingRandom,
	})

	var result model.MultiSnapshotResult
	for _, pc := range packetClasses {
		ecs := classifier.ClassifyFlowsFromPacketClass(pc, nil)
		_ = ecs
		r := sim.SimulateMultiSnapshot(rootNode, pc, snapshotDefs, nil)
		result.Snapshots = append(result.Snapshots, r.Snapshots...)
		result.Diffs = append(result.Diffs, r.Diffs...)
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// buildFIBFromTopoNodes creates a traffic FIB table from topology nodes.
// Each directly connected prefix becomes a FIB entry pointing to the
// adjacent node.
func buildFIBFromTopoNodes(nodes []model.Node) trafficengine.FIBTable {
	fibs := trafficengine.FIBTable{}
	for _, node := range nodes {
		if _, ok := fibs[node.Name]; !ok {
			fibs[node.Name] = nil
		}
		for _, prefix := range node.Prefixes {
			entry := trafficengine.TrafficFIBEntry{
				Prefix: prefix.NetIP(),
			}
			// If there's an adjacent node that owns this prefix,
			// create a next-hop to it
			entry.NextHops = []trafficengine.TrafficNextHop{
				{Node: node.Name, Weight: 1.0},
			}
			fibs[node.Name] = append(fibs[node.Name], entry)
		}
	}
	return fibs
}

// derivePacketClasses creates packet classes from topology node prefixes.
func derivePacketClasses(nodes []model.Node) []model.PacketClass {
	var classes []model.PacketClass
	seen := map[string]bool{}
	for _, node := range nodes {
		for _, prefix := range node.Prefixes {
			key := prefix.String()
			if seen[key] {
				continue
			}
			seen[key] = true
			classes = append(classes, model.PacketClass{
				ID:            model.PacketClassID(len(classes)),
				PrefixClassID: model.PrefixClassID(len(classes)),
				DstSet:        model.ExactPrefixSet{Prefix: prefix},
			})
		}
	}
	return classes
}
