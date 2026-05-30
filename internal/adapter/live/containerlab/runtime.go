package containerlab

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	liverib "github.com/81ueman/hoyan-lab/internal/adapter/live/rib"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	observationrib "github.com/81ueman/hoyan-lab/internal/domain/observation/rib"
)

const containerNftablesConfig = "/etc/hoyan/nftables.conf"

type Runtime struct {
	Runner observationrib.Runner
}

func (r Runtime) BuildLocalImages(ctx context.Context, topologyPath string, out io.Writer) error {
	root := filepath.Dir(topologyPath)
	dockerfile := filepath.Join(root, "images", "frr-nftables", "Dockerfile")
	if _, err := os.Stat(dockerfile); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	contextDir := filepath.Dir(dockerfile)
	if _, err := r.Runner.Run(ctx, "docker", "image", "inspect", "hoyan-frr-nftables:10.6.1"); err == nil {
		if out != nil {
			fmt.Fprintln(out, "using existing hoyan-frr-nftables:10.6.1")
		}
		return nil
	}
	if out != nil {
		fmt.Fprintln(out, "building hoyan-frr-nftables:10.6.1")
	}
	if _, err := r.Runner.Run(ctx, "docker", "build", "-t", "hoyan-frr-nftables:10.6.1", contextDir); err != nil {
		return fmt.Errorf("docker build hoyan-frr-nftables:10.6.1: %w", err)
	}
	return nil
}

func (r Runtime) Deploy(ctx context.Context, topologyPath string) error {
	if _, err := r.Runner.Run(ctx, "containerlab", "deploy", "--reconfigure", "-t", topologyPath); err != nil {
		return fmt.Errorf("containerlab deploy: %w", err)
	}
	return nil
}

func (r Runtime) Destroy(ctx context.Context, topologyPath string) error {
	if _, err := r.Runner.Run(ctx, "containerlab", "destroy", "--cleanup", "-t", topologyPath); err != nil {
		return fmt.Errorf("containerlab destroy: %w", err)
	}
	return nil
}

func (r Runtime) WaitContainers(ctx context.Context, nodes []model.Node, interval time.Duration) error {
	var lastErr error
	return poll(ctx, interval, func() (bool, error) {
		for _, n := range nodes {
			containerName := n.RuntimeName()
			out, err := r.Runner.Run(ctx, "docker", "inspect", "-f", "{{.State.Running}}", containerName)
			if err != nil {
				lastErr = fmt.Errorf("docker inspect -f {{.State.Running}} %s: %w", containerName, err)
				return false, nil
			}
			if strings.TrimSpace(string(out)) != "true" {
				lastErr = fmt.Errorf("container %s is not running", containerName)
				return false, nil
			}
		}
		return true, nil
	}, func() error {
		if lastErr != nil {
			return fmt.Errorf("containers did not become ready: %w", lastErr)
		}
		return fmt.Errorf("containers did not become ready")
	})
}

func (r Runtime) WaitSRLinuxCLI(ctx context.Context, nodes []model.Node, interval time.Duration) error {
	srlinuxNodes := liverib.NodesByKind(nodes, model.KindSRLinux)
	if len(srlinuxNodes) == 0 {
		return nil
	}
	var lastErr error
	return poll(ctx, interval, func() (bool, error) {
		for _, n := range srlinuxNodes {
			containerName := n.RuntimeName()
			if _, err := liverib.RunSRLinuxJSON(ctx, r.Runner, containerName, "show", "version"); err != nil {
				lastErr = fmt.Errorf("%s SR Linux CLI is not ready: %w", n.Name, err)
				return false, nil
			}
		}
		lastErr = nil
		return true, nil
	}, func() error {
		if lastErr != nil {
			return fmt.Errorf("SR Linux CLI did not become ready: %w", lastErr)
		}
		return fmt.Errorf("SR Linux CLI did not become ready")
	})
}

func (r Runtime) ApplyNftablesPolicies(ctx context.Context, topo *model.Topology, out io.Writer) error {
	nodes := nftablesPolicyNodes(topo)
	for _, node := range nodes {
		if out != nil {
			fmt.Fprintf(out, "applying nftables policy on %s\n", node.Name)
		}
		script := "command -v nft >/dev/null && nft -f " + containerNftablesConfig
		if _, err := r.Runner.Run(ctx, "docker", "exec", node.RuntimeName(), "sh", "-lc", script); err != nil {
			return fmt.Errorf("apply nftables policy on %s: %w", node.Name, err)
		}
	}
	return nil
}

func (r Runtime) SetLinkLoss(ctx context.Context, topo *model.Topology, node, intf string, lossPercent int) error {
	if _, err := r.Runner.Run(ctx, "containerlab", "tools", "netem", "set", "--name", topo.Name, "-n", node, "-i", intf, "--loss", fmt.Sprintf("%d", lossPercent)); err != nil {
		return fmt.Errorf("netem set %s:%s: %w", node, intf, err)
	}
	return nil
}

func (r Runtime) ResetLinkLoss(ctx context.Context, topo *model.Topology, node, intf string) error {
	if _, err := r.Runner.Run(ctx, "containerlab", "tools", "netem", "reset", "--name", topo.Name, "-n", node, "-i", intf); err != nil {
		return fmt.Errorf("netem reset %s:%s: %w", node, intf, err)
	}
	return nil
}

func (r Runtime) StopNode(ctx context.Context, node model.Node) error {
	containerName := node.RuntimeName()
	if _, err := r.Runner.Run(ctx, "docker", "stop", containerName); err != nil {
		return fmt.Errorf("docker stop %s: %w", containerName, err)
	}
	return nil
}

func nftablesPolicyNodes(topo *model.Topology) []model.Node {
	if topo == nil {
		return nil
	}
	wanted := map[string]bool{}
	for _, acl := range topo.ACLs {
		if acl.Source.Vendor == "nftables" {
			wanted[acl.Node] = true
		}
	}
	var nodes []model.Node
	for _, node := range topo.Nodes {
		if wanted[node.Name] {
			nodes = append(nodes, node)
		}
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
	return nodes
}

func poll(ctx context.Context, interval time.Duration, fn func() (bool, error), onTimeout func() error) error {
	if interval <= 0 {
		interval = time.Second
	}
	for {
		ok, err := fn()
		if err != nil || ok {
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			if onTimeout != nil {
				return onTimeout()
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
