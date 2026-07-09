package dataplane

import (
	"net/netip"
	"sort"
	"strconv"

	"github.com/81ueman/hoyan-lab/internal/domain/device"
	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type SymbolicPacketState struct {
	Node             string
	IngressInterface string
	Packet           device.PacketMessage
	Cond             failure.Cond
	Path             Path
}

type SymbolicFIBCandidate struct {
	Entry FIBEntry
	Cond  failure.Cond
}

type SymbolicPacketPath struct {
	Path   Path
	Cond   failure.Cond
	States []SymbolicPacketState
}

type SymbolicPacketBlockedPath struct {
	Path          Path
	Cond          failure.Cond
	Reason        string
	ACL           string
	RuleSeq       int
	Action        model.ACLAction
	Node          string
	Interface     string
	Stage         string
	DefaultAction model.ACLDefaultAction
	Source        model.ConfigSource
}

type SymbolicUnreachableReasonKind string

const (
	UnreachableNoRoute                    SymbolicUnreachableReasonKind = "no_route"
	UnreachableNoNextHop                  SymbolicUnreachableReasonKind = "no_next_hop"
	UnreachableDiscard                    SymbolicUnreachableReasonKind = "discard"
	UnreachableRecursiveNextHopUnresolved SymbolicUnreachableReasonKind = "recursive_next_hop_unresolved"
	UnreachableNextHopNotAdjacent         SymbolicUnreachableReasonKind = "next_hop_not_adjacent"
	UnreachableNextHopManagementFallback  SymbolicUnreachableReasonKind = "next_hop_management_fallback"
	UnreachableNodeFailed                 SymbolicUnreachableReasonKind = "node_failed"
	UnreachableLinkFailed                 SymbolicUnreachableReasonKind = "link_failed"
	UnreachableIngressPolicy              SymbolicUnreachableReasonKind = "ingress_policy"
	UnreachableEgressPolicy               SymbolicUnreachableReasonKind = "egress_policy"
	UnreachableLoop                       SymbolicUnreachableReasonKind = "loop"
	UnreachableDestinationNotAdvertised   SymbolicUnreachableReasonKind = "destination_not_advertised"
)

type SymbolicUnreachableReason struct {
	Kind          SymbolicUnreachableReasonKind
	Node          string
	Link          string
	Interface     string
	PolicyName    string
	ACLName       string
	RuleSeq       int
	Action        model.ACLAction
	DefaultAction model.ACLDefaultAction
	PolicyRaw     string
	Path          Path
	Cond          failure.Cond
	Message       string
}

type SymbolicReachabilityResult struct {
	Reachable          failure.Cond
	Unreachable        failure.Cond
	Paths              []SymbolicPacketPath
	Blocked            []SymbolicPacketBlockedPath
	UnreachableReasons []SymbolicUnreachableReason
	Reason             string
}

type SymbolicRoutePath struct {
	Path Path
	Cond failure.Cond
}

type SymbolicRouteReachabilityResult struct {
	Reachable   failure.Cond
	Unreachable failure.Cond
	Paths       []SymbolicRoutePath
	Reason      string
}

func (e *Engine) SymbolicLookupFIB(node, dst string) []SymbolicFIBCandidate {
	return e.SymbolicLookupFIBVRF(node, string(model.NetworkInstanceDefault), dst)
}

func (e *Engine) SymbolicLookupFIBVRF(node, vrf, dst string) []SymbolicFIBCandidate {
	ip, err := netip.ParseAddr(dst)
	if err != nil {
		return nil
	}
	entries := matchingFIBEntries(e.fib[model.NodeID(node)][model.NormalizeNetworkInstance(vrf)], ip)
	return e.symbolicLookupFIBEntries(entries)
}

func (e *Engine) SymbolicLookupFIBForPrefixSet(node string, dst model.PrefixSet) []SymbolicFIBCandidate {
	return e.SymbolicLookupFIBForPrefixSetVRF(node, string(model.NetworkInstanceDefault), dst)
}

func (e *Engine) SymbolicLookupFIBForPrefixSetVRF(node, vrf string, dst model.PrefixSet) []SymbolicFIBCandidate {
	entries := matchingFIBEntriesForPrefixSet(e.fib[model.NodeID(node)][model.NormalizeNetworkInstance(vrf)], dst)
	return e.symbolicLookupFIBEntries(entries)
}

func (e *Engine) symbolicLookupFIBEntries(entries []FIBEntry) []SymbolicFIBCandidate {
	var out []SymbolicFIBCandidate
	var higher []failure.Cond
	groupConds := map[string]failure.Cond{}
	groupIndexes := map[string]int{}
	for _, entry := range entries {
		entryCond := e.expandLinkVars(condOrTrue(entry.Condition))
		groupKey := fibCandidateGroupKey(entry, len(out))
		cond := entryCond
		higherForEntry := higher
		if ownIndex, ok := groupIndexes[groupKey]; ok {
			higherForEntry = append(append([]failure.Cond(nil), higher[:ownIndex]...), higher[ownIndex+1:]...)
		}
		if len(higherForEntry) > 0 {
			cond = failure.And(cond, failure.Not(failure.Or(higherForEntry...)))
		}
		out = append(out, SymbolicFIBCandidate{Entry: entry, Cond: cond})
		if existing, ok := groupConds[groupKey]; ok {
			merged := failure.Or(existing, entryCond)
			groupConds[groupKey] = merged
			higher[groupIndexes[groupKey]] = merged
			continue
		}
		groupConds[groupKey] = entryCond
		groupIndexes[groupKey] = len(higher)
		higher = append(higher, entryCond)
	}
	return out
}

func fibCandidateGroupKey(entry FIBEntry, ordinal int) string {
	if entry.GroupID == "" {
		return "entry#" + strconv.Itoa(ordinal)
	}
	return entry.Prefix.String() + "#" + strconv.Itoa(entry.Rank) + "#" + entry.GroupID
}

func (e *Engine) SymbolicRouteReachability(from, prefix string) SymbolicRouteReachabilityResult {
	return e.SymbolicRouteReachabilityVRF(from, string(model.NetworkInstanceDefault), prefix)
}

func (e *Engine) SymbolicRouteReachabilityVRF(from, vrf, prefix string) SymbolicRouteReachabilityResult {
	reachable := failure.False()
	result := SymbolicRouteReachabilityResult{Reachable: reachable, Unreachable: failure.True()}
	if e == nil || e.idx == nil {
		result.Reason = "topology index is unavailable"
		return result
	}
	pfx, err := model.ParsePrefix(prefix)
	if err != nil {
		result.Reason = "invalid prefix"
		return result
	}
	if _, ok := e.idx.Node(from); !ok {
		result.Reason = "source node not found"
		return result
	}
	routes := e.rib[model.NodeID(from)][model.NormalizeNetworkInstance(vrf)][pfx]
	paths := make([]SymbolicRoutePath, 0, len(routes))
	conds := make([]failure.Cond, 0, len(routes))
	for _, route := range routes {
		route = route.Normalize()
		if route.SelectedCond == nil {
			continue
		}
		cond := failure.And(failure.NodeVar(from), e.expandLinkVars(route.SelectedCond))
		path := routePath(e.idx, route)
		paths = append(paths, SymbolicRoutePath{Path: path, Cond: cond})
		conds = append(conds, cond)
	}
	reachable = failure.Or(conds...)
	return SymbolicRouteReachabilityResult{
		Reachable:   reachable,
		Unreachable: failure.Not(reachable),
		Paths:       paths,
	}
}

func (e *Engine) SymbolicRouteReachabilityForPrefixSet(from string, dst model.PrefixSet) SymbolicRouteReachabilityResult {
	return e.SymbolicRouteReachabilityForPrefixSetVRF(from, string(model.NetworkInstanceDefault), dst)
}

func (e *Engine) SymbolicRouteReachabilityForPrefixSetVRF(from, vrf string, dst model.PrefixSet) SymbolicRouteReachabilityResult {
	reachable := failure.False()
	result := SymbolicRouteReachabilityResult{Reachable: reachable, Unreachable: failure.True()}
	if e == nil || e.idx == nil {
		result.Reason = "topology index is unavailable"
		return result
	}
	if dst == nil {
		result.Reason = "destination prefix set is empty"
		return result
	}
	if _, ok := e.idx.Node(from); !ok {
		result.Reason = "source node not found"
		return result
	}
	candidates := e.SymbolicLookupFIBForPrefixSetVRF(from, vrf, dst)
	paths := make([]SymbolicRoutePath, 0, len(candidates))
	conds := make([]failure.Cond, 0, len(candidates))
	for _, candidate := range candidates {
		cond := failure.And(failure.NodeVar(from), candidate.Cond)
		paths = append(paths, SymbolicRoutePath{Path: candidate.Entry.Path, Cond: cond})
		conds = append(conds, cond)
	}
	reachable = failure.Or(conds...)
	if len(paths) == 0 {
		result.Reason = "no route for prefix set"
	}
	return SymbolicRouteReachabilityResult{
		Reachable:   reachable,
		Unreachable: failure.Not(reachable),
		Paths:       paths,
		Reason:      result.Reason,
	}
}

func (e *Engine) SymbolicRouteReachabilityForClass(from string, universe model.PrefixUniverse, classID model.PrefixClassID) SymbolicRouteReachabilityResult {
	return e.SymbolicRouteReachabilityForClassVRF(from, string(model.NetworkInstanceDefault), universe, classID)
}

func (e *Engine) SymbolicRouteReachabilityForClassVRF(from, vrf string, universe model.PrefixUniverse, classID model.PrefixClassID) SymbolicRouteReachabilityResult {
	for _, class := range universe.Classes {
		if class.ID == classID {
			return e.SymbolicRouteReachabilityForPrefixSetVRF(from, vrf, class.Space)
		}
	}
	return SymbolicRouteReachabilityResult{
		Reachable:   failure.False(),
		Unreachable: failure.True(),
		Reason:      "prefix class not found",
	}
}

func (e *Engine) SymbolicPacketReachability(from, to, protocol string) SymbolicReachabilityResult {
	return e.SymbolicPacketReachabilitySpec(from, to, model.PacketSpec{Protocol: protocol})
}

func (e *Engine) SymbolicPacketReachabilitySpec(from, to string, spec model.PacketSpec) SymbolicReachabilityResult {
	return e.SymbolicPacketReachabilitySpecVRF(from, string(model.NetworkInstanceDefault), to, spec)
}

func (e *Engine) SymbolicPacketReachabilitySpecVRF(from, vrf, to string, spec model.PacketSpec) SymbolicReachabilityResult {
	reachable := failure.False()
	result := SymbolicReachabilityResult{Reachable: reachable, Unreachable: failure.True()}
	if e == nil || e.idx == nil {
		result.Reason = "topology index is unavailable"
		return result
	}
	dstNode, dstPrefix, ok := e.originForIPVRF(to, vrf)
	if !ok {
		result.Reason = "destination prefix not advertised"
		result.UnreachableReasons = []SymbolicUnreachableReason{{
			Kind:    UnreachableDestinationNotAdvertised,
			Cond:    failure.True(),
			Message: result.Reason,
		}}
		return result
	}
	if _, ok := e.idx.Node(from); !ok {
		result.Reason = "source node not found"
		return result
	}
	dstSet := model.ExactPrefixSet{Prefix: dstPrefix}
	return e.symbolicPacketReachabilityForPrefixSet(from, string(model.NormalizeNetworkInstance(vrf)), dstSet, dstPrefix.NetIP(), spec, failure.And(failure.NodeVar(from), failure.NodeVar(dstNode)))
}

func (e *Engine) SymbolicPacketReachabilityForPrefixSet(from string, dst model.PrefixSet, protocol string) SymbolicReachabilityResult {
	return e.SymbolicPacketReachabilityForPrefixSetSpec(from, dst, model.PacketSpec{Protocol: protocol})
}

func (e *Engine) SymbolicPacketReachabilityForPrefixSetSpec(from string, dst model.PrefixSet, spec model.PacketSpec) SymbolicReachabilityResult {
	return e.SymbolicPacketReachabilityForPrefixSetSpecVRF(from, string(model.NetworkInstanceDefault), dst, spec)
}

func (e *Engine) SymbolicPacketReachabilityForPrefixSetSpecVRF(from, vrf string, dst model.PrefixSet, spec model.PacketSpec) SymbolicReachabilityResult {
	result := SymbolicReachabilityResult{Reachable: failure.False(), Unreachable: failure.True()}
	if e == nil || e.idx == nil {
		result.Reason = "topology index is unavailable"
		return result
	}
	if dst == nil {
		result.Reason = "destination prefix set is empty"
		return result
	}
	if _, ok := e.idx.Node(from); !ok {
		result.Reason = "source node not found"
		return result
	}
	rep, ok := representativePrefixForSet(dst)
	if !ok {
		result.Reason = "destination prefix set is unsupported"
		return result
	}
	if !e.hasOriginForPrefixSetVRF(vrf, dst) {
		result.Reason = "destination prefix not advertised"
		return result
	}
	return e.symbolicPacketReachabilityForPrefixSet(from, string(model.NormalizeNetworkInstance(vrf)), dst, rep.NetIP(), spec, failure.NodeVar(from))
}

func (e *Engine) SymbolicPacketReachabilityForClass(from string, universe model.PrefixUniverse, classID model.PrefixClassID, protocol string) SymbolicReachabilityResult {
	return e.SymbolicPacketReachabilityForClassVRF(from, string(model.NetworkInstanceDefault), universe, classID, protocol)
}

func (e *Engine) SymbolicPacketReachabilityForClassVRF(from, vrf string, universe model.PrefixUniverse, classID model.PrefixClassID, protocol string) SymbolicReachabilityResult {
	for _, class := range universe.Classes {
		if class.ID == classID {
			return e.SymbolicPacketReachabilityForPrefixSetSpecVRF(from, vrf, class.Space, model.PacketSpec{Protocol: protocol})
		}
	}
	return SymbolicReachabilityResult{
		Reachable:   failure.False(),
		Unreachable: failure.True(),
		Reason:      "prefix class not found",
	}
}

func (e *Engine) SymbolicPacketReachabilityForPacketClass(from string, class model.PacketClass) SymbolicReachabilityResult {
	if class.DstSet == nil {
		return SymbolicReachabilityResult{
			Reachable:   failure.False(),
			Unreachable: failure.True(),
			Reason:      "packet class destination prefix set is empty",
		}
	}
	return e.SymbolicPacketReachabilityForPrefixSetSpec(from, class.DstSet, class.Spec())
}

func (e *Engine) symbolicPacketReachabilityForPrefixSet(from, vrf string, dst model.PrefixSet, packetPrefix netip.Prefix, spec model.PacketSpec, initialCond failure.Cond) SymbolicReachabilityResult {
	maxHops := len(e.idx.NodesByName)
	if maxHops == 0 {
		maxHops = len(e.fib) + 1
	}
	spec = spec.WithNormalizedPorts()
	spec.DstSet = dst
	packet := device.PacketMessage{Node: from, Spec: spec}
	var reasons []SymbolicUnreachableReason
	handleNodeFailed(&reasons, from, failure.Not(failure.NodeVar(from)), Path{Nodes: []string{from}}, "source node is down")
	for _, dstNode := range e.originNodesForPrefixSetVRF(vrf, dst) {
		if dstNode == from {
			continue
		}
		handleNodeFailed(&reasons, dstNode, failure.Not(failure.NodeVar(dstNode)), Path{Nodes: []string{from}}, "destination node is down")
	}
	initial := SymbolicPacketState{
		Node:   from,
		Packet: packet,
		Cond:   initialCond,
		Path:   Path{Nodes: []string{from}},
	}
	var paths []SymbolicPacketPath
	var blocked []SymbolicPacketBlockedPath
	e.symbolicForward(initial, vrf, dst, packetPrefix, maxHops, map[string]bool{}, nil, &paths, &blocked, &reasons)
	conds := make([]failure.Cond, 0, len(paths))
	for _, path := range paths {
		conds = append(conds, path.Cond)
	}
	reachable := failure.Or(conds...)
	return SymbolicReachabilityResult{
		Reachable:          reachable,
		Unreachable:        failure.Not(reachable),
		Paths:              paths,
		Blocked:            blocked,
		UnreachableReasons: reasons,
	}
}

func routePath(idx *model.TopologyIndex, route domainroute.RIBEntry) Path {
	route = route.Normalize()
	nodes := append([]string(nil), route.Provenance.PathNodes...)
	links := append([]string(nil), route.Provenance.PathLinks...)
	reverse(nodes)
	reverse(links)
	return Path{Nodes: nodes, Links: links, Cost: idx.PathCost(route.Provenance.PathLinks)}
}

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

func matchingFIBEntries(entries []FIBEntry, ip netip.Addr) []FIBEntry {
	var out []FIBEntry
	for _, entry := range entries {
		if entry.Prefix.Contains(ip) {
			out = append(out, entry)
		}
	}
	sortFIBEntriesByPrefixAndRank(out)
	return out
}

func matchingFIBEntriesForPrefixSet(entries []FIBEntry, dst model.PrefixSet) []FIBEntry {
	if dst == nil {
		return nil
	}
	var out []FIBEntry
	for _, entry := range entries {
		entrySet := model.ExactPrefixSet{Prefix: model.PrefixFromNetIP(entry.Prefix)}
		if model.AddressSpaceOverlaps(entrySet, dst) {
			out = append(out, entry)
		}
	}
	sortFIBEntriesByPrefixAndRank(out)
	return out
}

// sortFIBEntriesByPrefixAndRank sorts FIB entries by prefix length (descending) then rank (ascending).
func sortFIBEntriesByPrefixAndRank(entries []FIBEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Prefix.Bits() == entries[j].Prefix.Bits() {
			if entries[i].Rank == entries[j].Rank {
				return false
			}
			return entries[i].Rank < entries[j].Rank
		}
		return entries[i].Prefix.Bits() > entries[j].Prefix.Bits()
	})
}

