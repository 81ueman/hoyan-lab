//go:build clab

package containerlab_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	liveexec "github.com/81ueman/hoyan-lab/internal/adapter/live"
	clabruntime "github.com/81ueman/hoyan-lab/internal/adapter/live/containerlab"
	liverib "github.com/81ueman/hoyan-lab/internal/adapter/live/rib"
	"github.com/81ueman/hoyan-lab/internal/usecase/livecheck"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

func TestContainerlabRIBsMatchSimulationUnderFailures(t *testing.T) {
	if _, err := exec.LookPath("containerlab"); err != nil {
		t.Skipf("containerlab not found: %v", err)
	}
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skipf("docker not found: %v", err)
	}

	topologyPath := filepath.Join("..", "..", "..", "..", "labs", "base-wan", "hoyan.clab.yml")
	topo, err := topology.LoadTopology(topologyPath)
	if err != nil {
		t.Fatalf("LoadLabTopology() error = %v", err)
	}
	runner := liveexec.ExecRunner{}
	runtime := clabruntime.Runtime{Runner: runner}
	ribCollector := liverib.NewCollector(runner)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()

	deploy := func() {
		t.Helper()
		if err := runtime.Deploy(ctx, topologyPath); err != nil {
			t.Fatalf("containerlab deploy: %v", err)
		}
		if err := runtime.WaitContainers(ctx, liverib.FRRNodes(topo.Nodes), 5*time.Second); err != nil {
			t.Fatalf("FRR containers did not become ready: %v", err)
		}
	}
	destroy := func() {
		t.Helper()
		if err := runtime.Destroy(context.Background(), topologyPath); err != nil {
			t.Logf("containerlab destroy: %v", err)
		}
	}
	t.Cleanup(destroy)

	deploy()
	if err := livecheck.CompareRIBsWithFailures(ctx, runtime, ribCollector, topo, livecheck.RIBFailureScenario{
		Name:        "baseline",
		ActiveNodes: liverib.FRRNodes(topo.Nodes),
	}, livecheck.RIBFailureCheckOptions{Interval: 5 * time.Second, MaxPolls: 24, Out: testLogWriter{t: t}}); err != nil {
		t.Fatalf("baseline RIB comparison failed: %v", err)
	}

	linkScenario, err := livecheck.LinkFailureScenario(topo, "core-hz-eth4--core-bj-eth4")
	if err != nil {
		t.Fatalf("LinkFailureScenario() error = %v", err)
	}
	linkScenario.ActiveNodes = liverib.FRRNodes(topo.Nodes)
	if err := livecheck.CompareRIBsWithFailures(ctx, runtime, ribCollector, topo, linkScenario, livecheck.RIBFailureCheckOptions{Interval: 5 * time.Second, MaxPolls: 18, Out: testLogWriter{t: t}}); err != nil {
		t.Fatalf("link-failure RIB comparison failed: %v", err)
	}

	destroy()
	deploy()
	nodeScenario, err := livecheck.NodeFailureScenario(topo, "transit-north")
	if err != nil {
		t.Fatalf("NodeFailureScenario() error = %v", err)
	}
	if err := livecheck.CompareRIBsWithFailures(ctx, runtime, ribCollector, topo, nodeScenario, livecheck.RIBFailureCheckOptions{Interval: 5 * time.Second, MaxPolls: 18, Out: testLogWriter{t: t}}); err != nil {
		t.Fatalf("node-failure RIB comparison failed: %v", err)
	}
}

type testLogWriter struct {
	t *testing.T
}

func (w testLogWriter) Write(p []byte) (int, error) {
	w.t.Logf("%s", p)
	return len(p), nil
}
