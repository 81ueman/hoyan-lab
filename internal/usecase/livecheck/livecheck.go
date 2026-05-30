package livecheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	livefib "github.com/81ueman/hoyan-lab/internal/adapter/live/fib"
	liverib "github.com/81ueman/hoyan-lab/internal/adapter/live/rib"
	"github.com/81ueman/hoyan-lab/internal/adapter/queryfile"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	observationfib "github.com/81ueman/hoyan-lab/internal/domain/observation/fib"
	observationrib "github.com/81ueman/hoyan-lab/internal/domain/observation/rib"
	fibcompare "github.com/81ueman/hoyan-lab/internal/usecase/fib"
	"github.com/81ueman/hoyan-lab/internal/usecase/livesnapshot"
	ribcompare "github.com/81ueman/hoyan-lab/internal/usecase/rib"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

type Options struct {
	Topology       string
	Queries        string
	Snapshot       string
	HashPolicy     HashPolicy
	Offline        bool
	Timeout        time.Duration
	PollInterval   time.Duration
	MaxPolls       int
	StrictConfig   bool
	CompareOptions observationrib.CompareOptions
	CheckFIB       bool
	FIBOptions     observationfib.Options
	KeepOnFailure  bool
	SkipDestroy    bool
	Out            io.Writer
}

const DefaultMaxPolls = 5

type HashPolicy string

type Usecase struct {
	runner observationrib.Runner
}

func New(runner observationrib.Runner) Usecase {
	return Usecase{runner: runner}
}

