package device

import (
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/bgp"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type frrBehavior struct{ BaseBehavior }

func NewFRRBehavior() DeviceBehavior {
	return frrBehavior{NewBaseBehavior(model.KindFRR, bgp.NewFRRDecisionProcess(bgp.DefaultDecisionOptions()))}
}

func (b frrBehavior) RouteInstallableInFIB(device model.Node, installed []domainroute.RIBEntry, route domainroute.RIBEntry) bool {
	if !b.BaseBehavior.RouteInstallableInFIB(device, installed, route) {
		return false
	}
	return !bgp.EquivalentInstalledRoute(b.DecisionProcess(), device, installed, route)
}
