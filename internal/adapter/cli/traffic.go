package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/engine/dataplane"
	simeng "github.com/81ueman/hoyan-lab/internal/engine/sim"
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
	cmd.Flags().StringVar(&opts.bandwidthPath, "bandwidth", "", "path to bandwidth override JSON file")

	// Register subcommands
	cmd.AddCommand(NewWhatIfCommand())
	cmd.AddCommand(NewKFailCommand())
	return cmd
}

type trafficOptions struct {
	topologyPath  string
	ecmpMode      string
	snapshotsPath string
	workers       int
	sampleRate    float64
	outputPath    string
	bandwidthPath string
	bandwidthData trafficengine.BandwidthOverride
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

	// Load bandwidth overrides if specified
	if opts.bandwidthPath != "" {
		overrides, err := loadBandwidthOverrides(opts.bandwidthPath)
		if err != nil {
			return ExitError{Code: 2, Err: fmt.Errorf("loading bandwidth overrides: %w", err)}
		}
		// Store overrides in traffic options for later use
		opts.bandwidthData = overrides
	}

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
// Traffic is simulated from each customer-facing ingress node and aggregated.
func runSingleSnapshot(sim *trafficengine.TrafficSimulator, opts trafficOptions, out io.Writer) error {
	topo, _, _, err := topology.LoadDomainTopologyWithRuntime(opts.topologyPath, topology.LoadOptions{})
	if err != nil {
		return fmt.Errorf("loading topology: %w", err)
	}

	// Apply bandwidth overrides if loaded
	if opts.bandwidthData != nil {
		trafficengine.ApplyBandwidthOverrides(topo, opts.bandwidthData)
	}

	if len(topo.Nodes) == 0 {
		return fmt.Errorf("topology has no nodes")
	}

	// Build graph with full routing simulation and derive FIB
	g, err := simeng.NewGraph(topo)
	if err != nil {
		return fmt.Errorf("building simulation graph: %w", err)
	}
	fibs := fibTableFromGraph(g)

	// Derive packet classes from the full network model (ACL, RIB, FIB)
	packetClasses, err := derivePacketClassesFromModel(topo, g)
	if err != nil {
		return fmt.Errorf("deriving packet classes: %w", err)
	}

	// Find ingress nodes (customer-facing) for multi-ingress simulation
	baseBytes := totalBytesForSample(1000000, opts.sampleRate)
	upstreamNodes := resolveUpstreamNodes(topo)

	// Aggregate link loads across all ingress nodes
	aggregated := make(map[string]uint64)

	for _, rootNode := range upstreamNodes {
		// Parallel mode: simulate all classes concurrently in one call
		if opts.workers > 1 && len(packetClasses) > 1 {
			ecList := buildECList(packetClasses, baseBytes)
			linkLoads := sim.SimulateParallel(rootNode, ecList, fibs, opts.workers)
			for link, bytes := range linkLoads {
				aggregated[link] += bytes
			}
		} else {
			// Serial mode: simulate each class individually
			for _, pc := range packetClasses {
				linkLoads := sim.SimulateClass(rootNode, pc, fibs, baseBytes)
				for link, bytes := range linkLoads {
					aggregated[link] += bytes
				}
			}
		}
	}

	allResults := make(map[string]map[string]uint64)
	allResults["all_ingress"] = aggregated

	output := map[string]interface{}{
		"ingress_nodes": upstreamNodes,
		"results":       allResults,
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
// Traffic is simulated from each customer-facing ingress node and aggregated.
func runMultiSnapshot(sim *trafficengine.TrafficSimulator, opts trafficOptions, out io.Writer) error {
	topo, _, _, err := topology.LoadDomainTopologyWithRuntime(opts.topologyPath, topology.LoadOptions{})
	if err != nil {
		return fmt.Errorf("loading topology: %w", err)
	}

	// Apply bandwidth overrides if loaded
	if opts.bandwidthData != nil {
		trafficengine.ApplyBandwidthOverrides(topo, opts.bandwidthData)
	}

	if len(topo.Nodes) == 0 {
		return fmt.Errorf("topology has no nodes")
	}

	// Load snapshot definitions from JSON file
	snapshotDefs, err := loadSnapshotDefs(opts.snapshotsPath)
	if err != nil {
		return fmt.Errorf("loading snapshots from %q: %w", opts.snapshotsPath, err)
	}
	if len(snapshotDefs) == 0 {
		return fmt.Errorf("no snapshot definitions found in %q", opts.snapshotsPath)
	}

	// Build graph with full routing simulation for packet class derivation
	g, err := simeng.NewGraph(topo)
	if err != nil {
		return fmt.Errorf("building simulation graph: %w", err)
	}

	// Derive packet classes from the full network model (ACL, RIB, FIB)
	packetClasses, err := derivePacketClassesFromModel(topo, g)
	if err != nil {
		return fmt.Errorf("deriving packet classes: %w", err)
	}

	upstreamNodes := resolveUpstreamNodes(topo)

	// Aggregate snapshots by label across all ingress nodes and packet classes
	snapshotAgg := make(map[string]map[string]uint64) // label → link → bytes

	for _, rootNode := range upstreamNodes {
		for _, pc := range packetClasses {
			r := sim.SimulateMultiSnapshot(rootNode, pc, snapshotDefs, nil)
			for _, snap := range r.Snapshots {
				if snapshotAgg[snap.Label] == nil {
					snapshotAgg[snap.Label] = make(map[string]uint64)
				}
				for _, ll := range snap.LinkLoads {
					snapshotAgg[snap.Label][ll.LinkName] += ll.Bytes
				}
			}
		}
	}

	// Build final result, sorted by label for deterministic output
	var result model.MultiSnapshotResult
	var labels []string
	for label := range snapshotAgg {
		labels = append(labels, label)
	}
	sort.Strings(labels)

	for _, label := range labels {
		linkLoads := make(map[string]model.LinkLoad)
		for link, bytes := range snapshotAgg[label] {
			linkLoads[link] = model.LinkLoad{LinkName: link, Bytes: bytes}
		}
		result.Snapshots = append(result.Snapshots, model.TrafficResult{
			Label:     label,
			LinkLoads: linkLoads,
		})
	}

	result.Diffs = trafficengine.ComputeDiffs(result.Snapshots)

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

// fibTableFromGraph converts a sim.Graph's FIB to the traffic engine's FIBTable format.
func fibTableFromGraph(g *simeng.Graph) trafficengine.FIBTable {
	fibs := trafficengine.FIBTable{}
	idx := g.TopoIndex()
	for _, node := range idx.Topology.Nodes {
		entries := g.FIB(model.NodeID(node.Name))
		var tfibEntries []trafficengine.TrafficFIBEntry
		for _, entry := range entries {
			nhs := convertNextHops(entry)
			if len(nhs) == 0 {
				continue
			}
			tfibEntries = append(tfibEntries, trafficengine.TrafficFIBEntry{
				Prefix:   entry.Prefix,
				NextHops: nhs,
			})
		}
		if len(tfibEntries) > 0 {
			fibs[node.Name] = tfibEntries
		}
	}
	return fibs
}

// convertNextHops converts dataplane FIB entry next-hops to traffic engine format.
func convertNextHops(entry dataplane.FIBEntry) []trafficengine.TrafficNextHop {
	if len(entry.NextHops) > 0 {
		result := make([]trafficengine.TrafficNextHop, len(entry.NextHops))
		for i, nh := range entry.NextHops {
			result[i] = trafficengine.TrafficNextHop{
				Node:   nh.Node,
				Weight: nh.Weight,
			}
		}
		return result
	}
	if entry.NextHop != "" {
		return []trafficengine.TrafficNextHop{{
			Node:   entry.NextHop,
			Weight: 1.0,
		}}
	}
	return nil
}

// loadBandwidthOverrides loads bandwidth overrides from a JSON file.
func loadBandwidthOverrides(path string) (trafficengine.BandwidthOverride, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var overrides trafficengine.BandwidthOverride
	if err := json.Unmarshal(data, &overrides); err != nil {
		return nil, fmt.Errorf("decoding bandwidth overrides: %w", err)
	}
	return overrides, nil
}

// derivePacketClassesFromModel creates packet classes from the full network model
// including ACL predicates, FIB entries, and RIB prefixes — not just node.Prefixes.
// This yields Protocol, Port, and Interface dimensions in addition to destination prefixes.
func derivePacketClassesFromModel(topo *model.Topology, graph *simeng.Graph) ([]model.PacketClass, error) {
	predicates := model.CollectPrefixPredicateMetadata(topo)
	predicates = append(predicates, simeng.CollectRIBPrefixPredicates(graph)...)
	predicates = append(predicates, simeng.CollectFIBPrefixPredicates(graph)...)

	universe, err := model.BuildPrefixUniverseFromPredicates(predicates)
	if err != nil {
		return nil, fmt.Errorf("building prefix universe: %w", err)
	}

	headerSpace := model.NewHeaderSpace(topo, universe)
	return headerSpace.Classes, nil
}

// buildECList converts packet classes into flow equivalence classes for simulation.
func buildECList(classes []model.PacketClass, baseBytes uint64) []trafficengine.FlowEquivalenceClass {
	ecList := make([]trafficengine.FlowEquivalenceClass, 0, len(classes))
	for _, pc := range classes {
		ecList = append(ecList, trafficengine.FlowEquivalenceClass{
			Key:        trafficengine.FlowEquivalenceClassKeyFromPacketClass(pc, trafficengine.DSCPDefault),
			DstSet:     pc.DstSet,
			TotalBytes: baseBytes,
		})
	}
	return ecList
}

// ---------------------------------------------------------------------------
// What-if subcommand
// ---------------------------------------------------------------------------

type whatIfOptions struct {
	topologyPath  string
	failLinks     []string
	failNodes     []string
	format        string
	ecmpMode      string
	sampleRate    float64
	workers       int
	bandwidthPath string
}

func NewWhatIfCommand() *cobra.Command {
	var opts whatIfOptions
	cmd := &cobra.Command{
		Use:   "what-if <topology-path>",
		Short: "What-if failure analysis for traffic simulation",
		Long: `Simulate traffic under link/node failures and compare with the base case.

Examples:
  hoyan traffic what-if labs/base-wan --fail-link l2-core-bj
  hoyan traffic what-if labs/base-wan --fail-link l2-core-bj --fail-node core-hz --format table
`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.topologyPath = args[0]
			return runWhatIf(cmd, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringArrayVar(&opts.failLinks, "fail-link", nil, "link name to fail (can be repeated)")
	cmd.Flags().StringArrayVar(&opts.failNodes, "fail-node", nil, "node name to fail (can be repeated)")
	cmd.Flags().StringVar(&opts.format, "format", "table", "output format: table or json")
	cmd.Flags().StringVar(&opts.ecmpMode, "ecmp-mode", "uniform", "ECMP mode: uniform or hash")
	cmd.Flags().Float64Var(&opts.sampleRate, "sample-rate", 1.0, "flow sampling rate (0.0-1.0)")
	cmd.Flags().IntVar(&opts.workers, "workers", runtime.GOMAXPROCS(0), "parallelism for simulation")
	cmd.Flags().StringVar(&opts.bandwidthPath, "bandwidth", "", "path to bandwidth override JSON file")
	return cmd
}

func runWhatIf(cmd *cobra.Command, opts whatIfOptions, out io.Writer) error {
	// Parse ECMP mode
	ecmpMode, err := parseECMPMode(opts.ecmpMode)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}

	// Load topology
	topo, _, _, err := topology.LoadDomainTopologyWithRuntime(opts.topologyPath, topology.LoadOptions{})
	if err != nil {
		return fmt.Errorf("loading topology: %w", err)
	}
	if len(topo.Nodes) == 0 {
		return fmt.Errorf("topology has no nodes")
	}

	// Apply bandwidth overrides if specified
	if opts.bandwidthPath != "" {
		overrides, err := loadBandwidthOverrides(opts.bandwidthPath)
		if err != nil {
			return ExitError{Code: 2, Err: fmt.Errorf("loading bandwidth overrides: %w", err)}
		}
		trafficengine.ApplyBandwidthOverrides(topo, overrides)
	}

	// Build graph with full routing simulation and derive FIB
	g, err := simeng.NewGraph(topo)
	if err != nil {
		return fmt.Errorf("building simulation graph: %w", err)
	}
	fibs := fibTableFromGraph(g)

	// Derive packet classes from the full network model (ACL, RIB, FIB)
	packetClasses, err := derivePacketClassesFromModel(topo, g)
	if err != nil {
		return fmt.Errorf("deriving packet classes: %w", err)
	}

	// Build failure set: convert []string to []model.LinkID/NodeID
	linkIDs := make([]model.LinkID, len(opts.failLinks))
	for i, name := range opts.failLinks {
		linkIDs[i] = model.LinkID(name)
	}
	nodeIDs := make([]model.NodeID, len(opts.failNodes))
	for i, name := range opts.failNodes {
		nodeIDs[i] = model.NodeID(name)
	}
	failSet := failure.NewSet(linkIDs, nodeIDs)

	// Create cache and what-if simulator
	cache := trafficengine.NewTDGCache()
	simConfig := trafficengine.SimulatorConfig{ECMPMode: ecmpMode}
	ws := trafficengine.NewWhatIfSimulator(simConfig)

	// Build EC list from packet classes
	baseBytes := totalBytesForSample(1000000, opts.sampleRate)
	ecList := buildECList(packetClasses, baseBytes)

	// Find ingress nodes for multi-ingress simulation
	upstreamNodes := resolveUpstreamNodes(topo)

	// Aggregate base and failed loads across all ingress nodes
	aggregatedBase := make(map[string]uint64)
	aggregatedFailed := make(map[string]uint64)

	for _, rootNode := range upstreamNodes {
		// Simulate base case (populates cache)
		baseResult := ws.Simulate(rootNode, failure.None(), ecList, cache, fibs)
		if baseResult != nil {
			for link := range baseResult.LinkLoads {
				aggregatedBase[link] += baseResult.LinkLoads[link].Bytes
			}
		}

		// Simulate with the specified failure
		failResult := ws.Simulate(rootNode, failSet, ecList, cache, fibs)
		if failResult != nil {
			for link := range failResult.LinkLoads {
				aggregatedFailed[link] += failResult.LinkLoads[link].Bytes
			}
		}
	}

	// Create aggregated result with diffs from aggregated loads
	result := &trafficengine.WhatIfResult{
		Failure:   failSet,
		LinkLoads: make(map[string]model.LinkLoad),
		Diffs:     computeLinkLoadChanges(aggregatedBase, aggregatedFailed),
	}
	for link, bytes := range aggregatedFailed {
		result.LinkLoads[link] = model.LinkLoad{LinkName: link, Bytes: bytes}
	}

	// Format output
	switch opts.format {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	case "table":
		return formatWhatIfTable(out, result)
	default:
		return fmt.Errorf("unsupported --format %q (use 'table' or 'json')", opts.format)
	}
}

func formatWhatIfTable(out io.Writer, result *trafficengine.WhatIfResult) error {
	// Print failure summary
	fmt.Fprintf(out, "WHAT-IF ANALYSIS\n")
	fmt.Fprintf(out, "%-20s %s\n", "Failures:", formatFailureSet(result.Failure))
	fmt.Fprintln(out, "")

	// Header
	fmt.Fprintf(out, "%-25s %-12s %-12s %-12s %s\n", "Link", "Before", "After", "Delta", "Status")
	fmt.Fprintf(out, "%-25s %-12s %-12s %-12s %s\n", "----", "------", "-----", "-----", "------")

	// Build index of changed link names
	changed := make(map[string]bool)
	for _, diff := range result.Diffs {
		changed[diff.LinkName] = true
	}

	// Collect all link names and sort for deterministic output
	allLinks := make([]string, 0, len(result.LinkLoads))
	for link := range result.LinkLoads {
		allLinks = append(allLinks, link)
	}
	sort.Strings(allLinks)

	for _, link := range allLinks {
		ll := result.LinkLoads[link]
		if changed[link] {
			// Find the matching diff
			for _, diff := range result.Diffs {
				if diff.LinkName == link {
					status := formatStatus(diff)
					fmt.Fprintf(out, "%-25s %-12s %-12s %-12s %s\n",
						link,
						formatBytes(diff.Before),
						formatBytes(diff.After),
						formatDelta(diff.Delta, diff.DeltaPct),
						status)
					break
				}
			}
		} else {
			// Unchanged link
			fmt.Fprintf(out, "%-25s %-12s %-12s %-12s %s\n",
				link,
				formatBytes(ll.Bytes),
				formatBytes(ll.Bytes),
				"0",
				"")
		}
	}

	return nil
}

func formatFailureSet(fs failure.Set) string {
	parts := make([]string, 0, len(fs.Links)+len(fs.Nodes))
	for link := range fs.Links {
		parts = append(parts, fmt.Sprintf("link:%s down", string(link)))
	}
	for node := range fs.Nodes {
		parts = append(parts, fmt.Sprintf("node:%s down", string(node)))
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func formatBytes(b uint64) string {
	switch {
	case b >= 1_000_000_000:
		return fmt.Sprintf("%.1f GB", float64(b)/1_000_000_000)
	case b >= 1_000_000:
		return fmt.Sprintf("%.1f MB", float64(b)/1_000_000)
	case b >= 1_000:
		return fmt.Sprintf("%.1f KB", float64(b)/1_000)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

func formatDelta(delta int64, pct float64) string {
	sign := "+"
	if delta < 0 {
		sign = ""
	}
	if pct == 0 {
		return "0"
	}
	if math.IsInf(pct, 1) {
		return fmt.Sprintf("%s∞", sign)
	}
	if math.IsInf(pct, -1) {
		return "-∞"
	}
	return fmt.Sprintf("%s%d (%.0f%%)", sign, delta, pct)
}

func formatStatus(diff trafficengine.LinkLoadChange) string {
	if diff.Delta > 0 && diff.DeltaPct > 50 {
		return "⚠ OVERLOAD"
	}
	if diff.Delta < 0 && diff.DeltaPct < -50 {
		return "↓ DRASTIC"
	}
	if diff.Delta > 0 {
		return "↑ INCREASE"
	}
	if diff.Delta < 0 {
		return "↓ DECREASE"
	}
	return ""
}

// ---------------------------------------------------------------------------
// k-failure subcommand
// ---------------------------------------------------------------------------

type kFailOptions struct {
	topologyPath  string
	threshold     float64
	maxK          int
	format        string
	ecmpMode      string
	sampleRate    float64
	bandwidthPath string
}

func NewKFailCommand() *cobra.Command {
	var opts kFailOptions
	cmd := &cobra.Command{
		Use:   "kfail <topology-path>",
		Short: "k-failure tolerance analysis for traffic",
		Long: `Analyze the minimum number of additional failures that would
cause overload on any link.

Examples:
  hoyan traffic kfail labs/base-wan --threshold 80 --max-k 2
`,
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.topologyPath = args[0]
			return runKFail(cmd, opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().Float64Var(&opts.threshold, "threshold", 80, "utilization threshold percentage")
	cmd.Flags().IntVar(&opts.maxK, "max-k", 2, "maximum number of simultaneous failures")
	cmd.Flags().StringVar(&opts.format, "format", "text", "output format: text or json")
	cmd.Flags().StringVar(&opts.ecmpMode, "ecmp-mode", "uniform", "ECMP mode: uniform or hash")
	cmd.Flags().Float64Var(&opts.sampleRate, "sample-rate", 1.0, "flow sampling rate (0.0-1.0)")
	cmd.Flags().StringVar(&opts.bandwidthPath, "bandwidth", "", "path to bandwidth override JSON file")
	return cmd
}

func runKFail(cmd *cobra.Command, opts kFailOptions, out io.Writer) error {
	// Parse ECMP mode
	ecmpMode, err := parseECMPMode(opts.ecmpMode)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}

	// Load topology
	topo, _, _, err := topology.LoadDomainTopologyWithRuntime(opts.topologyPath, topology.LoadOptions{})
	if err != nil {
		return fmt.Errorf("loading topology: %w", err)
	}
	if len(topo.Nodes) == 0 {
		return fmt.Errorf("topology has no nodes")
	}

	// Apply bandwidth overrides if specified
	if opts.bandwidthPath != "" {
		overrides, err := loadBandwidthOverrides(opts.bandwidthPath)
		if err != nil {
			return ExitError{Code: 2, Err: fmt.Errorf("loading bandwidth overrides: %w", err)}
		}
		trafficengine.ApplyBandwidthOverrides(topo, overrides)
	}

	// Build graph with full routing simulation and derive FIB
	g, err := simeng.NewGraph(topo)
	if err != nil {
		return fmt.Errorf("building simulation graph: %w", err)
	}
	fibs := fibTableFromGraph(g)

	// Get upstream nodes (customer-facing ingress resolved to upstream)
	upstreamNodes := resolveUpstreamNodes(topo)

	// Derive packet classes from the full network model (ACL, RIB, FIB)
	packetClasses, err := derivePacketClassesFromModel(topo, g)
	if err != nil {
		return fmt.Errorf("deriving packet classes: %w", err)
	}

	// Build EC list from packet classes
	baseBytes := totalBytesForSample(1000000, opts.sampleRate)
	ecList := buildECList(packetClasses, baseBytes)

	// Run k-failure analysis across all ingress nodes with dedup
	analyzer := trafficengine.NewKFailAnalyzer(trafficengine.SimulatorConfig{ECMPMode: ecmpMode})
	allFindings := make([]trafficengine.KFailFinding, 0)
	seen := make(map[string]bool) // dedup key: "linkName|k|failures-sorted"

	for _, rootNode := range upstreamNodes {
		result := analyzer.Analyze(rootNode, topo, fibs, ecList, opts.threshold, opts.maxK)
		if result == nil {
			continue
		}
		for _, f := range result.Findings {
			// Build dedup key from link name, k, and sorted failure strings
			failStrs := make([]string, len(f.Failures))
			for i, elem := range f.Failures {
				failStrs[i] = elem.String()
			}
			sort.Strings(failStrs)
			key := f.LinkName + "|" + fmt.Sprintf("%d", f.K) + "|" + strings.Join(failStrs, ",")
			if seen[key] {
				continue
			}
			seen[key] = true
			allFindings = append(allFindings, f)
		}
	}
	result := &trafficengine.KFailResult{Findings: allFindings}

	// Format output
	switch opts.format {
	case "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	case "text":
		return formatKFailText(out, result, opts.threshold)
	default:
		return fmt.Errorf("unsupported --format %q (use 'text' or 'json')", opts.format)
	}
}

func formatKFailText(out io.Writer, result *trafficengine.KFailResult, threshold float64) error {
	if len(result.Findings) == 0 {
		fmt.Fprintln(out, "No links exceed the utilization threshold.")
		return nil
	}

	for _, f := range result.Findings {
		if f.K == 0 {
			fmt.Fprintf(out, "%s exceeds %.0f%% utilization with k=0 (base overload)\n",
				f.LinkName, f.UtilizationPct)
		} else if len(f.Failures) > 0 {
			// Format failure list: e.g. "link:l2-core-bj down"
			failParts := make([]string, 0, len(f.Failures))
			for _, elem := range f.Failures {
				failParts = append(failParts, fmt.Sprintf("%s:%s down", elem.Kind, elem.Name))
			}
			fmt.Fprintf(out, "%s fails → %s exceeds %.0f%% with k=%d\n",
				strings.Join(failParts, ", "),
				f.LinkName,
				threshold,
				f.K)
		} else {
			fmt.Fprintf(out, "%s exceeds threshold with k=%d\n",
				f.LinkName, f.K)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Multi-ingress helpers
// ---------------------------------------------------------------------------

// findIngressNodes returns the names of customer-facing nodes that should
// be used as traffic ingress points. A node is considered an ingress if its
// Role is "customer" or its name has the "cust-" prefix.
func findIngressNodes(nodes []model.Node) []string {
	var ingressNodes []string
	for _, node := range nodes {
		if node.Role == "customer" || strings.HasPrefix(node.Name, "cust-") {
			ingressNodes = append(ingressNodes, node.Name)
		}
	}
	sort.Strings(ingressNodes)
	return ingressNodes
}

// findUpstreamNode returns the first upstream neighbor of the given node
// by looking at topology links. If no neighbor is found, returns the node
// itself as a safe fallback.
func findUpstreamNode(name string, topo *model.Topology) string {
	for _, link := range topo.Links {
		if link.A == name {
			return link.B
		}
		if link.B == name {
			return link.A
		}
	}
	return name
}

// resolveUpstreamNodes finds customer-facing ingress nodes and resolves each
// to its upstream (edge) node for traffic simulation. If no customer nodes
// exist, it falls back to the first topology node to preserve backward
// compatibility with non-customer topologies.
func resolveUpstreamNodes(topo *model.Topology) []string {
	ingressNodes := findIngressNodes(topo.Nodes)
	if len(ingressNodes) == 0 {
		// Fallback: no customer nodes, use first node as root
		return []string{topo.Nodes[0].Name}
	}
	upstreamNodes := make([]string, 0, len(ingressNodes))
	for _, ingress := range ingressNodes {
		upstreamNodes = append(upstreamNodes, findUpstreamNode(ingress, topo))
	}
	return upstreamNodes
}

// computeLinkLoadChanges computes diffs between base and failed link loads.
// This mirrors trafficengine.computeLinkLoadChanges but is in the cli package
// so it can be used for multi-ingress result aggregation.
func computeLinkLoadChanges(base, failed map[string]uint64) []trafficengine.LinkLoadChange {
	allLinks := make(map[string]bool)
	for link := range base {
		allLinks[link] = true
	}
	for link := range failed {
		allLinks[link] = true
	}

	var changes []trafficengine.LinkLoadChange
	for link := range allLinks {
		before := base[link]
		after := failed[link]
		delta := int64(after) - int64(before)
		if delta == 0 {
			continue
		}

		var deltaPct float64
		if before > 0 {
			deltaPct = float64(delta) / float64(before) * 100.0
			deltaPct = math.Round(deltaPct*100) / 100
		} else {
			deltaPct = math.Inf(1)
		}

		changes = append(changes, trafficengine.LinkLoadChange{
			LinkName: link,
			Before:   before,
			After:    after,
			Delta:    delta,
			DeltaPct: deltaPct,
		})
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].LinkName < changes[j].LinkName
	})

	return changes
}
