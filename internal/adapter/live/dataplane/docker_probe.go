package dataplane

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	"github.com/81ueman/hoyan-lab/internal/domain/query"
)

type DockerProber struct {
	Runner observation.RIBRunner
}

func (p DockerProber) Probe(ctx context.Context, topo *model.Topology, check query.PacketCheck) (bool, error) {
	src, ok := topo.Node(check.From)
	if !ok {
		return false, fmt.Errorf("source node %s not found", check.From)
	}
	switch strings.ToLower(check.Protocol) {
	case "icmp":
		out, err := p.runProbeCommand(ctx, src.RuntimeName(), check.VRF, "ping", "-c", "3", "-W", "1", check.To)
		if err != nil {
			return false, err
		}
		return pingSucceeded(string(out)), nil
	case "tcp":
		if check.DstPort <= 0 {
			return false, fmt.Errorf("tcp packet check requires dst_port")
		}
		if err := p.ensureTCPListener(ctx, topo, check.To, check.DstPort); err != nil {
			return false, err
		}
		out, err := p.runProbeCommand(ctx, src.RuntimeName(), check.VRF, "nc", "-z", "-v", "-w", "2", check.To, strconv.Itoa(check.DstPort))
		if err != nil {
			return false, err
		}
		return tcpConnectSucceeded(string(out)), nil
	default:
		return false, fmt.Errorf("unsupported live packet protocol %q", check.Protocol)
	}
}

func (p DockerProber) runProbeCommand(ctx context.Context, container, vrf string, args ...string) ([]byte, error) {
	if model.NormalizeNetworkInstance(vrf) == model.NetworkInstanceDefault {
		return p.runDockerExecTTY(ctx, container, args...)
	}
	wrapped := append([]string{"ip", "vrf", "exec", vrf}, args...)
	return p.runDockerExecTTY(ctx, container, wrapped...)
}

func (p DockerProber) runDockerExecTTY(ctx context.Context, container string, args ...string) ([]byte, error) {
	cmd := "docker exec -it " + shellQuote(container)
	for _, arg := range args {
		cmd += " " + shellQuote(arg)
	}
	return p.Runner.Run(ctx, "script", "-q", "/dev/null", "-c", cmd)
}

func pingSucceeded(out string) bool {
	out = strings.ReplaceAll(out, "\x00", "")
	return strings.Contains(out, " 0% packet loss") || strings.Contains(out, "0% packet loss") || strings.Contains(out, " 3 packets received") || strings.Contains(out, " bytes from ")
}

func tcpConnectSucceeded(out string) bool {
	out = strings.ReplaceAll(out, "\x00", "")
	return strings.Contains(out, " open") || strings.Contains(out, "succeeded") || strings.Contains(out, "Connected")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func (p DockerProber) ensureTCPListener(ctx context.Context, topo *model.Topology, dst string, port int) error {
	dstNode, _, ok := topo.OriginForIP(dst)
	if !ok {
		return fmt.Errorf("destination %s is not originated by any node", dst)
	}
	node, ok := topo.Node(dstNode)
	if !ok {
		return fmt.Errorf("destination node %s not found", dstNode)
	}
	portText := strconv.Itoa(port)
	script := "command -v nc >/dev/null || exit 127; " +
		"pkill -f 'nc -l -p " + portText + "' >/dev/null 2>&1 || true; " +
		"while true; do nc -l -p " + portText + " >/dev/null 2>&1; done"
	if _, err := p.Runner.Run(ctx, "docker", "exec", "-d", node.RuntimeName(), "sh", "-lc", script); err != nil {
		return fmt.Errorf("start tcp listener on %s port %s: %w", node.Name, portText, err)
	}
	return nil
}
