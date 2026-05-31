package cli

import (
	"fmt"
	"strings"
	"time"

	liveexec "github.com/81ueman/hoyan-lab/internal/adapter/live"
	clabruntime "github.com/81ueman/hoyan-lab/internal/adapter/live/containerlab"
	livedataplane "github.com/81ueman/hoyan-lab/internal/adapter/live/dataplane"
	livefib "github.com/81ueman/hoyan-lab/internal/adapter/live/fib"
	liverib "github.com/81ueman/hoyan-lab/internal/adapter/live/rib"
	"github.com/81ueman/hoyan-lab/internal/adapter/queryfile"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/usecase/livecheck"
	"github.com/81ueman/hoyan-lab/internal/usecase/livesnapshot"
	"github.com/spf13/cobra"
)

func NewLiveCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "live",
		Short:         "Run live lab checks and collect live device state",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(NewLiveCheckCommand(), NewLiveSnapshotCommand())
	return cmd
}

func NewLiveCheckCommand() *cobra.Command {
	var opts liveCheckOptions
	cmd := &cobra.Command{
		Use:           "check",
		Short:         "Deploy the lab and compare live device state with the model",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := opts.validate(); err != nil {
				return err
			}
			if err := resolveLabInputs(cmd, opts.labPath, &opts.topologyPath, &opts.queriesPath); err != nil {
				return err
			}
			runner := liveexec.ExecRunner{}
			err := livecheck.New(livecheck.Dependencies{
				Runtime:         clabruntime.Runtime{Runner: runner},
				QueryLoader:     queryfile.Loader{},
				RIBCollector:    liverib.NewCollector(runner),
				FIBCollector:    livefib.NewCollector(runner),
				DataplaneProber: livedataplane.DockerProber{Runner: runner},
			}).Run(cmd.Context(), livecheck.Options{
				Topology:      opts.topologyPath,
				Queries:       opts.queriesPath,
				Snapshot:      opts.snapshotPath,
				HashPolicy:    livecheck.HashPolicy(opts.snapshotHashPolicy),
				Offline:       opts.offline,
				StrictConfig:  opts.strictConfig,
				Timeout:       opts.timeout,
				PollInterval:  opts.pollInterval,
				MaxPolls:      opts.maxPolls,
				KeepOnFailure: opts.keepOnFailure,
				SkipDestroy:   opts.skipDestroy,
				CheckFIB:      opts.checkFIB && !opts.noCheckFIB,
				FIBOptions:    observation.Options{AllowUnsupported: opts.fibAllowUnsupported, UnresolvedPolicy: observation.UnresolvedPolicy(opts.fibUnresolvedPolicy)},
				Out:           cmd.OutOrStdout(),
			})
			if err != nil {
				return ExitError{Code: 1, Err: err}
			}
			return nil
		},
	}
	addLabFlag(cmd, &opts.labPath)
	addTopologyFlag(cmd, &opts.topologyPath, "containerlab topology YAML")
	addQueriesFlag(cmd, &opts.queriesPath, "query YAML for live dataplane checks")
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 5*time.Minute, "overall wait timeout")
	cmd.Flags().DurationVar(&opts.pollInterval, "poll-interval", 25*time.Second, "poll interval")
	cmd.Flags().IntVar(&opts.maxPolls, "max-polls", livecheck.DefaultMaxPolls, "maximum BGP collection polls before reporting diffs")
	cmd.Flags().BoolVar(&opts.keepOnFailure, "keep-on-failure", false, "leave lab running when the check fails")
	cmd.Flags().BoolVar(&opts.skipDestroy, "skip-destroy", false, "leave lab running after the check")
	cmd.Flags().StringVar(&opts.snapshotPath, "snapshot", "", "live snapshot JSON to use instead of collecting RIB/FIB from devices")
	cmd.Flags().StringVar(&opts.snapshotHashPolicy, "snapshot-hash-policy", string(livesnapshot.HashPolicyWarn), "handling for snapshot topology/config hash mismatch: warn, fail, or ignore")
	cmd.Flags().BoolVar(&opts.offline, "offline", false, "with --snapshot, skip deploy and live dataplane probes")
	cmd.Flags().BoolVar(&opts.strictConfig, "strict-config", false, "fail on unsupported config parser statements")
	cmd.Flags().BoolVar(&opts.checkFIB, "check-fib", true, "compare modeled FIB with live installed FIB after BGP convergence")
	cmd.Flags().BoolVar(&opts.noCheckFIB, "no-check-fib", false, "skip modeled-vs-live installed FIB comparison")
	cmd.Flags().BoolVar(&opts.fibAllowUnsupported, "fib-allow-unsupported", false, "skip nodes without a live FIB collector when FIB comparison is enabled")
	cmd.Flags().StringVar(&opts.fibUnresolvedPolicy, "fib-unresolved-policy", string(observation.UnresolvedPolicyWarn), "handling for unresolved live BGP FIB routes: warn, fail, or ignore")
	return cmd
}

type liveCheckOptions struct {
	labPath             string
	topologyPath        string
	queriesPath         string
	strictConfig        bool
	timeout             time.Duration
	pollInterval        time.Duration
	maxPolls            int
	keepOnFailure       bool
	skipDestroy         bool
	snapshotPath        string
	snapshotHashPolicy  string
	offline             bool
	checkFIB            bool
	noCheckFIB          bool
	fibAllowUnsupported bool
	fibUnresolvedPolicy string
}

func (o liveCheckOptions) validate() error {
	if o.timeout <= 0 {
		return fmt.Errorf("--timeout must be greater than zero")
	}
	if o.pollInterval <= 0 {
		return fmt.Errorf("--poll-interval must be greater than zero")
	}
	if o.maxPolls <= 0 {
		return fmt.Errorf("--max-polls must be greater than zero")
	}
	if err := validateFIBUnresolvedPolicy(o.fibUnresolvedPolicy); err != nil {
		return err
	}
	if _, ok := livesnapshot.ParseHashPolicy(o.snapshotHashPolicy); !ok {
		return fmt.Errorf("snapshot hash policy must be one of warn, fail, or ignore")
	}
	if o.offline && o.snapshotPath == "" {
		return fmt.Errorf("--offline requires --snapshot")
	}
	return nil
}

func validateFIBUnresolvedPolicy(policy string) error {
	if _, ok := observation.ParseUnresolvedPolicy(policy); ok {
		return nil
	}
	return fmt.Errorf("FIB unresolved policy must be one of warn, fail, or ignore")
}
