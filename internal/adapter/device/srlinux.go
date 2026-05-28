package deviceadapter

import (
	"github.com/81ueman/hoyan-lab/internal/domain/device"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/bgp"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type srlinuxBehavior struct{ device.BaseBehavior }

func NewSRLinuxBehavior() device.DeviceBehavior {
	return srlinuxBehavior{device.NewBaseBehavior(model.KindSRLinux, bgp.NewNeverEquivalentDecisionProcess(bgp.DefaultDecisionOptions()))}
}

func (b srlinuxBehavior) ImportRoute(to model.Node, from model.Node, session model.BGPNeighbor, route domainroute.RIBEntry) bgp.RouteDecision {
	return bgp.ImportRouteKeepASLoop(to, from, session, route)
}
