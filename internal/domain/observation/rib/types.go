package rib

import (
	"context"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type NormalizedRoute struct {
	Node            string
	NetworkInstance string
	AFI             string
	Prefix          string
	Protocol        string
	ConnectedClass  model.ConnectedRouteClass
	Paths           []NormalizedPath
}

type NormalizedPath struct {
	Best             bool
	Valid            bool
	NextHop          string
	ASPath           []uint32
	Origin           string
	LocalPref        int
	MED              int
	Weight           int
	Communities      []string
	LargeCommunities []string
	OriginatorID     string
	ClusterList      []string
	Peer             string
	PeerAS           uint32
}

type Collector interface {
	CollectBGPRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error)
	CollectOSPFRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error)
	CollectRouteTableRoutes(ctx context.Context, nodes []model.Node) ([]NormalizedRoute, error)
}

type Parser interface {
	Parse(node string, data []byte) ([]NormalizedRoute, error)
}

type ParserFunc func(node string, data []byte) ([]NormalizedRoute, error)

func (f ParserFunc) Parse(node string, data []byte) ([]NormalizedRoute, error) {
	return f(node, data)
}

type CompareOptions struct {
	CompareBest             bool
	CompareValid            bool
	CompareNextHop          bool
	CompareASPath           bool
	CompareOrigin           bool
	CompareLocalPref        bool
	CompareMED              bool
	CompareWeight           bool
	CompareCommunities      bool
	CompareLargeCommunities bool
	CompareOriginatorID     bool
	CompareClusterList      bool
	ComparePeer             bool
	ComparePeerAS           bool
	AllowExtraPrefixes      bool
	AllowExtraPaths         bool
}

type PathDiff struct {
	RouteKey string
	PathKey  string
}

type AttributeMismatch struct {
	RouteKey string
	PathKey  string
	Field    string
	Expected any
	Actual   any
}

type DuplicatePathConflict struct {
	RouteKey string
	PathKey  string
	Side     string
	Paths    []NormalizedPath
}

type CompareResult struct {
	OK                     bool
	MissingPrefixes        []string
	UnexpectedPrefixes     []string
	MissingPaths           []PathDiff
	UnexpectedPaths        []PathDiff
	Mismatched             []AttributeMismatch
	DuplicatePathConflicts []DuplicatePathConflict
}

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

func DefaultCompareOptions() CompareOptions {
	return CompareOptions{
		CompareBest:      true,
		CompareValid:     true,
		CompareNextHop:   true,
		CompareASPath:    true,
		CompareOrigin:    true,
		CompareLocalPref: true,
		CompareMED:       true,
	}
}
