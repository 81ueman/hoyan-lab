package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/adapter/inputhash"
	liveexec "github.com/81ueman/hoyan-lab/internal/adapter/live"
	livefib "github.com/81ueman/hoyan-lab/internal/adapter/live/fib"
	liverib "github.com/81ueman/hoyan-lab/internal/adapter/live/rib"
	"github.com/81ueman/hoyan-lab/internal/adapter/snapshotfile"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	fibcompare "github.com/81ueman/hoyan-lab/internal/usecase/fib"
	"github.com/81ueman/hoyan-lab/internal/usecase/livesnapshot"
	ribcompare "github.com/81ueman/hoyan-lab/internal/usecase/rib"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
	"github.com/spf13/cobra"
)

func NewCompareCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "compare",
		Short:         "Compare modeled state with live device state",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(NewRIBCompareCommand(), NewCompareFIBEntriesCommand())
	return cmd
}

func NewRIBCompareCommand() *cobra.Command {
	var opts ribCompareOptions
	cmd := &cobra.Command{
		Use:           "rib",
		Short:         "Compare modeled RIBs with live device RIBs",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := resolveLabInputs(cmd, opts.labPath, &opts.topologyPath, nil); err != nil {
				return err
			}
			if err := runRIBCompare(cmd.Context(), opts, cmd.OutOrStdout()); err != nil {
				return err
			}
			return nil
		},
	}
	addLabFlag(cmd, &opts.labPath)
	addTopologyFlag(cmd, &opts.topologyPath, "containerlab topology YAML")
	cmd.Flags().BoolVar(&opts.strictConfig, "strict-config", false, "fail on unsupported config parser statements")
	cmd.Flags().StringVar(&opts.snapshotPath, "snapshot", "", "live snapshot JSON to use instead of collecting from devices")
	cmd.Flags().StringVar(&opts.snapshotHashPolicy, "snapshot-hash-policy", string(livesnapshot.HashPolicyWarn), "handling for snapshot topology/config hash mismatch: warn, fail, or ignore")
	return cmd
}

type ribCompareOptions struct {
	labPath            string
	topologyPath       string
	strictConfig       bool
	snapshotPath       string
	snapshotHashPolicy string
}

func runRIBCompare(ctx context.Context, opts ribCompareOptions, out io.Writer) error {
	if _, ok := livesnapshot.ParseHashPolicy(opts.snapshotHashPolicy); !ok {
		return ExitError{Code: 2, Err: fmt.Errorf("snapshot hash policy must be one of warn, fail, or ignore")}
	}
	topo, _, err := topology.LoadTopologyWithOptions(opts.topologyPath, topology.LoadOptions{StrictConfig: opts.strictConfig})
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	nodes := liverib.SupportedNodes(topo.Nodes)
	expected := (ribcompare.ExpectedBuilder{}).BuildForNodes(topo, nodes)
	fmt.Fprintf(out, "comparing RIB routes (sources: %s)\n", observation.FormatSourceSummary(observation.SourceSummary(expected)))
	var actual []observation.RIBRoute
	if opts.snapshotPath != "" {
		snap, err := snapshotfile.Load(opts.snapshotPath)
		if err != nil {
			return ExitError{Code: 2, Err: err}
		}
		if err := checkSnapshotHashes(opts.topologyPath, snap, opts.snapshotHashPolicy, out); err != nil {
			return err
		}
		actual = livesnapshot.AllRIBRoutes(snap)
	} else {
		actual, err = ribcompare.New(liverib.NewCollector(liveexec.ExecRunner{})).Collect(ctx, nodes)
		if err != nil {
			return ExitError{Code: 2, Err: err}
		}
	}
	result := observation.CompareRoutes(expected, actual, observation.DefaultCompareOptions())
	for _, line := range observation.FormatDiffs(result) {
		fmt.Fprintln(out, line)
	}
	if !result.OK {
		return ExitError{Code: 1, Err: fmt.Errorf("RIB comparison found diff(s)")}
	}
	fmt.Fprintln(out, "RIBs match expected modeled paths")
	return nil
}