func (u Usecase) Run(ctx context.Context, opts Options) (err error) {
	if opts.Topology == "" {
		opts.Topology = "labs/base-wan/hoyan.clab.yml"
	}
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}
	if opts.PollInterval == 0 {
		opts.PollInterval = 25 * time.Second
	}
	if opts.MaxPolls == 0 {
		opts.MaxPolls = DefaultMaxPolls
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	if opts.HashPolicy == "" {
		opts.HashPolicy = HashPolicy(livesnapshot.HashPolicyWarn)
	}
	compareOptions := opts.CompareOptions
	if isZeroCompareOptions(compareOptions) {
		compareOptions = observationrib.DefaultCompareOptions()
	}
	topo, _, err := topology.LoadTopologyWithOptions(opts.Topology, topology.LoadOptions{StrictConfig: opts.StrictConfig})
	if err != nil {
		return err
	}
	runner := u.runner
	queriesPath := opts.Queries
	if queriesPath == "" {
		queriesPath = "labs/base-wan/intent/queries.yml"
	}
	queries, err := queryfile.Load(queriesPath)
	if err != nil {
		return err
	}
	nodes := liverib.SupportedNodes(topo.Nodes)
	expected := ribcompare.ExpectedForNodes(topo, nodes)
	expectedBGP := observationrib.BGPOnly(expected)

	var snap *livesnapshot.Snapshot
	if opts.Snapshot != "" {
		snap, err = livesnapshot.Load(opts.Snapshot)
		if err != nil {
			return err
		}
		if err := checkSnapshotHashes(opts.Topology, snap, livesnapshot.HashPolicy(opts.HashPolicy), opts.Out); err != nil {
			return err
		}
		if err := compareSnapshotRIBs(snap, expected, expectedBGP, compareOptions, opts.Out); err != nil {
			return err
		}
		if opts.CheckFIB {
			if err := compareSnapshotFIBs(snap, topo, opts.FIBOptions, opts.Out); err != nil {
				return err
			}
		}
		if opts.Offline {
			return nil
		}
	}

	if err := BuildLocalImages(ctx, runner, opts.Topology, opts.Out); err != nil {
		return err
	}
	fmt.Fprintf(opts.Out, "deploying %s\n", opts.Topology)
	if _, err := runner.Run(ctx, "containerlab", "deploy", "--reconfigure", "-t", opts.Topology); err != nil {
		return fmt.Errorf("containerlab deploy: %w", err)
	}
	defer func() {
		if opts.SkipDestroy || (err != nil && opts.KeepOnFailure) {
			return
		}
		fmt.Fprintf(opts.Out, "destroying %s\n", opts.Topology)
		if _, destroyErr := runner.Run(context.Background(), "containerlab", "destroy", "--cleanup", "-t", opts.Topology); err == nil && destroyErr != nil {
			err = fmt.Errorf("containerlab destroy: %w", destroyErr)
		}
	}()

	deadlineCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	if err := WaitForContainers(deadlineCtx, runner, nodes, opts.PollInterval); err != nil {
		return err
	}
	if err := WaitForSRLinuxCLI(deadlineCtx, runner, nodes, opts.PollInterval); err != nil {
		return err
	}
	if err := ApplyNftablesPolicies(deadlineCtx, runner, topo, opts.Out); err != nil {
		return err
	}
	if snap == nil {
		fmt.Fprintf(opts.Out, "waiting for live RIB routes (sources: %s)\n", observationrib.FormatSourceSummary(observationrib.SourceSummary(expected)))
		actual, result, err := WaitForMatchingRIBs(deadlineCtx, runner, nodes, expected, opts.PollInterval, opts.MaxPolls, compareOptions)
		if err != nil {
			if len(actual) > 0 {
				for _, line := range observationrib.FormatDiffs(result) {
					fmt.Fprintln(opts.Out, line)
				}
			}
			return err
		}
		for _, line := range observationrib.FormatDiffs(result) {
			fmt.Fprintln(opts.Out, line)
		}
		if !result.OK {
			return fmt.Errorf("live RIB comparison found diff(s)")
		}
		fmt.Fprintln(opts.Out, "live RIBs converged to modeled paths")
	}
	if opts.CheckFIB && snap == nil {
		fibNodes := topo.Nodes
		if opts.FIBOptions.AllowUnsupported {
			fibNodes = livefib.NewCollector(nil).SupportedNodes(fibNodes)
		}
		expectedFIB := observationfib.AnalyzeComparableRoutes(topo, fibcompare.NewExpectedBuilder().ExpectedForNodes(topo, fibNodes), opts.FIBOptions)
		actualFIB, err := fibcompare.New(livefib.NewCollector(runner)).Collect(deadlineCtx, fibNodes, opts.FIBOptions)
		if err != nil {
			return err
		}
		actualFIBResult := observationfib.AnalyzeComparableRoutes(topo, actualFIB, opts.FIBOptions)
		for _, line := range observationfib.FormatWarnings(observationfib.WarningDiagnostics(actualFIBResult, opts.FIBOptions)) {
			fmt.Fprintln(opts.Out, line)
		}
		fibResult := observationfib.CompareFilterResults(expectedFIB, actualFIBResult, opts.FIBOptions)
		for _, line := range observationfib.FormatDiffs(fibResult) {
			fmt.Fprintln(opts.Out, line)
		}
		if !fibResult.OK {
			return fmt.Errorf("live FIB comparison found diff(s)")
		}
		fmt.Fprintln(opts.Out, "live FIBs match modeled forwarding entries")
	}
	if err := RunDataplaneChecks(deadlineCtx, runner, topo, queries, opts.Out); err != nil {
		return err
	}
	return nil
}

