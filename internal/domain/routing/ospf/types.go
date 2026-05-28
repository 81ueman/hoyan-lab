package ospf

import (
	"net/netip"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/routing/route"
)

type InterfaceState struct {
	Node            string
	Name            string
	NetworkInstance model.NetworkInstanceID
	Prefix          netip.Prefix
	Area            string
	Cost            int
	Passive         bool
	NetworkType     string
}

type Advertisement struct {
	Node            string
	NetworkInstance model.NetworkInstanceID
	Prefix          model.Prefix
	Cost            int
	Area            string
	External        bool
	MetricType      int
	ExternalArea    string
	DefaultArea     string
	Source          route.RIBEntry
}

type Path struct {
	Cost  int
	Nodes []string
	Links []string
	Areas []string
	Cond  failure.Cond
}

type Adjacency struct {
	From string
	To   string
	Link string
	Area string
	Cost int
}

type SPFNode struct {
	Cost         int
	Predecessors []SPFPredecessor
}

type SPFPredecessor struct {
	Node string
	Link string
	Area string
}

type SPFQueueItem struct {
	Node string
	Cost int
}

type AdjacencyFilter func(InterfaceState, InterfaceState) (string, bool)
