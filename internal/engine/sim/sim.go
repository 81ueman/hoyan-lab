package sim

import (
	"fmt"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	domainroute "github.com/81ueman/hoyan-lab/internal/domain/routing/route"
	"github.com/81ueman/hoyan-lab/internal/domain/solver"
	"github.com/81ueman/hoyan-lab/internal/engine/controlplane"
	"github.com/81ueman/hoyan-lab/internal/engine/dataplane"
)

type RIBEntry = domainroute.RIBEntry
type FIBEntry = dataplane.FIBEntry
type Path = dataplane.Path
type SymbolicFIBCandidate = dataplane.SymbolicFIBCandidate
type SymbolicPacketBlockedPath = dataplane.SymbolicPacketBlockedPath
type SymbolicPacketPath = dataplane.SymbolicPacketPath
type SymbolicPacketState = dataplane.SymbolicPacketState
type SymbolicReachabilityResult = dataplane.SymbolicReachabilityResult
type SymbolicUnreachableReason = dataplane.SymbolicUnreachableReason
type SymbolicUnreachableReasonKind = dataplane.SymbolicUnreachableReasonKind
type SymbolicRoutePath = dataplane.SymbolicRoutePath
type SymbolicRouteReachabilityResult = dataplane.SymbolicRouteReachabilityResult
type FailureSet = failure.Set
type FailureContext = failure.Context
type FailureSearchOptions = failure.SearchOptions
type Cond = failure.Cond

type Graph struct {
	topo      *model.Topology
	topoIndex *model.TopologyIndex
	rib       domainroute.RIBTable
	fib       dataplane.FIBTable
	solver    solver.Backend
}

type GraphOption func(*Graph)

func WithSolverBackend(backend solver.Backend) GraphOption {
	return func(g *Graph) {
		g.solver = backend
	}
}

func NoFailures() FailureSet { return failure.None() }
func LinkFailures(names ...model.LinkID) FailureSet {
	return failure.Links(names...)
}
func NodeFailures(names ...model.NodeID) FailureSet {
	return failure.Nodes(names...)
}
func NewFailureSet(links []model.LinkID, nodes []model.NodeID) FailureSet {
	return failure.NewSet(links, nodes)
}
func FailureSetFromMap(raw map[string]bool) FailureSet {
	return failure.SetFromMap(raw)
}
func FailureSetFromElements(elements []solver.FailureElement) FailureSet {
	return failure.SetFromElements(elements)
}
func DefaultWANFailureDomain() model.FailureDomain {
	return failure.DefaultWANFailureDomain()
}

func True() Cond  { return failure.True() }
func False() Cond { return failure.False() }
func LinkVar(name string) Cond {
	return failure.LinkVar(name)
}
func NodeVar(name string) Cond {
	return failure.NodeVar(name)
}
func And(cs ...Cond) Cond { return failure.And(cs...) }
func Or(cs ...Cond) Cond  { return failure.Or(cs...) }
func Not(c Cond) Cond     { return failure.Not(c) }

func NewGraph(topo *model.Topology, opts ...GraphOption) *Graph {
	idx, err := model.BuildTopologyIndex(topo)
	if err != nil {
		panic(err)
	}
	g := &Graph{
		topo:      topo,
		topoIndex: idx,
		rib:       domainroute.RIBTable{},
		fib:       dataplane.FIBTable{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(g)
		}
	}
	controlplane.NewEngine(idx, g.rib).Simulate()
	dataplane.NewEngine(idx, g.rib, g.fib).DeriveFIB()
	return g
}

func (g *Graph) RIB(node model.NodeID, prefix model.Prefix) []RIBEntry {
	return g.RIBVRF(node, model.NetworkInstanceDefault, prefix)
}

func (g *Graph) RIBVRF(node model.NodeID, vrf model.NetworkInstanceID, prefix model.Prefix) []RIBEntry {
	return append([]RIBEntry(nil), g.rib[node][model.NormalizeNetworkInstance(string(vrf))][prefix]...)
}

