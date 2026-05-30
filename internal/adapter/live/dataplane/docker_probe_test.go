package dataplane

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/query"
)

type fakeRunner struct {
	calls []string
	fn    func(name string, args ...string) ([]byte, error)
}

func (f *fakeRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, name+" "+strings.Join(args, " "))
	return f.fn(name, args...)
}

func TestDockerProberProbesICMPAndTCP(t *testing.T) {
	topo := &model.Topology{
		Nodes: []model.Node{
			{Name: "src", ContainerName: "clab-test-src", Kind: model.KindFRR},
			{Name: "dst", ContainerName: "clab-test-dst", Kind: model.KindFRR, Prefixes: model.MustPrefixes("10.0.0.10/32")},
		},
	}
	runner := &fakeRunner{fn: func(name string, args ...string) ([]byte, error) {
		cmd := name + " " + strings.Join(args, " ")
		switch {
		case strings.HasPrefix(cmd, "script -q /dev/null -c docker exec -it 'clab-test-src' 'ping'"):
			return []byte("1 packets transmitted, 1 packets received, 0% packet loss"), nil
		case strings.HasPrefix(cmd, "docker exec -d clab-test-dst sh -lc"):
			return []byte(""), nil
		case strings.HasPrefix(cmd, "script -q /dev/null -c docker exec -it 'clab-test-src' 'nc'"):
			return []byte("10.0.0.10 (10.0.0.10:80) open"), nil
		default:
			return nil, errors.New("unexpected command: " + cmd)
		}
	}}
	prober := DockerProber{Runner: runner}
	ok, err := prober.Probe(context.Background(), topo, query.PacketCheck{Name: "icmp-ok", From: "src", To: "10.0.0.10", Protocol: "icmp"})
	if err != nil || !ok {
		t.Fatalf("icmp Probe() = %v, %v; want true, nil", ok, err)
	}
	ok, err = prober.Probe(context.Background(), topo, query.PacketCheck{Name: "tcp-ok", From: "src", To: "10.0.0.10", Protocol: "tcp", DstPort: 80})
	if err != nil || !ok {
		t.Fatalf("tcp Probe() = %v, %v; want true, nil", ok, err)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("calls = %v, want listener plus two probes", runner.calls)
	}
}

func TestDockerProberWrapsNonDefaultVRF(t *testing.T) {
	runner := &fakeRunner{fn: func(name string, args ...string) ([]byte, error) {
		return []byte("1 packets transmitted, 1 packets received, 0% packet loss"), nil
	}}
	topo := &model.Topology{Nodes: []model.Node{{Name: "src", ContainerName: "clab-test-src", Kind: model.KindFRR}}}
	prober := DockerProber{Runner: runner}
	ok, err := prober.Probe(context.Background(), topo, query.PacketCheck{Name: "icmp-ok", From: "src", To: "10.0.0.10", Protocol: "icmp", VRF: "tenant-a"})
	if err != nil || !ok {
		t.Fatalf("Probe() = %v, %v; want true, nil", ok, err)
	}
	want := []string{"script -q /dev/null -c docker exec -it 'clab-test-src' 'ip' 'vrf' 'exec' 'tenant-a' 'ping' '-c' '3' '-W' '1' '10.0.0.10'"}
	if !reflect.DeepEqual(runner.calls, want) {
		t.Fatalf("calls = %v, want %v", runner.calls, want)
	}
}
