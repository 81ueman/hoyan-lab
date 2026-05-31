package livecheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/81ueman/hoyan-lab/internal/adapter/inputhash"
	"github.com/81ueman/hoyan-lab/internal/adapter/snapshotfile"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
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
	CompareOptions observation.CompareOptions
	CheckFIB       bool
	FIBOptions     observation.Options
	KeepOnFailure  bool
	SkipDestroy    bool
	Out            io.Writer
}

const DefaultMaxPolls = 5

type HashPolicy string

type Usecase struct {
	deps Dependencies
}

func New(deps Dependencies) Usecase {
	return Usecase{deps: deps}
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
		compareOptions = observation.DefaultCompareOptions()
	}
	topo, _, err := topology.LoadTopologyWithOptions(opts.Topology, topology.LoadOptions{StrictConfig: opts.StrictConfig})
	if err != nil {
		return err
	}
	if u.deps.Runtime == nil {
		return fmt.Errorf("livecheck runtime is required")
	}
	if u.deps.QueryLoader == nil {
		return fmt.Errorf("livecheck query loader is required")
	}
	if u.deps.RIBCollector == nil {
		return fmt.Errorf("livecheck RIB collector is required")
	}
	if opts.CheckFIB && u.deps.FIBCollector == nil {
		return fmt.Errorf("livecheck FIB collector is required")
	}
	if u.deps.DataplaneProber == nil {
		return fmt.Errorf("livecheck dataplane prober is required")
	}
	queriesPath := opts.Queries
	if queriesPath == "" {
		queriesPath = "labs/base-wan/intent/queries.yml"
	}
	queries, err := u.deps.QueryLoader.Load(queriesPath)
	if err != nil {
		return err
	}
	nodes := u.deps.RIBCollector.SupportedNodes(topo.Nodes)
	expected := (ribcompare.ExpectedBuilder{}).BuildForNodes(topo, nodes)
	expectedBGP := observation.BGPOnly(expected)

	var snap *livesnapshot.Snapshot
	if opts.Snapshot != "" {
		snap, err = snapshotfile.Load(opts.Snapshot)
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
			if err := compareSnapshotFIBs(snap, topo, u.deps.FIBCollector, opts.FIBOptions, opts.Out); err != nil {
				return err
			}
		}
		if opts.Offline {
			return nil
		}
	}

	if err := u.deps.Runtime.BuildLocalImages(ctx, opts.Topology, opts.Out); err != nil {
		return err
	}
	fmt.Fprintf(opts.Out, "deploying %s\n", opts.Topology)
	if err := u.deps.Runtime.Deploy(ctx, opts.Topology); err != nil {
		return err
	}
	defer func() {
		if opts.SkipDestroy || (err != nil && opts.KeepOnFailure) {
			return
		}
		fmt.Fprintf(opts.Out, "destroying %s\n", opts.Topology)
		if destroyErr := u.deps.Runtime.Destroy(context.Background(), opts.Topology); err == nil && destroyErr != nil {
			err = destroyErr
		}
	}()

	deadlineCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	if err := u.deps.Runtime.WaitContainers(deadlineCtx, nodes, opts.PollInterval); err != nil {
		return err
	}
	if err := u.deps.Runtime.WaitSRLinuxCLI(deadlineCtx, nodes, opts.PollInterval); err != nil {
		return err
	}
	if err := u.deps.Runtime.ApplyNftablesPolicies(deadlineCtx, topo, opts.Out); err != nil {
		return err
	}
	if snap == nil {
		fmt.Fprintf(opts.Out, "waiting for live RIB routes (sources: %s)\n", observation.FormatSourceSummary(observation.SourceSummary(expected)))
		actual, result, err := WaitForMatchingRIBs(deadlineCtx, u.deps.RIBCollector, nodes, expected, opts.PollInterval, opts.MaxPolls, compareOptions)
		if err != nil {
			if len(actual) > 0 {
				for _, line := range observation.FormatDiffs(result) {
					fmt.Fprintln(opts.Out, line)
				}
			}
			return err
		}
		for _, line := range observation.FormatDiffs(result) {
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
			fibNodes = u.deps.FIBCollector.SupportedNodes(fibNodes)
		}
		expectedFIB := observation.AnalyzeComparableRoutes(topo, fibcompare.NewExpectedBuilder().ExpectedForNodes(topo, fibNodes), opts.FIBOptions)
		actualFIB, err := fibcompare.New(u.deps.FIBCollector).Collect(deadlineCtx, fibNodes, opts.FIBOptions)
		if err != nil {
			return err
		}
		actualFIBResult := observation.AnalyzeComparableRoutes(topo, actualFIB, opts.FIBOptions)
		for _, line := range observation.FormatFIBWarnings(observation.WarningDiagnostics(actualFIBResult, opts.FIBOptions)) {
			fmt.Fprintln(opts.Out, line)
		}
		fibResult := observation.CompareFilterResults(expectedFIB, actualFIBResult, opts.FIBOptions)
		for _, line := range observation.FormatFIBDiffs(fibResult) {
			fmt.Fprintln(opts.Out, line)
		}
		if !fibResult.OK {
			return fmt.Errorf("live FIB comparison found diff(s)")
		}
		fmt.Fprintln(opts.Out, "live FIBs match modeled forwarding entries")
	}
	if err := RunDataplaneChecks(deadlineCtx, u.deps.DataplaneProber, topo, queries, opts.Out); err != nil {
		return err
	}
	return nil
}