func (g *Graph) RIBTable(node model.NodeID) map[model.Prefix][]RIBEntry {
	return g.RIBTableVRF(node, model.NetworkInstanceDefault)
}

func (g *Graph) RIBTableVRF(node model.NodeID, vrf model.NetworkInstanceID) map[model.Prefix][]RIBEntry {
	out := map[model.Prefix][]RIBEntry{}
	for prefix, routes := range g.rib[node][model.NormalizeNetworkInstance(string(vrf))] {
		out[prefix] = append([]RIBEntry(nil), routes...)
	}
	return out
}

func (g *Graph) RIBTables(node model.NodeID) map[model.NetworkInstanceID]map[model.Prefix][]RIBEntry {
	out := map[model.NetworkInstanceID]map[model.Prefix][]RIBEntry{}
	for vrf, byPrefix := range g.rib[node] {
		out[vrf] = map[model.Prefix][]RIBEntry{}
		for prefix, routes := range byPrefix {
			out[vrf][prefix] = append([]RIBEntry(nil), routes...)
		}
	}
	return out
}

func (g *Graph) FIB(node model.NodeID) []FIBEntry {
	return g.FIBVRF(node, model.NetworkInstanceDefault)
}

func (g *Graph) FIBVRF(node model.NodeID, vrf model.NetworkInstanceID) []FIBEntry {
	return append([]FIBEntry(nil), g.fib[node][model.NormalizeNetworkInstance(string(vrf))]...)
}

func (g *Graph) FIBTables(node model.NodeID) map[model.NetworkInstanceID][]FIBEntry {
	out := map[model.NetworkInstanceID][]FIBEntry{}
	for vrf, entries := range g.fib[node] {
		out[vrf] = append([]FIBEntry(nil), entries...)
	}
	return out
}

func (g *Graph) FailureContext(failures FailureSet) FailureContext {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).FailureContext(failures)
}

func (g *Graph) RouteReachable(from, prefix string, failures FailureSet) (Path, bool) {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).RouteReachable(from, prefix, failures)
}

func (g *Graph) RouteReachableVRF(from, vrf, prefix string, failures FailureSet) (Path, bool) {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).RouteReachableVRF(from, vrf, prefix, failures)
}

func (g *Graph) RouteReachableForPrefixSet(from string, dst model.PrefixSet, failures FailureSet) (Path, bool) {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).RouteReachableForPrefixSet(from, dst, failures)
}

func (g *Graph) RouteReachableForPrefixSetVRF(from, vrf string, dst model.PrefixSet, failures FailureSet) (Path, bool) {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).RouteReachableForPrefixSetVRF(from, vrf, dst, failures)
}

func (g *Graph) PacketReachable(from, to, protocol string, failures FailureSet) (Path, bool, string) {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).PacketReachable(from, to, protocol, failures)
}

func (g *Graph) PacketReachableSpec(from, to string, spec model.PacketSpec, failures FailureSet) (Path, bool, string) {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).PacketReachableSpec(from, to, spec, failures)
}

func (g *Graph) PacketReachableSpecVRF(from, vrf, to string, spec model.PacketSpec, failures FailureSet) (Path, bool, string) {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).PacketReachableSpecVRF(from, vrf, to, spec, failures)
}

func (g *Graph) SymbolicPacketReachability(from, to, protocol string) SymbolicReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicPacketReachability(from, to, protocol)
}

func (g *Graph) SymbolicPacketReachabilitySpec(from, to string, spec model.PacketSpec) SymbolicReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicPacketReachabilitySpec(from, to, spec)
}

func (g *Graph) SymbolicPacketReachabilitySpecVRF(from, vrf, to string, spec model.PacketSpec) SymbolicReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicPacketReachabilitySpecVRF(from, vrf, to, spec)
}

func (g *Graph) SymbolicPacketReachabilityForPrefixSet(from string, dst model.PrefixSet, protocol string) SymbolicReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicPacketReachabilityForPrefixSet(from, dst, protocol)
}

func (g *Graph) SymbolicPacketReachabilityForPrefixSetSpec(from string, dst model.PrefixSet, spec model.PacketSpec) SymbolicReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicPacketReachabilityForPrefixSetSpec(from, dst, spec)
}

