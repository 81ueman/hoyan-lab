package controlplane

import (
	"strings"

	"github.com/81ueman/hoyan-lab/internal/model"
)

type BGPDecisionProcess interface {
	Less(receiver model.Node, a, b RIBEntry) bool
	Equivalent(receiver model.Node, a, b RIBEntry) bool
}

// BGPDecisionOptions documents vendor bestpath knobs that the model can expose
// explicitly. Not every option is implemented by every decision process yet;
// unsupported knobs are intentionally visible so modeled/live RIB differences
// can point at a known approximation instead of an implicit gap.
type BGPDecisionOptions struct {
	// AlwaysCompareMED preserves the current Hoyan approximation: MED is
	// compared even when candidate paths were learned from different neighbor
	// ASNs. Set false to compare MED only within the same neighboring AS.
	AlwaysCompareMED bool
	// DeterministicMED is documented but not implemented. The model currently
	// sorts all candidate routes in one pass instead of grouping by neighbor AS.
	DeterministicMED bool
	// CompareRouterID is documented but not implemented because modeled routes
	// do not carry router-id or originator-id attributes yet.
	CompareRouterID bool
	// PreferLowerRouterID documents the common router-id tie-break direction
	// for vendors that enable CompareRouterID. It is unused until router-id
	// comparison is implemented.
	PreferLowerRouterID bool
	// Multipath is documented but not implemented here. ECMP/FIB equivalence is
	// tracked separately from single-best BGP route ordering.
	Multipath bool
}

func DefaultBGPDecisionOptions() BGPDecisionOptions {
	return BGPDecisionOptions{
		PreferLowerRouterID: true,
	}
}

type defaultBGPDecisionProcess struct {
	options BGPDecisionOptions
}

func DefaultBGPDecisionProcess() BGPDecisionProcess {
	return NewBGPDecisionProcess(DefaultBGPDecisionOptions())
}

func NewBGPDecisionProcess(options BGPDecisionOptions) BGPDecisionProcess {
	return defaultBGPDecisionProcess{options: options}
}

func (d defaultBGPDecisionProcess) Options() BGPDecisionOptions {
	return d.options
}

func (d defaultBGPDecisionProcess) Less(receiver model.Node, a, b RIBEntry) bool {
	a = a.Normalize()
	b = b.Normalize()
	if a.SourceKind == model.RouteSourceOSPF && b.SourceKind == model.RouteSourceOSPF {
		if ospfRouteTypeRank(a.RouteSource.OSPFRouteType) != ospfRouteTypeRank(b.RouteSource.OSPFRouteType) {
			return ospfRouteTypeRank(a.RouteSource.OSPFRouteType) < ospfRouteTypeRank(b.RouteSource.OSPFRouteType)
		}
		if a.RouteSource.Metric != b.RouteSource.Metric {
			return a.RouteSource.Metric < b.RouteSource.Metric
		}
		if len(a.Provenance.PathLinks) != len(b.Provenance.PathLinks) {
			return len(a.Provenance.PathLinks) < len(b.Provenance.PathLinks)
		}
		return strings.Join(a.Provenance.PathNodes, ",") < strings.Join(b.Provenance.PathNodes, ",")
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
	return strings.Join(a.Provenance.PathNodes, ",") < strings.Join(b.Provenance.PathNodes, ",")
}

func (d defaultBGPDecisionProcess) Equivalent(receiver model.Node, a, b RIBEntry) bool {
	a = a.Normalize()
	b = b.Normalize()
	if a.SourceKind == model.RouteSourceOSPF || b.SourceKind == model.RouteSourceOSPF {
		return a.SourceKind == b.SourceKind && a.RouteSource.OSPFRouteType == b.RouteSource.OSPFRouteType && a.RouteSource.Metric == b.RouteSource.Metric
	}
	if a.Attrs.LocalPref != b.Attrs.LocalPref {
		return false
	}
	if (a.Provenance.OriginNode == receiver.Name) != (b.Provenance.OriginNode == receiver.Name) {
		return false
	}
	if len(a.Attrs.ASPath) != len(b.Attrs.ASPath) {
		return false
	}
	if originCodeRank(a.Attrs.OriginCode) != originCodeRank(b.Attrs.OriginCode) {
		return false
	}
	if d.shouldCompareMED(a, b) && a.Attrs.MED != b.Attrs.MED {
		return false
	}
	return a.Attrs.LearnedIBGP == b.Attrs.LearnedIBGP
}

func ospfRouteTypeRank(routeType string) int {
	switch routeType {
	case ospfRouteTypeIntraArea, "":
		return 0
	case ospfRouteTypeInterArea:
		return 1
	case ospfRouteTypeExternal1:
		return 2
	case ospfRouteTypeExternal2:
		return 3
	default:
		return 4
	}
}

func (d defaultBGPDecisionProcess) shouldCompareMED(a, b RIBEntry) bool {
	if d.options.AlwaysCompareMED {
		return true
	}
	return neighboringAS(a) == neighboringAS(b)
}

func neighboringAS(route RIBEntry) uint32 {
	if len(route.Attrs.ASPath) == 0 {
		return 0
	}
	return route.Attrs.ASPath[0]
}

func originCodeRank(origin BGPOriginCode) int {
	switch origin {
	case BGPOriginIGP:
		return 0
	case BGPOriginEGP:
		return 1
	case BGPOriginIncomplete:
		return 2
	default:
		return 3
	}
}
