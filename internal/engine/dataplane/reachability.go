package dataplane

import (
	"github.com/81ueman/hoyan-lab/internal/core/netaddr"
	"github.com/81ueman/hoyan-lab/internal/core/predicate"
	"net/netip"

	"github.com/81ueman/hoyan-lab/internal/core/failure"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
	"github.com/81ueman/hoyan-lab/internal/engine/controlplane"
)

func (e *Engine) RouteReachable(from, prefix string, failures failure.Set) (Path, bool) {
	return e.RouteReachableVRF(from, string(topology.NetworkInstanceDefault), prefix, failures)
}

func (e *Engine) RouteReachableVRF(from, vrf, prefix string, failures failure.Set) (Path, bool) {
	pfx, err := netaddr.ParsePrefix(prefix)
	if err != nil {
		return Path{}, false
	}
	ctx := e.FailureContext(failures)
	if ctx.NodeFailed(topology.NodeID(from)) {
		return Path{}, false
	}
	var best *controlplane.RIBEntry
	for _, r := range e.rib[from][string(topology.NormalizeNetworkInstance(vrf))][pfx.String()] {
		r = r.Normalize()
		if r.SelectedCond != nil && r.SelectedCond.Eval(ctx) {
			cp := r
			best = &cp
			break
		}
	}
	if best == nil {
		return Path{}, false
	}
	return routePath(e.idx, *best), true
}

func (e *Engine) RouteReachableForPrefixSet(from string, dst predicate.PrefixSet, failures failure.Set) (Path, bool) {
	return e.RouteReachableForPrefixSetVRF(from, string(topology.NetworkInstanceDefault), dst, failures)
}

func (e *Engine) RouteReachableForPrefixSetVRF(from, vrf string, dst predicate.PrefixSet, failures failure.Set) (Path, bool) {
	result := e.SymbolicRouteReachabilityForPrefixSetVRF(from, vrf, dst)
	ctx := e.FailureContext(failures)
	for _, path := range result.Paths {
		if path.Cond.Eval(ctx) {
			return path.Path, true
		}
	}
	return Path{}, false
}

func (e *Engine) PacketReachable(from, to, protocol string, failures failure.Set) (Path, bool, string) {
	return e.PacketReachableSpec(from, to, predicate.PacketSpec{Protocol: protocol}, failures)
}

func (e *Engine) PacketReachableSpec(from, to string, spec predicate.PacketSpec, failures failure.Set) (Path, bool, string) {
	return e.PacketReachableSpecVRF(from, string(topology.NetworkInstanceDefault), to, spec, failures)
}

func (e *Engine) PacketReachableSpecVRF(from, vrf, to string, spec predicate.PacketSpec, failures failure.Set) (Path, bool, string) {
	ctx := e.FailureContext(failures)
	dstNode, dstPrefix, ok := e.originForIPVRF(to, vrf)
	if !ok {
		return Path{}, false, "destination prefix not advertised"
	}
	if ctx.NodeFailed(topology.NodeID(from)) {
		return Path{}, false, "source node is down"
	}
	if ctx.NodeFailed(topology.NodeID(dstNode)) {
		return Path{}, false, "destination node is down"
	}
	return e.packetReachableFrom(packetReachableState{
		current:   from,
		vrf:       string(topology.NormalizeNetworkInstance(vrf)),
		to:        to,
		spec:      spec,
		dstPrefix: dstPrefix.NetIP(),
		ctx:       ctx,
		visited:   map[string]bool{},
		full:      Path{Nodes: []string{from}},
	})
}

type packetReachableState struct {
	current          string
	vrf              string
	to               string
	spec             predicate.PacketSpec
	dstPrefix        netip.Prefix
	ingressInterface string
	ctx              failure.Context
	visited          map[string]bool
	full             Path
}

func (e *Engine) packetReachableFrom(state packetReachableState) (Path, bool, string) {
	if state.ctx.NodeFailed(topology.NodeID(state.current)) {
		return state.full, false, "current node is down"
	}
	if state.visited[state.current] {
		return state.full, false, "forwarding loop"
	}
	if e.originatesVRF(state.current, state.vrf, state.dstPrefix) {
		return state.full, true, ""
	}
	currentNode, _ := e.idx.Node(state.current)
	packetSpec := state.spec.WithNormalizedPorts()
	packetSpec.DstSet = predicate.ExactPrefixSet{Prefix: netaddr.PrefixFromNetIP(state.dstPrefix)}
	packetSpec.IngressInterface = state.ingressInterface
	packet := controlplane.PacketMessage{Node: state.current, Spec: packetSpec}
	if decision := e.dataACLDecision(currentNode, packet, "ingress"); decision.Denied {
		return state.full, false, decision.Reason
	}
	candidates := e.SymbolicLookupFIBVRF(state.current, state.vrf, state.to)
	if len(candidates) == 0 {
		return state.full, false, "no forwarding route"
	}
	nextVisited := copyVisited(state.visited)
	nextVisited[state.current] = true
	var firstReason string
	for _, candidate := range candidates {
		if !candidate.Cond.Eval(state.ctx) {
			continue
		}
		rule := candidate.Entry
		nextFull, ok, reason := e.tryPacketCandidate(state, nextVisited, packet, currentNode, rule)
		if ok {
			return nextFull, true, ""
		}
		if firstReason == "" {
			firstReason = reason
		}
	}
	if firstReason == "" {
		return state.full, false, "no forwarding route"
	}
	return state.full, false, firstReason
}