func (g *Graph) SymbolicPacketReachabilityForPrefixSetSpecVRF(from, vrf string, dst model.PrefixSet, spec model.PacketSpec) SymbolicReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicPacketReachabilityForPrefixSetSpecVRF(from, vrf, dst, spec)
}

func (g *Graph) SymbolicPacketReachabilityForClass(from string, universe model.PrefixUniverse, classID model.PrefixClassID, protocol string) SymbolicReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicPacketReachabilityForClass(from, universe, classID, protocol)
}

func (g *Graph) SymbolicPacketReachabilityForPacketClass(from string, class model.PacketClass) SymbolicReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicPacketReachabilityForPacketClass(from, class)
}

func (g *Graph) SymbolicRouteReachability(from, prefix string) SymbolicRouteReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicRouteReachability(from, prefix)
}

func (g *Graph) SymbolicRouteReachabilityForPrefixSet(from string, dst model.PrefixSet) SymbolicRouteReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicRouteReachabilityForPrefixSet(from, dst)
}

func (g *Graph) SymbolicRouteReachabilityForPrefixSetVRF(from, vrf string, dst model.PrefixSet) SymbolicRouteReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicRouteReachabilityForPrefixSetVRF(from, vrf, dst)
}

func (g *Graph) SymbolicRouteReachabilityVRF(from, vrf, prefix string) SymbolicRouteReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicRouteReachabilityVRF(from, vrf, prefix)
}

func (g *Graph) SymbolicRouteReachabilityForClass(from string, universe model.PrefixUniverse, classID model.PrefixClassID) SymbolicRouteReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicRouteReachabilityForClass(from, universe, classID)
}

func (g *Graph) FindBreakingFailures(from string, target Target, maxFailures int) ([]string, bool) {
	ans, ok := g.FindBreakingFailuresWithOptions(from, target, FailureSearchOptions{
		IncludeLinks: true,
		MaxFailures:  maxFailures,
	})
	if !ok {
		return nil, false
	}
	out := make([]string, 0, len(ans))
	for _, element := range ans {
		if element.Kind == solver.FailureLink {
			out = append(out, element.Name)
			continue
		}
		out = append(out, element.String())
	}
	return out, true
}

func (g *Graph) FindBreakingFailuresWithOptions(from string, target Target, opts FailureSearchOptions) ([]solver.FailureElement, bool) {
	symbolicTarget, ok := target.(SymbolicTarget)
	if !ok {
		return nil, false
	}
	result, err := g.FindBreakingFailuresSymbolic(from, symbolicTarget, opts)
	if err != nil || !result.Sat {
		return nil, false
	}
	return result.Failures, true
}

type FailureSearchResult struct {
	Sat      bool
	Failures []solver.FailureElement
	Solver   SolverTrace
}

type SolverTrace struct {
	Backend     string `json:"backend,omitempty"`
	Elements    int    `json:"elements"`
	MaxFailures int    `json:"max_failures"`
}

func (g *Graph) FindBreakingFailuresSymbolic(from string, target SymbolicTarget, opts FailureSearchOptions) (FailureSearchResult, error) {
	trace := SolverTrace{
		MaxFailures: opts.MaxFailures,
	}
	if opts.MaxFailures < 0 {
		return FailureSearchResult{Solver: trace}, fmt.Errorf("max failures must be non-negative")
	}
	elements := g.failureElements(opts)
	trace.Elements = len(elements)
	if len(elements) == 0 {
		return FailureSearchResult{Solver: trace}, fmt.Errorf("failure search has no candidate failure elements")
	}
	problem := g.symbolicFailureProblem(from, target, opts, elements)
	ans, err := g.solveSymbolicFailureProblem(problem)
	if ans.Backend != "" {
		trace.Backend = ans.Backend
	}
	result := FailureSearchResult{Sat: ans.Sat, Failures: ans.Failures, Solver: trace}
	if err != nil {
		return result, err
	}
	return result, nil
}

