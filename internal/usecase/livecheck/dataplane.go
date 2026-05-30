package livecheck

import (
	"context"
	"fmt"
	"io"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/query"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
)

func RunDataplaneChecks(ctx context.Context, prober DataplaneProber, topo *model.Topology, queries *query.Queries, out io.Writer) error {
	if out == nil {
		out = io.Discard
	}
	if queries == nil || len(queries.PacketChecks) == 0 {
		return nil
	}
	graph := sim.NewGraph(topo)
	for _, check := range queries.PacketChecks {
		if check.MaxFailures != 0 {
			continue
		}
		if check.LiveProbe != nil && !*check.LiveProbe {
			fmt.Fprintf(out, "[dataplane] %s skipped live probe\n", check.Name)
			continue
		}
		expected := true
		if check.ExpectReachable != nil {
			expected = *check.ExpectReachable
		}
		ports := check.DstPortValues()
		for _, port := range ports {
			checkForPort := check
			checkForPort.DstPort = port
			checkName := packetCheckName(check.Name, port, len(ports))
			vrf := string(model.NormalizeNetworkInstance(check.VRF))
			spec := model.PacketSpec{Protocol: check.Protocol, DstPort: model.ExactPort(port)}
			_, modeled, reason := graph.PacketReachableSpecVRF(check.From, vrf, check.To, spec, failure.None())
			live, err := prober.Probe(ctx, topo, checkForPort)
			if err != nil {
				return fmt.Errorf("%s live dataplane probe: %w", checkName, err)
			}
			fmt.Fprintf(out, "[dataplane] %s live=%v modeled=%v expected=%v\n", checkName, live, modeled, expected)
			if reason != "" {
				fmt.Fprintf(out, "  modeled reason: %s\n", reason)
			}
			if live != expected {
				return fmt.Errorf("%s live dataplane reachable=%v expected=%v", checkName, live, expected)
			}
			if live != modeled {
				return fmt.Errorf("%s live dataplane reachable=%v modeled=%v", checkName, live, modeled)
			}
		}
	}
	return nil
}

func packetCheckName(name string, port int, portCount int) string {
	if portCount <= 1 || port <= 0 {
		return name
	}
	return fmt.Sprintf("%s:dst-port-%d", name, port)
}
