package controlplane

import "github.com/81ueman/hoyan-lab/internal/core/topology"

type srlinuxBehavior struct{ baseDeviceBehavior }

func NewSRLinuxBehavior() DeviceBehavior {
	return srlinuxBehavior{baseDeviceBehavior{kind: topology.KindSRLinux, decision: srlinuxDecisionProcess{defaultBGPDecisionProcess{options: DefaultBGPDecisionOptions()}}}}
}

type srlinuxDecisionProcess struct{ defaultBGPDecisionProcess }

func (d srlinuxDecisionProcess) Less(receiver topology.Node, a, b RIBEntry) bool {
	return d.defaultBGPDecisionProcess.Less(receiver, a, b)
}

func (srlinuxDecisionProcess) Equivalent(receiver topology.Node, a, b RIBEntry) bool {
	return false
}

func (b srlinuxBehavior) ImportRoute(to topology.Node, from topology.Node, session topology.BGPNeighbor, route RIBEntry) BGPRouteDecision {
	route = route.Normalize()
	if containsASN(route.Attrs.ASPath, to.ASN) {
		route.Attrs.Invalid = true
		return BGPRouteDecision{Route: route, Accept: true, Reason: "as loop"}
	}
	return b.baseDeviceBehavior.ImportRoute(to, from, session, route)
}
