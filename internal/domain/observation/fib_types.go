package observation

import (
	"fmt"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type Options struct {
	AllowUnsupported bool
	UnresolvedPolicy UnresolvedPolicy
	AFI              model.AFI
}

type UnresolvedPolicy string

const (
	UnresolvedPolicyWarn   UnresolvedPolicy = "warn"
	UnresolvedPolicyFail   UnresolvedPolicy = "fail"
	UnresolvedPolicyIgnore UnresolvedPolicy = "ignore"
)

func (p UnresolvedPolicy) normalized() UnresolvedPolicy {
	switch p {
	case "", UnresolvedPolicyWarn:
		return UnresolvedPolicyWarn
	case UnresolvedPolicyFail:
		return UnresolvedPolicyFail
	case UnresolvedPolicyIgnore:
		return UnresolvedPolicyIgnore
	default:
		return p
	}
}

func ParseUnresolvedPolicy(policy string) (UnresolvedPolicy, bool) {
	p := UnresolvedPolicy(policy).normalized()
	switch p {
	case UnresolvedPolicyWarn, UnresolvedPolicyFail, UnresolvedPolicyIgnore:
		return p, true
	default:
		return p, false
	}
}

type FilterResult struct {
	FIBs       []FIB
	Unresolved []UnresolvedRoute
}

type UnresolvedRoute struct {
	RouteKey string
	Node     string
	VRF      string
	AFI      string
	Prefix   string
	Protocol string
	NextHops []UnresolvedNextHop
	Reason   string
}

type UnresolvedNextHop struct {
	Address   string
	Interface string
	Reason    string
}

type RouteDiff struct {
	RouteKey string
}

type NextHopDiff struct {
	RouteKey   string
	NextHopKey string
}

type FIBAttributeMismatch struct {
	RouteKey string
	Field    string
	Expected any
	Actual   any
}

type DuplicateRouteConflict struct {
	RouteKey string     `json:"route_key"`
	Side     string     `json:"side"`
	Reason   string     `json:"reason"`
	Routes   []FIBEntry `json:"routes"`
}

type Result struct {
	OK                      bool
	UnsupportedNodes        []string
	UnresolvedRoutes        []UnresolvedRoute
	DuplicateRouteConflicts []DuplicateRouteConflict
	MissingRoutes           []string
	UnexpectedRoutes        []string
	MissingNextHops         []NextHopDiff
	UnexpectedNextHops      []NextHopDiff
	Mismatched              []FIBAttributeMismatch
}

type UnsupportedNodesError struct {
	Nodes []string
}

func (e UnsupportedNodesError) Error() string {
	return fmt.Sprintf("unsupported live FIB collector for node(s): %s", strings.Join(e.Nodes, ", "))
}
