package deviceadapter

import (
	"github.com/81ueman/hoyan-lab/internal/domain/device"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/bgp"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type ceosBehavior struct{ device.BaseBehavior }

func NewCEOSBehavior() device.DeviceBehavior {
	return ceosBehavior{device.NewBaseBehavior(model.KindCEOS, bgp.NewNeverEquivalentDecisionProcess(bgp.DefaultDecisionOptions()))}
}

func (b ceosBehavior) SelectRoutes(device model.Node, routes []domainroute.RIBEntry) []domainroute.RIBEntry {
	return bgp.SelectRoutes(b.DecisionProcess(), device, routes)
}

func (b ceosBehavior) RouteValidForRIB(device model.Node, route domainroute.RIBEntry) bool {
	route = route.Normalize()
	if !b.BaseBehavior.RouteValidForRIB(device, route) {
		return false
	}
	if route.SourceKind != model.RouteSourceBGP {
		return true
	}
	return route.ForwardingNextHop.Node == "" || route.ForwardingNextHop.Node == route.Provenance.FromNode
}
