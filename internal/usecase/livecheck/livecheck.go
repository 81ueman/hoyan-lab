package livecheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	snapshotdomain "github.com/81ueman/hoyan-lab/internal/domain/snapshot"
	"github.com/81ueman/hoyan-lab/internal/usecase/collect"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

type Options struct {
	Topology       string
	Snapshot       string
	HashPolicy     snapshotdomain.HashPolicy
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

type Usecase struct {
	deps Dependencies
}

func New(deps Dependencies) (Usecase, error) {
	if deps.Runtime == nil {
		return Usecase{}, fmt.Errorf("livecheck runtime is required")
	}
	if deps.Collector == nil {
		return Usecase{}, fmt.Errorf("livecheck collector is required")
	}
	return Usecase{deps: deps}, nil
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
		opts.HashPolicy = snapshotdomain.HashPolicyWarn
	}
	topo, _, err := topology.LoadTopologyWithOptions(opts.Topology, topology.LoadOptions{StrictConfig: opts.StrictConfig})
	if err != nil {
		return err
	}
	simulator, err := collect.NewSimulator(topo)
	if err != nil {
		return err
	}
	collectOpts := observation.CollectOptions{IncludeInactive: true, IncludeModelInfo: true}
	snapshotCompareOpts := observation.SnapshotCompareOptions{IgnoreMetadata: true}

	var snap *snapshotdomain.Snapshot
	if opts.Snapshot != "" {
		if u.deps.SnapshotRepository == nil {
			return fmt.Errorf("livecheck snapshot repository is required when snapshot is specified")
		}
		snap, err = u.deps.SnapshotRepository.Load(opts.Snapshot)
		if err != nil {
			return err
		}
		if err := checkSnapshotHashes(u.deps.InputHashChecker, opts.Topology, snap, opts.HashPolicy, opts.Out); err != nil {
			return err
		}
		expectedSnapshot, err := collect.CollectSnapshot(ctx, simulator, collectOpts)
		if err != nil {
			return err
		}
		result := observation.CompareSnapshots(expectedSnapshot, snap.Network, snapshotCompareOpts)
		result = filterSnapshotComparison(result, opts.CheckFIB)
		formatSnapshotComparison(opts.Out, result, opts.CheckFIB)
		if !result.OK {
			return fmt.Errorf("snapshot comparison found diff(s)")
		}
		fmt.Fprintln(opts.Out, "snapshot matches modeled RIB/FIB state")
		if opts.Offline {
			return nil
		}
	}

	deadlineCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	if err := u.deps.Runtime.Start(deadlineCtx, opts.Topology, topo, opts.PollInterval, opts.Out); err != nil {
		return err
	}
	defer func() {
		if opts.SkipDestroy || (err != nil && opts.KeepOnFailure) {
			return
		}
		fmt.Fprintf(opts.Out, "destroying %s\n", opts.Topology)
		if destroyErr := u.deps.Runtime.Stop(context.Background(), opts.Topology); err == nil && destroyErr != nil {
			err = destroyErr
		}
	}()

	if snap == nil {
		fmt.Fprintln(opts.Out, "waiting for live collector snapshot")
		result, err := WaitForMatchingCollectors(deadlineCtx, simulator, u.deps.Collector, collectOpts, opts.PollInterval, opts.MaxPolls, snapshotCompareOpts, opts.CheckFIB)
		if err != nil {
			formatSnapshotComparison(opts.Out, result, opts.CheckFIB)
			return err
		}
		formatSnapshotComparison(opts.Out, result, opts.CheckFIB)
		fmt.Fprintln(opts.Out, "live collector snapshot matches modeled RIB/FIB state")
	}
	return nil
}

func WaitForMatchingCollectors(ctx context.Context, expected, actual collect.Collector, collectOpts observation.CollectOptions, interval time.Duration, maxPolls int, compareOpts observation.SnapshotCompareOptions, checkFIB bool) (observation.SnapshotComparison, error) {
	var lastResult observation.SnapshotComparison
	var lastErr error
	bestDiffCount := -1
	polls := 0
	err := poll(ctx, interval, func() (bool, error) {
		polls++
		result, err := collect.CompareCollectors(ctx, expected, actual, collectOpts, compareOpts)
		if err != nil {
			lastErr = err
			if maxPolls > 0 && polls >= maxPolls {
				return false, snapshotConvergenceError(lastErr, bestDiffCount)
			}
			return false, nil
		}
		lastErr = nil
		lastResult = filterSnapshotComparison(result, checkFIB)
		diffCount := countSnapshotDiffs(lastResult)
		if bestDiffCount == -1 || diffCount < bestDiffCount {
			bestDiffCount = diffCount
		}
		if lastResult.OK {
			return true, nil
		}
		if maxPolls > 0 && polls >= maxPolls {
			return false, snapshotConvergenceError(lastErr, bestDiffCount)
		}
		return false, nil
	}, func() error {
		return snapshotConvergenceError(lastErr, bestDiffCount)
	})
	if err != nil {
		return lastResult, err
	}
	return lastResult, nil
}

