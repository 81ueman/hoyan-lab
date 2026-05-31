package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/81ueman/hoyan-lab/internal/adapter/snapshotfile"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/spf13/cobra"
)

func NewCollectCommand() *cobra.Command {
	var opts collectOptions
	cmd := &cobra.Command{
		Use:           "collect <path>",
		Short:         "Collect a target into a canonical snapshot",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.path = args[0]
			return runCollect(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	cmd.Flags().StringVar(&opts.targetType, "type", "", "collector type: model, clab, snapshot, device")
	cmd.Flags().StringVar(&opts.outputPath, "out", "", "snapshot JSON output path, or - for stdout")
	cmd.Flags().StringVar(&opts.afi, "afi", "", "address family filter: ipv4 or ipv6")
	cmd.Flags().BoolVar(&opts.includeInactive, "include-inactive", false, "include inactive/non-best RIB routes")
	cmd.Flags().BoolVar(&opts.includeModelInfo, "include-model-info", false, "preserve simulator/model explanation metadata")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

type collectOptions struct {
	path             string
	targetType       string
	outputPath       string
	afi              string
	includeInactive  bool
	includeModelInfo bool
}

func runCollect(ctx context.Context, opts collectOptions, out io.Writer) error {
	target, err := newCollectorTarget(opts.path, opts.targetType)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	collectOpts, err := collectOptionsFromCollectOptions(opts)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	collector, err := resolveCollector(ctx, target)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	snap, err := observation.CollectSnapshot(ctx, collector, collectOpts)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	if err := snapshotfile.SaveObservation(opts.outputPath, snap); err != nil {
		return ExitError{Code: 2, Err: err}
	}
	if opts.outputPath != "" && opts.outputPath != "-" {
		fmt.Fprintf(out, "wrote snapshot %s\n", opts.outputPath)
	}
	return nil
}

func collectOptionsFromCollectOptions(opts collectOptions) (observation.CollectOptions, error) {
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