func (g *Graph) FindBreakingFailuresTargetSymbolic(from string, target Target, opts FailureSearchOptions) (FailureSearchResult, error) {
	symbolicTarget, ok := target.(SymbolicTarget)
	if !ok {
		return FailureSearchResult{}, fmt.Errorf("failure search target %T does not implement sim.SymbolicTarget", target)
	}
	return g.FindBreakingFailuresSymbolic(from, symbolicTarget, opts)
}

func (g *Graph) failureElements(opts FailureSearchOptions) []solver.FailureElement {
	if !opts.IncludeLinks && !opts.IncludeNodes {
		return nil
	}
	return failure.SearchElements(g.topo, opts)
}

func (g *Graph) symbolicFailureProblem(from string, target SymbolicTarget, opts FailureSearchOptions, elements []solver.FailureElement) solver.SymbolicFailureProblem {
	result := target.SymbolicResult(g, from)
	return solver.SymbolicFailureProblem{
		Elements:    elements,
		MaxFailures: opts.MaxFailures,
		Goal:        failure.BoolExpr(result.Unreachable),
	}
}

func (g *Graph) solveSymbolicFailureProblem(problem solver.SymbolicFailureProblem) (solver.Answer, error) {
	if g.solver == nil {
		return solver.Answer{}, fmt.Errorf("symbolic failure solver backend is not configured")
	}
	return g.solver.SolveSymbolic(problem)
}

type Target interface {
	Reachable(g *Graph, from string, failures FailureSet) bool
}

type SymbolicTarget interface {
	Target
	SymbolicResult(g *Graph, from string) SymbolicTargetResult
}

type SymbolicTargetResult struct {
	Reachable   failure.Cond
	Unreachable failure.Cond
	Reason      string
}

type PrefixTarget string

func (t PrefixTarget) Reachable(g *Graph, from string, failures FailureSet) bool {
	_, ok := g.RouteReachable(from, string(t), failures)
	return ok
}

func (t PrefixTarget) SymbolicResult(g *Graph, from string) SymbolicTargetResult {
	result := g.SymbolicRouteReachability(from, string(t))
	return routeSymbolicTargetResult(result)
}

type RoutePrefixSetTarget struct {
	Space model.PrefixSet
	VRF   string
}

func (t RoutePrefixSetTarget) Reachable(g *Graph, from string, failures FailureSet) bool {
	_, ok := g.RouteReachableForPrefixSetVRF(from, t.VRF, t.Space, failures)
	return ok
}

func (t RoutePrefixSetTarget) SymbolicResult(g *Graph, from string) SymbolicTargetResult {
	result := g.SymbolicRouteReachabilityForPrefixSetVRF(from, t.VRF, t.Space)
	return routeSymbolicTargetResult(result)
}

type RouteClassTarget struct {
	Universe model.PrefixUniverse
	ClassID  model.PrefixClassID
	VRF      string
}

func (t RouteClassTarget) Reachable(g *Graph, from string, failures FailureSet) bool {
	result := t.symbolicReachability(g, from)
	return result.Reachable.Eval(g.FailureContext(failures))
}

func (t RouteClassTarget) symbolicReachability(g *Graph, from string) SymbolicRouteReachabilityResult {
	return dataplane.NewEngine(g.topoIndex, g.rib, g.fib).SymbolicRouteReachabilityForClassVRF(from, t.VRF, t.Universe, t.ClassID)
}

func (t RouteClassTarget) SymbolicResult(g *Graph, from string) SymbolicTargetResult {
	result := t.symbolicReachability(g, from)
	return routeSymbolicTargetResult(result)
}

type PacketTarget struct {
	To       string
	Protocol string
	DstPort  int
	VRF      string
}

func (t PacketTarget) Reachable(g *Graph, from string, failures FailureSet) bool {
	_, ok, _ := g.PacketReachableSpecVRF(from, t.VRF, t.To, t.Spec(), failures)
	return ok
}

func (t PacketTarget) Spec() model.PacketSpec {
	return model.PacketSpec{Protocol: t.Protocol, DstPort: model.ExactPort(t.DstPort)}
}

