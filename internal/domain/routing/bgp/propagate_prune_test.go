package bgp

import (
	"testing"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

func TestWalkRoutePruningSkipsRecursionWhenConditionIsImpossible(t *testing.T) {
	var addRIBCalls int

	ctx := PropagationContext{
		Adjacencies: func(id model.NodeID) []model.AdjEdge {
			return []model.AdjEdge{
				{To: "router-b", Link: model.Link{Name: "a-b", A: "router-a", B: "router-b"}},
			}
		},
		Node: func(s string) (model.Node, bool) {
			if s == "router-a" || s == "router-b" {
				return model.Node{Name: s, Kind: model.KindFRR}, true
			}
			return model.Node{}, false
		},
		BGPSession: func(a, b string, vrf model.NetworkInstanceID) (model.BGPNeighbor, bool) {
			return model.BGPNeighbor{PeerNode: b, Activated: true}, true
		},
		ExportRoute: func(from, to model.Node, session model.BGPNeighbor, entry route.RIBEntry) RouteDecision {
			return RouteDecision{Accept: true, Route: entry}
		},
		ImportRoute: func(to, from model.Node, session model.BGPNeighbor, entry route.RIBEntry) RouteDecision {
			return RouteDecision{Accept: true, Route: entry}
		},
		ApplyRoutePolicy: func(node model.Node, peer, policy string, entry route.RIBEntry) RouteDecision {
			return RouteDecision{Accept: true, Route: entry}
		},
		AddRIB: func(node string, prefix model.Prefix, entry route.RIBEntry) {
			addRIBCalls++
		},
		ControlEgress:  func(from, to string, entry route.RIBEntry) bool { return true },
		ControlIngress: func(from, to string, entry route.RIBEntry) bool { return true },
		EligibleForAdvertisement: func(node model.Node, entry route.RIBEntry) bool {
			return true
		},
		Prune: func(cond failure.Cond) bool {
			// Prune if condition is impossible
			return cond.Key() == failure.False().Key()
		},
	}

	// Route with impossibility: NodeVar(router-a) && Not(NodeVar(router-a))
	impossibleCond := failure.And(failure.NodeVar("router-a"), failure.Not(failure.NodeVar("router-a")))
	route := route.RIBEntry{
		NLRI:        route.NLRI{Prefix: model.MustPrefix("10.0.0.0/24")},
		Attrs:       route.BGPAttributes{OriginCode: model.BGPOriginIGP},
		Provenance:  route.Provenance{OriginNode: "router-a", PathNodes: []string{"router-a"}},
		SourceKind:  model.RouteSourceBGP,
		RouteSource: model.ConfiguredRoute{Node: "router-a", NetworkInstance: model.NetworkInstanceDefault, AFI: model.AFIIPv4, Kind: model.RouteSourceBGP},
		Condition:   impossibleCond,
		BaseCond:    impossibleCond,
	}
	WalkRoute(ctx, route)

	// Should add the route to RIB on router-b, but NOT propagate further
	if addRIBCalls != 1 {
		t.Fatalf("expected 1 RIB add (router-b), got %d (pruning should prevent further propagation)", addRIBCalls)
	}
}

func TestWalkRoutePruningSkipsRecursionWhenNegatedLinkCountExceedsMax(t *testing.T) {
	var addRIBCalls int

	ctx := PropagationContext{
		Adjacencies: func(id model.NodeID) []model.AdjEdge {
			return []model.AdjEdge{
				{To: "router-b", Link: model.Link{Name: "a-b", A: "router-a", B: "router-b"}},
				{To: "router-c", Link: model.Link{Name: "a-c", A: "router-a", B: "router-c"}},
			}
		},
		Node: func(s string) (model.Node, bool) {
			if s == "router-a" || s == "router-b" || s == "router-c" {
				return model.Node{Name: s, Kind: model.KindFRR}, true
			}
			return model.Node{}, false
		},
		BGPSession: func(a, b string, vrf model.NetworkInstanceID) (model.BGPNeighbor, bool) {
			return model.BGPNeighbor{PeerNode: b, Activated: true}, true
		},
		ExportRoute: func(from, to model.Node, session model.BGPNeighbor, entry route.RIBEntry) RouteDecision {
			return RouteDecision{Accept: true, Route: entry}
		},
		ImportRoute: func(to, from model.Node, session model.BGPNeighbor, entry route.RIBEntry) RouteDecision {
			return RouteDecision{Accept: true, Route: entry}
		},
		ApplyRoutePolicy: func(node model.Node, peer, policy string, entry route.RIBEntry) RouteDecision {
			return RouteDecision{Accept: true, Route: entry}
		},
		AddRIB: func(node string, prefix model.Prefix, entry route.RIBEntry) {
			addRIBCalls++
		},
		ControlEgress:  func(from, to string, entry route.RIBEntry) bool { return true },
		ControlIngress: func(from, to string, entry route.RIBEntry) bool { return true },
		EligibleForAdvertisement: func(node model.Node, entry route.RIBEntry) bool {
			return true
		},
		Prune: func(cond failure.Cond) bool {
			// Prune if condition has > 1 negated link variable
			return failure.NegatedLinkCount(cond) > 1
		},
	}

	// Route with condition that has 2 negated link vars already
	cond := failure.And(failure.Not(failure.LinkVar("x")), failure.Not(failure.LinkVar("y")))
	route := route.RIBEntry{
		NLRI:        route.NLRI{Prefix: model.MustPrefix("10.0.0.0/24")},
		Attrs:       route.BGPAttributes{OriginCode: model.BGPOriginIGP},
		Provenance:  route.Provenance{OriginNode: "router-a", PathNodes: []string{"router-a"}},
		SourceKind:  model.RouteSourceBGP,
		RouteSource: model.ConfiguredRoute{Node: "router-a", NetworkInstance: model.NetworkInstanceDefault, AFI: model.AFIIPv4, Kind: model.RouteSourceBGP},
		Condition:   cond,
		BaseCond:    cond,
	}
	WalkRoute(ctx, route)
	// Should add to RIB for each adjacency but not propagate further
	if addRIBCalls != 2 {
		t.Fatalf("expected 2 RIB adds (b, c), got %d", addRIBCalls)
	}
}

func TestWalkRouteNoPruningWhenFuncIsNil(t *testing.T) {
	var addRIBCalls int

	ctx := PropagationContext{
		Adjacencies: func(id model.NodeID) []model.AdjEdge {
			return []model.AdjEdge{
				{To: "router-b", Link: model.Link{Name: "a-b", A: "router-a", B: "router-b"}},
			}
		},
		Node: func(s string) (model.Node, bool) {
			if s == "router-a" || s == "router-b" {
				return model.Node{Name: s, Kind: model.KindFRR}, true
			}
			return model.Node{}, false
		},
		BGPSession: func(a, b string, vrf model.NetworkInstanceID) (model.BGPNeighbor, bool) {
			return model.BGPNeighbor{PeerNode: b, Activated: true}, true
		},
		ExportRoute: func(from, to model.Node, session model.BGPNeighbor, entry route.RIBEntry) RouteDecision {
			return RouteDecision{Accept: true, Route: entry}
		},
		ImportRoute: func(to, from model.Node, session model.BGPNeighbor, entry route.RIBEntry) RouteDecision {
			return RouteDecision{Accept: true, Route: entry}
		},
		ApplyRoutePolicy: func(node model.Node, peer, policy string, entry route.RIBEntry) RouteDecision {
			return RouteDecision{Accept: true, Route: entry}
		},
		AddRIB: func(node string, prefix model.Prefix, entry route.RIBEntry) {
			addRIBCalls++
		},
		ControlEgress:  func(from, to string, entry route.RIBEntry) bool { return true },
		ControlIngress: func(from, to string, entry route.RIBEntry) bool { return true },
		EligibleForAdvertisement: func(node model.Node, entry route.RIBEntry) bool {
			return true
		},
		// Prune is nil -> no pruning
	}

	cond := failure.And(failure.Not(failure.LinkVar("x")), failure.Not(failure.LinkVar("y")))
	route := route.RIBEntry{
		NLRI:        route.NLRI{Prefix: model.MustPrefix("10.0.0.0/24")},
		Attrs:       route.BGPAttributes{OriginCode: model.BGPOriginIGP},
		Provenance:  route.Provenance{OriginNode: "router-a", PathNodes: []string{"router-a"}},
		SourceKind:  model.RouteSourceBGP,
		RouteSource: model.ConfiguredRoute{Node: "router-a", NetworkInstance: model.NetworkInstanceDefault, AFI: model.AFIIPv4, Kind: model.RouteSourceBGP},
		Condition:   cond,
		BaseCond:    cond,
	}
	WalkRoute(ctx, route)
	if addRIBCalls < 1 {
		t.Fatalf("expected at least 1 RIB add without pruning, got %d", addRIBCalls)
	}
}
