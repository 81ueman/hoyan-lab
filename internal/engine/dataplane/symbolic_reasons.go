package dataplane

import (
	"github.com/81ueman/hoyan-lab/internal/domain/device"
	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

// ---- Unreachable reason handler functions ----

func handleLoop(reasons *[]SymbolicUnreachableReason, state SymbolicPacketState) {
	addUnreachableReason(reasons, SymbolicUnreachableReason{
		Kind:    UnreachableLoop,
		Node:    state.Node,
		Cond:    condOrTrue(state.Cond),
		Path:    state.Path,
		Message: "forwarding loop",
	})
}

func handleIngressPolicy(reasons *[]SymbolicUnreachableReason, state SymbolicPacketState, ingressDecision device.PolicyDecision) {
	denyCond := failure.And(state.Cond, ingressDecision.Cond)
	addUnreachableReason(reasons, SymbolicUnreachableReason{
		Kind:          UnreachableIngressPolicy,
		Node:          state.Node,
		Interface:     state.IngressInterface,
		PolicyName:    ingressDecision.PolicyName,
		ACLName:       ingressDecision.ACLName,
		RuleSeq:       ingressDecision.RuleSeq,
		Action:        ingressDecision.Action,
		DefaultAction: ingressDecision.DefaultAction,
		PolicyRaw:     ingressDecision.Source.Raw,
		Cond:          denyCond,
		Path:          state.Path,
		Message:       ingressDecision.Reason,
	})
}

func handleNoRoute(reasons *[]SymbolicUnreachableReason, state SymbolicPacketState, candidateConds []failure.Cond) {
	addUnreachableReason(reasons, SymbolicUnreachableReason{
		Kind:    UnreachableNoRoute,
		Node:    state.Node,
		Cond:    failure.And(state.Cond, failure.Not(failure.Or(candidateConds...))),
		Path:    state.Path,
		Message: "no forwarding route",
	})
}

func handleDiscard(reasons *[]SymbolicUnreachableReason, state SymbolicPacketState, candidate SymbolicFIBCandidate) {
	addUnreachableReason(reasons, SymbolicUnreachableReason{
		Kind:    UnreachableDiscard,
		Node:    state.Node,
		Cond:    failure.And(state.Cond, candidate.Cond),
		Path:    state.Path,
		Message: "discard route selected",
	})
}

func handleRecursiveNextHop(reasons *[]SymbolicUnreachableReason, state SymbolicPacketState, candidate SymbolicFIBCandidate) {
	addUnreachableReason(reasons, SymbolicUnreachableReason{
		Kind:    UnreachableRecursiveNextHopUnresolved,
		Node:    state.Node,
		Cond:    failure.And(state.Cond, candidate.Cond),
		Path:    state.Path,
		Message: "recursive next-hop unresolved",
	})
}

func handleManagementFallback(reasons *[]SymbolicUnreachableReason, state SymbolicPacketState, candidate SymbolicFIBCandidate, iface string) {
	addUnreachableReason(reasons, SymbolicUnreachableReason{
		Kind:      UnreachableNextHopManagementFallback,
		Node:      state.Node,
		Interface: iface,
		Cond:      failure.And(state.Cond, candidate.Cond),
		Path:      state.Path,
		Message:   "next-hop resolved via management interface",
	})
}

func handleNoNextHop(reasons *[]SymbolicUnreachableReason, state SymbolicPacketState, candidate SymbolicFIBCandidate) {
	addUnreachableReason(reasons, SymbolicUnreachableReason{
		Kind:    UnreachableNoNextHop,
		Node:    state.Node,
		Cond:    failure.And(state.Cond, candidate.Cond),
		Path:    state.Path,
		Message: "selected route has no next-hop",
	})
}

func handleNodeFailed(reasons *[]SymbolicUnreachableReason, node string, cond failure.Cond, path Path, message string) {
	addUnreachableReason(reasons, SymbolicUnreachableReason{
		Kind:    UnreachableNodeFailed,
		Node:    node,
		Cond:    cond,
		Path:    path,
		Message: message,
	})
}

func handleNextHopNotAdjacent(reasons *[]SymbolicUnreachableReason, state SymbolicPacketState, candidate SymbolicFIBCandidate, nextHop string) {
	addUnreachableReason(reasons, SymbolicUnreachableReason{
		Kind:    UnreachableNextHopNotAdjacent,
		Node:    state.Node,
		Link:    state.Node + "-" + nextHop,
		Cond:    failure.And(state.Cond, candidate.Cond, failure.NodeVar(nextHop)),
		Path:    state.Path,
		Message: "next-hop is not adjacent",
	})
}

func handleLinkFailed(reasons *[]SymbolicUnreachableReason, state SymbolicPacketState, candidate SymbolicFIBCandidate, link model.Link, linkUpCond failure.Cond) {
	addUnreachableReason(reasons, SymbolicUnreachableReason{
		Kind:    UnreachableLinkFailed,
		Node:    state.Node,
		Link:    link.Name,
		Cond:    failure.And(state.Cond, candidate.Cond, failure.NodeVar(candidate.Entry.NextHop), failure.Not(linkUpCond)),
		Path:    state.Path,
		Message: "next-hop link is down",
	})
}

func handleEgressPolicy(reasons *[]SymbolicUnreachableReason, state SymbolicPacketState, candidate SymbolicFIBCandidate, egressDecision device.PolicyDecision, path Path, egressInterface string) {
	denyCond := failure.And(state.Cond, candidate.Cond, egressDecision.Cond)
	addUnreachableReason(reasons, SymbolicUnreachableReason{
		Kind:          UnreachableEgressPolicy,
		Node:          state.Node,
		Interface:     egressInterface,
		PolicyName:    egressDecision.PolicyName,
		ACLName:       egressDecision.ACLName,
		RuleSeq:       egressDecision.RuleSeq,
		Action:        egressDecision.Action,
		DefaultAction: egressDecision.DefaultAction,
		PolicyRaw:     egressDecision.Source.Raw,
		Cond:          denyCond,
		Path:          path,
		Message:       egressDecision.Reason,
	})
}

func addUnreachableReason(reasons *[]SymbolicUnreachableReason, reason SymbolicUnreachableReason) {
	if reasons == nil || isFalseCond(reason.Cond) {
		return
	}
	reason.Cond = condOrTrue(reason.Cond)
	reason.Path.Nodes = append([]string(nil), reason.Path.Nodes...)
	reason.Path.Links = append([]string(nil), reason.Path.Links...)
	*reasons = append(*reasons, reason)
}