func (t PacketTarget) SymbolicResult(g *Graph, from string) SymbolicTargetResult {
	result := g.SymbolicPacketReachabilitySpecVRF(from, t.VRF, t.To, t.Spec())
	return packetSymbolicTargetResult(result)
}

type PacketPrefixTarget struct {
	Prefix   model.Prefix
	Protocol string
	DstPort  int
	VRF      string
}

func (t PacketPrefixTarget) Reachable(g *Graph, from string, failures FailureSet) bool {
	if t.Prefix.IsZero() {
		return false
	}
	_, ok, _ := g.PacketReachableSpecVRF(from, t.VRF, t.Prefix.Addr().String(), t.Spec(), failures)
	return ok
}

func (t PacketPrefixTarget) Spec() model.PacketSpec {
	return model.PacketSpec{Protocol: t.Protocol, DstPort: model.ExactPort(t.DstPort)}
}

func (t PacketPrefixTarget) SymbolicResult(g *Graph, from string) SymbolicTargetResult {
	result := g.SymbolicPacketReachabilityForPrefixSetSpecVRF(from, t.VRF, model.ExactPrefixSet{Prefix: t.Prefix}, t.Spec())
	return packetSymbolicTargetResult(result)
}

type PacketClassTarget struct {
	Universe model.PrefixUniverse
	ClassID  model.PrefixClassID
	Protocol string
	DstPort  int
	VRF      string
}

func (t PacketClassTarget) Reachable(g *Graph, from string, failures FailureSet) bool {
	result := t.symbolicReachability(g, from)
	return result.Reachable.Eval(g.FailureContext(failures))
}

func (t PacketClassTarget) Spec() model.PacketSpec {
	return model.PacketSpec{Protocol: t.Protocol, DstPort: model.ExactPort(t.DstPort)}
}

func (t PacketClassTarget) symbolicReachability(g *Graph, from string) SymbolicReachabilityResult {
	for _, class := range t.Universe.Classes {
		if class.ID == t.ClassID {
			return g.SymbolicPacketReachabilityForPrefixSetSpecVRF(from, t.VRF, class.Space, t.Spec())
		}
	}
	return SymbolicReachabilityResult{
		Reachable:   False(),
		Unreachable: True(),
		Reason:      "prefix class not found",
	}
}

func (t PacketClassTarget) SymbolicResult(g *Graph, from string) SymbolicTargetResult {
	result := t.symbolicReachability(g, from)
	return packetSymbolicTargetResult(result)
}

type HeaderPacketClassTarget struct {
	Class model.PacketClass
}

func (t HeaderPacketClassTarget) Reachable(g *Graph, from string, failures FailureSet) bool {
	result := t.symbolicReachability(g, from)
	return result.Reachable.Eval(g.FailureContext(failures))
}

func (t HeaderPacketClassTarget) Spec() model.PacketSpec {
	return t.Class.Spec()
}

func (t HeaderPacketClassTarget) symbolicReachability(g *Graph, from string) SymbolicReachabilityResult {
	return g.SymbolicPacketReachabilityForPacketClass(from, t.Class)
}

func (t HeaderPacketClassTarget) SymbolicResult(g *Graph, from string) SymbolicTargetResult {
	result := t.symbolicReachability(g, from)
	return packetSymbolicTargetResult(result)
}

func packetSymbolicTargetResult(result SymbolicReachabilityResult) SymbolicTargetResult {
	return SymbolicTargetResult{
		Reachable:   result.Reachable,
		Unreachable: result.Unreachable,
		Reason:      result.Reason,
	}
}

func routeSymbolicTargetResult(result SymbolicRouteReachabilityResult) SymbolicTargetResult {
	return SymbolicTargetResult{
		Reachable:   result.Reachable,
		Unreachable: result.Unreachable,
		Reason:      result.Reason,
	}
}

func FormatPath(p Path) string {
	if len(p.Nodes) == 0 {
		return ""
	}
	return fmt.Sprintf("%s cost=%d", strings.Join(p.Nodes, " -> "), p.Cost)
}
