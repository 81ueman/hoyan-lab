package controlplane

import (
	"reflect"
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func TestConcatPathsRejectsLoop(t *testing.T) {
	_, ok := ConcatPaths(
		Path{Nodes: []string{"r1", "r2"}, Links: []string{"r1-r2"}, Areas: []string{BackboneArea}, Cost: 10},
		Path{Nodes: []string{"r2", "r1"}, Links: []string{"r1-r2"}, Areas: []string{BackboneArea}, Cost: 10},
	)
	if ok {
		t.Fatalf("ConcatPaths() accepted a path that loops back to r1")
	}
}

func TestShortestPathTreeKeepsEqualCostPredecessors(t *testing.T) {
	nodes := []model.Node{{Name: "r1"}, {Name: "r2"}, {Name: "r3"}, {Name: "r4"}}
	adj := map[string][]Adjacency{
		"r1": {{From: "r1", To: "r2", Link: "r1-r2", Area: BackboneArea, Cost: 10}, {From: "r1", To: "r3", Link: "r1-r3", Area: BackboneArea, Cost: 10}},
		"r2": {{From: "r2", To: "r4", Link: "r2-r4", Area: BackboneArea, Cost: 10}},
		"r3": {{From: "r3", To: "r4", Link: "r3-r4", Area: BackboneArea, Cost: 10}},
	}
	spf := ShortestPathTree(nodes, "r1", "", func(node string) []Adjacency { return adj[node] })
	got := spf["r4"]
	if got.Cost != 20 || len(got.Predecessors) != 2 {
		t.Fatalf("r4 SPF node = %#v, want cost 20 with two predecessors", got)
	}
	if !reflect.DeepEqual(got.Predecessors, []SPFPredecessor{
		{Node: "r2", Link: "r2-r4", Area: BackboneArea},
		{Node: "r3", Link: "r3-r4", Area: BackboneArea},
	}) {
		t.Fatalf("r4 predecessors = %#v", got.Predecessors)
	}
}

func TestAdvertisementAllowedBlocksExternalIntoStubArea(t *testing.T) {
	processes := map[string]model.OSPFProcess{
		"r1": {Areas: map[string]model.OSPFArea{"1": {ID: "1", Kind: model.OSPFAreaStub}}},
	}
	adv := Advertisement{External: true}
	path := Path{Nodes: []string{"r1", "r2"}, Areas: []string{"1"}}
	if AdvertisementAllowed(model.Node{Name: "r1"}, adv, path, processes) {
		t.Fatalf("external advertisement should be blocked across stub area")
	}
}
