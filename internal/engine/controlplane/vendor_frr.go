package controlplane

import (
	"strings"

	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

type frrBehavior struct{ baseDeviceBehavior }

func NewFRRBehavior() DeviceBehavior {
	return frrBehavior{baseDeviceBehavior{kind: topology.KindFRR, decision: frrDecisionProcess{defaultBGPDecisionProcess{options: DefaultBGPDecisionOptions()}}}}
}

type frrDecisionProcess struct{ defaultBGPDecisionProcess }

func (d frrDecisionProcess) Less(receiver topology.Node, a, b RIBEntry) bool {
	a = a.Normalize()
	b = b.Normalize()
	if a.SourceKind == topology.RouteSourceOSPF && b.SourceKind == topology.RouteSourceOSPF {
		if ospfRouteTypeRank(a.RouteSource.OSPFRouteType) != ospfRouteTypeRank(b.RouteSource.OSPFRouteType) {
			return ospfRouteTypeRank(a.RouteSource.OSPFRouteType) < ospfRouteTypeRank(b.RouteSource.OSPFRouteType)
		}
		if a.RouteSource.Metric != b.RouteSource.Metric {
			return a.RouteSource.Metric < b.RouteSource.Metric
		}
		if len(a.Provenance.PathLinks) != len(b.Provenance.PathLinks) {
			return len(a.Provenance.PathLinks) < len(b.Provenance.PathLinks)
		}
		return strings.Join(a.Provenance.PathNodes, ",") > strings.Join(b.Provenance.PathNodes, ",")
	}
	if a.Attrs.LocalPref != b.Attrs.LocalPref {
		return a.Attrs.LocalPref > b.Attrs.LocalPref
	}
	if a.Provenance.OriginNode == receiver.Name || b.Provenance.OriginNode == receiver.Name {
		return a.Provenance.OriginNode == receiver.Name
	}
	if len(a.Attrs.ASPath) != len(b.Attrs.ASPath) {
		return len(a.Attrs.ASPath) < len(b.Attrs.ASPath)
	}
	if originCodeRank(a.Attrs.OriginCode) != originCodeRank(b.Attrs.OriginCode) {
		return originCodeRank(a.Attrs.OriginCode) < originCodeRank(b.Attrs.OriginCode)
	}
	if d.shouldCompareMED(a, b) && a.Attrs.MED != b.Attrs.MED {
		return a.Attrs.MED < b.Attrs.MED
	}
	aExternal := !a.Attrs.LearnedIBGP
	bExternal := !b.Attrs.LearnedIBGP
	if aExternal != bExternal {
		return aExternal
	}
	if len(a.Provenance.PathLinks) != len(b.Provenance.PathLinks) {
		return len(a.Provenance.PathLinks) < len(b.Provenance.PathLinks)
	}
	return strings.Join(a.Provenance.PathNodes, ",") > strings.Join(b.Provenance.PathNodes, ",")
}

func (d frrDecisionProcess) Equivalent(receiver topology.Node, a, b RIBEntry) bool {
	return d.defaultBGPDecisionProcess.Equivalent(receiver, a, b)
}

func (b frrBehavior) RouteInstallableInFIB(device topology.Node, installed []RIBEntry, route RIBEntry) bool {
	if !b.baseDeviceBehavior.RouteInstallableInFIB(device, installed, route) {
		return false
	}
	return !EquivalentInstalledRoute(b.DecisionProcess(), device, installed, route)
}
