package device

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type fakeRunner struct {
	fn func(ctx context.Context, name string, args ...string) ([]byte, error)
}

func (f fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return f.fn(ctx, name, args...)
}

func TestDockerSessionExecDelegatesCorrectly(t *testing.T) {
	var captured struct {
		name string
		args []string
	}
	runner := fakeRunner{fn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		captured.name = name
		captured.args = args
		return []byte("output"), nil
	}}
	session := NewDockerSession(runner, "test-container")
	data, err := session.Exec(context.Background(), "vtysh", "-c", "show ip bgp json")
	if err != nil {
		t.Fatalf("Exec() error = %v", err)
	}
	if string(data) != "output" {
		t.Fatalf("Exec() = %q, want %q", data, "output")
	}
	if captured.name != "docker" {
		t.Fatalf("runner name = %q, want %q", captured.name, "docker")
	}
	got := strings.Join(captured.args, " ")
	want := "exec -i test-container vtysh -c show ip bgp json"
	if got != want {
		t.Fatalf("runner args = %q, want %q", got, want)
	}
}

func TestDockerSessionExecTTYDelegatesCorrectly(t *testing.T) {
	var captured struct {
		name string
		args []string
	}
	runner := fakeRunner{fn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		captured.name = name
		captured.args = args
		return []byte("output"), nil
	}}
	session := NewDockerSession(runner, "test-container")
	data, err := session.ExecTTY(context.Background(), "sr_cli", "--output-format", "json", "--", "show", "version")
	if err != nil {
		t.Fatalf("ExecTTY() error = %v", err)
	}
	if string(data) != "output" {
		t.Fatalf("ExecTTY() = %q, want %q", data, "output")
	}
	if captured.name != "script" {
		t.Fatalf("runner name = %q, want %q", captured.name, "script")
	}
	if len(captured.args) < 4 || captured.args[0] != "-q" || captured.args[1] != "/dev/null" || captured.args[2] != "-c" {
		t.Fatalf("runner args = %v, want script -q /dev/null -c ...", captured.args)
	}
	scriptCmd := captured.args[3]
	if !strings.Contains(scriptCmd, "docker exec -it 'test-container'") {
		t.Fatalf("script command = %q, missing docker exec -it", scriptCmd)
	}
	if !strings.Contains(scriptCmd, "sr_cli") {
		t.Fatalf("script command = %q, missing sr_cli", scriptCmd)
	}
}

func TestVendorCollectorSessionForNode(t *testing.T) {
	runner := fakeRunner{fn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("unused")
	}}
	vc := NewVendorCollectorWithResolver(runner, func(name string) string {
		if name == "r1" {
			return "clab-test-r1"
		}
		return name
	})

	node := model.Node{Name: "r1"}
	session := vc.SessionForNode(node)

	ds, ok := session.(*DockerSession)
	if !ok {
		t.Fatalf("SessionForNode() returned %T, want *DockerSession", session)
	}
	if ds.containerName != "clab-test-r1" {
		t.Fatalf("DockerSession.containerName = %q, want %q", ds.containerName, "clab-test-r1")
	}
}

func TestVendorCollectorSessionForNodeFallsBackToName(t *testing.T) {
	runner := fakeRunner{fn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("unused")
	}}
	vc := NewVendorCollector(runner)

	node := model.Node{Name: "r1-alone"}
	session := vc.SessionForNode(node)

	ds, ok := session.(*DockerSession)
	if !ok {
		t.Fatalf("SessionForNode() returned %T, want *DockerSession", session)
	}
	if ds.containerName != "r1-alone" {
		t.Fatalf("DockerSession.containerName = %q, want fallback to node name %q", ds.containerName, "r1-alone")
	}
}

func TestNewVendorCollectorPreservesRunner(t *testing.T) {
	runner := fakeRunner{fn: func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("unused")
	}}
	vc := NewVendorCollector(runner)
	if vc.Runner == nil {
		t.Fatalf("NewVendorCollector().Runner is nil")
	}
}