func (e *Engine) tryPacketCandidate(state packetReachableState, nextVisited map[string]bool, packet controlplane.PacketMessage, currentNode topology.Node, rule FIBEntry) (Path, bool, string) {
	if rule.Discard {
		return state.full, false, "discard route selected"
	}
	switch rule.effectiveResolutionStatus() {
	case NextHopResolutionUnresolvedRecursive:
		return state.full, false, "recursive next-hop unresolved"
	case NextHopResolutionManagementFallback:
		return state.full, false, "next-hop resolved via management interface"
	}
	if rule.NextHop == "" {
		return state.full, false, "selected route has no next-hop"
	}
	if state.ctx.NodeFailed(topology.NodeID(rule.NextHop)) {
		return state.full, false, "next-hop node is down"
	}
	link, ok := e.idx.LinkBetween(state.current, rule.NextHop)
	if !ok {
		return state.full, false, "next-hop is not adjacent"
	}
	if state.ctx.LinkFailed(topology.LinkID(link.Name)) {
		return state.full, false, "next-hop link is down"
	}
	packet.Spec.EgressInterface = interfaceOnLink(link, state.current)
	if decision := e.dataACLDecision(currentNode, packet, "egress"); decision.Denied {
		return state.full, false, decision.Reason
	}
	nextFull := Path{
		Nodes: append(append([]string(nil), state.full.Nodes...), rule.NextHop),
		Links: append(append([]string(nil), state.full.Links...), link.Name),
		Cost:  state.full.Cost + link.Cost,
	}
	return e.packetReachableFrom(packetReachableState{
		current:          rule.NextHop,
		vrf:              state.vrf,
		to:               state.to,
		spec:             state.spec,
		dstPrefix:        state.dstPrefix,
		ingressInterface: interfaceOnLink(link, rule.NextHop),
		ctx:              state.ctx,
		visited:          nextVisited,
		full:             nextFull,
	})
}

func (e *Engine) dataACLDecision(node topology.Node, packet controlplane.PacketMessage, stage string) controlplane.PolicyDecision {
	behavior := controlplane.BehaviorFor(node.Kind)
	if e != nil {
		return behavior.EvaluateDataACL(node, packet, stage, e.routing.ACLs, e.routing.ACLBindings)
	}
	return controlplane.PolicyDecision{}
}

func (e *Engine) FailureContext(failures failure.Set) failure.Context {
	if failures.Links == nil {
		failures.Links = map[topology.LinkID]bool{}
	}
	if failures.Nodes == nil {
		failures.Nodes = map[topology.NodeID]bool{}
	}
	return failure.Context{Failures: failures, LinksByName: e.idx.LinksByName}
}

func (e *Engine) originatesVRF(node, vrf string, prefix netip.Prefix) bool {
	n, ok := e.idx.Node(node)
	if !ok {
		return false
	}
	for _, raw := range e.nodePrefixesVRF(n, vrf) {
		if raw.NetIP() == prefix {
			return true
		}
	}
	return false
}

func (e *Engine) originatesPrefixSetVRF(node, vrf string, dst predicate.PrefixSet) bool {
	n, ok := e.idx.Node(node)
	if !ok || dst == nil {
		return false
	}
	for _, raw := range e.nodePrefixesVRF(n, vrf) {
		if !raw.IsZero() && predicate.AddressSpaceOverlaps(predicate.ExactPrefixSet{Prefix: raw}, dst) {
			return true
		}
	}
	return false
}

func (e *Engine) hasOriginForPrefixSetVRF(vrf string, dst predicate.PrefixSet) bool {
	if e == nil || e.idx == nil || dst == nil {
		return false
	}
	for _, node := range e.idx.NodesByName {
		for _, raw := range e.nodePrefixesVRF(node, vrf) {
			if !raw.IsZero() && predicate.AddressSpaceOverlaps(predicate.ExactPrefixSet{Prefix: raw}, dst) {
				return true
			}
		}
	}
	return false
}

func (e *Engine) originForIPVRF(addr, vrf string) (string, netaddr.Prefix, bool) {
	if e == nil || e.idx == nil {
		return "", netaddr.Prefix{}, false
	}
	ip, err := netip.ParseAddr(addr)
	if err != nil {
		return "", netaddr.Prefix{}, false
	}
	for _, node := range e.idx.NodesByName {
		for _, prefix := range e.nodePrefixesVRF(node, vrf) {
			if prefix.NetIP().Contains(ip) {
				return node.Name, prefix, true
			}
		}
	}
	return "", netaddr.Prefix{}, false
}

func (e *Engine) nodePrefixesVRF(node topology.Node, vrf string) []netaddr.Prefix {
	want := topology.NormalizeNetworkInstance(vrf)
	var out []netaddr.Prefix
	if want == topology.NetworkInstanceDefault {
		out = append(out, node.Prefixes...)
	}
	for _, iface := range node.Interfaces {
		if topology.NormalizeNetworkInstance(string(iface.VRF)) != want {
			continue
		}
		pfx, err := netip.ParsePrefix(iface.Address)
		if err != nil {
			continue
		}
		out = append(out, netaddr.PrefixFromNetIP(pfx.Masked()))
	}
	return out
}

func reverse[T any](xs []T) {
	for i, j := 0, len(xs)-1; i < j; i, j = i+1, j-1 {
		xs[i], xs[j] = xs[j], xs[i]
	}
}

func interfaceOnLink(link topology.Link, node string) string {
	switch node {
	case link.A:
		return link.AIntf
	case link.B:
		return link.BIntf
	default:
		return ""
	}
}