func filterSnapshotComparison(result observation.SnapshotComparison, checkFIB bool) observation.SnapshotComparison {
	if !checkFIB {
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

func formatSnapshotComparison(out io.Writer, result observation.SnapshotComparison, checkFIB bool) {
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
	for _, mismatch := range result.RIBMismatches {
		fmt.Fprintf(out, "RIB mismatch: %s/%s expected=%s actual=%s\n", mismatch.Node, mismatch.VRF, mismatch.Expected, mismatch.Actual)
	}
	if checkFIB {
		for _, mismatch := range result.FIBMismatches {
			fmt.Fprintf(out, "FIB mismatch: %s/%s expected=%s actual=%s\n", mismatch.Node, mismatch.VRF, mismatch.Expected, mismatch.Actual)
		}
	}
}

func snapshotConvergenceError(lastErr error, bestDiffCount int) error {
	if lastErr != nil {
		return fmt.Errorf("collector snapshots did not converge to modeled state; last collection error: %w", lastErr)
	}
	if bestDiffCount < 0 {
		return fmt.Errorf("collector snapshots did not converge to modeled state")
	}
	return fmt.Errorf("collector snapshots did not converge to modeled state: best diff count %d", bestDiffCount)
}

func countSnapshotDiffs(result observation.SnapshotComparison) int {
	return len(result.MissingNodes) + len(result.UnexpectedNodes) +
		len(result.MissingVRFs) + len(result.UnexpectedVRFs) +
		len(result.RIBMismatches) + len(result.FIBMismatches)
}

func collectRIBRoutes(ctx context.Context, collector RIBCollector, nodes []model.Node, opts observation.CollectOptions) ([]observation.RIBRoute, error) {
	var out []observation.RIBRoute
	for _, node := range nodes {
		for _, vrf := range model.NetworkInstancesForNode(node) {
			rib, err := collector.CollectRIB(ctx, node, model.NormalizeNetworkInstance(vrf), opts)
			if err != nil {
				return nil, err
			}
			out = append(out, rib.Routes...)
		}
	}
	observation.SortRoutes(out)
	return out, nil
}

func checkSnapshotHashes(checker InputHashChecker, topologyPath string, snap *snapshotdomain.Snapshot, policy snapshotdomain.HashPolicy, out io.Writer) error {
	policy, ok := snapshotdomain.ParseHashPolicy(string(policy))
	if !ok {
		return fmt.Errorf("snapshot hash policy must be one of warn, fail, or ignore")
	}
	if policy == snapshotdomain.HashPolicyIgnore {
		return nil
	}
	if checker == nil {
		return fmt.Errorf("livecheck input hash checker is required when snapshot hash policy is %s", policy)
	}
	result, err := checker.CheckHashes(topologyPath, snap)
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
	if policy == snapshotdomain.HashPolicyFail {
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
	opts := observation.CollectOptions{IncludeInactive: true, IncludeModelInfo: true}
	var out []observation.RIBRoute
	for _, node := range nodes {
		for _, vrf := range model.NetworkInstancesForNode(node) {
			rib, err := collector.CollectRIB(ctx, node, model.NormalizeNetworkInstance(vrf), opts)
			if err != nil {
				return nil, err
			}
			out = append(out, rib.Routes...)
		}
	}
	observation.SortRoutes(out)
	if !expectedHasNonBGP(expected) {
		return observation.BGPOnly(out), nil
	}
	return out, nil
}

func expectedHasNonBGP(routes []observation.RIBRoute) bool {
	for _, route := range routes {
		protocol := strings.ToLower(strings.TrimSpace(string(route.Common.Protocol)))
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
	afi := string(route.Common.AFI)
	protocol := strings.ToLower(strings.TrimSpace(string(route.Common.Protocol)))
	if protocol == "" {
		protocol = "bgp"
	}
	return afi + "|" + protocol + "|" + route.Common.Prefix
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
