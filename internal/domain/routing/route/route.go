package route

import (
	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

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

type BGPOriginCode string

const (
	BGPOriginIGP        BGPOriginCode = "igp"
	BGPOriginEGP        BGPOriginCode = "egp"
	BGPOriginIncomplete BGPOriginCode = "incomplete"
)

type BGPAttributes struct {
	ASPath      []uint32
	Communities []string
	OriginCode  BGPOriginCode
	LocalPref   int
	MED         int
	LearnedIBGP bool
	Invalid     bool
}

type Provenance struct {
	OriginNode string
	FromNode   string
	PathNodes  []string
	PathLinks  []string
}

type NextHop struct {
	Node string
	Addr string
}

func (h NextHop) Valid() bool {
	return h.Node != "" || h.Addr != ""
}

func (r RIBEntry) Normalize() RIBEntry {
	if r.Attrs.OriginCode == "" {
		r.Attrs.OriginCode = BGPOriginIGP
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
