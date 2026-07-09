package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/81ueman/hoyan-lab/internal/adapter/inputhash"
	liveexec "github.com/81ueman/hoyan-lab/internal/adapter/live"
	clabruntime "github.com/81ueman/hoyan-lab/internal/adapter/live/containerlab"
	"github.com/81ueman/hoyan-lab/internal/adapter/snapshotfile"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/usecase/livecheck"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

const (
	labTopologyFile     = "hoyan.clab.yml"
	labIntentPath       = "intent/hoyan.hoyan"
	defaultLabsDir      = "labs"
	defaultLabDir       = "labs/base-wan"
	defaultTopologyPath = defaultLabDir + "/" + labTopologyFile
)

type labDescriptor struct {
	Name        string   `yaml:"name" json:"name"`
	Description string   `yaml:"description" json:"description,omitempty"`
	NOS         []string `yaml:"nos" json:"nos,omitempty"`
	Checks      []string `yaml:"checks" json:"checks,omitempty"`
	Features    []string `yaml:"features" json:"features,omitempty"`
	Path        string   `yaml:"-" json:"path"`
}

func NewLabsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "labs",
		Short:         "List and describe scenario labs",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(NewLabsListCommand(), NewLabsDescribeCommand(), NewLabsCheckCommand())
	return cmd
}

func NewLabsListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "list",
		Short:         "List scenario labs",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unexpected arguments: %s", strings.Join(args, " "))
			}
			return runLabsList(cmd.OutOrStdout())
		},
	}
	return cmd
}

func NewLabsDescribeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "describe <name-or-path>",
		Short:         "Describe a scenario lab",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLabsDescribe(args[0], cmd.OutOrStdout())
		},
	}
	return cmd
}

func NewLabsCheckCommand() *cobra.Command {
	var opts labsLiveCheckOptions
	cmd := &cobra.Command{
		Use:           "check [name-or-path...]",
		Short:         "Run live checks serially for scenario labs",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := opts.validate(); err != nil {
				return err
			}
			if err := runLabsLiveCheck(cmd.Context(), args, opts, cmd.OutOrStdout(), liveexec.ExecRunner{}); err != nil {
				return ExitError{Code: 1, Err: err}
			}
			return nil
		},
	}
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 5*time.Minute, "overall wait timeout per lab")
	cmd.Flags().DurationVar(&opts.pollInterval, "poll-interval", 25*time.Second, "poll interval")
	cmd.Flags().IntVar(&opts.maxPolls, "max-polls", livecheck.DefaultMaxPolls, "maximum BGP collection polls per lab before reporting diffs")
	cmd.Flags().BoolVar(&opts.keepOnFailure, "keep-on-failure", false, "leave a lab running when that lab check fails")
	cmd.Flags().BoolVar(&opts.skipDestroy, "skip-destroy", false, "leave each lab running after its check")
	cmd.Flags().BoolVar(&opts.strictConfig, "strict-config", false, "fail on unsupported config parser statements")
	cmd.Flags().BoolVar(&opts.checkFIB, "check-fib", true, "compare modeled FIB with live installed FIB after BGP convergence")
	cmd.Flags().BoolVar(&opts.noCheckFIB, "no-check-fib", false, "skip modeled-vs-live installed FIB comparison")
	cmd.Flags().BoolVar(&opts.fibAllowUnsupported, "fib-allow-unsupported", false, "ignore next-hops that resolve through nodes without live FIB collection support")
	cmd.Flags().StringVar(&opts.fibUnresolvedPolicy, "fib-unresolved-policy", string(observation.UnresolvedPolicyWarn), "handling for unresolved live BGP FIB routes: warn, fail, or ignore")
	cmd.Flags().BoolVar(&opts.continueOnError, "continue-on-error", false, "continue running later labs after a lab fails")
	return cmd
}

type labsLiveCheckOptions struct {
	strictConfig        bool
	timeout             time.Duration
	pollInterval        time.Duration
	maxPolls            int
	keepOnFailure       bool
	skipDestroy         bool
	checkFIB            bool
	noCheckFIB          bool
	fibAllowUnsupported bool
	fibUnresolvedPolicy string
	continueOnError     bool
}

func (o labsLiveCheckOptions) validate() error {
	if o.timeout <= 0 {
		return fmt.Errorf("--timeout must be greater than zero")
	}
	if o.pollInterval <= 0 {
		return fmt.Errorf("--poll-interval must be greater than zero")
	}
	if o.maxPolls <= 0 {
		return fmt.Errorf("--max-polls must be greater than zero")
	}
	if _, ok := observation.ParseUnresolvedPolicy(o.fibUnresolvedPolicy); !ok {
		return fmt.Errorf("FIB unresolved policy must be one of warn, fail, or ignore")
	}
	return nil
}

