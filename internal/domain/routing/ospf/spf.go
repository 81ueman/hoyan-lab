package ospf

import (
	"container/heap"
	"math"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

const (
	MaxPathsPerDestination = 8
	UnreachableCost        = math.MaxInt
)

type SPFQueue []SPFQueueItem

func ExternalArea(process model.OSPFProcess, states map[string]InterfaceState) string {
	for _, state := range states {
		if area := process.Areas[state.Area]; area.Kind == model.OSPFAreaNSSA {
			return state.Area
		}
	}
	return ""
}

func NodeAttachedToArea(states map[string]InterfaceState, area string) bool {
	for _, state := range states {
		if state.Area == area {
			return true
		}
	}
	return false
}

func NodeAttachedToOtherArea(states map[string]InterfaceState, area string) bool {
	for _, state := range states {
		if state.Area != "" && state.Area != area {
			return true
		}
	}
	return false
}

func AdvertisementAllowed(src model.Node, adv Advertisement, path Path, processes map[string]model.OSPFProcess) bool {
	if adv.DefaultArea != "" {
		return PathUsesOnlyArea(path, adv.DefaultArea)
	}
	if !adv.External {
		return true
	}
	for _, areaID := range path.Areas {
		area := AreaForPathArea(processes, path, areaID)
		switch area.Kind {
		case model.OSPFAreaStub:
			return false
		case model.OSPFAreaNSSA:
			if adv.ExternalArea != areaID {
				return false
			}
		}
	}
	if adv.ExternalArea != "" {
		return true
	}
	return !NodeInStubOrNSSA(processes[src.Name])
}

func PathUsesOnlyArea(path Path, area string) bool {
	if len(path.Areas) == 0 {
		return false
	}
	for _, pathArea := range path.Areas {
		if pathArea != area {
			return false
		}
	}
	return true
}

func NodeInStubOrNSSA(process model.OSPFProcess) bool {
	for _, area := range process.Areas {
		if area.Kind == model.OSPFAreaStub || area.Kind == model.OSPFAreaNSSA {
			return true
		}
	}
	return false
}

func AreaForPathArea(processes map[string]model.OSPFProcess, path Path, areaID string) model.OSPFArea {
	for _, nodeName := range path.Nodes {
		process, ok := processes[nodeName]
		if !ok {
			continue
		}
		area := process.Areas[areaID]
		if area.Kind != "" {
			return area
		}
	}
	return model.OSPFArea{ID: areaID, Kind: model.OSPFAreaNormal}
}

func NodeAreas(states map[string]map[string]InterfaceState) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for node, byIface := range states {
		for _, state := range byIface {
			if state.Area == "" {
				continue
			}
			if out[node] == nil {
				out[node] = map[string]bool{}
			}
			out[node][state.Area] = true
		}
	}
	return out
}

func ABRs(areas map[string]map[string]bool) map[string]bool {
	out := map[string]bool{}
	for node, nodeAreas := range areas {
		if len(nodeAreas) > 1 && nodeAreas[BackboneArea] {
			out[node] = true
		}
	}
	return out
}

