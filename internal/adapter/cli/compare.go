package cli

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	liveadapter "github.com/81ueman/hoyan-lab/internal/adapter/live"
	clabruntime "github.com/81ueman/hoyan-lab/internal/adapter/live/containerlab"
	"github.com/81ueman/hoyan-lab/internal/adapter/snapshotfile"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
	"github.com/spf13/cobra"
)

type compareCheck string

const (
	compareCheckRIB compareCheck = "rib"
	compareCheckFIB compareCheck = "fib"
)

func NewCompareCommand() *cobra.Command {
	var opts compareOptions
	cmd := &cobra.Command{
		Use:           "compare <left-path> <right-path>",
		Short:         "Compare two collector targets",
		Args:          cobra.ExactArgs(2),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.leftPath = args[0]
			opts.rightPath = args[1]
			return runCompare(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.leftType, "left-type", "", "collector type for left path: model, clab, snapshot, device")
	cmd.Flags().StringVar(&opts.rightType, "right-type", "", "collector type for right path: model, clab, snapshot, device")
	cmd.Flags().StringVar(&opts.checks, "check", "rib,fib", "checks to run: rib, fib, or rib,fib")
	cmd.Flags().StringVar(&opts.afi, "afi", "", "address family filter: ipv4 or ipv6")
	cmd.Flags().BoolVar(&opts.includeInactive, "include-inactive", false, "include inactive/non-best RIB routes")
	cmd.Flags().BoolVar(&opts.includeModelInfo, "include-model-info", false, "preserve simulator/model explanation metadata")
	cmd.Flags().StringVar(&opts.saveLeft, "save-left", "", "save collected left snapshot")
	cmd.Flags().StringVar(&opts.saveRight, "save-right", "", "save collected right snapshot")
	cmd.Flags().StringVar(&opts.saveSnapshotsDir, "save-snapshots", "", "save both snapshots under a directory")
	return cmd
}

type compareOptions struct {
	leftPath         string
	rightPath        string
	leftType         string
	rightType        string
	checks           string
	afi              string
	includeInactive  bool
	includeModelInfo bool
	saveLeft         string
	saveRight        string
	saveSnapshotsDir string
}

func runCompare(ctx context.Context, opts compareOptions, out io.Writer) error {
	leftTarget, err := newCollectorTargetWithTypeHint(opts.leftPath, opts.leftType, "--left-type, --right-type, or --type")
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	rightTarget, err := newCollectorTargetWithTypeHint(opts.rightPath, opts.rightType, "--left-type, --right-type, or --type")
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	checks, err := parseCompareChecks(opts.checks)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	collectOpts, err := collectOptionsFromCompareOptions(opts)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	if err := startCompareClabTargets(ctx, []CollectorTarget{leftTarget, rightTarget}, out, time.Second, newLiveRunner()); err != nil {
		return ExitError{Code: 2, Err: err}
	}
	leftCollector, err := resolveCollector(ctx, leftTarget)
	if err != nil {
		return ExitError{Code: 2, Err: fmt.Errorf("left target: %w", err)}
	}
	rightCollector, err := resolveCollector(ctx, rightTarget)
	if err != nil {
		return ExitError{Code: 2, Err: fmt.Errorf("right target: %w", err)}
	}
	leftSnapshot, err := observation.CollectSnapshot(ctx, leftCollector, collectOpts)
	if err != nil {
		return ExitError{Code: 2, Err: fmt.Errorf("collect left target: %w", err)}
	}
	rightSnapshot, err := observation.CollectSnapshot(ctx, rightCollector, collectOpts)
	if err != nil {
		return ExitError{Code: 2, Err: fmt.Errorf("collect right target: %w", err)}
	}
	if err := saveOptionalSnapshots(opts, leftSnapshot, rightSnapshot); err != nil {
		return ExitError{Code: 2, Err: err}
	}
	result := observation.CompareSnapshots(leftSnapshot, rightSnapshot, observation.SnapshotCompareOptions{
		IgnoreMetadata:  true,
		IgnoreModelInfo: !opts.includeModelInfo,
	})
	result = filterSnapshotComparison(result, checks)
	formatSnapshotComparison(out, result, checks)
	if !result.OK {
		return ExitError{Code: 1, Err: fmt.Errorf("snapshot comparison found diff(s)")}
	}
	fmt.Fprintf(out, "snapshots match (%s)\n", formatCompareChecks(checks))
	return nil
}

func parseCompareChecks(raw string) (map[compareCheck]bool, error) {
	out := map[compareCheck]bool{}
	for _, part := range strings.Split(raw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		check := compareCheck(part)
		switch check {
		case compareCheckRIB, compareCheckFIB:
			out[check] = true
		default:
			return nil, fmt.Errorf("--check must be rib, fib, or rib,fib")
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("--check must include at least one of rib or fib")
	}
	return out, nil
}

func collectOptionsFromCompareOptions(opts compareOptions) (observation.CollectOptions, error) {
	collectOpts := observation.CollectOptions{
		IncludeInactive:  opts.includeInactive,
		IncludeModelInfo: opts.includeModelInfo,
	}
	if opts.afi != "" {
		afi := model.NormalizeAFI(model.AFI(opts.afi))
		switch afi {
		case model.AFIIPv4, model.AFIIPv6:
			collectOpts.AFI = afi
		default:
			return observation.CollectOptions{}, fmt.Errorf("--afi must be ipv4 or ipv6")
		}
	}
	return collectOpts, nil
}

func saveOptionalSnapshots(opts compareOptions, left, right observation.NetworkSnapshot) error {
	if opts.saveSnapshotsDir != "" {
		if opts.saveLeft == "" {
			opts.saveLeft = filepath.Join(opts.saveSnapshotsDir, "left.json")
		}
		if opts.saveRight == "" {
			opts.saveRight = filepath.Join(opts.saveSnapshotsDir, "right.json")
		}
	}
	if opts.saveLeft != "" {
		if err := snapshotfile.SaveObservation(opts.saveLeft, left); err != nil {
			return fmt.Errorf("save left snapshot: %w", err)
		}
	}
	if opts.saveRight != "" {
		if err := snapshotfile.SaveObservation(opts.saveRight, right); err != nil {
			return fmt.Errorf("save right snapshot: %w", err)
		}
	}
	return nil
}

func filterSnapshotComparison(result observation.SnapshotComparison, checks map[compareCheck]bool) observation.SnapshotComparison {
	if !checks[compareCheckRIB] {
		result.RIBMismatches = nil
	}
	if !checks[compareCheckFIB] {
		result.FIBMismatches = nil
	}
	result.OK = len(result.MissingNodes) == 0 &&
		len(result.UnexpectedNodes) == 0 &&
		len(result.MissingVRFs) == 0 &&
		len(result.UnexpectedVRFs) == 0 &&
		len(result.RIBMismatches) == 0 &&
		len(result.FIBMismatches) == 0
	return result
}

func formatSnapshotComparison(out io.Writer, result observation.SnapshotComparison, checks map[compareCheck]bool) {
	for _, node := range result.MissingNodes {
		fmt.Fprintf(out, "missing node: %s\n", node)
	}
	for _, node := range result.UnexpectedNodes {
		fmt.Fprintf(out, "unexpected node: %s\n", node)
	}
	for _, vrf := range result.MissingVRFs {
		fmt.Fprintf(out, "missing VRF: %s/%s\n", vrf.Node, vrf.VRF)
	}
	for _, vrf := range result.UnexpectedVRFs {
		fmt.Fprintf(out, "unexpected VRF: %s/%s\n", vrf.Node, vrf.VRF)
	}
	if checks[compareCheckRIB] {
		for _, mismatch := range result.RIBMismatches {
			fmt.Fprintf(out, "RIB mismatch: %s/%s expected=%s actual=%s\n", mismatch.Node, mismatch.VRF, mismatch.Expected, mismatch.Actual)
		}
	}
	if checks[compareCheckFIB] {
		for _, mismatch := range result.FIBMismatches {
			fmt.Fprintf(out, "FIB mismatch: %s/%s expected=%s actual=%s\n", mismatch.Node, mismatch.VRF, mismatch.Expected, mismatch.Actual)
		}
	}
}

func formatCompareChecks(checks map[compareCheck]bool) string {
	var parts []string
	for _, check := range []compareCheck{compareCheckRIB, compareCheckFIB} {
		if checks[check] {
			parts = append(parts, string(check))
		}
	}
	return strings.Join(parts, ",")
}

func startCompareClabTargets(ctx context.Context, targets []CollectorTarget, out io.Writer, pollInterval time.Duration, runner liveadapter.Runner) error {
	started := map[string]bool{}
	runtime := clabruntime.Runtime{Runner: runner}
	for _, target := range targets {
		if target.Type != TargetClab {
			continue
		}
		path := filepath.Clean(target.Path)
		if started[path] {
			continue
		}
		topo, err := topology.LoadTopology(target.Path)
		if err != nil {
			return fmt.Errorf("load containerlab topology %s: %w", target.Path, err)
		}
		if err := runtime.Start(ctx, target.Path, topo, pollInterval, out); err != nil {
			return fmt.Errorf("start containerlab topology %s: %w", target.Path, err)
		}
		started[path] = true
	}
	return nil
}
