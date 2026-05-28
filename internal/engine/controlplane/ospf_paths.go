package controlplane

import (
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
	out := map[string][]Path{}
	for _, firstHop := range e.ospfAdjacencies(src, states, allowed) {
		spf := ShortestPathTree(e.idx.Topology.Nodes, firstHop.To, src, func(from string) []Adjacency {
			return e.ospfAdjacencies(from, states, allowed)
		})
		condMemo := map[string]failure.Cond{}
		for dst, state := range spf {
			if dst == firstHop.To {
				path := Path{
					Cost:  firstHop.Cost,
					Nodes: []string{src, firstHop.To},
					Links: []string{firstHop.Link},
					Areas: []string{firstHop.Area},
					Cond:  failure.And(failure.NodeVar(src), failure.LinkVar(firstHop.Link), failure.NodeVar(firstHop.To)),
				}
				out[dst] = append(out[dst], path)
				continue
			}
			if state.Cost == UnreachableCost {
				continue
			}
			nodes, links, areas, ok := RepresentativePath(firstHop.To, dst, spf)
			if !ok {
				continue
			}
			path := Path{
				Cost:  firstHop.Cost + state.Cost,
				Nodes: append([]string{src}, nodes...),
				Links: append([]string{firstHop.Link}, links...),
				Areas: append([]string{firstHop.Area}, areas...),
				Cond:  failure.And(failure.NodeVar(src), failure.LinkVar(firstHop.Link), SPFCondition(firstHop.To, dst, spf, condMemo)),
			}
			out[dst] = append(out[dst], path)
		}
	}
	for node, paths := range out {
		SortPaths(paths)
		out[node] = paths
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
