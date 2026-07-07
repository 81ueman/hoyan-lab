package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/usecase/modelinspect"
	"github.com/spf13/cobra"
)

const (
	modelFormatTable = "table"
	modelFormatJSON  = "json"
)

type modelInspectOptions struct {
	LabPath          string
	TopologyPath     string
	Node             string
	Prefix           string
	Format           string
	From             string
	To               string
	Protocol         string
	DstPort          int
	StrictConfig     bool
	ShowCond         bool
	ShowPreds        bool
	Summary          bool
	MaxPrefixClasses int
}

func NewModelCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "model",
		Short:         "Inspect modeled RIB, FIB, and symbolic reachability",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(NewModelRIBCommand(), NewModelFIBCommand(), NewModelSymbolicPacketCommand(), NewModelSymbolicRouteCommand(), NewModelPrefixClassesCommand(), NewModelPacketClassesCommand())
	return cmd
}

func NewModelPrefixClassesCommand() *cobra.Command {
	var opts modelInspectOptions
	cmd := &cobra.Command{
		Use:           "prefix-classes",
		Short:         "Inspect PrefixUniverse prefix classes",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := resolveLabInputs(cmd, opts.LabPath, &opts.TopologyPath); err != nil {
				return err
			}
			return runModelPrefixClasses(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	addLabFlag(cmd, &opts.LabPath)
	addTopologyFlag(cmd, &opts.TopologyPath, "containerlab topology YAML")
	cmd.Flags().StringVar(&opts.Prefix, "prefix", "", "prefix overlap filter")
	cmd.Flags().StringVar(&opts.Format, "format", modelFormatTable, "output format: table or json")
	cmd.Flags().BoolVar(&opts.ShowPreds, "show-predicates", false, "show matched prefix predicates in table output")
	cmd.Flags().BoolVar(&opts.Summary, "summary", false, "show PrefixUniverse build statistics before table output")
	cmd.Flags().IntVar(&opts.MaxPrefixClasses, "max-prefix-classes", 10000, "maximum PrefixUniverse classes before failing; 0 disables the guard")
	return cmd
}

func NewModelPacketClassesCommand() *cobra.Command {
	var opts modelInspectOptions
	cmd := &cobra.Command{
		Use:           "packet-classes",
		Short:         "Inspect HeaderSpace packet classes",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := resolveLabInputs(cmd, opts.LabPath, &opts.TopologyPath); err != nil {
				return err
			}
			return runModelPacketClasses(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	addLabFlag(cmd, &opts.LabPath)
	addTopologyFlag(cmd, &opts.TopologyPath, "containerlab topology YAML")
	cmd.Flags().BoolVar(&opts.StrictConfig, "strict-config", false, "fail on unsupported config parser statements")
	cmd.Flags().StringVar(&opts.Prefix, "prefix", "", "destination prefix overlap filter")
	cmd.Flags().StringVar(&opts.Protocol, "protocol", "", "protocol filter")
	cmd.Flags().IntVar(&opts.DstPort, "dst-port", 0, "destination transport port filter")
	cmd.Flags().StringVar(&opts.Format, "format", modelFormatTable, "output format: table or json")
	cmd.Flags().BoolVar(&opts.ShowPreds, "show-predicates", false, "show matched header predicates in table output")
	return cmd
}

func NewModelRIBCommand() *cobra.Command {
	var opts modelInspectOptions
	cmd := &cobra.Command{
		Use:           "rib [bgp|connected|static|ospf|aggregate|blackhole]",
		Short:         "Inspect modeled RIB entries",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if len(args) == 1 {
				opts.Protocol = args[0]
			}
			if err := resolveLabInputs(cmd, opts.LabPath, &opts.TopologyPath); err != nil {
				return err
			}
			return runModelRIB(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	addModelCommonFlags(cmd, &opts)
	return cmd
}

func NewModelFIBCommand() *cobra.Command {
	var opts modelInspectOptions
	cmd := &cobra.Command{
		Use:           "fib",
		Short:         "Inspect modeled FIB entries",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := resolveLabInputs(cmd, opts.LabPath, &opts.TopologyPath); err != nil {
				return err
			}
			return runModelFIB(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	addModelCommonFlags(cmd, &opts)
	return cmd
}

func NewModelSymbolicPacketCommand() *cobra.Command {
	var opts modelInspectOptions
	cmd := &cobra.Command{
		Use:           "symbolic-packet",
		Short:         "Inspect symbolic packet reachability",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := resolveLabInputs(cmd, opts.LabPath, &opts.TopologyPath); err != nil {
				return err
			}
			return runModelSymbolicPacket(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	addLabFlag(cmd, &opts.LabPath)
	addTopologyFlag(cmd, &opts.TopologyPath, "containerlab topology YAML")
	cmd.Flags().BoolVar(&opts.StrictConfig, "strict-config", false, "fail on unsupported config parser statements")
	cmd.Flags().StringVar(&opts.Format, "format", modelFormatTable, "output format: table or json")
	cmd.Flags().StringVar(&opts.From, "from", "", "source node")
	cmd.Flags().StringVar(&opts.To, "to", "", "destination IP address")
	cmd.Flags().StringVar(&opts.Protocol, "protocol", "tcp", "packet protocol")
	cmd.Flags().IntVar(&opts.DstPort, "dst-port", 0, "destination transport port")
	cmd.Flags().BoolVar(&opts.ShowCond, "show-conditions", false, "show symbolic conditions in table output")
	return cmd
}

func NewModelSymbolicRouteCommand() *cobra.Command {
	var opts modelInspectOptions
	cmd := &cobra.Command{
		Use:           "symbolic-route",
		Short:         "Inspect symbolic route reachability",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := resolveLabInputs(cmd, opts.LabPath, &opts.TopologyPath); err != nil {
				return err
			}
			return runModelSymbolicRoute(cmd.Context(), opts, cmd.OutOrStdout())
		},
	}
	addLabFlag(cmd, &opts.LabPath)
	addTopologyFlag(cmd, &opts.TopologyPath, "containerlab topology YAML")
	cmd.Flags().BoolVar(&opts.StrictConfig, "strict-config", false, "fail on unsupported config parser statements")
	cmd.Flags().StringVar(&opts.Format, "format", modelFormatTable, "output format: table or json")
	cmd.Flags().StringVar(&opts.From, "from", "", "source node")
	cmd.Flags().StringVar(&opts.Prefix, "prefix", "", "destination prefix")
	cmd.Flags().BoolVar(&opts.ShowCond, "show-conditions", false, "show symbolic conditions in table output")
	cmd.Flags().BoolVar(&opts.ShowPreds, "show-predicates", false, "show matched prefix predicates in table output")
	return cmd
}

func addModelCommonFlags(cmd *cobra.Command, opts *modelInspectOptions) {
	addLabFlag(cmd, &opts.LabPath)
	addTopologyFlag(cmd, &opts.TopologyPath, "containerlab topology YAML")
	cmd.Flags().BoolVar(&opts.StrictConfig, "strict-config", false, "fail on unsupported config parser statements")
	cmd.Flags().StringVar(&opts.Node, "node", "", "node name filter")
	cmd.Flags().StringVar(&opts.Prefix, "prefix", "", "prefix filter")
	cmd.Flags().StringVar(&opts.Format, "format", modelFormatTable, "output format: table or json")
	cmd.Flags().BoolVar(&opts.ShowCond, "show-conditions", false, "show symbolic conditions in table output")
}

func modelInspectRequest(opts modelInspectOptions) modelinspect.Request {
	return modelinspect.Request{
		TopologyPath:     opts.TopologyPath,
		Node:             opts.Node,
		Prefix:           opts.Prefix,
		From:             opts.From,
		To:               opts.To,
		Protocol:         opts.Protocol,
		DstPort:          opts.DstPort,
		StrictConfig:     opts.StrictConfig,
		MaxPrefixClasses: opts.MaxPrefixClasses,
	}
}

func runModelRIB(_ context.Context, opts modelInspectOptions, out io.Writer) error {
	result, err := modelinspect.InspectRIB(modelInspectRequest(opts))
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	switch opts.Format {
	case modelFormatTable:
		return writeRIBTable(out, result.Rows, opts.ShowCond, result.Protocol)
	case modelFormatJSON:
		return writeJSON(out, result.Rows)
	default:
		return ExitError{Code: 2, Err: fmt.Errorf("--format must be %q or %q", modelFormatTable, modelFormatJSON)}
	}
}

func runModelFIB(_ context.Context, opts modelInspectOptions, out io.Writer) error {
	result, err := modelinspect.InspectFIB(modelInspectRequest(opts))
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	switch opts.Format {
	case modelFormatTable:
		return writeFIBTable(out, result.Rows, opts.ShowCond)
	case modelFormatJSON:
		return writeJSON(out, result.Rows)
	default:
		return ExitError{Code: 2, Err: fmt.Errorf("--format must be %q or %q", modelFormatTable, modelFormatJSON)}
	}
}

func runModelPrefixClasses(_ context.Context, opts modelInspectOptions, out io.Writer) error {
	result, err := modelinspect.InspectPrefixClasses(modelInspectRequest(opts))
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	switch opts.Format {
	case modelFormatTable:
		if opts.Summary {
			writePrefixUniverseStats(out, result.Stats)
		}
		return writePrefixClassTable(out, result.Classes, opts.ShowPreds)
	case modelFormatJSON:
		if opts.Summary {
			return writeJSON(out, struct {
				Stats   model.PrefixUniverseStats     `json:"prefix_universe_stats"`
				Classes []modelinspect.PrefixClassRow `json:"classes"`
			}{Stats: result.Stats, Classes: result.Classes})
		}
		return writeJSON(out, result.Classes)
	default:
		return ExitError{Code: 2, Err: fmt.Errorf("--format must be %q or %q", modelFormatTable, modelFormatJSON)}
	}
}

func runModelPacketClasses(_ context.Context, opts modelInspectOptions, out io.Writer) error {
	result, err := modelinspect.InspectPacketClasses(modelInspectRequest(opts))
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	switch opts.Format {
	case modelFormatTable:
		return writePacketClassTable(out, result.Classes, opts.ShowPreds)
	case modelFormatJSON:
		return writeJSON(out, result.Classes)
	default:
		return ExitError{Code: 2, Err: fmt.Errorf("--format must be %q or %q", modelFormatTable, modelFormatJSON)}
	}
}

func runModelSymbolicPacket(_ context.Context, opts modelInspectOptions, out io.Writer) error {
	result, err := modelinspect.InspectSymbolicPacket(modelInspectRequest(opts))
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	switch opts.Format {
	case modelFormatTable:
		return writeSymbolicPacketTable(out, result, opts.ShowCond)
	case modelFormatJSON:
		return writeJSON(out, result)
	default:
		return ExitError{Code: 2, Err: fmt.Errorf("--format must be %q or %q", modelFormatTable, modelFormatJSON)}
	}
}

func runModelSymbolicRoute(_ context.Context, opts modelInspectOptions, out io.Writer) error {
	result, err := modelinspect.InspectSymbolicRoute(modelInspectRequest(opts))
	if err != nil {
		return ExitError{Code: 2, Err: err}
	}
	switch opts.Format {
	case modelFormatTable:
		return writeSymbolicRouteTable(out, result.Results, opts.ShowCond, opts.ShowPreds)
	case modelFormatJSON:
		return writeJSON(out, result.Results)
	default:
		return ExitError{Code: 2, Err: fmt.Errorf("--format must be %q or %q", modelFormatTable, modelFormatJSON)}
	}
}
