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
	rootNode := topo.Nodes[0].Name

	// Build FIB table from topology
	fibs := buildFIBFromTopo(topo)

	// Derive packet classes from topology
	packetClasses := derivePacketClasses(topo.Nodes)

	allResults := make(map[string]map[string]uint64)
	baseBytes := totalBytesForSample(1000000, opts.sampleRate)

	// Parallel mode: simulate all classes concurrently in one call
	if opts.workers > 1 && len(packetClasses) > 1 {
		ecList := buildECList(packetClasses, baseBytes)
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

	// Apply bandwidth overrides if loaded
	if opts.bandwidthData != nil {
		trafficengine.ApplyBandwidthOverrides(topo, opts.bandwidthData)
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

// buildFIBFromTopo creates a traffic FIB table from topology nodes and links.
// Each prefix is matched against link neighbors: if an adjacent node also
// owns the prefix, a next-hop edge is created to that neighbor.
func buildFIBFromTopo(topo *model.Topology) trafficengine.FIBTable {
	fibs := trafficengine.FIBTable{}

	// Build prefix → owning nodes index
	prefixNodes := map[string][]string{}
	for _, node := range topo.Nodes {
		for _, prefix := range node.Prefixes {
			key := prefix.String()
			prefixNodes[key] = append(prefixNodes[key], node.Name)
		}
	}

	// Build adjacency from links
	adj := map[string][]string{}
	for _, link := range topo.Links {
		adj[link.A] = append(adj[link.A], link.B)
		adj[link.B] = append(adj[link.B], link.A)
	}

	for _, node := range topo.Nodes {
		for _, prefix := range node.Prefixes {
			key := prefix.String()
			var nextHops []trafficengine.TrafficNextHop
			for _, peer := range adj[node.Name] {
				for _, pn := range prefixNodes[key] {
					if pn == peer {
						nextHops = append(nextHops, trafficengine.TrafficNextHop{
							Node:   peer,
							Weight: 1.0,
						})
					}
				}
			}
			if len(nextHops) == 0 {
				continue // sink — no adjacent node to forward to
			}
			fibs[node.Name] = append(fibs[node.Name], trafficengine.TrafficFIBEntry{
				Prefix:   prefix.NetIP(),
				NextHops: nextHops,
			})
		}
	}
	return fibs
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

	// Build FIB table from topology
	fibs := buildFIBFromTopo(topo)

	// Derive packet classes
	packetClasses := derivePacketClasses(topo.Nodes)

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

	// Get root node (first node in topology)
	rootNode := ""
	for node := range fibs {
		rootNode = node
		break
	}

	// First simulate base case to populate cache and get base loads
	ws.Simulate(rootNode, failure.None(), ecList, cache, fibs)

	// Then simulate with the specified failure
	result := ws.Simulate(rootNode, failSet, ecList, cache, fibs)
	if result == nil {
		return fmt.Errorf("what-if simulation returned nil")
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

	// Build FIB table from topology
	fibs := buildFIBFromTopo(topo)

	// Get root node (first node with FIB entries)
	var rootNode string
	for node := range fibs {
		rootNode = node
		break
	}
	if rootNode == "" {
		return fmt.Errorf("no FIB entries in topology")
	}

	// Derive packet classes
	packetClasses := derivePacketClasses(topo.Nodes)

	// Build EC list from packet classes
	baseBytes := totalBytesForSample(1000000, opts.sampleRate)
	ecList := buildECList(packetClasses, baseBytes)

	// Run k-failure analysis
	analyzer := trafficengine.NewKFailAnalyzer(trafficengine.SimulatorConfig{ECMPMode: ecmpMode})
	result := analyzer.Analyze(rootNode, topo, fibs, ecList, opts.threshold, opts.maxK)

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
