package controlplane

import (
	"sort"

	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

type ceosBehavior struct{ baseDeviceBehavior }

func NewCEOSBehavior() DeviceBehavior {
	return ceosBehavior{baseDeviceBehavior{kind: topology.KindCEOS, decision: ceosDecisionProcess{defaultBGPDecisionProcess{options: DefaultBGPDecisionOptions()}}}}
}

type ceosDecisionProcess struct{ defaultBGPDecisionProcess }

func (d ceosDecisionProcess) Less(receiver topology.Node, a, b RIBEntry) bool {
	return d.defaultBGPDecisionProcess.Less(receiver, a, b)
}

func (ceosDecisionProcess) Equivalent(receiver topology.Node, a, b RIBEntry) bool {
	return false
}

func (b ceosBehavior) SelectRoutes(device topology.Node, routes []RIBEntry) []RIBEntry {
	out := append([]RIBEntry(nil), routes...)
	sort.Slice(out, func(i, j int) bool {
		return b.DecisionProcess().Less(device, out[i], out[j])
	})
	return out
}

func (b ceosBehavior) RouteValidForRIB(device topology.Node, route RIBEntry) bool {
	route = route.Normalize()
	if !b.baseDeviceBehavior.RouteValidForRIB(device, route) {
		return false
	}
	if route.SourceKind != topology.RouteSourceBGP {
		return true
	}
	return route.ForwardingNextHop.Node == "" || route.ForwardingNextHop.Node == route.Provenance.FromNode
}
