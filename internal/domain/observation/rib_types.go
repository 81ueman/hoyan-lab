package observation

import (
	"context"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type RIBPath struct {
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

type RIBCollector interface {
	CollectBGPRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error)
	CollectOSPFRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error)
	CollectRouteTableRoutes(ctx context.Context, nodes []model.Node) ([]RIBRoute, error)
}

type RIBParser interface {
	Parse(node string, data []byte) ([]RIBRoute, error)
}

type RIBParserFunc func(node string, data []byte) ([]RIBRoute, error)

func (f RIBParserFunc) Parse(node string, data []byte) ([]RIBRoute, error) {
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
	Paths    []RIBPath
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

type RIBRunner interface {
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
