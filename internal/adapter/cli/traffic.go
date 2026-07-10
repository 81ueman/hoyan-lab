package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
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

// totalBytesForSample returns the effective byte count after applying the
// sample rate. A rate of 0.5 means simulate 50% of the traffic.
func totalBytesForSample(base uint64, rate float64) uint64 {
	if rate >= 1.0 {
		return base
	}
	if rate <= 0.0 {
		return 0
	}
	return uint64(float64(base) * rate)
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

	allResults := make(map[string]map[string]uint64)
	baseBytes := totalBytesForSample(1000000, opts.sampleRate)

	// Parallel mode: simulate all classes concurrently in one call
	if opts.workers > 1 && len(packetClasses) > 1 {
		ecList := make([]trafficengine.FlowEquivalenceClass, 0, len(packetClasses))
		for _, pc := range packetClasses {
			ecList = append(ecList, trafficengine.FlowEquivalenceClass{
				Key:        trafficengine.FlowEquivalenceClassKeyFromPacketClass(pc, trafficengine.DSCPDefault),
				DstSet:     pc.DstSet,
				TotalBytes: baseBytes,
			})
		}
		linkLoads := sim.SimulateParallel(rootNode, ecList, fibs, opts.workers)
		allResults["all_classes"] = linkLoads
	} else {
		// Serial mode: simulate each class individually
		for _, pc := range packetClasses {
			linkLoads := sim.SimulateClass(rootNode, pc, fibs, baseBytes)
			allResults[fmt.Sprintf("class_%d", pc.ID)] = linkLoads
		}
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
// Snapshot definitions are loaded from the JSON file specified by --snapshots.
func runMultiSnapshot(sim *trafficengine.TrafficSimulator, opts trafficOptions, out io.Writer) error {
	topo, _, _, err := topology.LoadDomainTopologyWithRuntime(opts.topologyPath, topology.LoadOptions{})
	if err != nil {
		return fmt.Errorf("loading topology: %w", err)
	}

	if len(topo.Nodes) == 0 {
		return fmt.Errorf("topology has no nodes")
	}
	rootNode := topo.Nodes[0].Name

	// Load snapshot definitions from JSON file
	snapshotDefs, err := loadSnapshotDefs(opts.snapshotsPath)
	if err != nil {
		return fmt.Errorf("loading snapshots from %q: %w", opts.snapshotsPath, err)
	}
	if len(snapshotDefs) == 0 {
		return fmt.Errorf("no snapshot definitions found in %q", opts.snapshotsPath)
	}

	packetClasses := derivePacketClasses(topo.Nodes)

	var result model.MultiSnapshotResult
	for _, pc := range packetClasses {
		r := sim.SimulateMultiSnapshot(rootNode, pc, snapshotDefs, nil)
		result.Snapshots = append(result.Snapshots, r.Snapshots...)
		result.Diffs = append(result.Diffs, r.Diffs...)
	}

	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(result)
}

// loadSnapshotDefs loads snapshot definitions from a JSON file.
// The file should contain an array of SnapshotDef objects with label, fibs, and total_bytes.
func loadSnapshotDefs(path string) ([]trafficengine.SnapshotDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Wrap in a struct since JSON array top-level is valid
	var defs []trafficengine.SnapshotDef
	if err := json.Unmarshal(data, &defs); err != nil {
		return nil, fmt.Errorf("decoding snapshot definitions: %w", err)
	}
	return defs, nil
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
