package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/adapter/intentfile"
	"github.com/81ueman/hoyan-lab/internal/adapter/solver"
	domainintent "github.com/81ueman/hoyan-lab/internal/domain/intent"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/intent"
	"github.com/spf13/cobra"
)

type intentOptions struct {
	file   string
	lab    string
	format string
}

func NewIntentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "intent",
		Short:         "Validate, expand, and verify intent DSL files",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(NewIntentValidateCommand(), NewIntentExpandCommand(), NewIntentVerifyCommand())
	return cmd
}

func NewIntentValidateCommand() *cobra.Command {
	var opts intentOptions
	cmd := &cobra.Command{
		Use:           "validate",
		Short:         "Validate an intent file",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			path, err := resolveIntentInput(cmd, opts)
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			doc, err := loadIntentFile(path)
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			if err := intent.Validate(doc); err != nil {
				return ExitError{Code: 2, Err: err}
			}
			return nil
		},
	}
	addIntentInputFlags(cmd, &opts)
	return cmd
}

func NewIntentExpandCommand() *cobra.Command {
	var opts intentOptions
	cmd := &cobra.Command{
		Use:           "expand",
		Short:         "Expand vars and forall clauses in an intent file",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			path, err := resolveIntentInput(cmd, opts)
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			doc, err := loadIntentFile(path)
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			expanded, err := intent.Expand(doc)
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			return writeFormatJSONOnly(cmd.OutOrStdout(), opts.format, expanded)
		},
	}
	addIntentInputFlags(cmd, &opts)
	cmd.Flags().StringVar(&opts.format, "format", "json", "output format: json")
	return cmd
}

func NewIntentVerifyCommand() *cobra.Command {
	var opts intentOptions
	cmd := &cobra.Command{
		Use:           "verify",
		Short:         "Verify RIB/FIB intents against modeled facts",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			path, err := resolveIntentInput(cmd, opts)
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			doc, err := loadIntentFile(path)
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			if err := resolveIntentSnapshotLabs(doc); err != nil {
				return ExitError{Code: 2, Err: err}
			}
			report, err := intent.VerifyWithProvider(doc, intent.DefaultSnapshotProvider{
				GraphOptions: []sim.GraphOption{sim.WithSolverBackend(solveradapter.DefaultBackend())},
			})
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			if err := writeFormatJSONOnly(cmd.OutOrStdout(), opts.format, report); err != nil {
				return err
			}
			if report.Summary.Failed > 0 {
				return ExitError{Code: 1, Err: fmt.Errorf("intent verification failed")}
			}
			return nil
		},
	}
	addIntentInputFlags(cmd, &opts)
	cmd.Flags().StringVar(&opts.format, "format", "json", "output format: json")
	return cmd
}

func addIntentInputFlags(cmd *cobra.Command, opts *intentOptions) {
	cmd.Flags().StringVar(&opts.file, "file", "", "intent YAML file")
	cmd.Flags().StringVar(&opts.lab, "lab", "", "scenario lab directory; reads intent/hoyan.yml")
}

func loadIntentFile(path string) (*domainintent.Document, error) {
	if path == "" {
		return nil, fmt.Errorf("--file is required")
	}
	return intentfile.Load(path)
}

func resolveIntentInput(cmd *cobra.Command, opts intentOptions) (string, error) {
	fileChanged := cmd.Flags().Changed("file")
	labChanged := cmd.Flags().Changed("lab")
	if fileChanged && labChanged {
		return "", fmt.Errorf("--file and --lab are mutually exclusive")
	}
	if fileChanged {
		return opts.file, nil
	}
	if labChanged {
		labDir, err := resolveLabDir(opts.lab)
		if err != nil {
			return "", err
		}
		return filepath.Join(labDir, labIntentPath), nil
	}
	return "", fmt.Errorf("--file or --lab is required")
}

func resolveIntentSnapshotLabs(doc *domainintent.Document) error {
	for name, snapshot := range doc.Snapshots {
		labDir, err := resolveLabDir(snapshot.Lab)
		if err != nil {
			return fmt.Errorf("snapshots[%q].lab: %w", name, err)
		}
		snapshot.Lab = labDir
		doc.Snapshots[name] = snapshot
	}
	return nil
}
