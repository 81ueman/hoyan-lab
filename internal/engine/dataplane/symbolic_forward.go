package dataplane

import (
	"net/netip"

	"github.com/81ueman/hoyan-lab/internal/domain/device"
	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

func (e *Engine) symbolicForward(state SymbolicPacketState, vrf string, dst model.PrefixSet, packetPrefix netip.Prefix, maxHops int, visited map[string]bool, states []SymbolicPacketState, paths *[]SymbolicPacketPath, blocked *[]SymbolicPacketBlockedPath, reasons *[]SymbolicUnreachableReason) {
	if isFalseCond(state.Cond) {
		return
	}
	if visited[state.Node] {
		handleLoop(reasons, state)
		return
	}
	states = append(states, state)
	if e.originatesPrefixSetVRF(state.Node, vrf, dst) {
		cond := failure.And(condOrTrue(state.Cond), failure.NodeVar(state.Node))
		*paths = append(*paths, SymbolicPacketPath{Path: state.Path, Cond: cond, States: append([]SymbolicPacketState(nil), states...)})
		return
	}
	if len(state.Path.Nodes) > maxHops {
		return
	}
	currentNode, ok := e.idx.Node(state.Node)
	if !ok {
		return
	}
	packet := state.Packet
	packet.Node = state.Node
	packet.Spec.IngressInterface = state.IngressInterface
	ingressDecision := e.dataACLDecision(currentNode, packet, "ingress")
	if ingressDecision.Denied {
		e.appendBlockedPolicyPath(blocked, state.Path, failure.And(state.Cond, ingressDecision.Cond), ingressDecision, state.Node, packet.Spec.IngressInterface, "ingress")
		handleIngressPolicy(reasons, state, ingressDecision)
		return
	}
	nextVisited := copyVisited(visited)
	nextVisited[state.Node] = true
	candidates := e.SymbolicLookupFIBForPrefixSetVRF(state.Node, vrf, dst)
	candidateConds := make([]failure.Cond, 0, len(candidates))
	for _, candidate := range candidates {
		candidateConds = append(candidateConds, candidate.Cond)
	}
	handleNoRoute(reasons, state, candidateConds)
	for _, candidate := range candidates {
		entry := candidate.Entry
		if entry.Discard {
			handleDiscard(reasons, state, candidate)
			continue
		}
		switch entry.effectiveResolutionStatus() {
		case NextHopResolutionUnresolvedRecursive:
			handleRecursiveNextHop(reasons, state, candidate)
			continue
		case NextHopResolutionManagementFallback:
			handleManagementFallback(reasons, state, candidate, entry.Interface)
			continue
		}
		if entry.NextHop == "" {
			handleNoNextHop(reasons, state, candidate)
			continue
		}
		handleNodeFailed(reasons, entry.NextHop, failure.And(state.Cond, candidate.Cond, failure.Not(failure.NodeVar(entry.NextHop))), state.Path, "next-hop node is down")
		link, ok := e.idx.LinkBetween(state.Node, entry.NextHop)
		if !ok {
			handleNextHopNotAdjacent(reasons, state, candidate, entry.NextHop)
			continue
		}
		packet.Spec.EgressInterface = ingressInterface(link, state.Node)
		linkUpCond := e.linkUpCond(link)
		handleLinkFailed(reasons, state, candidate, link, linkUpCond)
		nextPath := Path{
			Nodes: append(append([]string(nil), state.Path.Nodes...), entry.NextHop),
			Links: append(append([]string(nil), state.Path.Links...), link.Name),
			Cost:  state.Path.Cost + link.Cost,
		}
		egressDecision := e.dataACLDecision(currentNode, packet, "egress")
		if egressDecision.Denied {
			e.appendBlockedPolicyPath(blocked, nextPath, failure.And(state.Cond, candidate.Cond, egressDecision.Cond), egressDecision, state.Node, packet.Spec.EgressInterface, "egress")
			handleEgressPolicy(reasons, state, candidate, egressDecision, nextPath, packet.Spec.EgressInterface)
			continue
		}
		nextCond := failure.And(
			state.Cond,
			candidate.Cond,
			failure.Not(egressDecision.Cond),
			linkUpCond,
		)
		nextState := SymbolicPacketState{
			Node:             entry.NextHop,
			IngressInterface: ingressInterface(link, entry.NextHop),
			Packet:           packet,
			Cond:             nextCond,
			Path:             nextPath,
		}
		e.symbolicForward(nextState, vrf, dst, packetPrefix, maxHops, nextVisited, states, paths, blocked, reasons)
	}
}

func (e *Engine) appendBlockedPolicyPath(blocked *[]SymbolicPacketBlockedPath, path Path, cond failure.Cond, decision device.PolicyDecision, node, iface, stage string) {
	if blocked == nil {
		return
	}
	*blocked = append(*blocked, SymbolicPacketBlockedPath{
		Path:          clonePath(path),
		Cond:          condOrTrue(cond),
		Reason:        decision.Reason,
		ACL:           decision.ACLName,
		RuleSeq:       decision.RuleSeq,
		Action:        decision.Action,
		Node:          node,
		Interface:     iface,
		Stage:         stage,
		DefaultAction: decision.DefaultAction,
		Source:        decision.Source,
	})
}

func clonePath(path Path) Path {
	return Path{
		Nodes: append([]string(nil), path.Nodes...),
		Links: append([]string(nil), path.Links...),
		Cost:  path.Cost,
	}
}
