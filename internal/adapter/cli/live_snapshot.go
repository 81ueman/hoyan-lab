package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/usecase/fibcompare"
	"github.com/81ueman/hoyan-lab/internal/usecase/livesnapshot"
	"github.com/81ueman/hoyan-lab/internal/usecase/ribcompare"
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
	cmd.Flags().StringVar(&opts.fibUnresolvedPolicy, "fib-unresolved-policy", string(fibcompare.UnresolvedPolicyWarn), "handling for unresolved live BGP FIB routes: warn, fail, or ignore")
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
	snap, err := livesnapshot.Build(ctx, opts.topologyPath, opts.labPath, ribcompare.ExecRunner{}, opts.rawDir, fibcompare.Options{
		AllowUnsupported: opts.fibAllowUnsupported,
		UnresolvedPolicy: fibcompare.UnresolvedPolicy(opts.fibUnresolvedPolicy),
	})
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	if opts.outputPath == "" || opts.outputPath == "-" {
		data, err := livesnapshot.Marshal(snap)
		if err != nil {
			return ExitError{Code: 2, Err: err}
		}
		_, err = out.Write(data)
		return err
	}
	if err := livesnapshot.Save(opts.outputPath, snap); err != nil {
		return ExitError{Code: 2, Err: err}
	}
	fmt.Fprintf(out, "wrote live snapshot %s\n", opts.outputPath)
	if opts.rawDir != "" {
		fmt.Fprintf(out, "wrote raw command output under %s\n", opts.rawDir)
	}
	return nil
}
