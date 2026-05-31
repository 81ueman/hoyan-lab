package livesnapshot

import (
	"context"
	"path/filepath"
	"sort"
	"time"

	"github.com/81ueman/hoyan-lab/internal/adapter/configparse"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
	snapshotdomain "github.com/81ueman/hoyan-lab/internal/domain/snapshot"
	fibcompare "github.com/81ueman/hoyan-lab/internal/usecase/fib"
	ribcompare "github.com/81ueman/hoyan-lab/internal/usecase/rib"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

const Version = snapshotdomain.Version

const (
	HashPolicyWarn   = snapshotdomain.HashPolicyWarn
	HashPolicyFail   = snapshotdomain.HashPolicyFail
	HashPolicyIgnore = snapshotdomain.HashPolicyIgnore
)

type Snapshot = snapshotdomain.Snapshot
type NodeSnapshot = snapshotdomain.NodeSnapshot
type HashPolicy = snapshotdomain.HashPolicy
type HashMismatch = snapshotdomain.HashMismatch
type HashCheckResult = snapshotdomain.HashCheckResult
type InputHashSet = snapshotdomain.InputHashSet

type HashProvider interface {
	InputHashes(topologyPath string) (snapshotdomain.InputHashSet, error)
}

type CommitProvider interface {
	Commit() string
}

type Option func(*Usecase)

type Usecase struct {
	ribCollector   observation.RIBCollector
	fibCollector   observation.FIBCollector
	hashProvider   HashProvider
	commitProvider CommitProvider
	now            func() time.Time
}

func New(ribCollector observation.RIBCollector, fibCollector observation.FIBCollector, opts ...Option) Usecase {
	u := Usecase{
		ribCollector: ribCollector,
		fibCollector: fibCollector,
		now:          func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(&u)
	}
	return u
}

func WithHashProvider(provider HashProvider) Option {
	return func(u *Usecase) {
		u.hashProvider = provider
	}
}

func WithCommitProvider(provider CommitProvider) Option {
	return func(u *Usecase) {
		u.commitProvider = provider
	}
}

func WithClock(now func() time.Time) Option {
	return func(u *Usecase) {
		u.now = now
	}
}

func ParseHashPolicy(raw string) (HashPolicy, bool) {
	return snapshotdomain.ParseHashPolicy(raw)
}

func (u Usecase) Build(ctx context.Context, topologyPath, labName string, fibOpts observation.Options) (*Snapshot, error) {
	topo, warnings, err := topology.LoadTopologyWithOptions(topologyPath, topology.LoadOptions{CollectWarnings: true})
	if err != nil {
		return nil, err
	}
	if labName == "" {
		labName = topo.Name
	}
	ribUsecase := ribcompare.New(u.ribCollector)
	fibUsecase := fibcompare.New(u.fibCollector)
	routes, err := ribUsecase.Collect(ctx, topo.Nodes)
	if err != nil {
		return nil, err
	}
	bgp := observation.BGPOnly(routes)
	routeTable := observation.FilterRIBRoutes(routes, func(route observation.RIBRoute) bool {
		return route.Common.Protocol != model.RouteSourceBGP
	})
	fib, err := fibUsecase.Collect(ctx, topo.Nodes, fibOpts)
	if err != nil {
		return nil, err
	}
	fibResult := observation.AnalyzeComparableRoutes(topo, fib, fibOpts)

	hashes, err := u.inputHashes(topologyPath)
	if err != nil {
		return nil, err
	}
	snap := &Snapshot{
		Version:      Version,
		Lab:          labName,
		TopologyPath: filepath.ToSlash(topologyPath),
		TopologyHash: hashes.TopologyHash,
		ConfigHashes: hashes.ConfigHashes,
		GitCommit:    u.commit(),
		CollectedAt:  u.now(),
		Nodes:        map[string]NodeSnapshot{},
		Warnings:     warningStrings(warnings),
	}
	for _, node := range topo.Nodes {
		snap.Nodes[node.Name] = NodeSnapshot{Kind: node.Kind}
	}
	addRIBRoutes(snap.Nodes, bgp, func(ns NodeSnapshot, routes []observation.RIBRoute) NodeSnapshot {
		ns.BGPRIB = routes
		return ns
	})
	addRIBRoutes(snap.Nodes, routeTable, func(ns NodeSnapshot, routes []observation.RIBRoute) NodeSnapshot {
		ns.RouteTable = routes
		return ns
	})
	addFIBRoutes(snap.Nodes, fib)
	addUnresolvedFIB(snap.Nodes, fibResult.Unresolved)
	return snap, nil
}

func BGPRoutes(snap *Snapshot) []observation.RIBRoute {
	var out []observation.RIBRoute
	for _, name := range sortedNodeNames(snap.Nodes) {
		out = append(out, snap.Nodes[name].BGPRIB...)
	}
	return out
}

func AllRIBRoutes(snap *Snapshot) []observation.RIBRoute {
	var out []observation.RIBRoute
	for _, name := range sortedNodeNames(snap.Nodes) {
		out = append(out, snap.Nodes[name].BGPRIB...)
		out = append(out, snap.Nodes[name].RouteTable...)
	}
	return out
}

func FIBRoutes(snap *Snapshot) []observation.FIBEntry {
	var out []observation.FIBEntry
	for _, fib := range FIBs(snap) {
		out = append(out, fib.Entries...)
	}
	return out
}

func FIBs(snap *Snapshot) []observation.FIB {
	var out []observation.FIB
	for _, name := range sortedNodeNames(snap.Nodes) {
		out = append(out, snap.Nodes[name].FIB...)
	}
	return out
}

func UnresolvedFIB(snap *Snapshot) []observation.UnresolvedRoute {
	var out []observation.UnresolvedRoute
	for _, name := range sortedNodeNames(snap.Nodes) {
		out = append(out, snap.Nodes[name].UnresolvedFIB...)
	}
	return out
}

func (u Usecase) inputHashes(topologyPath string) (snapshotdomain.InputHashSet, error) {
	if u.hashProvider == nil {
		return snapshotdomain.InputHashSet{ConfigHashes: map[string]string{}}, nil
	}
	return u.hashProvider.InputHashes(topologyPath)
}

func (u Usecase) commit() string {
	if u.commitProvider == nil {
		return ""
	}
	return u.commitProvider.Commit()
}

func addRIBRoutes(nodes map[string]NodeSnapshot, routes []observation.RIBRoute, update func(NodeSnapshot, []observation.RIBRoute) NodeSnapshot) {
	for name := range nodes {
		ns := nodes[name]
		nodes[name] = update(ns, routes)
	}
}

func addFIBRoutes(nodes map[string]NodeSnapshot, fibs []observation.FIB) {
	for _, fib := range fibs {
		ns := nodes[string(fib.Node)]
		ns.FIB = append(ns.FIB, fib)
		nodes[string(fib.Node)] = ns
	}
}

func addUnresolvedFIB(nodes map[string]NodeSnapshot, routes []observation.UnresolvedRoute) {
	for _, route := range routes {
		ns := nodes[route.Node]
		ns.UnresolvedFIB = append(ns.UnresolvedFIB, route)
		nodes[route.Node] = ns
	}
}

func sortedNodeNames(nodes map[string]NodeSnapshot) []string {
	names := make([]string, 0, len(nodes))
	for name := range nodes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func warningStrings(warnings []configparse.UnsupportedStatement) []string {
	out := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		out = append(out, warning.String())
	}
	return out
}
