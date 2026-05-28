package controlplane

import (
	"sort"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	domainospf "github.com/81ueman/hoyan-lab/internal/domain/routing/ospf"
)

func (e *Engine) ospfCandidatePaths(src, area string, states map[string]map[string]domainospf.InterfaceState) map[string][]domainospf.Path {
	return e.ospfCandidatePathsWithArea(src, states, func(fromState, toState domainospf.InterfaceState) (string, bool) {
		if fromState.Area != area || toState.Area != area {
			return "", false
		}
		return area, true
	})
}

func (e *Engine) ospfCandidatePathsAnyArea(src string, states map[string]map[string]domainospf.InterfaceState) map[string][]domainospf.Path {
	return e.ospfCandidatePathsWithArea(src, states, func(fromState, toState domainospf.InterfaceState) (string, bool) {
		if fromState.Area != toState.Area {
			return "", false
		}
		return fromState.Area, true
	})
}

func (e *Engine) ospfCandidatePathsWithArea(src string, states map[string]map[string]domainospf.InterfaceState, allowed domainospf.AdjacencyFilter) map[string][]domainospf.Path {
	out := map[string][]domainospf.Path{}
	for _, firstHop := range e.ospfAdjacencies(src, states, allowed) {
		spf := domainospf.ShortestPathTree(e.idx.Topology.Nodes, firstHop.To, src, func(from string) []domainospf.Adjacency {
			return e.ospfAdjacencies(from, states, allowed)
		})
		condMemo := map[string]failure.Cond{}
		for dst, state := range spf {
			if dst == firstHop.To {
				path := domainospf.Path{
					Cost:  firstHop.Cost,
					Nodes: []string{src, firstHop.To},
					Links: []string{firstHop.Link},
					Areas: []string{firstHop.Area},
					Cond:  failure.And(failure.NodeVar(src), failure.LinkVar(firstHop.Link), failure.NodeVar(firstHop.To)),
				}
				out[dst] = append(out[dst], path)
				continue
			}
			if state.Cost == domainospf.UnreachableCost {
				continue
			}
			nodes, links, areas, ok := domainospf.RepresentativePath(firstHop.To, dst, spf)
			if !ok {
				continue
			}
			path := domainospf.Path{
				Cost:  firstHop.Cost + state.Cost,
				Nodes: append([]string{src}, nodes...),
				Links: append([]string{firstHop.Link}, links...),
				Areas: append([]string{firstHop.Area}, areas...),
				Cond:  failure.And(failure.NodeVar(src), failure.LinkVar(firstHop.Link), domainospf.SPFCondition(firstHop.To, dst, spf, condMemo)),
			}
			out[dst] = append(out[dst], path)
		}
	}
	for node, paths := range out {
		domainospf.SortPaths(paths)
		out[node] = paths
	}
	return out
}

func (e *Engine) ospfInterAreaPaths(src string, adv domainospf.Advertisement, states map[string]map[string]domainospf.InterfaceState, areas map[string]map[string]bool, abrs map[string]bool) []domainospf.Path {
	if areas[src][adv.Area] {
		return nil
	}
	srcAreas := sortedAreaKeys(areas[src])
	var out []domainospf.Path
	for _, srcArea := range srcAreas {
		srcPaths := e.ospfCandidatePaths(src, srcArea, states)
		backbonePathsBySrcABR := map[string]map[string][]domainospf.Path{}
		for _, srcABR := range domainospf.AreaBoundaries(src, srcArea, areas, abrs) {
			toSrcABR := domainospf.ZeroPath(src, srcABR, srcPaths)
			if len(toSrcABR.Nodes) == 0 {
				continue
			}
			if _, ok := backbonePathsBySrcABR[srcABR]; !ok {
				backbonePathsBySrcABR[srcABR] = e.ospfCandidatePaths(srcABR, domainospf.BackboneArea, states)
			}
			for _, dstABR := range domainospf.AreaBoundaries(adv.Node, adv.Area, areas, abrs) {
				toDstABR := domainospf.ZeroPath(srcABR, dstABR, backbonePathsBySrcABR[srcABR])
				if len(toDstABR.Nodes) == 0 {
					continue
				}
				dstPaths := e.ospfCandidatePaths(dstABR, adv.Area, states)
				toAdv := domainospf.ZeroPath(dstABR, adv.Node, dstPaths)
				if len(toAdv.Nodes) == 0 {
					continue
				}
				combined, ok := domainospf.ConcatPaths(toSrcABR, toDstABR, toAdv)
				if ok {
					out = append(out, combined)
				}
			}
		}
	}
	domainospf.SortPaths(out)
	if len(out) > domainospf.MaxPathsPerDestination {
		out = out[:domainospf.MaxPathsPerDestination]
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

func (e *Engine) ospfAdjacencies(from string, states map[string]map[string]domainospf.InterfaceState, allowed domainospf.AdjacencyFilter) []domainospf.Adjacency {
	var out []domainospf.Adjacency
	for _, edge := range e.idx.Adj[model.NodeID(from)] {
		to := string(edge.To)
		cost, area, ok := domainospf.AdjacencyCost(e.idx, from, to, edge.Link, states, allowed)
		if !ok {
			continue
		}
		out = append(out, domainospf.Adjacency{From: from, To: to, Link: edge.Link.Name, Area: area, Cost: cost})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].To == out[j].To {
			return out[i].Link < out[j].Link
		}
		return out[i].To < out[j].To
	})
	return out
}
