package livecheck

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/collect"
)

type RIBFailureScenario struct {
	Name        string
	Failures    sim.FailureSet
	ActiveNodes []model.Node
	Inject      func(context.Context, FailureRuntime) error
	Cleanup     func(context.Context, FailureRuntime) error
}

type RIBFailureCheckOptions struct {
	Interval       time.Duration
	MaxPolls       int
	CompareOptions observation.CompareOptions
	Out            io.Writer
}

func CompareRIBsWithFailures(ctx context.Context, runtime FailureRuntime, collector RIBCollector, topo *model.Topology, scenario RIBFailureScenario, opts RIBFailureCheckOptions) error {
	if opts.Interval == 0 {
		opts.Interval = 25 * time.Second
	}
	if opts.MaxPolls == 0 {
		opts.MaxPolls = DefaultMaxPolls
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	compareOptions := opts.CompareOptions
	if isZeroCompareOptions(compareOptions) {
		compareOptions = observation.DefaultCompareOptions()
	}
	activeNodes := scenario.ActiveNodes
	if activeNodes == nil {
		activeNodes = topo.Nodes
	}
	simulator, err := collect.NewSimulator(topo)
	if err != nil {
		return err
	}
	expected, err := collectRIBRoutes(ctx, simulator.CollectorFor(scenario.Failures), activeNodes, observation.CollectOptions{IncludeInactive: true, IncludeModelInfo: true})
	if err != nil {
		return err
	}
	if scenario.Inject != nil {
		fmt.Fprintf(opts.Out, "injecting failure scenario %s\n", scenario.Name)
		if err := scenario.Inject(ctx, runtime); err != nil {
			return err
		}
	}
	if scenario.Cleanup != nil {
		defer func() {
			_ = scenario.Cleanup(context.Background(), runtime)
		}()
	}
	actual, diffs, err := WaitForMatchingRIBs(ctx, collector, activeNodes, expected, opts.Interval, opts.MaxPolls, compareOptions)
	if err != nil {
		printRIBDiffs(opts.Out, expected, actual, compareOptions)
		return err
	}
	printDiffs(opts.Out, diffs)
	if !diffs.OK {
		return fmt.Errorf("failure scenario %s found live BGP RIB diff(s)", scenario.Name)
	}
	fmt.Fprintf(opts.Out, "failure scenario %s live BGP RIBs match modeled paths\n", scenario.Name)
	return nil
}

func LinkFailureScenario(topo *model.Topology, linkName string) (RIBFailureScenario, error) {
	link, ok := findLink(topo, linkName)
	if !ok {
		return RIBFailureScenario{}, fmt.Errorf("link %s not found", linkName)
	}
	if link.AIntf == "" || link.BIntf == "" {
		return RIBFailureScenario{}, fmt.Errorf("link %s is missing endpoint interface names", linkName)
	}
	aIntf, bIntf := linkEndpointClabInterfaces(topo, link)
	return RIBFailureScenario{
		Name:     "link-" + link.Name,
		Failures: sim.LinkFailures(model.LinkID(link.Name)),
		Inject: func(ctx context.Context, runtime FailureRuntime) error {
			if err := runtime.SetLinkLoss(ctx, topo, link.A, aIntf, 100); err != nil {
				return err
			}
			if err := runtime.SetLinkLoss(ctx, topo, link.B, bIntf, 100); err != nil {
				return err
			}
			return nil
		},
		Cleanup: func(ctx context.Context, runtime FailureRuntime) error {
			var firstErr error
			if err := runtime.ResetLinkLoss(ctx, topo, link.A, aIntf); err != nil {
				firstErr = err
			}
			if err := runtime.ResetLinkLoss(ctx, topo, link.B, bIntf); firstErr == nil && err != nil {
				firstErr = err
			}
			return firstErr
		},
	}, nil
}

func linkEndpointClabInterfaces(topo *model.Topology, link model.Link) (string, string) {
	aIntf, bIntf := link.AIntf, link.BIntf
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		return aIntf, bIntf
	}
	if ref, ok := idx.InterfaceOnLink(link.A, link.Name); ok {
		aIntf = ref.ClabName
	}
	if ref, ok := idx.InterfaceOnLink(link.B, link.Name); ok {
		bIntf = ref.ClabName
	}
	return aIntf, bIntf
}

func NodeFailureScenario(topo *model.Topology, nodeName string) (RIBFailureScenario, error) {
	node, ok := topo.Node(nodeName)
	if !ok {
		return RIBFailureScenario{}, fmt.Errorf("node %s not found", nodeName)
	}
	return RIBFailureScenario{
		Name:        "node-" + nodeName,
		Failures:    sim.NodeFailures(model.NodeID(nodeName)),
		ActiveNodes: activeSupportedNodes(topo.Nodes, map[string]bool{nodeName: true}),
		Inject: func(ctx context.Context, runtime FailureRuntime) error {
			return runtime.StopNode(ctx, node)
		},
	}, nil
}

func activeSupportedNodes(nodes []model.Node, failed map[string]bool) []model.Node {
	var out []model.Node
	for _, node := range nodes {
		if !failed[node.Name] {
			out = append(out, node)
		}
	}
	return out
}

func findLink(topo *model.Topology, name string) (model.Link, bool) {
	for _, link := range topo.Links {
		if link.Name == name {
			return link, true
		}
	}
	return model.Link{}, false
}

func printRIBDiffs(out io.Writer, expected []observation.RIBRoute, actual []observation.RIBRoute, compareOptions observation.CompareOptions) {
	if out == nil {
		return
	}
	printDiffs(out, observation.CompareRoutes(expected, actual, compareOptions))
}

func printDiffs(out io.Writer, diffs observation.CompareResult) {
	if out == nil {
		return
	}
	for _, line := range observation.FormatDiffs(diffs) {
		fmt.Fprintln(out, line)
	}
}