func NewCompareFIBEntriesCommand() *cobra.Command {
	var opts fibCompareOptions
	cmd := &cobra.Command{
		Use:           "fib",
		Short:         "Compare modeled FIBs with live installed kernel FIBs",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := resolveLabInputs(cmd, opts.labPath, &opts.topologyPath, nil); err != nil {
				return err
			}
			return runCompareFIBEntries(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	addLabFlag(cmd, &opts.labPath)
	addTopologyFlag(cmd, &opts.topologyPath, "containerlab topology YAML")
	cmd.Flags().BoolVar(&opts.strictConfig, "strict-config", false, "fail on unsupported config parser statements")
	cmd.Flags().BoolVar(&opts.allowUnsupported, "allow-unsupported", false, "skip nodes without a live FIB collector")
	cmd.Flags().StringVar(&opts.unresolvedPolicy, "unresolved-policy", string(observation.UnresolvedPolicyWarn), "handling for unresolved live BGP FIB routes: warn, fail, or ignore")
	cmd.Flags().StringVar(&opts.snapshotPath, "snapshot", "", "live snapshot JSON to use instead of collecting from devices")
	cmd.Flags().StringVar(&opts.snapshotHashPolicy, "snapshot-hash-policy", string(livesnapshot.HashPolicyWarn), "handling for snapshot topology/config hash mismatch: warn, fail, or ignore")
	return cmd
}

type fibCompareOptions struct {
	labPath            string
	topologyPath       string
	strictConfig       bool
	allowUnsupported   bool
	unresolvedPolicy   string
	snapshotPath       string
	snapshotHashPolicy string
}

func runCompareFIBEntries(ctx context.Context, opts fibCompareOptions, out io.Writer) error {
	if err := validateFIBUnresolvedPolicy(opts.unresolvedPolicy); err != nil {
		return ExitError{Code: 2, Err: err}
	}
	if _, ok := livesnapshot.ParseHashPolicy(opts.snapshotHashPolicy); !ok {
		return ExitError{Code: 2, Err: fmt.Errorf("snapshot hash policy must be one of warn, fail, or ignore")}
	}
	topo, _, err := topology.LoadTopologyWithOptions(opts.topologyPath, topology.LoadOptions{StrictConfig: opts.strictConfig})
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	nodes := topo.Nodes
	if opts.allowUnsupported {
		nodes = livefib.NewCollector(nil).SupportedNodes(nodes)
	}
	fibOpts := observation.Options{AllowUnsupported: opts.allowUnsupported, UnresolvedPolicy: observation.UnresolvedPolicy(opts.unresolvedPolicy)}
	expected := observation.AnalyzeComparableRoutes(topo, fibcompare.NewExpectedBuilder().ExpectedFIBsForNodes(topo, nodes), fibOpts)
	var actualFiltered observation.FilterResult
	if opts.snapshotPath != "" {
		snap, err := snapshotfile.Load(opts.snapshotPath)
		if err != nil {
			return ExitError{Code: 2, Err: err}
		}
		if err := checkSnapshotHashes(opts.topologyPath, snap, opts.snapshotHashPolicy, out); err != nil {
			return err
		}
		actualFiltered = observation.AnalyzeComparableRoutes(topo, livesnapshot.FIBs(snap), fibOpts)
	} else {
		actual, err := fibcompare.New(livefib.NewCollector(liveexec.ExecRunner{})).Collect(ctx, nodes, fibOpts)
		if err != nil {
			return ExitError{Code: 2, Err: err}
		}
		actualFiltered = observation.AnalyzeComparableRoutes(topo, actual, fibOpts)
	}
	for _, line := range observation.FormatFIBWarnings(observation.WarningDiagnostics(actualFiltered, fibOpts)) {
		fmt.Fprintln(out, line)
	}
	result := observation.CompareFilterResults(expected, actualFiltered, fibOpts)
	for _, line := range observation.FormatFIBDiffs(result) {
		fmt.Fprintln(out, line)
	}
	if !result.OK {
		return ExitError{Code: 1, Err: fmt.Errorf("FIB comparison found diff(s)")}
	}
	fmt.Fprintln(out, "FIBs match expected modeled forwarding entries")
	return nil
}

func checkSnapshotHashes(topologyPath string, snap *livesnapshot.Snapshot, policyRaw string, out io.Writer) error {
	policy, ok := livesnapshot.ParseHashPolicy(policyRaw)
	if !ok {
		return ExitError{Code: 2, Err: fmt.Errorf("snapshot hash policy must be one of warn, fail, or ignore")}
	}
	if policy == livesnapshot.HashPolicyIgnore {
		return nil
	}
	result, err := inputhash.CheckHashes(topologyPath, snap)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	if len(result.Mismatches) == 0 && len(result.Missing) == 0 {
		return nil
	}
	var lines []string
	for _, mismatch := range result.Mismatches {
		lines = append(lines, fmt.Sprintf("snapshot hash mismatch: %s snapshot=%s current=%s", mismatch.Path, mismatch.Want, mismatch.Got))
	}
	for _, missing := range result.Missing {
		lines = append(lines, fmt.Sprintf("snapshot hash missing current input: %s", missing))
	}
	if policy == livesnapshot.HashPolicyFail {
		return ExitError{Code: 2, Err: errors.New(strings.Join(lines, "; "))}
	}
	for _, line := range lines {
		fmt.Fprintf(out, "warning: %s\n", line)
	}
	return nil
}
