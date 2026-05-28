package device

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/bgp"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type BaseBehavior struct {
	kind     model.DeviceKind
	decision bgp.DecisionProcess
}

func NewGenericBehavior(kind model.DeviceKind) DeviceBehavior {
	return NewBaseBehavior(kind, bgp.DefaultProcess())
}

func NewBaseBehavior(kind model.DeviceKind, decision bgp.DecisionProcess) BaseBehavior {
	return BaseBehavior{kind: kind, decision: decision}
}

func (b BaseBehavior) SelectRoutes(device model.Node, routes []domainroute.RIBEntry) []domainroute.RIBEntry {
	return bgp.SelectRoutes(b.DecisionProcess(), device, routes)
}

func (b BaseBehavior) ExportRoute(from model.Node, to model.Node, session model.BGPNeighbor, route domainroute.RIBEntry) bgp.RouteDecision {
	return bgp.ExportRoute(from, to, session, route)
}

func (b BaseBehavior) ImportRoute(to model.Node, from model.Node, session model.BGPNeighbor, route domainroute.RIBEntry) bgp.RouteDecision {
	return bgp.ImportRoute(to, from, session, route)
}

func (b BaseBehavior) DecisionProcess() bgp.DecisionProcess {
	if b.decision == nil {
		return bgp.DefaultProcess()
	}
	return b.decision
}

func (b BaseBehavior) DecisionOptions() bgp.DecisionOptions {
	if withOptions, ok := b.DecisionProcess().(interface{ Options() bgp.DecisionOptions }); ok {
		return withOptions.Options()
	}
	return bgp.DefaultDecisionOptions()
}