func runLabsLiveCheck(ctx context.Context, args []string, opts labsLiveCheckOptions, out io.Writer, runner liveexec.Runner) error {
	labs, err := selectedLabDescriptors(args)
	if err != nil {
		return err
	}
	if len(labs) == 0 {
		return fmt.Errorf("no labs found")
	}
	var failures []string
	for _, lab := range labs {
		topologyPath := filepath.Join(lab.Path, labTopologyFile)
		fmt.Fprintf(out, "==> live check %s (%s)\n", lab.Name, lab.Path)
		topo, runtimeMeta, _, err := topology.LoadDomainTopologyWithRuntime(topologyPath, topology.LoadOptions{StrictConfig: opts.strictConfig})
		if err != nil {
			return err
		}
		env := clabruntime.NewEnvironment(
			topo.Nodes,
			runner,
			observation.Options{AllowUnsupported: opts.fibAllowUnsupported, UnresolvedPolicy: observation.UnresolvedPolicy(opts.fibUnresolvedPolicy)},
			runtimeMeta.RuntimeName,
		)
		usecase, err := livecheck.New(livecheck.Dependencies{
			Runtime:            env,
			Collector:          env,
			SnapshotRepository: snapshotfile.NewRepository(),
			InputHashChecker:   inputhash.NewProvider(),
		})
		if err != nil {
			return err
		}
		err = usecase.Run(ctx, livecheck.Options{
			Topology:      topologyPath,
			StrictConfig:  opts.strictConfig,
			Timeout:       opts.timeout,
			PollInterval:  opts.pollInterval,
			MaxPolls:      opts.maxPolls,
			KeepOnFailure: opts.keepOnFailure,
			SkipDestroy:   opts.skipDestroy,
			CheckFIB:      opts.checkFIB && !opts.noCheckFIB,
			FIBOptions:    observation.Options{AllowUnsupported: opts.fibAllowUnsupported, UnresolvedPolicy: observation.UnresolvedPolicy(opts.fibUnresolvedPolicy)},
			Out:           out,
		})
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", lab.Name, err))
			fmt.Fprintf(out, "[FAIL] %s: %v\n", lab.Name, err)
			if !opts.continueOnError {
				return fmt.Errorf("live check failed for %s: %w", lab.Name, err)
			}
			continue
		}
		fmt.Fprintf(out, "[PASS] %s\n", lab.Name)
	}
	if len(failures) > 0 {
		return fmt.Errorf("%d lab live check(s) failed: %s", len(failures), strings.Join(failures, "; "))
	}
	return nil
}

func selectedLabDescriptors(args []string) ([]labDescriptor, error) {
	if len(args) == 0 {
		return loadLabDescriptors(defaultLabsDir)
	}
	labs := make([]labDescriptor, 0, len(args))
	for _, arg := range args {
		labDir, err := resolveLabDir(arg)
		if err != nil {
			return nil, err
		}
		desc, err := loadLabDescriptor(labDir)
		if err != nil {
			return nil, err
		}
		labs = append(labs, desc)
	}
	return labs, nil
}

func runLabsList(out io.Writer) error {
	labs, err := loadLabDescriptors(defaultLabsDir)
	if err != nil {
		return err
	}
	if len(labs) == 0 {
		fmt.Fprintln(out, "no labs found")
		return nil
	}
	for _, lab := range labs {
		fmt.Fprintf(out, "%s\t%s\t%s\n", lab.Name, lab.Path, lab.Description)
	}
	return nil
}

func runLabsDescribe(raw string, out io.Writer) error {
	labDir, err := resolveLabDir(raw)
	if err != nil {
		return err
	}
	desc, err := loadLabDescriptor(labDir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "name: %s\n", desc.Name)
	fmt.Fprintf(out, "path: %s\n", desc.Path)
	if desc.Description != "" {
		fmt.Fprintf(out, "description: %s\n", desc.Description)
	}
	writeStringList(out, "nos", desc.NOS)
	writeStringList(out, "checks", desc.Checks)
	writeStringList(out, "features", desc.Features)
	fmt.Fprintf(out, "topology: %s\n", filepath.Join(desc.Path, labTopologyFile))
	return nil
}

func writeStringList(out io.Writer, name string, values []string) {
	if len(values) == 0 {
		return
	}
	fmt.Fprintf(out, "%s: %s\n", name, strings.Join(values, ", "))
}

func loadLabDescriptors(root string) ([]labDescriptor, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	labs := make([]labDescriptor, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		desc, err := loadLabDescriptor(filepath.Join(root, entry.Name()))
		if err != nil {
			return nil, err
		}
		labs = append(labs, desc)
	}
	sort.Slice(labs, func(i, j int) bool {
		return labs[i].Name < labs[j].Name
	})
	return labs, nil
}

func loadLabDescriptor(labDir string) (labDescriptor, error) {
	path := filepath.Join(labDir, "lab.yml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return labDescriptor{
				Name: filepath.Base(labDir),
				Path: filepath.Clean(labDir),
			}, nil
		}
		return labDescriptor{}, err
	}
	var desc labDescriptor
	if err := yaml.Unmarshal(data, &desc); err != nil {
		return labDescriptor{}, fmt.Errorf("%s: %w", path, err)
	}
	if desc.Name == "" {
		desc.Name = filepath.Base(labDir)
	}
	desc.Path = filepath.Clean(labDir)
	return desc, nil
}
