package cli

import (
	"fmt"

	solveradapter "github.com/81ueman/hoyan-lab/internal/adapter/solver"
	"github.com/81ueman/hoyan-lab/internal/usecase/intent"
	"github.com/spf13/cobra"
)

type racingOptions struct {
	lab    string
	format string
}

func NewRacingCommand() *cobra.Command {
	var opts racingOptions
	cmd := &cobra.Command{
		Use:           "racing",
		Short:         "Detect BGP route update racing for a lab topology",
		Long:          `Run BGP route update racing detection (SIGCOMM 2020 §5.4, §7.1, Appendix B) on a lab topology. All routes are propagated regardless of best-path selection, and symbolic conditions are checked for multiple satisfiable route selections.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", args[0])
			}
			if opts.lab == "" {
				return fmt.Errorf("--lab is required")
			}
			labDir, err := resolveLabDir(opts.lab)
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			report, err := intent.DetectRacing(labDir, solveradapter.DefaultBackend())
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			return outputRacingResult(cmd, opts.format, report)
		},
	}
	cmd.Flags().StringVar(&opts.lab, "lab", "", "lab directory or name under labs/")
	cmd.Flags().StringVar(&opts.format, "format", "text", "output format: text or json")
	return cmd
}

func outputRacingResult(cmd *cobra.Command, format string, report *intent.RacingReport) error {
	switch format {
	case "json":
		return writeFormatJSONOnly(cmd.OutOrStdout(), format, report)
	default:
		return outputRacingText(cmd, report)
	}
}

func outputRacingText(cmd *cobra.Command, report *intent.RacingReport) error {
	w := cmd.OutOrStdout()

	if report.Racing {
		fmt.Fprintf(w, "⚠️  BGP route update racing DETECTED!\n")
	} else {
		fmt.Fprintf(w, "✅ No BGP route update racing detected.\n")
	}
	fmt.Fprintf(w, "Lab: %s\n", report.LabPath)
	fmt.Fprintf(w, "Prefixes with racing: %d/%d\n\n", report.PrefixesWithRacing, len(report.Prefixes))

	for _, pr := range report.Prefixes {
		fmt.Fprintf(w, "Prefix: %s\n", pr.Prefix)
		if pr.Racing {
			fmt.Fprintf(w, "  ⚠️  RACING FOUND\n")
		} else {
			fmt.Fprintf(w, "  ✅ No racing\n")
		}
		for _, rr := range pr.Routers {
			fmt.Fprintf(w, "  Router %s: %d/%d satisfiable routes",
				rr.Node, rr.SatisfiableCount, rr.RouteCount)
			if rr.RacingFound {
				fmt.Fprintf(w, " ⚠️")
			}
			fmt.Fprintln(w)
			if rr.FirstModel != nil {
				fmt.Fprintf(w, "    Model 1: %v\n", rr.FirstModel)
			}
			if rr.SecondModel != nil {
				fmt.Fprintf(w, "    Model 2: %v\n", rr.SecondModel)
			}
		}
	}
	return nil
}
