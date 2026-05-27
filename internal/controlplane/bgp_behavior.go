package controlplane

import (
	"sort"

	"github.com/81ueman/hoyan-lab/internal/model"
)

type BGPRouteDecision struct {
	Route  RIBEntry
	Accept bool
	Reason string
}

type BGPBehavior interface {
	SelectRoutes(device model.Node, routes []RIBEntry) []RIBEntry
	ExportRoute(from model.Node, to model.Node, session model.BGPNeighbor, route RIBEntry) BGPRouteDecision
	ImportRoute(to model.Node, from model.Node, session model.BGPNeighbor, route RIBEntry) BGPRouteDecision
	DecisionOptions() BGPDecisionOptions
	DecisionProcess() BGPDecisionProcess
}

type baseDeviceBehavior struct {
	kind     model.DeviceKind
	decision BGPDecisionProcess
}

func NewGenericBehavior(kind model.DeviceKind) DeviceBehavior {
	return baseDeviceBehavior{kind: kind, decision: DefaultBGPDecisionProcess()}
}

func (b baseDeviceBehavior) SelectRoutes(device model.Node, routes []RIBEntry) []RIBEntry {
	out := append([]RIBEntry(nil), routes...)
	sort.Slice(out, func(i, j int) bool {
		return b.DecisionProcess().Less(device, out[i], out[j])
	})
	return out
}

func (b baseDeviceBehavior) ExportRoute(from model.Node, to model.Node, session model.BGPNeighbor, route RIBEntry) BGPRouteDecision {
	route = route.Normalize()
	isIBGP := from.ASN == to.ASN
	if isIBGP && route.Attrs.LearnedIBGP {
		return BGPRouteDecision{Route: route, Accept: false, Reason: "ibgp readvertisement"}
	}

	out := route
	out.Attrs.ASPath = append([]uint32(nil), route.Attrs.ASPath...)
	out.Attrs.Communities = append([]string(nil), route.Attrs.Communities...)
	if !isIBGP {
		out.Attrs.ASPath = prependASN(from.ASN, out.Attrs.ASPath)
	}
	if !isIBGP || session.NextHopSelf || !out.ForwardingNextHop.Valid() {
		out.ForwardingNextHop.Node = from.Name
		out.ForwardingNextHop.Addr = ""
	}
	out.Attrs.LearnedIBGP = isIBGP

	return BGPRouteDecision{Route: out.Normalize(), Accept: true}
}

func (b baseDeviceBehavior) ImportRoute(to model.Node, from model.Node, session model.BGPNeighbor, route RIBEntry) BGPRouteDecision {
	route = route.Normalize()
	if containsASN(route.Attrs.ASPath, to.ASN) {
		return BGPRouteDecision{Route: route, Accept: false, Reason: "as loop"}
	}
	out := route
	if from.ASN != to.ASN {
		out.Attrs.LocalPref = 0
	}
	return BGPRouteDecision{Route: out.Normalize(), Accept: true}
}

func (b baseDeviceBehavior) DecisionProcess() BGPDecisionProcess {
	if b.decision == nil {
		return DefaultBGPDecisionProcess()
	}
	return b.decision
}

func (b baseDeviceBehavior) DecisionOptions() BGPDecisionOptions {
	if withOptions, ok := b.DecisionProcess().(interface{ Options() BGPDecisionOptions }); ok {
		return withOptions.Options()
	}
	return DefaultBGPDecisionOptions()
}
