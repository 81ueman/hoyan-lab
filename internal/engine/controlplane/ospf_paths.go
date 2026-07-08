package controlplane

import (
	"container/heap"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func (e *Engine) ospfCandidatePaths(src, area string, states map[string]map[string]InterfaceState) map[string][]Path {
	return e.ospfCandidatePathsWithArea(src, states, func(fromState, toState InterfaceState) (string, bool) {
		if fromState.Area != area || toState.Area != area {
			return "", false
		}
		return area, true
	})
}

func (e *Engine) ospfCandidatePathsAnyArea(src string, states map[string]map[string]InterfaceState) map[string][]Path {
	return e.ospfCandidatePathsWithArea(src, states, func(fromState, toState InterfaceState) (string, bool) {
		if fromState.Area != toState.Area {
			return "", false
		}
		return fromState.Area, true
	})
}

func (e *Engine) ospfCandidatePathsWithArea(src string, states map[string]map[string]InterfaceState, allowed AdjacencyFilter) map[string][]Path {
	return e.ospfCandidatePathsBounded(src, states, allowed)
}

// pathQueueItem represents a partial path in the bounded enumeration queue.
type pathQueueItem struct {
	Cost  int
	Nodes []string
	Links []string
	Areas []string
	Last  string
}

type pathQueue []pathQueueItem

func (q pathQueue) Len() int { return len(q) }

func (q pathQueue) Less(i, j int) bool {
	if q[i].Cost != q[j].Cost {
		return q[i].Cost < q[j].Cost
	}
	// Deterministic tie-breaking: prefer shorter paths, then lexicographic node order
	if len(q[i].Nodes) != len(q[j].Nodes) {
		return len(q[i].Nodes) < len(q[j].Nodes)
	}
	for k := 0; k < len(q[i].Nodes) && k < len(q[j].Nodes); k++ {
		if q[i].Nodes[k] != q[j].Nodes[k] {
			return q[i].Nodes[k] < q[j].Nodes[k]
		}
	}
	return len(q[i].Nodes) < len(q[j].Nodes)
}

func (q pathQueue) Swap(i, j int) { q[i], q[j] = q[j], q[i] }
func (q *pathQueue) Push(x any)   { *q = append(*q, x.(pathQueueItem)) }
func (q *pathQueue) Pop() any {
	old := *q
	n := len(old)
	item := old[n-1]
	*q = old[:n-1]
	return item
}

// ospfCandidatePathsBounded enumerates simple paths in increasing OSPF cost order
// bounded by MaxPathsPerDestination per destination. It uses a priority-queue
// approach to find multiple paths (not just the SPF shortest) including
// same-first-hop higher-cost alternates.
func (e *Engine) ospfCandidatePathsBounded(src string, states map[string]map[string]InterfaceState, allowed AdjacencyFilter) map[string][]Path {
	out := map[string][]Path{}
	found := map[string]int{}    // paths recorded per destination
	expanded := map[string]int{} // expansions performed per node
	maxHops := len(e.idx.Topology.Nodes)

	q := &pathQueue{}
	heap.Init(q)

	// Seed with first-hop adjacencies
	for _, adj := range e.ospfAdjacencies(src, states, allowed) {
		heap.Push(q, pathQueueItem{
			Cost:  adj.Cost,
			Nodes: []string{src, adj.To},
			Links: []string{adj.Link},
			Areas: []string{adj.Area},
			Last:  adj.To,
		})
	}

	for q.Len() > 0 {
		item := heap.Pop(q).(pathQueueItem)
		dst := item.Last

		// Record path to this destination if we haven't reached the cap
		if found[dst] < MaxPathsPerDestination {
			path := Path{
				Cost:  item.Cost,
				Nodes: item.Nodes,
				Links: item.Links,
				Areas: item.Areas,
				Cond:  failure.And(PathCondition(Path{Nodes: item.Nodes, Links: item.Links})...),
			}
			out[dst] = append(out[dst], path)
			found[dst]++
		}

		// Per-node expansion cap: only expand the first MaxPathsPerDestination
		// paths ending at this node to prevent factorial queue explosion.
		if expanded[dst] >= MaxPathsPerDestination {
			continue
		}
		expanded[dst]++

		if len(item.Nodes) >= maxHops {
			continue
		}

		// Expand from the last node
		for _, adj := range e.ospfAdjacencies(item.Last, states, allowed) {
			// Skip already-visited nodes (loop prevention)
			visited := false
			for _, n := range item.Nodes {
				if n == adj.To {
					visited = true
					break
				}
			}
			if visited {
				continue
			}

			newNodes := make([]string, len(item.Nodes)+1)
			copy(newNodes, item.Nodes)
			newNodes[len(item.Nodes)] = adj.To

			newLinks := make([]string, len(item.Links)+1)
			copy(newLinks, item.Links)
			newLinks[len(item.Links)] = adj.Link

			newAreas := make([]string, len(item.Areas)+1)
			copy(newAreas, item.Areas)
			newAreas[len(item.Areas)] = adj.Area

			heap.Push(q, pathQueueItem{
				Cost:  item.Cost + adj.Cost,
				Nodes: newNodes,
				Links: newLinks,
				Areas: newAreas,
				Last:  adj.To,
			})
		}
	}

	// Sort paths per destination
	for _, paths := range out {
		SortPaths(paths)
	}

	return out
}

func (e *Engine) ospfInterAreaPaths(src string, adv Advertisement, states map[string]map[string]InterfaceState, areas map[string]map[string]bool, abrs map[string]bool) []Path {
	if areas[src][adv.Area] {
		return nil
	}
	srcAreas := sortedAreaKeys(areas[src])
	var out []Path
	for _, srcArea := range srcAreas {
		srcPaths := e.ospfCandidatePaths(src, srcArea, states)
		backbonePathsBySrcABR := map[string]map[string][]Path{}
		for _, srcABR := range AreaBoundaries(src, srcArea, areas, abrs) {
			toSrcABR := ZeroPath(src, srcABR, srcPaths)
			if len(toSrcABR.Nodes) == 0 {
				continue
			}
			if _, ok := backbonePathsBySrcABR[srcABR]; !ok {
				backbonePathsBySrcABR[srcABR] = e.ospfCandidatePaths(srcABR, BackboneArea, states)
			}
			for _, dstABR := range AreaBoundaries(adv.Node, adv.Area, areas, abrs) {
				toDstABR := ZeroPath(srcABR, dstABR, backbonePathsBySrcABR[srcABR])
				if len(toDstABR.Nodes) == 0 {
					continue
				}
				dstPaths := e.ospfCandidatePaths(dstABR, adv.Area, states)
				toAdv := ZeroPath(dstABR, adv.Node, dstPaths)
				if len(toAdv.Nodes) == 0 {
					continue
				}
				combined, ok := ConcatPaths(toSrcABR, toDstABR, toAdv)
				if ok {
					out = append(out, combined)
				}
			}
		}
	}
	SortPaths(out)
	if len(out) > MaxPathsPerDestination {
		out = out[:MaxPathsPerDestination]
	}
	return out
}

func sortedAreaKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for area := range m {
		out = append(out, area)
	}
	sort.Strings(out)
	return out
}

func (e *Engine) ospfAdjacencies(from string, states map[string]map[string]InterfaceState, allowed AdjacencyFilter) []Adjacency {
	var out []Adjacency
	for _, edge := range e.idx.Adj[model.NodeID(from)] {
		to := string(edge.To)
		cost, area, ok := AdjacencyCost(e.idx, from, to, edge.Link, states, allowed)
		if !ok {
			continue
		}
		out = append(out, Adjacency{From: from, To: to, Link: edge.Link.Name, Area: area, Cost: cost})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].To == out[j].To {
			return out[i].Link < out[j].Link
		}
		return out[i].To < out[j].To
	})
	return out
}
