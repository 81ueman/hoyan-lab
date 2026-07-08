package device

import (
	"context"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// Runner executes external commands.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// DeviceSession represents a session to a network device for running commands.
// Implementations abstract how commands reach the device (docker exec, SSH, etc.).
type DeviceSession interface {
	Exec(ctx context.Context, args ...string) ([]byte, error)
}

// DockerSession implements DeviceSession using docker exec.
type DockerSession struct {
	runner        Runner
	containerName string
}

func NewDockerSession(runner Runner, containerName string) *DockerSession {
	return &DockerSession{runner: runner, containerName: containerName}
}

func (s *DockerSession) Exec(ctx context.Context, args ...string) ([]byte, error) {
	dockerArgs := append([]string{"exec", "-i", s.containerName}, args...)
	return s.runner.Run(ctx, "docker", dockerArgs...)
}

// ExecTTY runs a command with a pseudo-TTY, using script(1) to force TTY allocation.
// Used for SR Linux CLI fallback where non-TTY exec may return empty/malformed output.
func (s *DockerSession) ExecTTY(ctx context.Context, args ...string) ([]byte, error) {
	command := "docker exec -it " + shellQuote(s.containerName) + " " + shellJoin(args)
	return s.runner.Run(ctx, "script", "-q", "/dev/null", "-c", command)
}

// VendorCollector carries a Runner and creates per-node DeviceSessions.
// Vendor-specific collectors embed this and call SessionForNode to get a session
// for each target node, rather than constructing docker exec commands directly.
type VendorCollector struct {
	Runner           Runner
	containerNameFor func(string) string // resolves container name from node name, nil = use node name directly
}

func NewVendorCollector(runner Runner) VendorCollector {
	return VendorCollector{Runner: runner}
}

// NewVendorCollectorWithResolver creates a VendorCollector that resolves container
// names via the given function. When nil or when the resolver returns an empty
// string, the node's Name is used as the container name.
func NewVendorCollectorWithResolver(runner Runner, resolveContainerName func(string) string) VendorCollector {
	return VendorCollector{Runner: runner, containerNameFor: resolveContainerName}
}

// SessionForNode creates a DeviceSession for the given node.
// The container name is resolved from the node name using the optional resolver,
// falling back to the node name itself.
func (c VendorCollector) SessionForNode(node model.Node) DeviceSession {
	containerName := node.Name
	if c.containerNameFor != nil {
		if cn := c.containerNameFor(node.Name); cn != "" {
			containerName = cn
		}
	}
	return NewDockerSession(c.Runner, containerName)
}

// NewRegistry builds the standard FRR/cEOS/SR Linux live collector registry.
type Factory[T any] func(VendorCollector) T

func NewRegistry[T any](runner Runner, factory func(model.LiveCollectorID, VendorCollector) T) map[model.LiveCollectorID]T {
	base := NewVendorCollector(runner)
	return newRegistryFromBase(base, factory)
}

// NewRegistryWithResolver builds the standard registry with a container name resolver.
func NewRegistryWithResolver[T any](runner Runner, resolveContainerName func(string) string, factory func(model.LiveCollectorID, VendorCollector) T) map[model.LiveCollectorID]T {
	base := NewVendorCollectorWithResolver(runner, resolveContainerName)
	return newRegistryFromBase(base, factory)
}

func newRegistryFromBase[T any](base VendorCollector, factory func(model.LiveCollectorID, VendorCollector) T) map[model.LiveCollectorID]T {
	return map[model.LiveCollectorID]T{
		model.LiveCollectorFRR:     factory(model.LiveCollectorFRR, base),
		model.LiveCollectorCEOS:    factory(model.LiveCollectorCEOS, base),
		model.LiveCollectorSRLinux: factory(model.LiveCollectorSRLinux, base),
	}
}

func NodesByKind(nodes []model.Node, kind model.DeviceKind) []model.Node {
	var out []model.Node
	for _, n := range nodes {
		if n.Kind == kind {
			out = append(out, n)
		}
	}
	return out
}

func shellJoin(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = shellQuote(arg)
	}
	return strings.Join(quoted, " ")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
