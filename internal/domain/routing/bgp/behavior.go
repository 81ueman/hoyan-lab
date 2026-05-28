package bgp

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type Behavior interface {
	SelectRoutes(device model.Node, routes []route.RIBEntry) []route.RIBEntry
	ExportRoute(from model.Node, to model.Node, session model.BGPNeighbor, route route.RIBEntry) RouteDecision
	ImportRoute(to model.Node, from model.Node, session model.BGPNeighbor, route route.RIBEntry) RouteDecision
	DecisionOptions() DecisionOptions
	DecisionProcess() DecisionProcess
}
