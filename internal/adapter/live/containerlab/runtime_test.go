package containerlab

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type fakeRunner struct {
	calls []string
	fn    func(name string, args ...string) ([]byte, error)
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return f.fn(name, args...)
}

func TestBuildLocalImagesSkipsExistingImage(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "images", "frr-nftables"), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "images", "frr-nftables", "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runner := &fakeRunner{fn: func(name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		if cmd == "docker image inspect hoyan-frr-nftables:10.6.1" {
			return []byte("[]"), nil
		}
		return nil, errors.New("unexpected command: " + cmd)
	}}
	if err := (Runtime{Runner: runner}).BuildLocalImages(context.Background(), filepath.Join(root, "lab.clab.yml"), io.Discard); err != nil {
		t.Fatalf("BuildLocalImages() error = %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls = %v, want only image inspect", runner.calls)
	}
}

func TestBuildLocalImagesBuildsMissingImage(t *testing.T) {
	root := t.TempDir()
	imageDir := filepath.Join(root, "images", "frr-nftables")
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(imageDir, "Dockerfile"), []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	runner := &fakeRunner{fn: func(name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch {
		case cmd == "docker image inspect hoyan-frr-nftables:10.6.1":
			return nil, errors.New("missing")
		case cmd == "docker build -t hoyan-frr-nftables:10.6.1 "+imageDir:
			return []byte("built"), nil
		default:
			return nil, errors.New("unexpected command: " + cmd)
		}
	}}
	if err := (Runtime{Runner: runner}).BuildLocalImages(context.Background(), filepath.Join(root, "lab.clab.yml"), io.Discard); err != nil {
		t.Fatalf("BuildLocalImages() error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %v, want inspect and build", runner.calls)
	}
}

func TestRuntimeWaitContainersAndNftables(t *testing.T) {
	resolve := func(name string) string {
		return "clab-test-" + name
	}
	runner := &fakeRunner{fn: func(name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch cmd {
		case "docker inspect -f {{.State.Running}} clab-test-r1":
			return []byte("true\n"), nil
		case "docker exec clab-test-core-hz sh -lc command -v nft >/dev/null && nft -f /etc/hoyan/nftables.conf":
			return nil, nil
		default:
			return nil, errors.New("unexpected command: " + cmd)
		}
	}}
	runtime := NewRuntime(runner, resolve)
	if err := runtime.WaitContainers(context.Background(), []model.Node{{Name: "r1"}}, time.Millisecond); err != nil {
		t.Fatalf("WaitContainers() error = %v", err)
	}
	topo := &model.Topology{
		Nodes: []model.Node{
			{Name: "core-hz", Kind: model.KindFRR},
			{Name: "core-bj", Kind: model.KindFRR},
		},
		ACLs: []model.ACL{
			{Name: "BLOCK-HTTP-TO-HZ", Node: "core-hz", Source: model.ConfigSource{Vendor: "nftables"}},
			{Name: "OTHER", Node: "core-bj", Source: model.ConfigSource{Vendor: "ceos"}},
		},
	}
	if err := runtime.ApplyNftablesPolicies(context.Background(), topo, io.Discard); err != nil {
		t.Fatalf("ApplyNftablesPolicies() error = %v", err)
	}
	if len(runner.calls) != 2 {
		t.Fatalf("calls = %v, want inspect and nft apply", runner.calls)
	}
}

func TestRuntimeFailureCommands(t *testing.T) {
	resolve := func(name string) string {
		return "clab-test-" + name
	}
	runner := &fakeRunner{fn: func(name string, args ...string) ([]byte, error) { return []byte("ok"), nil }}
	runtime := NewRuntime(runner, resolve)
	topo := &model.Topology{Name: "test-lab"}
	if err := runtime.SetLinkLoss(context.Background(), topo, "a", "eth1", 100); err != nil {
		t.Fatalf("SetLinkLoss() error = %v", err)
	}
	if err := runtime.ResetLinkLoss(context.Background(), topo, "b", "eth2"); err != nil {
		t.Fatalf("ResetLinkLoss() error = %v", err)
	}
	if err := runtime.StopNode(context.Background(), "r1"); err != nil {
		t.Fatalf("StopNode() error = %v", err)
	}
	want := []string{
		"containerlab tools netem set --name test-lab -n a -i eth1 --loss 100",
		"containerlab tools netem reset --name test-lab -n b -i eth2",
		"docker stop clab-test-r1",
	}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %v, want %v", runner.calls, want)
	}
}
