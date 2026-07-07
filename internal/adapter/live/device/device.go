package device

import (
	"context"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// Runner executes external commands.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// DockerExecutor centralizes docker exec invocation used by live collectors.
type DockerExecutor struct {
	runner Runner
}

func NewDockerExecutor(runner Runner) DockerExecutor {
	return DockerExecutor{runner: runner}
}

func (e DockerExecutor) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return e.runner.Run(ctx, name, args...)
}

func (e DockerExecutor) Exec(ctx context.Context, containerName string, args ...string) ([]byte, error) {
	dockerArgs := append([]string{"exec", "-i", containerName}, args...)
	return e.runner.Run(ctx, "docker", dockerArgs...)
}

// VendorCollector carries the shared live command executor for a vendor collector.
type VendorCollector struct {
	Exec DockerExecutor
}

func NewVendorCollector(runner Runner) VendorCollector {
	return VendorCollector{Exec: NewDockerExecutor(runner)}
}

// Registry builds the standard FRR/cEOS/SR Linux live collector registry.
type Factory[T any] func(VendorCollector) T

func NewRegistry[T any](runner Runner, factory func(model.LiveCollectorID, VendorCollector) T) map[model.LiveCollectorID]T {
	base := NewVendorCollector(runner)
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
