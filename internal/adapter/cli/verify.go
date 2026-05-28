package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/adapter/queryfile"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/intent"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
	"github.com/81ueman/hoyan-lab/internal/usecase/verify"
	"github.com/spf13/cobra"
)

func NewVerifyCommand() *cobra.Command {
	var opts verifyOptions
	cmd := &cobra.Command{
		Use:           "verify",
		Short:         "Run offline route, packet, and failure reachability checks",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := resolveLabInputs(cmd, opts.labPath, &opts.topologyPath, &opts.queriesPath); err != nil {
				return err
			}
			if !cmd.Flags().Changed("queries") {
				if path, ok := labIntentFile(opts.labPath); ok {
					return runIntentVerifyFile(cmd, path, opts.format)
				}
			}
			if err := runVerify(cmd.Context(), opts, cmd.OutOrStdout(), cmd.ErrOrStderr()); err != nil {
				return err
			}
			return nil
		},
	}
	addLabFlag(cmd, &opts.labPath)
	addTopologyFlag(cmd, &opts.topologyPath, "containerlab topology YAML")
	addQueriesFlag(cmd, &opts.queriesPath, "query YAML")
	cmd.Flags().BoolVar(&opts.strictConfig, "strict-config", false, "fail on unsupported config parser statements")
	cmd.Flags().IntVar(&opts.maxPrefixClasses, "max-prefix-classes", 10000, "maximum PrefixUniverse classes before failing; 0 disables the guard")
	cmd.Flags().BoolVar(&opts.showPrefixUniverseStats, "show-prefix-universe-stats", false, "show PrefixUniverse build statistics")
	cmd.Flags().BoolVar(&opts.noCollapse, "no-collapse", false, "show raw prefix-class results instead of collapsed equivalent groups")
	cmd.Flags().StringVar(&opts.format, "format", "table", "output format: table or json")
	return cmd
}

type verifyOptions struct {
	labPath                 string
	topologyPath            string
	queriesPath             string
	strictConfig            bool
	maxPrefixClasses        int
	showPrefixUniverseStats bool
	noCollapse              bool
	format                  string
}

func labIntentFile(labPath string) (string, bool) {
	labDir, err := resolveLabDir(labPath)
	if err != nil {
		return "", false
	}
	path := filepath.Join(labDir, labIntentPath)
	info, err := os.Stat(path)
	return path, err == nil && !info.IsDir()
}

func runIntentVerifyFile(cmd *cobra.Command, path string, format string) error {
	doc, err := loadIntentFile(path)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	report, err := intent.Verify(doc)
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	if format == "json" {
		if err := writeFormatJSONOnly(cmd.OutOrStdout(), "json", report); err != nil {
			return err
		}
	} else if format == "" || format == "table" {
		for _, result := range report.Results {
			status := strings.ToUpper(result.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s table=%s scenario=%s\n", status, result.Name, result.Table, result.Scenario)
			if result.Actual.Reachable != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "  reachable: %v\n", *result.Actual.Reachable)
			}
			if result.Actual.Reason != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "  reason: %s\n", result.Actual.Reason)
			}
		}
	} else {
		return fmt.Errorf("unsupported --format %q", format)
	}
	if report.Summary.Failed > 0 {
		return ExitError{Code: 1, Err: fmt.Errorf("intent verification failed")}
	}
	return nil
}

func runVerify(_ context.Context, opts verifyOptions, out, errOut io.Writer) error {
	topo, warnings, err := topology.LoadTopologyWithOptions(opts.topologyPath, topology.LoadOptions{
		CollectWarnings: true,
		StrictConfig:    opts.strictConfig,
	})
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		fmt.Fprintf(errOut, "warning: %s\n", warning)
	}
	queries, err := queryfile.Load(opts.queriesPath)
	if err != nil {
		return err
	}
	verifyOpts := verify.VerifyOptions{
		CollapseEquivalentResults: !opts.noCollapse,
		MaxPrefixClasses:          opts.maxPrefixClasses,
	}
	report := verify.RunWithOptions(topo, queries, verifyOpts)
	if opts.format == "json" {
		enc := json.NewEncoder(out)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		jsonReport := report
		if !opts.showPrefixUniverseStats {
			jsonReport.Stats = nil
		}
		if err := enc.Encode(jsonReport); err != nil {
			return err
		}
		if !report.OK() {
			return ExitError{Code: 1, Err: fmt.Errorf("verification failed")}
		}
		return nil
	}
	if opts.format != "" && opts.format != "table" {
		return fmt.Errorf("unsupported --format %q", opts.format)
	}
	if opts.showPrefixUniverseStats && report.Stats != nil {
		writePrefixUniverseStats(out, *report.Stats)
	}
	for _, result := range report.Results {
		status := "PASS"
		if result.Metadata.Reachable != result.Metadata.Expected {
			status = "FAIL"
		}
		fmt.Fprintf(out, "[%s] %s reachable=%v expected=%v\n", status, result.Name, result.Metadata.Reachable, result.Metadata.Expected)
		if result.PrefixClass != nil && len(result.PrefixClass.ClassIDs) > 0 {
			fmt.Fprintf(out, "  classes: %s\n", formatClassIDs(result.PrefixClass.ClassIDs))
		}
		if result.PrefixClass != nil && len(result.PrefixClass.Spaces) > 0 {
			fmt.Fprintf(out, "  spaces: %s\n", strings.Join(result.PrefixClass.Spaces, ", "))
		} else if result.PrefixClass != nil && result.PrefixClass.Space != "" {
			fmt.Fprintf(out, "  space: %s\n", result.PrefixClass.Space)
		}
		if result.PrefixClass != nil && len(result.PrefixClass.MatchedPredicates) > 0 {
			fmt.Fprintf(out, "  matched predicates: %s\n", strings.Join(result.PrefixClass.MatchedPredicates, ", "))
		}
		if path := result.Path(); len(path.Nodes) > 0 {
			fmt.Fprintf(out, "  path: %s\n", sim.FormatPath(path))
		}
		if counterexample := result.Counterexample(); len(counterexample) > 0 {
			fmt.Fprintf(out, "  counterexample: %s\n", strings.Join(counterexample, ", "))
		}
		if result.Metadata.Reason != "" {
			fmt.Fprintf(out, "  reason: %s\n", result.Metadata.Reason)
		}
	}
	if !report.OK() {
		return ExitError{Code: 1, Err: fmt.Errorf("verification failed")}
	}
	return nil
}
