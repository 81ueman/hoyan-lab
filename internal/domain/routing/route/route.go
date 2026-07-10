package route

import (
	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type RIBTable map[model.NodeID]map[model.NetworkInstanceID]map[model.Prefix][]RIBEntry

// RIBEntry is a simulated control-plane route candidate.
//
// It is not the observation model used for live RIB snapshots. A single
// RIBEntry represents one candidate path with symbolic failure conditions;
// observation.RIBRoute groups visible paths by prefix and protocol for
// collection, snapshots, and comparison.
type RIBEntry struct {
	NLRI                  NLRI
	Attrs                 BGPAttributes
	Provenance            Provenance
	ForwardingNextHop     NextHop
	SourceKind            model.RouteSourceKind
	RouteSource           model.ConfiguredRoute
	AggregateContributors []string
	BaseCond              failure.Cond
	Condition             failure.Cond
	SelectedCond          failure.Cond
}

type NLRI struct {
	Prefix model.Prefix
}

type BGPAttributes struct {
	ASPath       []uint32
	Communities  []string
	OriginCode   model.BGPOriginCode
	OriginatorID string
	ClusterList  []string
	LocalPref    int
	MED          int
	Weight       int
	LearnedIBGP  bool
	Invalid      bool
}

type Provenance struct {
	OriginNode string
	FromNode   string
	PathNodes  []string
	PathLinks  []string
}

// NextHop is the control-plane forwarding next-hop selected for a modeled route.
// It identifies either a topology node or a resolved address; observed interface,
// weight, and resolution metadata belong to observation.NextHop.
type NextHop struct {
	Node string
	Addr string
}

func (h NextHop) Valid() bool {
	return h.Node != "" || h.Addr != ""
}

func (r RIBEntry) Normalize() RIBEntry {
	if r.Attrs.OriginCode == "" {
		r.Attrs.OriginCode = model.BGPOriginIGP
	} else {
		r.Attrs.OriginCode = model.NormalizeBGPOriginCode(r.Attrs.OriginCode)
	}
	if r.SourceKind == "" {
		r.SourceKind = model.RouteSourceBGP
	}
	if r.RouteSource.Kind == "" {
		r.RouteSource.Kind = r.SourceKind
	}
	if r.RouteSource.NetworkInstance == "" {
		r.RouteSource.NetworkInstance = model.NetworkInstanceDefault
	}
	if r.RouteSource.Prefix.IsZero() {
		r.RouteSource.Prefix = r.NLRI.Prefix
	}
	return r
}
