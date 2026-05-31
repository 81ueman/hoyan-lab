package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/adapter/gitmeta"
	"github.com/81ueman/hoyan-lab/internal/adapter/inputhash"
	liveexec "github.com/81ueman/hoyan-lab/internal/adapter/live"
	livefib "github.com/81ueman/hoyan-lab/internal/adapter/live/fib"
	liverib "github.com/81ueman/hoyan-lab/internal/adapter/live/rib"
	"github.com/81ueman/hoyan-lab/internal/adapter/snapshotfile"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/usecase/livesnapshot"
	"github.com/spf13/cobra"
)

func NewLiveSnapshotCommand() *cobra.Command {
	var opts liveSnapshotOptions
	cmd := &cobra.Command{
		Use:           "snapshot",
		Short:         "Collect live RIB and FIB state into a snapshot JSON file",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := resolveLabInputs(cmd, opts.labPath, &opts.topologyPath, nil); err != nil {
				return err
			}
			if err := runLiveSnapshot(cmd.Context(), opts, cmd.OutOrStdout()); err != nil {
				return err
			}
			return nil
		},
	}
	addLabFlag(cmd, &opts.labPath)
	addTopologyFlag(cmd, &opts.topologyPath, "containerlab topology YAML")
	cmd.Flags().StringVarP(&opts.outputPath, "output", "o", "live-state.json", "snapshot JSON output path, or - for stdout")
	cmd.Flags().StringVar(&opts.rawDir, "raw-dir", "", "optional directory for raw vendor command output")
	cmd.Flags().BoolVar(&opts.fibAllowUnsupported, "fib-allow-unsupported", true, "skip nodes without a live FIB collector")
	cmd.Flags().StringVar(&opts.fibUnresolvedPolicy, "fib-unresolved-policy", string(observation.UnresolvedPolicyWarn), "handling for unresolved live BGP FIB routes: warn, fail, or ignore")
	return cmd
}

type liveSnapshotOptions struct {
	labPath             string
	topologyPath        string
	outputPath          string
	rawDir              string
	fibAllowUnsupported bool
	fibUnresolvedPolicy string
}

func runLiveSnapshot(ctx context.Context, opts liveSnapshotOptions, out io.Writer) error {
	if err := validateFIBUnresolvedPolicy(opts.fibUnresolvedPolicy); err != nil {
		return ExitError{Code: 2, Err: err}
	}
	runner := liveexec.Runner(liveexec.ExecRunner{})
	if opts.rawDir != "" {
		runner = liveexec.NewRawRecordingRunner(runner, opts.rawDir)
	}
	snap, err := livesnapshot.New(
		liverib.NewCollector(runner),
		livefib.NewCollector(runner),
		livesnapshot.WithHashProvider(inputhash.NewProvider()),
		livesnapshot.WithCommitProvider(gitmeta.NewProvider()),
	).Build(ctx, opts.topologyPath, opts.labPath, observation.Options{
		AllowUnsupported: opts.fibAllowUnsupported,
		UnresolvedPolicy: observation.UnresolvedPolicy(opts.fibUnresolvedPolicy),
	})
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	if opts.outputPath == "" || opts.outputPath == "-" {
		data, err := snapshotfile.Marshal(snap)
		if err != nil {
			return ExitError{Code: 2, Err: err}
		}
		_, err = out.Write(data)
		return err
	}
	if err := snapshotfile.Save(opts.outputPath, snap); err != nil {
		return ExitError{Code: 2, Err: err}
	}
	fmt.Fprintf(out, "wrote live snapshot %s\n", opts.outputPath)
	if opts.rawDir != "" {
		fmt.Fprintf(out, "wrote raw command output under %s\n", opts.rawDir)
	}
	return nil
}
