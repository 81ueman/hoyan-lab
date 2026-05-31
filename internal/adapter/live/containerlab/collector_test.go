package containerlab

import (
	"context"
	"reflect"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

func TestCollectorExposesContainerlabTopology(t *testing.T) {
	nodes := []model.Node{{
		Name: "r1",
		Kind: model.KindFRR,
		Interfaces: []model.Interface{{
			Name: "eth1",
			VRF:  model.NetworkInstanceID("blue"),
		}},
	}, {
		Name: "r2",
		Kind: model.KindFRR,
	}}
	collector := NewCollector(nodes, nil, observation.Options{})

	var _ observation.Collector = collector

	metadata := collector.Metadata(context.Background())
	if metadata.Source != "containerlab" {
		t.Fatalf("metadata source = %q, want containerlab", metadata.Source)
	}
	gotNodes, err := collector.Nodes(context.Background())
	if err != nil {
		t.Fatalf("Nodes() error = %v", err)
	}
	wantNodes := []model.NodeID{"r1", "r2"}
	if !reflect.DeepEqual(gotNodes, wantNodes) {
		t.Fatalf("Nodes() = %v, want %v", gotNodes, wantNodes)
	}
	gotVRFs, err := collector.VRFs(context.Background(), "r1")
	if err != nil {
		t.Fatalf("VRFs() error = %v", err)
	}
	wantVRFs := []model.NetworkInstanceID{model.NetworkInstanceID("blue"), model.NetworkInstanceDefault}
	if !reflect.DeepEqual(gotVRFs, wantVRFs) {
		t.Fatalf("VRFs() = %v, want %v", gotVRFs, wantVRFs)
	}
}

func TestCollectorRejectsUnknownNode(t *testing.T) {
	collector := NewCollector([]model.Node{{Name: "r1", Kind: model.KindFRR}}, nil, observation.Options{})

	if _, err := collector.VRFs(context.Background(), "missing"); err == nil {
		t.Fatalf("VRFs() error = nil, want unknown node error")
	}
	if _, err := collector.CollectRIB(context.Background(), model.Node{Name: "missing"}, model.NetworkInstanceDefault, observation.CollectOptions{}); err == nil {
		t.Fatalf("CollectRIB() error = nil, want unknown node error")
	}
	if _, err := collector.CollectFIB(context.Background(), model.Node{Name: "missing"}, model.NetworkInstanceDefault, observation.Options{}); err == nil {
		t.Fatalf("CollectFIB() error = nil, want unknown node error")
	}
}
