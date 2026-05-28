package sim

import (
	"github.com/81ueman/hoyan-lab/internal/core/netaddr"
	"fmt"
	"github.com/81ueman/hoyan-lab/internal/core/predicate"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/config/routing"
	"github.com/81ueman/hoyan-lab/internal/core/failure"
	"github.com/81ueman/hoyan-lab/internal/core/solver"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
	"github.com/81ueman/hoyan-lab/internal/engine/controlplane"
	"github.com/81ueman/hoyan-lab/internal/engine/dataplane"
	"github.com/81ueman/hoyan-lab/internal/engine/space"
)

type RIBEntry = controlplane.RIBEntry
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
type ControlMessage = controlplane.ControlMessage
type PacketMessage = controlplane.PacketMessage
type BGPRouteDecision = controlplane.BGPRouteDecision
type BGPBehavior = controlplane.BGPBehavior
type BGPDecisionProcess = controlplane.BGPDecisionProcess
type BGPDecisionOptions = controlplane.BGPDecisionOptions
type DeviceBehavior = controlplane.DeviceBehavior

type Graph struct {
	topo      *topology.Topology
	routing   routing.TopologyRouting
	topoIndex *topology.TopologyIndex
	rib       map[string]map[string]map[string][]RIBEntry
	fib       map[string]map[string][]FIBEntry
}