func representativePrefixForSet(set model.PrefixSet) (model.Prefix, bool) {
	switch s := set.(type) {
	case model.ExactPrefixSet:
		return s.Prefix, !s.Prefix.IsZero()
	case model.PrefixRangeSet:
		return s.Base, !s.Base.IsZero()
	default:
		return model.Prefix{}, false
	}
}

func (e *Engine) originNodesForPrefixSetVRF(vrf string, dst model.PrefixSet) []string {
	if e == nil || e.idx == nil || dst == nil {
		return nil
	}
	var out []string
	for _, node := range e.idx.NodesByName {
		for _, raw := range e.nodePrefixesVRF(node, vrf) {
			if !raw.IsZero() && model.AddressSpaceOverlaps(model.ExactPrefixSet{Prefix: raw}, dst) {
				out = append(out, node.Name)
				break
			}
		}
	}
	sort.Strings(out)
	return out
}

func condOrTrue(cond failure.Cond) failure.Cond {
	if cond == nil {
		return failure.True()
	}
	return cond
}

func (e *Engine) expandLinkVars(cond failure.Cond) failure.Cond {
	if e == nil || e.idx == nil {
		return cond
	}
	return failure.ExpandLinkVars(cond, e.idx.LinksByName)
}

func (e *Engine) linkUpCond(link model.Link) failure.Cond {
	return e.expandLinkVars(failure.LinkVar(link.Name))
}

func isFalseCond(cond failure.Cond) bool {
	return cond != nil && cond.Key() == failure.False().Key()
}

func copyVisited(in map[string]bool) map[string]bool {
	out := make(map[string]bool, len(in)+1)
	for k, v := range in {
		out[k] = v
	}
	return out
}

func ingressInterface(link model.Link, node string) string {
	switch node {
	case link.A:
		return link.AIntf
	case link.B:
		return link.BIntf
	default:
		return ""
	}
}
