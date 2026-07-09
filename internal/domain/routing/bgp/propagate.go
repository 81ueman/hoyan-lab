package bgp

import (
	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

// PropagationContext provides the engine-specific operations needed for
// BGP route propagation through the adjacency graph. The domain-level
// WalkRoute function uses these callbacks instead of depending directly
// on the engine, topology index, or device behavior registry.
type PropagationContext struct {
	Adjacencies               func(model.NodeID) []model.AdjEdge
	Node                      func(string) (model.Node, bool)
	BGPSession                func(string, string, model.NetworkInstanceID) (model.BGPNeighbor, bool)
	ExportRoute               func(model.Node, model.Node, model.BGPNeighbor, route.RIBEntry) RouteDecision
	ImportRoute               func(model.Node, model.Node, model.BGPNeighbor, route.RIBEntry) RouteDecision
	ApplyRoutePolicy          func(model.Node, string, string, route.RIBEntry) RouteDecision
	AddRIB                    func(string, model.Prefix, route.RIBEntry)
	ControlEgress             func(string, string, route.RIBEntry) bool
	ControlIngress            func(string, string, route.RIBEntry) bool
	EligibleForAdvertisement  func(model.Node, route.RIBEntry) bool
	ApplyAggregateSuppression func(model.Node, route.RIBEntry) route.RIBEntry
}

// WalkRoute propagates a BGP route through all neighbor adjacencies.
// It handles session lookup, export/import policy application, aggregate
// suppression, loop detection, and route attribute updates. Newly
// installed routes are recursively propagated.
func WalkRoute(ctx PropagationContext, route route.RIBEntry) {
	route = route.Normalize()
	current := route.Provenance.PathNodes[len(route.Provenance.PathNodes)-1]
	curNode, ok := ctx.Node(current)
	if !ok {
		return
	}
	for _, adj := range ctx.Adjacencies(model.NodeID(current)) {
		next := string(adj.To)

		session, ok := ctx.BGPSession(current, next, route.RouteSource.NetworkInstance)
		if !ok {
			continue
		}
		nextNode, ok := ctx.Node(next)
		if !ok {
			continue
		}

		if !ctx.ControlEgress(current, next, route) {
			continue
		}

		routeForExport := route
		if ctx.ApplyAggregateSuppression != nil {
			routeForExport = ctx.ApplyAggregateSuppression(curNode, route)
		}

		exported := ctx.ExportRoute(curNode, nextNode, session, routeForExport)
		if !exported.Accept {
			continue
		}

		exportPolicy := ctx.ApplyRoutePolicy(curNode, next, session.ExportPolicy, exported.Route)
		if !exportPolicy.Accept {
			continue
		}
		exported.Route = exportPolicy.Route

		if !ctx.ControlIngress(current, next, exported.Route) {
			continue
		}

		receiverSession, ok := ctx.BGPSession(next, current, route.RouteSource.NetworkInstance)
		if !ok {
			continue
		}

		imported := ctx.ImportRoute(nextNode, curNode, receiverSession, exported.Route)
		if !imported.Accept {
			continue
		}

		importPolicy := ctx.ApplyRoutePolicy(nextNode, current, receiverSession.ImportPolicy, imported.Route)
		if !importPolicy.Accept {
			continue
		}
		imported.Route = importPolicy.Route

		// Loop detection: skip if the next node is already in the path,
		// unless the route is marked invalid (e.g., SR Linux retains an
		// AS-path loop as a poisoned route with Invalid=true).
		if containsString(route.Provenance.PathNodes, next) && !imported.Route.Attrs.Invalid {
			continue
		}

		nextLinks := append(append([]string(nil), imported.Route.Provenance.PathLinks...), adj.Link.Name)
		nextNodes := append(append([]string(nil), imported.Route.Provenance.PathNodes...), next)
		nextCond := failure.And(imported.Route.Condition, failure.LinkVar(adj.Link.Name), failure.NodeVar(next))

		entry := imported.Route
		entry.Provenance.FromNode = current
		entry.Provenance.PathNodes = append([]string(nil), nextNodes...)
		entry.Provenance.PathLinks = append([]string(nil), nextLinks...)
		entry.BaseCond = nextCond
		entry.Condition = nextCond
		entry.Attrs.LocalPref = DefaultLocalPref(entry.Attrs.LocalPref)
		entry = entry.Normalize()

		ctx.AddRIB(next, entry.NLRI.Prefix, entry)

		if !ctx.EligibleForAdvertisement(nextNode, entry) {
			continue
		}

		WalkRoute(ctx, entry)
	}
}

func containsString(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