func NoFailures() FailureSet { return failure.None() }
func LinkFailures(names ...topology.LinkID) FailureSet {
	return failure.Links(names...)
}
func NodeFailures(names ...topology.NodeID) FailureSet {
	return failure.Nodes(names...)
}
func NewFailureSet(links []topology.LinkID, nodes []topology.NodeID) FailureSet {
	return failure.NewSet(links, nodes)
}
func FailureSetFromMap(raw map[string]bool) FailureSet {
	return failure.SetFromMap(raw)
}
func FailureSetFromElements(elements []solver.FailureElement) FailureSet {
	return failure.SetFromElements(elements)
}
func DefaultWANFailureDomain() topology.FailureDomain {
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

func RegisterBehavior(kind topology.DeviceKind, behavior DeviceBehavior) func() {
	return controlplane.RegisterBehavior(kind, behavior)
}
func BehaviorFor(kind topology.DeviceKind) DeviceBehavior {
	return controlplane.BehaviorFor(kind)
}
func NewGenericBehavior(kind topology.DeviceKind) DeviceBehavior {
	return controlplane.NewGenericBehavior(kind)
}
func NewFRRBehavior() DeviceBehavior {
	return controlplane.NewFRRBehavior()
}
func NewCEOSBehavior() DeviceBehavior {
	return controlplane.NewCEOSBehavior()
}
func NewSRLinuxBehavior() DeviceBehavior {
	return controlplane.NewSRLinuxBehavior()
}
func DefaultBGPDecisionProcess() BGPDecisionProcess {
	return controlplane.DefaultBGPDecisionProcess()
}
func DefaultBGPDecisionOptions() BGPDecisionOptions {
	return controlplane.DefaultBGPDecisionOptions()
}
func NewBGPDecisionProcess(options BGPDecisionOptions) BGPDecisionProcess {
	return controlplane.NewBGPDecisionProcess(options)
}

func NewGraphWithRouting(topo *topology.Topology, routes routing.TopologyRouting) *Graph {
	idx, err := topology.BuildTopologyIndex(topo)
	if err != nil {
		panic(err)
	}
	g := &Graph{
		topo:      topo,
		routing:   routes,
		topoIndex: idx,
		rib:       map[string]map[string]map[string][]RIBEntry{},
		fib:       map[string]map[string][]FIBEntry{},
	}
	controlplane.NewEngineWithRouting(idx, routes, g.rib).Simulate()
	g.dataplaneEngine().DeriveFIB()
	return g
}

func (g *Graph) dataplaneEngine() *dataplane.Engine {
	return dataplane.NewEngineWithRouting(g.topoIndex, g.routing, g.rib, g.fib)
}

func (g *Graph) RIB(node, prefix string) []RIBEntry {
	return g.RIBVRF(node, string(topology.NetworkInstanceDefault), prefix)
}

func (g *Graph) RIBVRF(node, vrf, prefix string) []RIBEntry {
	return append([]RIBEntry(nil), g.rib[node][string(topology.NormalizeNetworkInstance(vrf))][prefix]...)
}

func (g *Graph) RIBTable(node string) map[string][]RIBEntry {
	return g.RIBTableVRF(node, string(topology.NetworkInstanceDefault))
}

func (g *Graph) RIBTableVRF(node, vrf string) map[string][]RIBEntry {
	out := map[string][]RIBEntry{}
	for prefix, routes := range g.rib[node][string(topology.NormalizeNetworkInstance(vrf))] {
		out[prefix] = append([]RIBEntry(nil), routes...)
	}
	return out
}

func (g *Graph) RIBTables(node string) map[string]map[string][]RIBEntry {
	out := map[string]map[string][]RIBEntry{}
	for vrf, byPrefix := range g.rib[node] {
		out[vrf] = map[string][]RIBEntry{}
		for prefix, routes := range byPrefix {
			out[vrf][prefix] = append([]RIBEntry(nil), routes...)
		}
	}
	return out
}

func (g *Graph) FIB(node string) []FIBEntry {
	return g.FIBVRF(node, string(topology.NetworkInstanceDefault))
}

func (g *Graph) FIBVRF(node, vrf string) []FIBEntry {
	return append([]FIBEntry(nil), g.fib[node][string(topology.NormalizeNetworkInstance(vrf))]...)
}

func (g *Graph) FIBTables(node string) map[string][]FIBEntry {
	out := map[string][]FIBEntry{}
	for vrf, entries := range g.fib[node] {
		out[vrf] = append([]FIBEntry(nil), entries...)
	}
	return out
}

func (g *Graph) FailureContext(failures FailureSet) FailureContext {
	return g.dataplaneEngine().FailureContext(failures)
}

func (g *Graph) RouteReachable(from, prefix string, failures FailureSet) (Path, bool) {
	return g.dataplaneEngine().RouteReachable(from, prefix, failures)
}

func (g *Graph) RouteReachableVRF(from, vrf, prefix string, failures FailureSet) (Path, bool) {
	return g.dataplaneEngine().RouteReachableVRF(from, vrf, prefix, failures)
}

func (g *Graph) RouteReachableForPrefixSet(from string, dst predicate.PrefixSet, failures FailureSet) (Path, bool) {
	return g.dataplaneEngine().RouteReachableForPrefixSet(from, dst, failures)
}

func (g *Graph) RouteReachableForPrefixSetVRF(from, vrf string, dst predicate.PrefixSet, failures FailureSet) (Path, bool) {
	return g.dataplaneEngine().RouteReachableForPrefixSetVRF(from, vrf, dst, failures)
}

func (g *Graph) PacketReachable(from, to, protocol string, failures FailureSet) (Path, bool, string) {
	return g.dataplaneEngine().PacketReachable(from, to, protocol, failures)
}

func (g *Graph) PacketReachableSpec(from, to string, spec predicate.PacketSpec, failures FailureSet) (Path, bool, string) {
	return g.dataplaneEngine().PacketReachableSpec(from, to, spec, failures)
}

func (g *Graph) PacketReachableSpecVRF(from, vrf, to string, spec predicate.PacketSpec, failures FailureSet) (Path, bool, string) {
	return g.dataplaneEngine().PacketReachableSpecVRF(from, vrf, to, spec, failures)
}

func (g *Graph) SymbolicPacketReachability(from, to, protocol string) SymbolicReachabilityResult {
	return g.dataplaneEngine().SymbolicPacketReachability(from, to, protocol)
}

func (g *Graph) SymbolicPacketReachabilitySpec(from, to string, spec predicate.PacketSpec) SymbolicReachabilityResult {
	return g.dataplaneEngine().SymbolicPacketReachabilitySpec(from, to, spec)
}

func (g *Graph) SymbolicPacketReachabilitySpecVRF(from, vrf, to string, spec predicate.PacketSpec) SymbolicReachabilityResult {
	return g.dataplaneEngine().SymbolicPacketReachabilitySpecVRF(from, vrf, to, spec)
}

func (g *Graph) SymbolicPacketReachabilityForPrefixSet(from string, dst predicate.PrefixSet, protocol string) SymbolicReachabilityResult {
	return g.dataplaneEngine().SymbolicPacketReachabilityForPrefixSet(from, dst, protocol)
}

func (g *Graph) SymbolicPacketReachabilityForPrefixSetSpec(from string, dst predicate.PrefixSet, spec predicate.PacketSpec) SymbolicReachabilityResult {
	return g.dataplaneEngine().SymbolicPacketReachabilityForPrefixSetSpec(from, dst, spec)
}

func (g *Graph) SymbolicPacketReachabilityForPrefixSetSpecVRF(from, vrf string, dst predicate.PrefixSet, spec predicate.PacketSpec) SymbolicReachabilityResult {
	return g.dataplaneEngine().SymbolicPacketReachabilityForPrefixSetSpecVRF(from, vrf, dst, spec)
}

func (g *Graph) SymbolicPacketReachabilityForClass(from string, universe space.PrefixUniverse, classID space.PrefixClassID, protocol string) SymbolicReachabilityResult {
	return g.dataplaneEngine().SymbolicPacketReachabilityForClass(from, universe, classID, protocol)
}

func (g *Graph) SymbolicPacketReachabilityForPacketClass(from string, class space.PacketClass) SymbolicReachabilityResult {
	return g.dataplaneEngine().SymbolicPacketReachabilityForPacketClass(from, class)
}

func (g *Graph) SymbolicRouteReachability(from, prefix string) SymbolicRouteReachabilityResult {
	return g.dataplaneEngine().SymbolicRouteReachability(from, prefix)
}

func (g *Graph) SymbolicRouteReachabilityForPrefixSet(from string, dst predicate.PrefixSet) SymbolicRouteReachabilityResult {
	return g.dataplaneEngine().SymbolicRouteReachabilityForPrefixSet(from, dst)
}

func (g *Graph) SymbolicRouteReachabilityForPrefixSetVRF(from, vrf string, dst predicate.PrefixSet) SymbolicRouteReachabilityResult {
	return g.dataplaneEngine().SymbolicRouteReachabilityForPrefixSetVRF(from, vrf, dst)
}

func (g *Graph) SymbolicRouteReachabilityVRF(from, vrf, prefix string) SymbolicRouteReachabilityResult {
	return g.dataplaneEngine().SymbolicRouteReachabilityVRF(from, vrf, prefix)
}

func (g *Graph) SymbolicRouteReachabilityForClass(from string, universe space.PrefixUniverse, classID space.PrefixClassID) SymbolicRouteReachabilityResult {
	return g.dataplaneEngine().SymbolicRouteReachabilityForClass(from, universe, classID)
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
	ans, err := solveSymbolicFailureProblem(problem)
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

func solveSymbolicFailureProblem(problem solver.SymbolicFailureProblem) (solver.Answer, error) {
	ans, err := solver.DefaultBackend().SolveSymbolic(problem)
	if err != nil {
		return ans, err
	}
	return ans, nil
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
	Space predicate.PrefixSet
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
	Universe space.PrefixUniverse
	ClassID  space.PrefixClassID
	VRF      string
}

func (t RouteClassTarget) Reachable(g *Graph, from string, failures FailureSet) bool {
	result := t.symbolicReachability(g, from)
	return result.Reachable.Eval(g.FailureContext(failures))
}

func (t RouteClassTarget) symbolicReachability(g *Graph, from string) SymbolicRouteReachabilityResult {
	return g.dataplaneEngine().SymbolicRouteReachabilityForClassVRF(from, t.VRF, t.Universe, t.ClassID)
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

func (t PacketTarget) Spec() predicate.PacketSpec {
	return predicate.PacketSpec{Protocol: t.Protocol, DstPort: predicate.ExactPort(t.DstPort)}
}

func (t PacketTarget) SymbolicResult(g *Graph, from string) SymbolicTargetResult {
	result := g.SymbolicPacketReachabilitySpecVRF(from, t.VRF, t.To, t.Spec())
	return packetSymbolicTargetResult(result)
}

type PacketPrefixTarget struct {
	Prefix   netaddr.Prefix
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

func (t PacketPrefixTarget) Spec() predicate.PacketSpec {
	return predicate.PacketSpec{Protocol: t.Protocol, DstPort: predicate.ExactPort(t.DstPort)}
}

func (t PacketPrefixTarget) SymbolicResult(g *Graph, from string) SymbolicTargetResult {
	result := g.SymbolicPacketReachabilityForPrefixSetSpecVRF(from, t.VRF, predicate.ExactPrefixSet{Prefix: t.Prefix}, t.Spec())
	return packetSymbolicTargetResult(result)
}

type PacketClassTarget struct {
	Universe space.PrefixUniverse
	ClassID  space.PrefixClassID
	Protocol string
	DstPort  int
	VRF      string
}

func (t PacketClassTarget) Reachable(g *Graph, from string, failures FailureSet) bool {
	result := t.symbolicReachability(g, from)
	return result.Reachable.Eval(g.FailureContext(failures))
}

func (t PacketClassTarget) Spec() predicate.PacketSpec {
	return predicate.PacketSpec{Protocol: t.Protocol, DstPort: predicate.ExactPort(t.DstPort)}
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
	Class space.PacketClass
}

func (t HeaderPacketClassTarget) Reachable(g *Graph, from string, failures FailureSet) bool {
	result := t.symbolicReachability(g, from)
	return result.Reachable.Eval(g.FailureContext(failures))
}

func (t HeaderPacketClassTarget) Spec() predicate.PacketSpec {
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
