package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/usecase/intent"
	"github.com/spf13/cobra"
)

func NewVerifyCommand() *cobra.Command {
	var opts verifyOptions
	cmd := &cobra.Command{
		Use:           "verify",
		Short:         "Run intent verification checks",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := resolveLabInputs(cmd, opts.labPath, &opts.topologyPath); err != nil {
				return err
			}
			path, ok := labIntentFile(opts.labPath)
			if !ok {
				return ExitError{Code: 2, Err: fmt.Errorf("intent file not found for lab %q", opts.labPath)}
			}
			return runIntentVerifyFile(cmd, path, opts.format)
		},
	}
	addLabFlag(cmd, &opts.labPath)
	addTopologyFlag(cmd, &opts.topologyPath, "containerlab topology YAML")
	cmd.Flags().BoolVar(&opts.strictConfig, "strict-config", false, "fail on unsupported config parser statements")
	cmd.Flags().StringVar(&opts.format, "format", "table", "output format: table or json")
	return cmd
}

type verifyOptions struct {
	labPath      string
	topologyPath string
	strictConfig bool
	format       string
}

func labIntentFile(labPath string) (string, bool) {
	labDir, err := resolveLabDir(labPath)
	if err != nil {
		labDir = defaultLabDir
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