func compareSnapshotRIBs(snap *livesnapshot.Snapshot, expected, expectedBGP []observation.RIBRoute, compareOptions observation.CompareOptions, out io.Writer) error {
	actualBGP := livesnapshot.BGPRoutes(snap)
	fmt.Fprintf(out, "comparing snapshot BGP RIB routes (sources: %s)\n", observation.FormatSourceSummary(observation.SourceSummary(expectedBGP)))
	result := observation.CompareRoutes(expectedBGP, actualBGP, compareOptions)
	for _, line := range observation.FormatDiffs(result) {
		fmt.Fprintln(out, line)
	}
	if !result.OK {
		return fmt.Errorf("snapshot BGP RIB comparison found diff(s)")
	}
	fmt.Fprintf(out, "comparing snapshot RIB routes (sources: %s)\n", observation.FormatSourceSummary(observation.SourceSummary(expected)))
	result = observation.CompareRoutes(expected, livesnapshot.AllRIBRoutes(snap), compareOptions)
	for _, line := range observation.FormatDiffs(result) {
		fmt.Fprintln(out, line)
	}
	if !result.OK {
		return fmt.Errorf("snapshot RIB comparison found diff(s)")
	}
	fmt.Fprintln(out, "snapshot RIBs match modeled paths")
	return nil
}

func compareSnapshotFIBs(snap *livesnapshot.Snapshot, topo *model.Topology, collector FIBCollector, opts observation.Options, out io.Writer) error {
	fibNodes := topo.Nodes
	if opts.AllowUnsupported && collector != nil {
		fibNodes = collector.SupportedNodes(fibNodes)
	}
	expected := observation.AnalyzeComparableRoutes(topo, fibcompare.NewExpectedBuilder().ExpectedForNodes(topo, fibNodes), opts)
	actual := observation.AnalyzeComparableRoutes(topo, livesnapshot.FIBRoutes(snap), opts)
	for _, line := range observation.FormatFIBWarnings(observation.WarningDiagnostics(actual, opts)) {
		fmt.Fprintln(out, line)
	}
	result := observation.CompareFilterResults(expected, actual, opts)
	for _, line := range observation.FormatFIBDiffs(result) {
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
	result, err := inputhash.CheckHashes(topologyPath, snap)
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

func isZeroCompareOptions(opts observation.CompareOptions) bool {
	return reflect.DeepEqual(opts, observation.CompareOptions{})
}

func WaitForExpectedRoutes(ctx context.Context, collector RIBCollector, nodes []model.Node, expected []observation.RIBRoute, interval time.Duration, maxPolls int) ([]observation.RIBRoute, error) {
	var last []observation.RIBRoute
	var lastErr error
	bestSeen := 0
	polls := 0
	err := poll(ctx, interval, func() (bool, error) {
		polls++
		actual, err := collectExpectedRIBSources(ctx, collector, nodes, expected)
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

func WaitForMatchingRIBs(ctx context.Context, collector RIBCollector, nodes []model.Node, expected []observation.RIBRoute, interval time.Duration, maxPolls int, compareOptions observation.CompareOptions) ([]observation.RIBRoute, observation.CompareResult, error) {
	if isZeroCompareOptions(compareOptions) {
		compareOptions = observation.DefaultCompareOptions()
	}
	var last []observation.RIBRoute
	var lastResult observation.CompareResult
	var lastErr error
	bestSeen := 0
	bestDiffCount := -1
	polls := 0
	err := poll(ctx, interval, func() (bool, error) {
		polls++
		actual, err := collectExpectedRIBSources(ctx, collector, nodes, expected)
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
		lastResult = observation.CompareRoutes(expected, actual, compareOptions)
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

func collectExpectedRIBSources(ctx context.Context, collector RIBCollector, nodes []model.Node, expected []observation.RIBRoute) ([]observation.RIBRoute, error) {
	if expectedHasNonBGP(expected) {
		return collector.Collect(ctx, nodes)
	}
	return collector.CollectBGPRoutes(ctx, nodes)
}

func expectedHasNonBGP(routes []observation.RIBRoute) bool {
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

func HasExpectedRoutes(expected []observation.RIBRoute, actual []observation.RIBRoute) bool {
	return CountExpectedRoutes(expected, actual) == len(expected)
}

func CountExpectedRoutes(expected []observation.RIBRoute, actual []observation.RIBRoute) int {
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

func ribRouteSourceKey(route observation.RIBRoute) string {
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

func countDiffs(result observation.CompareResult) int {
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