func AreaBoundaries(node, area string, areas map[string]map[string]bool, abrs map[string]bool) []string {
	if area == BackboneArea && areas[node][BackboneArea] {
		return []string{node}
	}
	var out []string
	for n := range abrs {
		if areas[n][area] {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

func ZeroPath(src, dst string, paths map[string][]Path) Path {
	if src == dst {
		return Path{Nodes: []string{src}}
	}
	if len(paths[dst]) == 0 {
		return Path{}
	}
	return paths[dst][0]
}

func ConcatPaths(parts ...Path) (Path, bool) {
	var out Path
	seen := map[string]bool{}
	var conds []failure.Cond
	for i, part := range parts {
		if len(part.Nodes) == 0 {
			return Path{}, false
		}
		out.Cost += part.Cost
		if part.Cond != nil {
			conds = append(conds, part.Cond)
		}
		if i == 0 {
			out.Nodes = append(out.Nodes, part.Nodes...)
		} else {
			if out.Nodes[len(out.Nodes)-1] != part.Nodes[0] {
				return Path{}, false
			}
			out.Nodes = append(out.Nodes, part.Nodes[1:]...)
		}
		out.Links = append(out.Links, part.Links...)
		out.Areas = append(out.Areas, part.Areas...)
	}
	for _, node := range out.Nodes {
		if seen[node] {
			return Path{}, false
		}
		seen[node] = true
	}
	if len(conds) > 0 {
		out.Cond = failure.And(conds...)
	}
	return out, true
}

func SortPaths(paths []Path) {
	sort.Slice(paths, func(i, j int) bool {
		if paths[i].Cost != paths[j].Cost {
			return paths[i].Cost < paths[j].Cost
		}
		return strings.Join(paths[i].Nodes, ",") < strings.Join(paths[j].Nodes, ",")
	})
}

func ShortestPathTree(nodes []model.Node, src, excluded string, adjacencies func(string) []Adjacency) map[string]SPFNode {
	dist := map[string]SPFNode{}
	for _, node := range nodes {
		if node.Name == excluded {
			continue
		}
		dist[node.Name] = SPFNode{Cost: UnreachableCost}
	}
	if _, ok := dist[src]; !ok {
		return dist
	}
	dist[src] = SPFNode{Cost: 0}
	q := &SPFQueue{{Node: src}}
	heap.Init(q)
	for q.Len() > 0 {
		item := heap.Pop(q).(SPFQueueItem)
		current := dist[item.Node]
		if item.Cost != current.Cost {
			continue
		}
		for _, adj := range adjacencies(item.Node) {
			if adj.To == excluded {
				continue
			}
			next, ok := dist[adj.To]
			if !ok {
				continue
			}
			cost := item.Cost + adj.Cost
			pred := SPFPredecessor{Node: item.Node, Link: adj.Link, Area: adj.Area}
			switch {
			case cost < next.Cost:
				next.Cost = cost
				next.Predecessors = []SPFPredecessor{pred}
				dist[adj.To] = next
				heap.Push(q, SPFQueueItem{Node: adj.To, Cost: cost})
			case cost == next.Cost:
				next.Predecessors = append(next.Predecessors, pred)
				sort.Slice(next.Predecessors, func(i, j int) bool {
					if next.Predecessors[i].Node == next.Predecessors[j].Node {
						return next.Predecessors[i].Link < next.Predecessors[j].Link
					}
					return next.Predecessors[i].Node < next.Predecessors[j].Node
				})
				dist[adj.To] = next
			}
		}
	}
	return dist
}

func RepresentativePath(src, dst string, spf map[string]SPFNode) ([]string, []string, []string, bool) {
	if src == dst {
		return []string{src}, nil, nil, true
	}
	state, ok := spf[dst]
	if !ok || state.Cost == UnreachableCost || len(state.Predecessors) == 0 {
		return nil, nil, nil, false
	}
	pred := state.Predecessors[0]
	nodes, links, areas, ok := RepresentativePath(src, pred.Node, spf)
	if !ok {
		return nil, nil, nil, false
	}
	return append(nodes, dst), append(links, pred.Link), append(areas, pred.Area), true
}

func SPFCondition(src, dst string, spf map[string]SPFNode, memo map[string]failure.Cond) failure.Cond {
	if cond, ok := memo[dst]; ok {
		return cond
	}
	if src == dst {
		cond := failure.NodeVar(src)
		memo[dst] = cond
		return cond
	}
	state, ok := spf[dst]
	if !ok || state.Cost == UnreachableCost || len(state.Predecessors) == 0 {
		return failure.False()
	}
	branches := make([]failure.Cond, 0, len(state.Predecessors))
	for _, pred := range state.Predecessors {
		branches = append(branches, failure.And(SPFCondition(src, pred.Node, spf, memo), failure.LinkVar(pred.Link), failure.NodeVar(dst)))
	}
	cond := failure.Or(branches...)
	memo[dst] = cond
	return cond
}

func AdjacencyCost(idx *model.TopologyIndex, from, to string, link model.Link, states map[string]map[string]InterfaceState, allowed AdjacencyFilter) (int, string, bool) {
	fromRef, ok := idx.InterfaceOnLink(from, link.Name)
	if !ok {
		return 0, "", false
	}
	toRef, ok := idx.InterfaceOnLink(to, link.Name)
	if !ok {
		return 0, "", false
	}
	fromState, ok := states[from][fromRef.ConfigName]
	if !ok || fromState.Passive {
		return 0, "", false
	}
	toState, ok := states[to][toRef.ConfigName]
	if !ok || toState.Passive {
		return 0, "", false
	}
	area, ok := allowed(fromState, toState)
	if !ok {
		return 0, "", false
	}
	if !NetworkTypesMatchForAdjacency(fromState.NetworkType, toState.NetworkType) {
		return 0, "", false
	}
	return fromState.Cost, area, true
}

func NetworkTypesMatchForAdjacency(a, b string) bool {
	a = NormalizeNetworkType(a)
	b = NormalizeNetworkType(b)
	if a == "" || b == "" {
		return true
	}
	return a == b
}

func NormalizeNetworkType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "p2p":
		return "point-to-point"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func PathCondition(path Path) []failure.Cond {
	conds := make([]failure.Cond, 0, len(path.Nodes)+len(path.Links))
	for _, node := range path.Nodes {
		conds = append(conds, failure.NodeVar(node))
	}
	for _, link := range path.Links {
		conds = append(conds, failure.LinkVar(link))
	}
	return conds
}

func (q SPFQueue) Len() int { return len(q) }

func (q SPFQueue) Less(i, j int) bool {
	if q[i].Cost == q[j].Cost {
		return q[i].Node < q[j].Node
	}
	return q[i].Cost < q[j].Cost
}

func (q SPFQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }

func (q *SPFQueue) Push(x any) {
	*q = append(*q, x.(SPFQueueItem))
}

func (q *SPFQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[:n-1]
	return item
}