func BuildLocalImages(ctx context.Context, runner observationrib.Runner, topologyPath string, out io.Writer) error {
	root := filepath.Dir(topologyPath)
	dockerfile := filepath.Join(root, "images", "frr-nftables", "Dockerfile")
	if _, err := os.Stat(dockerfile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	contextDir := filepath.Dir(dockerfile)
	if _, err := runner.Run(ctx, "docker", "image", "inspect", "hoyan-frr-nftables:10.6.1"); err == nil {
		if out != nil {
			fmt.Fprintln(out, "using existing hoyan-frr-nftables:10.6.1")
		}
		return nil
	}
	if out != nil {
		fmt.Fprintln(out, "building hoyan-frr-nftables:10.6.1")
	}
	if _, err := runner.Run(ctx, "docker", "build", "-t", "hoyan-frr-nftables:10.6.1", contextDir); err != nil {
		return fmt.Errorf("docker build hoyan-frr-nftables:10.6.1: %w", err)
	}
	return nil
}

func compareSnapshotRIBs(snap *livesnapshot.Snapshot, expected, expectedBGP []observationrib.NormalizedRoute, compareOptions observationrib.CompareOptions, out io.Writer) error {
	actualBGP := livesnapshot.BGPRoutes(snap)
	fmt.Fprintf(out, "comparing snapshot BGP RIB routes (sources: %s)\n", observationrib.FormatSourceSummary(observationrib.SourceSummary(expectedBGP)))
	result := observationrib.CompareRoutes(expectedBGP, actualBGP, compareOptions)
	for _, line := range observationrib.FormatDiffs(result) {
		fmt.Fprintln(out, line)
	}
	if !result.OK {
		return fmt.Errorf("snapshot BGP RIB comparison found diff(s)")
	}
	fmt.Fprintf(out, "comparing snapshot RIB routes (sources: %s)\n", observationrib.FormatSourceSummary(observationrib.SourceSummary(expected)))
	result = observationrib.CompareRoutes(expected, livesnapshot.AllRIBRoutes(snap), compareOptions)
	for _, line := range observationrib.FormatDiffs(result) {
		fmt.Fprintln(out, line)
	}
	if !result.OK {
		return fmt.Errorf("snapshot RIB comparison found diff(s)")
	}
	fmt.Fprintln(out, "snapshot RIBs match modeled paths")
	return nil
}

func compareSnapshotFIBs(snap *livesnapshot.Snapshot, topo *model.Topology, opts observationfib.Options, out io.Writer) error {
	fibNodes := topo.Nodes
	if opts.AllowUnsupported {
		fibNodes = livefib.NewCollector(nil).SupportedNodes(fibNodes)
	}
	expected := observationfib.AnalyzeComparableRoutes(topo, fibcompare.NewExpectedBuilder().ExpectedForNodes(topo, fibNodes), opts)
	actual := observationfib.AnalyzeComparableRoutes(topo, livesnapshot.FIBRoutes(snap), opts)
	for _, line := range observationfib.FormatWarnings(observationfib.WarningDiagnostics(actual, opts)) {
		fmt.Fprintln(out, line)
	}
	result := observationfib.CompareFilterResults(expected, actual, opts)
	for _, line := range observationfib.FormatDiffs(result) {
		fmt.Fprintln(out, line)
	}
	if !result.OK {
		return fmt.Errorf("snapshot FIB comparison found diff(s)")
	}
	fmt.Fprintln(out, "snapshot FIBs match modeled forwarding entries")
	return nil
}

func checkSnapshotHashes(topologyPath string, snap *livesnapshot.Snapshot, policy livesnapshot.HashPolicy, out io.Writer) error {
	policy, ok := livesnapshot.ParseHashPolicy(string(policy))
	if !ok {
		return fmt.Errorf("snapshot hash policy must be one of warn, fail, or ignore")
	}
	if policy == livesnapshot.HashPolicyIgnore {
		return nil
	}
	result, err := livesnapshot.CheckHashes(topologyPath, snap)
	if err != nil {
		return err
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
		return errors.New(strings.Join(lines, "; "))
	}
	for _, line := range lines {
		fmt.Fprintf(out, "warning: %s\n", line)
	}
	return nil
}

func isZeroCompareOptions(opts observationrib.CompareOptions) bool {
	return reflect.DeepEqual(opts, observationrib.CompareOptions{})
}

func WaitForFRRContainers(ctx context.Context, runner observationrib.Runner, nodes []model.Node, interval time.Duration) error {
	return WaitForContainers(ctx, runner, nodes, interval)
}

func WaitForContainers(ctx context.Context, runner observationrib.Runner, nodes []model.Node, interval time.Duration) error {
	var lastErr error
	return poll(ctx, interval, func() (bool, error) {
		for _, n := range nodes {
			containerName := n.RuntimeName()
			out, err := runner.Run(ctx, "docker", "inspect", "-f", "{{.State.Running}}", containerName)
			if err != nil {
				lastErr = fmt.Errorf("docker inspect -f {{.State.Running}} %s: %w", containerName, err)
				return false, nil
			}
			if strings.TrimSpace(string(out)) != "true" {
				lastErr = fmt.Errorf("container %s is not running", containerName)
				return false, nil
			}
		}
		return true, nil
	}, func() error {
		if lastErr != nil {
			return fmt.Errorf("containers did not become ready: %w", lastErr)
		}
		return fmt.Errorf("containers did not become ready")
	})
}

func WaitForSRLinuxCLI(ctx context.Context, runner observationrib.Runner, nodes []model.Node, interval time.Duration) error {
	srlinuxNodes := liverib.NodesByKind(nodes, model.KindSRLinux)
	if len(srlinuxNodes) == 0 {
		return nil
	}
	var lastErr error
	return poll(ctx, interval, func() (bool, error) {
		for _, n := range srlinuxNodes {
			containerName := n.RuntimeName()
			if _, err := liverib.RunSRLinuxJSON(ctx, runner, containerName, "show", "version"); err != nil {
				lastErr = fmt.Errorf("%s SR Linux CLI is not ready: %w", n.Name, err)
				return false, nil
			}
		}
		lastErr = nil
		return true, nil
	}, func() error {
		if lastErr != nil {
			return fmt.Errorf("SR Linux CLI did not become ready: %w", lastErr)
		}
		return fmt.Errorf("SR Linux CLI did not become ready")
	})
}

func WaitForExpectedRoutes(ctx context.Context, runner observationrib.Runner, nodes []model.Node, expected []observationrib.NormalizedRoute, interval time.Duration, maxPolls int) ([]observationrib.NormalizedRoute, error) {
	var last []observationrib.NormalizedRoute
	var lastErr error
	bestSeen := 0
	polls := 0
	err := poll(ctx, interval, func() (bool, error) {
		polls++
		actual, err := collectExpectedRIBSources(ctx, runner, nodes, expected)
		if err != nil {
			lastErr = err
			if maxPolls > 0 && polls >= maxPolls {
				return false, convergenceError(lastErr, bestSeen, len(expected))
			}
			return false, nil
		}
		lastErr = nil
		last = actual
		if seen := CountExpectedRoutes(expected, actual); seen > bestSeen {
			bestSeen = seen
		}
		if HasExpectedRoutes(expected, actual) {
			return true, nil
		}
		if maxPolls > 0 && polls >= maxPolls {
			return false, convergenceError(lastErr, bestSeen, len(expected))
		}
		return false, nil
	}, func() error {
		return convergenceError(lastErr, bestSeen, len(expected))
	})
	if err != nil {
		return last, err
	}
	return last, nil
}

func WaitForMatchingRIBs(ctx context.Context, runner observationrib.Runner, nodes []model.Node, expected []observationrib.NormalizedRoute, interval time.Duration, maxPolls int, compareOptions observationrib.CompareOptions) ([]observationrib.NormalizedRoute, observationrib.CompareResult, error) {
	if isZeroCompareOptions(compareOptions) {
		compareOptions = observationrib.DefaultCompareOptions()
	}
	var last []observationrib.NormalizedRoute
	var lastResult observationrib.CompareResult
	var lastErr error
	bestSeen := 0
	bestDiffCount := -1
	polls := 0
	err := poll(ctx, interval, func() (bool, error) {
		polls++
		actual, err := collectExpectedRIBSources(ctx, runner, nodes, expected)
		if err != nil {
			lastErr = err
			if maxPolls > 0 && polls >= maxPolls {
				return false, ribMatchConvergenceError(lastErr, bestSeen, len(expected), bestDiffCount)
			}
			return false, nil
		}
		lastErr = nil
		last = actual
		if seen := CountExpectedRoutes(expected, actual); seen > bestSeen {
			bestSeen = seen
		}
		lastResult = observationrib.CompareRoutes(expected, actual, compareOptions)
		diffCount := countDiffs(lastResult)
		if bestDiffCount == -1 || diffCount < bestDiffCount {
			bestDiffCount = diffCount
		}
		if lastResult.OK {
			return true, nil
		}
		if maxPolls > 0 && polls >= maxPolls {
			return false, ribMatchConvergenceError(lastErr, bestSeen, len(expected), bestDiffCount)
		}
		return false, nil
	}, func() error {
		return ribMatchConvergenceError(lastErr, bestSeen, len(expected), bestDiffCount)
	})
	if err != nil {
		return last, lastResult, err
	}
	return last, lastResult, nil
}

func collectExpectedRIBSources(ctx context.Context, runner observationrib.Runner, nodes []model.Node, expected []observationrib.NormalizedRoute) ([]observationrib.NormalizedRoute, error) {
	if expectedHasNonBGP(expected) {
		return ribcompare.New(liverib.NewCollector(runner)).Collect(ctx, nodes)
	}
	return ribcompare.New(liverib.NewCollector(runner)).CollectBGPRoutes(ctx, nodes)
}

func expectedHasNonBGP(routes []observationrib.NormalizedRoute) bool {
	for _, route := range routes {
		protocol := strings.ToLower(strings.TrimSpace(route.Protocol))
		if protocol != "" && protocol != "bgp" {
			return true
		}
	}
	return false
}

func convergenceError(lastErr error, seen, total int) error {
	if lastErr != nil {
		return fmt.Errorf("expected RIB routes did not converge; last collection error: %w", lastErr)
	}
	return fmt.Errorf("expected RIB routes did not converge: saw %d/%d expected routes", seen, total)
}

func ribMatchConvergenceError(lastErr error, seen, total, bestDiffCount int) error {
	if lastErr != nil {
		return fmt.Errorf("RIBs did not converge to modeled paths; last collection error: %w", lastErr)
	}
	if bestDiffCount < 0 {
		return fmt.Errorf("RIBs did not converge to modeled paths: saw %d/%d expected routes", seen, total)
	}
	return fmt.Errorf("RIBs did not converge to modeled paths: saw %d/%d expected routes, best diff count %d", seen, total, bestDiffCount)
}

func HasExpectedRoutes(expected []observationrib.NormalizedRoute, actual []observationrib.NormalizedRoute) bool {
	return CountExpectedRoutes(expected, actual) == len(expected)
}

func CountExpectedRoutes(expected []observationrib.NormalizedRoute, actual []observationrib.NormalizedRoute) int {
	seen := map[string]bool{}
	for _, route := range actual {
		seen[ribRouteSourceKey(route)] = true
	}
	count := 0
	for _, route := range expected {
		if seen[ribRouteSourceKey(route)] {
			count++
		}
	}
	return count
}

func ribRouteSourceKey(route observationrib.NormalizedRoute) string {
	ni := route.NetworkInstance
	if ni == "" {
		ni = "default"
	}
	afi := route.AFI
	if afi == "" {
		afi = "ipv4"
	}
	protocol := strings.ToLower(strings.TrimSpace(route.Protocol))
	if protocol == "" {
		protocol = "bgp"
	}
	return route.Node + "|" + ni + "|" + afi + "|" + protocol + "|" + route.Prefix
}

func countDiffs(result observationrib.CompareResult) int {
	return len(result.MissingPrefixes) + len(result.UnexpectedPrefixes) + len(result.MissingPaths) + len(result.UnexpectedPaths) + len(result.Mismatched) + len(result.DuplicatePathConflicts)
}

func poll(ctx context.Context, interval time.Duration, fn func() (bool, error), onTimeout func() error) error {
	if interval <= 0 {
		interval = time.Second
	}
	for {
		ok, err := fn()
		if err != nil || ok {
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if onTimeout != nil {
				return onTimeout()
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
