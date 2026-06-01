package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/spf13/cobra"
)

func NewTopologyCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "topology",
		Short:         "Render and transform topology files",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(NewRenderTopologyCommand())
	return cmd
}

func NewRenderTopologyCommand() *cobra.Command {
	var opts renderTopologyOptions
	cmd := &cobra.Command{
		Use:           "render",
		Short:         "Render an isolated containerlab topology",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			if err := resolveLabInputs(cmd, opts.labPath, &opts.topologyPath); err != nil {
				return err
			}
			if err := runRenderTopology(opts, cmd.OutOrStdout()); err != nil {
				return ExitError{Code: 2, Err: err}
			}
			return nil
		},
	}
	addLabFlag(cmd, &opts.labPath)
	addTopologyFlag(cmd, &opts.topologyPath, "source containerlab topology YAML")
	cmd.Flags().StringVar(&opts.outputPath, "output", "-", "generated topology path, or - for stdout")
	cmd.Flags().StringVar(&opts.suffix, "suffix", "", "isolation suffix appended to the lab name")
	cmd.Flags().StringVar(&opts.labName, "lab-name", "", "generated lab name")
	cmd.Flags().StringVar(&opts.mgmtNetwork, "mgmt-network", "", "generated Docker management network name")
	cmd.Flags().StringVar(&opts.mgmtSubnet, "mgmt-subnet", "", "generated Docker management IPv4 /24 subnet")
	return cmd
}

type renderTopologyOptions struct {
	labPath      string
	topologyPath string
	outputPath   string
	suffix       string
	labName      string
	mgmtNetwork  string
	mgmtSubnet   string
}

func runRenderTopology(opts renderTopologyOptions, out io.Writer) error {
	data, err := os.ReadFile(opts.topologyPath)
	if err != nil {
		return err
	}
	sourceDir, err := filepath.Abs(filepath.Dir(opts.topologyPath))
	if err != nil {
		return err
	}
	renderOpts := model.TopologyRenderOptions{
		Suffix:      opts.suffix,
		LabName:     opts.labName,
		MgmtNetwork: opts.mgmtNetwork,
		MgmtSubnet:  opts.mgmtSubnet,
	}
	if shouldRewriteConfigPaths(sourceDir, opts.outputPath) {
		renderOpts.SourceDir = sourceDir
	}
	rendered, err := model.RenderIsolatedTopology(data, renderOpts)
	if err != nil {
		return err
	}
	if opts.outputPath == "" || opts.outputPath == "-" {
		_, err := out.Write(rendered)
		return err
	}
	return os.WriteFile(opts.outputPath, rendered, 0o644)
}

func shouldRewriteConfigPaths(sourceDir, outputPath string) bool {
	if outputPath == "" || outputPath == "-" {
		return false
	}
	outputDir, err := filepath.Abs(filepath.Dir(outputPath))
	if err != nil {
		return true
	}
	return outputDir != sourceDir
}
