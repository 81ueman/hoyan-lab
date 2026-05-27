package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/facts"
	"github.com/81ueman/hoyan-lab/internal/intent"
	"github.com/spf13/cobra"
)

type intentOptions struct {
	file   string
	lab    string
	format string
}

type factsOptions struct {
	labPath string
	format  string
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
			report, err := intent.Verify(doc)
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

func NewFactsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "facts",
		Short:         "Emit modeled fact tables",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(NewFactsRIBCommand(), NewFactsFIBCommand())
	return cmd
}

func NewFactsRIBCommand() *cobra.Command {
	var opts factsOptions
	cmd := &cobra.Command{
		Use:           "rib",
		Short:         "Emit modeled RIB facts",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			snapshot, err := facts.Build(opts.labPath, "current")
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			return writeFormatJSONOnly(cmd.OutOrStdout(), opts.format, snapshot.RIB)
		},
	}
	addFactsFlags(cmd, &opts)
	return cmd
}

func NewFactsFIBCommand() *cobra.Command {
	var opts factsOptions
	cmd := &cobra.Command{
		Use:           "fib",
		Short:         "Emit modeled FIB facts",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			snapshot, err := facts.Build(opts.labPath, "current")
			if err != nil {
				return ExitError{Code: 2, Err: err}
			}
			return writeFormatJSONOnly(cmd.OutOrStdout(), opts.format, snapshot.FIB)
		},
	}
	addFactsFlags(cmd, &opts)
	return cmd
}

func addIntentInputFlags(cmd *cobra.Command, opts *intentOptions) {
	cmd.Flags().StringVar(&opts.file, "file", "", "intent YAML file")
	cmd.Flags().StringVar(&opts.lab, "lab", "", "scenario lab directory; reads intent/hoyan.yml")
}

func addFactsFlags(cmd *cobra.Command, opts *factsOptions) {
	cmd.Flags().StringVar(&opts.labPath, "lab", defaultLabDir, "scenario lab directory")
	cmd.Flags().StringVar(&opts.format, "format", "json", "output format: json")
}

func loadIntentFile(path string) (*intent.Document, error) {
	if path == "" {
		return nil, fmt.Errorf("--file is required")
	}
	return intent.Load(path)
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

func writeFormatJSONOnly(out io.Writer, format string, value any) error {
	if format != "json" {
		return ExitError{Code: 2, Err: fmt.Errorf("--format must be %q", "json")}
	}
	enc := json.NewEncoder(out)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(value)
}
