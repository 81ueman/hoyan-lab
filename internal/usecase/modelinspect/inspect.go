package modelinspect

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/failure"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

type Request struct {
	TopologyPath     string
	Node             string
	Prefix           string
	From             string
	To               string
	Protocol         string
	DstPort          int
	StrictConfig     bool
	MaxPrefixClasses int
}

type PrefixClassRow struct {
	ClassID           model.PrefixClassID `json:"class_id"`
	Space             string              `json:"space"`
	MatchedPredicates []string            `json:"matched_predicates,omitempty"`
}

type PacketClassRow struct {
	ClassID           model.PacketClassID `json:"class_id"`
	PrefixClassID     model.PrefixClassID `json:"prefix_class_id"`
	Space             string              `json:"space"`
	Protocol          string              `json:"protocol,omitempty"`
	SrcPort           string              `json:"src_port,omitempty"`
	DstPort           string              `json:"dst_port,omitempty"`
	IngressInterface  string              `json:"ingress_interface,omitempty"`
	EgressInterface   string              `json:"egress_interface,omitempty"`
	MatchedPredicates []string            `json:"matched_predicates,omitempty"`
}

type RIBRow struct {
	Node                  string   `json:"node"`
	Prefix                string   `json:"prefix"`
	SourceKind            string   `json:"source_kind,omitempty"`
	ConnectedClass        string   `json:"connected_class,omitempty"`
	OSPFRouteType         string   `json:"ospf_route_type,omitempty"`
	Metric                *int     `json:"metric,omitempty"`
	RouteInterface        string   `json:"interface,omitempty"`
	NextHopNode           string   `json:"next_hop_node,omitempty"`
	NextHopAddr           string   `json:"next_hop_addr,omitempty"`
	OriginNode            string   `json:"origin_node,omitempty"`
	FromNode              string   `json:"from_node,omitempty"`
	PathNodes             []string `json:"path_nodes,omitempty"`
	PathLinks             []string `json:"path_links,omitempty"`
	ASPath                []uint32 `json:"as_path,omitempty"`
	Communities           []string `json:"communities,omitempty"`
	OriginCode            *string  `json:"origin_code,omitempty"`
	LocalPref             *int     `json:"local_pref,omitempty"`
	MED                   *int     `json:"med,omitempty"`
	LearnedIBGP           *bool    `json:"learned_ibgp,omitempty"`
	Invalid               *bool    `json:"invalid,omitempty"`
	AggregateContributors []string `json:"aggregate_contributors,omitempty"`
	Condition             string   `json:"condition,omitempty"`
	SelectedCondition     string   `json:"selected_condition,omitempty"`
	BaseCondition         string   `json:"base_condition,omitempty"`
}

type FIBRow struct {
	Node             string   `json:"node"`
	Prefix           string   `json:"prefix"`
	SourceKind       string   `json:"source_kind,omitempty"`
	Discard          bool     `json:"discard,omitempty"`
	ConnectedClass   string   `json:"connected_class,omitempty"`
	Interface        string   `json:"interface,omitempty"`
	NextHop          string   `json:"next_hop_node,omitempty"`
	RawNextHop       string   `json:"raw_next_hop,omitempty"`
	NextHopAddress   string   `json:"next_hop_addr,omitempty"`
	ResolutionStatus string   `json:"resolution_status,omitempty"`
	ResolutionReason string   `json:"resolution_reason,omitempty"`
	Rank             int      `json:"rank"`
	GroupID          string   `json:"group_id,omitempty"`
	Equivalent       bool     `json:"equivalent"`
	PathNodes        []string `json:"path_nodes,omitempty"`
	PathLinks        []string `json:"path_links,omitempty"`
	Cost             int      `json:"cost"`
	Condition        string   `json:"condition,omitempty"`
}

type SymbolicPacketInspect struct {
	From               string                               `json:"from"`
	To                 string                               `json:"to"`
	Protocol           string                               `json:"protocol"`
	DstPort            int                                  `json:"dst_port,omitempty"`
	Reachable          string                               `json:"reachable_condition"`
	Unreachable        string                               `json:"unreachable_condition"`
	Reason             string                               `json:"reason,omitempty"`
	Paths              []SymbolicPacketInspectPath          `json:"paths,omitempty"`
	Blocked            []SymbolicPacketBlockedPath          `json:"blocked_paths,omitempty"`
	UnreachableReasons []SymbolicPacketInspectBlockedReason `json:"unreachable_reasons,omitempty"`
}

type SymbolicPacketInspectPath struct {
	PathNodes []string                     `json:"path_nodes,omitempty"`
	PathLinks []string                     `json:"path_links,omitempty"`
	Cost      int                          `json:"cost"`
	Condition string                       `json:"condition,omitempty"`
	States    []SymbolicPacketInspectState `json:"states,omitempty"`
}

type SymbolicPacketInspectState struct {
	Node             string   `json:"node"`
	IngressInterface string   `json:"ingress_interface,omitempty"`
	Condition        string   `json:"condition,omitempty"`
	PathNodes        []string `json:"path_nodes,omitempty"`
	PathLinks        []string `json:"path_links,omitempty"`
	Cost             int      `json:"cost"`
}

type SymbolicPacketBlockedPath struct {
	PathNodes     []string               `json:"path_nodes,omitempty"`
	PathLinks     []string               `json:"path_links,omitempty"`
	Cost          int                    `json:"cost"`
	Condition     string                 `json:"condition,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
	ACL           string                 `json:"acl,omitempty"`
	RuleSeq       int                    `json:"rule_seq,omitempty"`
	Action        model.ACLAction        `json:"action,omitempty"`
	DefaultAction model.ACLDefaultAction `json:"default_action,omitempty"`
	Node          string                 `json:"node,omitempty"`
	Interface     string                 `json:"interface,omitempty"`
	Stage         string                 `json:"stage,omitempty"`
	Source        model.ConfigSource     `json:"source,omitempty"`
}

type SymbolicPacketInspectBlockedReason struct {
	Kind          string                 `json:"kind"`
	Node          string                 `json:"node,omitempty"`
	Link          string                 `json:"link,omitempty"`
	Interface     string                 `json:"interface,omitempty"`
	PolicyName    string                 `json:"policy_name,omitempty"`
	ACLName       string                 `json:"acl_name,omitempty"`
	RuleSeq       int                    `json:"rule_seq,omitempty"`
	Action        model.ACLAction        `json:"action,omitempty"`
	DefaultAction model.ACLDefaultAction `json:"default_action,omitempty"`
	PolicyRaw     string                 `json:"policy_raw,omitempty"`
	PathNodes     []string               `json:"path_nodes,omitempty"`
	PathLinks     []string               `json:"path_links,omitempty"`
	Cost          int                    `json:"cost"`
	Condition     string                 `json:"condition,omitempty"`
	Message       string                 `json:"message,omitempty"`
}

type SymbolicRouteInspect struct {
	From              string                     `json:"from"`
	Prefix            string                     `json:"prefix"`
	ClassID           model.PrefixClassID        `json:"class_id"`
	Space             string                     `json:"space"`
	MatchedPredicates []string                   `json:"matched_predicates,omitempty"`
	Reachable         string                     `json:"reachable_condition"`
	Unreachable       string                     `json:"unreachable_condition"`
	Reason            string                     `json:"reason,omitempty"`
	Paths             []SymbolicRouteInspectPath `json:"paths,omitempty"`
}

type SymbolicRouteInspectPath struct {
	PathNodes []string `json:"path_nodes,omitempty"`
	PathLinks []string `json:"path_links,omitempty"`
	Cost      int      `json:"cost"`
	Condition string   `json:"condition,omitempty"`
}

type RIBResult struct {
	Protocol model.RouteSourceKind
	Rows     []RIBRow
}

type FIBResult struct {
	Rows []FIBRow
}

type PrefixClassesResult struct {
	Stats   model.PrefixUniverseStats
	Classes []PrefixClassRow
}

type PacketClassesResult struct {
	Classes []PacketClassRow
}

type SymbolicRouteResult struct {
	Results []SymbolicRouteInspect
}

func InspectRIB(req Request) (RIBResult, error) {
	protocol, err := CanonicalRouteProtocol(req.Protocol)
	if err != nil {
		return RIBResult{}, err
	}
	topo, graph, err := loadGraph(req.TopologyPath, req.StrictConfig)
	if err != nil {
		return RIBResult{}, err
	}
	nodes, err := inspectNodes(topo, req.Node)
	if err != nil {
		return RIBResult{}, err
	}
	prefix, err := CanonicalPrefix(req.Prefix)
	if err != nil {
		return RIBResult{}, err
	}
	return RIBResult{Protocol: protocol, Rows: collectRIBRows(graph, nodes, prefix, protocol)}, nil
}

func InspectFIB(req Request) (FIBResult, error) {
	topo, graph, err := loadGraph(req.TopologyPath, req.StrictConfig)
	if err != nil {
		return FIBResult{}, err
	}
	nodes, err := inspectNodes(topo, req.Node)
	if err != nil {
		return FIBResult{}, err
	}
	prefix, err := CanonicalPrefix(req.Prefix)
	if err != nil {
		return FIBResult{}, err
	}
	return FIBResult{Rows: collectFIBRows(graph, nodes, prefix)}, nil
}

func InspectPrefixClasses(req Request) (PrefixClassesResult, error) {
	topo, err := topology.LoadTopology(req.TopologyPath)
	if err != nil {
		return PrefixClassesResult{}, err
	}
	graph := sim.NewGraph(topo)
	var filter model.PrefixSet
	var request []model.PrefixPredicate
	if req.Prefix != "" {
		prefix, err := model.ParsePrefix(req.Prefix)
		if err != nil {
			return PrefixClassesResult{}, fmt.Errorf("--prefix %q: %w", req.Prefix, err)
		}
		filter = model.ExactPrefixSet{Prefix: prefix}
		request = append(request, model.PrefixPredicate{Source: "request:prefix-classes:" + prefix.String(), Set: filter})
	}
	universe, err := modelPrefixUniverse(topo, graph, request)
	if err != nil {
		return PrefixClassesResult{}, err
	}
	if req.MaxPrefixClasses > 0 && universe.Stats.ClassCount > req.MaxPrefixClasses {
		return PrefixClassesResult{}, fmt.Errorf("prefix universe class count %d exceeds --max-prefix-classes %d", universe.Stats.ClassCount, req.MaxPrefixClasses)
	}
	return PrefixClassesResult{Stats: universe.Stats, Classes: collectPrefixClassRows(universe, filter)}, nil
}

func InspectPacketClasses(req Request) (PacketClassesResult, error) {
	topo, graph, err := loadGraph(req.TopologyPath, req.StrictConfig)
	if err != nil {
		return PacketClassesResult{}, err
	}
	var filter model.PrefixSet
	var request []model.PrefixPredicate
	if req.Prefix != "" {
		prefix, err := model.ParsePrefix(req.Prefix)
		if err != nil {
			return PacketClassesResult{}, fmt.Errorf("--prefix %q: %w", req.Prefix, err)
		}
		filter = model.ExactPrefixSet{Prefix: prefix}
		request = append(request, model.PrefixPredicate{Source: "request:packet-classes:" + prefix.String(), Set: filter})
	}
	universe, err := modelPrefixUniverse(topo, graph, request)
	if err != nil {
		return PacketClassesResult{}, err
	}
	headerSpace := model.NewHeaderSpace(topo, universe)
	return PacketClassesResult{Classes: collectPacketClassRows(headerSpace, filter, req.Protocol, req.DstPort)}, nil
}

func InspectSymbolicPacket(req Request) (SymbolicPacketInspect, error) {
	if req.From == "" {
		return SymbolicPacketInspect{}, fmt.Errorf("--from is required")
	}
	if req.To == "" {
		return SymbolicPacketInspect{}, fmt.Errorf("--to is required")
	}
	topo, graph, err := loadGraph(req.TopologyPath, req.StrictConfig)
	if err != nil {
		return SymbolicPacketInspect{}, err
	}
	if _, ok := topo.Node(req.From); !ok {
		return SymbolicPacketInspect{}, fmt.Errorf("unknown node %q", req.From)
	}
	if _, err := netip.ParseAddr(req.To); err != nil {
		return SymbolicPacketInspect{}, fmt.Errorf("--to must be an IP address: %w", err)
	}
	spec := model.PacketSpec{Protocol: req.Protocol, DstPort: model.ExactPort(req.DstPort)}
	return buildSymbolicPacketInspect(req, graph.SymbolicPacketReachabilitySpec(req.From, req.To, spec)), nil
}

func InspectSymbolicRoute(req Request) (SymbolicRouteResult, error) {
	if req.From == "" {
		return SymbolicRouteResult{}, fmt.Errorf("--from is required")
	}
	if req.Prefix == "" {
		return SymbolicRouteResult{}, fmt.Errorf("--prefix is required")
	}
	topo, graph, err := loadGraph(req.TopologyPath, req.StrictConfig)
	if err != nil {
		return SymbolicRouteResult{}, err
	}
	if _, ok := topo.Node(req.From); !ok {
		return SymbolicRouteResult{}, fmt.Errorf("unknown node %q", req.From)
	}
	prefix, err := CanonicalPrefix(req.Prefix)
	if err != nil {
		return SymbolicRouteResult{}, err
	}
	parsedPrefix, err := model.ParsePrefix(prefix)
	if err != nil {
		return SymbolicRouteResult{}, err
	}
	filter := model.ExactPrefixSet{Prefix: parsedPrefix}
	universe, err := modelPrefixUniverse(topo, graph, []model.PrefixPredicate{{
		Source: "request:symbolic-route:" + parsedPrefix.String(),
		Set:    filter,
	}})
	if err != nil {
		return SymbolicRouteResult{}, err
	}
	return SymbolicRouteResult{Results: buildSymbolicRouteClassInspects(req.From, prefix, universe, filter, graph.SymbolicRouteReachability(req.From, prefix))}, nil
}

func loadGraph(topologyPath string, strictConfig bool) (*model.Topology, *sim.Graph, error) {
	topo, _, err := topology.LoadTopologyWithOptions(topologyPath, topology.LoadOptions{StrictConfig: strictConfig})
	if err != nil {
		return nil, nil, err
	}
	return topo, sim.NewGraph(topo), nil
}

func inspectNodes(topo *model.Topology, node string) ([]string, error) {
	if node != "" {
		if _, ok := topo.Node(node); !ok {
			return nil, fmt.Errorf("unknown node %q", node)
		}
		return []string{node}, nil
	}
	nodes := make([]string, 0, len(topo.Nodes))
	for _, n := range topo.Nodes {
		nodes = append(nodes, n.Name)
	}
	sort.Strings(nodes)
	return nodes, nil
}

func CanonicalPrefix(raw string) (string, error) {
	if raw == "" {
		return "", nil
	}
	prefix, err := model.ParsePrefix(raw)
	if err != nil {
		return "", fmt.Errorf("--prefix %q: %w", raw, err)
	}
	return prefix.String(), nil
}

func CanonicalRouteProtocol(raw string) (model.RouteSourceKind, error) {
	protocol := model.RouteSourceKind(strings.ToLower(strings.TrimSpace(raw)))
	switch protocol {
	case "":
		return "", nil
	case model.RouteSourceBGP, model.RouteSourceConnected, model.RouteSourceStatic, model.RouteSourceOSPF, model.RouteSourceAggregate, model.RouteSourceBlackhole:
		return protocol, nil
	default:
		return "", fmt.Errorf("protocol must be one of bgp, connected, static, ospf, aggregate, or blackhole")
	}
}

func ptr[T any](v T) *T {
	return &v
}

func modelPrefixUniverse(topo *model.Topology, graph *sim.Graph, request []model.PrefixPredicate) (model.PrefixUniverse, error) {
	predicates := model.CollectPrefixPredicateMetadata(topo)
	predicates = append(predicates, sim.CollectRIBPrefixPredicates(graph)...)
	predicates = append(predicates, sim.CollectFIBPrefixPredicates(graph)...)
	predicates = append(predicates, request...)
	return model.BuildPrefixUniverseFromPredicates(predicates)
}

func collectPrefixClassRows(universe model.PrefixUniverse, filter model.PrefixSet) []PrefixClassRow {
	var rows []PrefixClassRow
	for _, class := range universe.Classes {
		if filter != nil && !model.AddressSpaceOverlaps(class.Space, filter) {
			continue
		}
		rows = append(rows, PrefixClassRow{
			ClassID:           class.ID,
			Space:             class.Space.String(),
			MatchedPredicates: matchedPrefixPredicates(universe, class),
		})
	}
	return rows
}

func collectPacketClassRows(headerSpace model.HeaderSpace, filter model.PrefixSet, protocol string, dstPort int) []PacketClassRow {
	protocol = strings.ToLower(strings.TrimSpace(protocol))
	var rows []PacketClassRow
	for _, class := range headerSpace.Classes {
		if filter != nil && !model.AddressSpaceOverlaps(class.DstSet, filter) {
			continue
		}
		if protocol != "" && class.Protocol != "" && class.Protocol != protocol {
			continue
		}
		if dstPort > 0 && class.DstPort != nil && !class.DstPort.Contains(dstPort) {
			continue
		}
		rows = append(rows, PacketClassRow{
			ClassID:           class.ID,
			PrefixClassID:     class.PrefixClassID,
			Space:             prefixSetString(class.DstSet),
			Protocol:          class.Protocol,
			SrcPort:           portSetInspectString(class.SrcPort),
			DstPort:           portSetInspectString(class.DstPort),
			IngressInterface:  class.IngressInterface,
			EgressInterface:   class.EgressInterface,
			MatchedPredicates: matchedHeaderPredicates(headerSpace, class),
		})
	}
	return rows
}

func collectRIBRows(graph *sim.Graph, nodes []string, prefix string, protocol model.RouteSourceKind) []RIBRow {
	var rows []RIBRow
	for _, node := range nodes {
		nodeID := model.NodeID(node)
		if prefix != "" {
			pfx, err := model.ParsePrefix(prefix)
			if err != nil {
				continue
			}
			rows = append(rows, ribRowsForRoutes(node, graph.RIB(nodeID, pfx), protocol)...)
			continue
		}
		table := graph.RIBTable(nodeID)
		prefixes := make([]model.Prefix, 0, len(table))
		for p := range table {
			prefixes = append(prefixes, p)
		}
		sort.Slice(prefixes, func(i, j int) bool { return prefixes[i].String() < prefixes[j].String() })
		for _, p := range prefixes {
			rows = append(rows, ribRowsForRoutes(node, table[p], protocol)...)
		}
	}
	return rows
}

func ribRowsForRoutes(node string, routes []sim.RIBEntry, protocol model.RouteSourceKind) []RIBRow {
	rows := make([]RIBRow, 0, len(routes))
	for _, route := range routes {
		route = route.Normalize()
		if protocol != "" && route.SourceKind != protocol {
			continue
		}
		rows = append(rows, RIBRow{
			Node:                  node,
			Prefix:                route.NLRI.Prefix.String(),
			SourceKind:            string(route.SourceKind),
			ConnectedClass:        string(route.RouteSource.ConnectedClass),
			OSPFRouteType:         route.RouteSource.OSPFRouteType,
			RouteInterface:        route.RouteSource.Interface,
			NextHopNode:           route.ForwardingNextHop.Node,
			NextHopAddr:           route.ForwardingNextHop.Addr,
			OriginNode:            route.Provenance.OriginNode,
			FromNode:              route.Provenance.FromNode,
			PathNodes:             append([]string(nil), route.Provenance.PathNodes...),
			PathLinks:             append([]string(nil), route.Provenance.PathLinks...),
			AggregateContributors: append([]string(nil), route.AggregateContributors...),
			Condition:             condString(route.Condition),
			SelectedCondition:     condString(route.SelectedCond),
			BaseCondition:         condString(route.BaseCond),
		})
		if route.SourceKind == model.RouteSourceBGP {
			last := &rows[len(rows)-1]
			last.ASPath = append([]uint32(nil), route.Attrs.ASPath...)
			last.Communities = append([]string(nil), route.Attrs.Communities...)
			last.OriginCode = ptr(string(route.Attrs.OriginCode))
			last.LocalPref = ptr(route.Attrs.LocalPref)
			last.MED = ptr(route.Attrs.MED)
			last.LearnedIBGP = ptr(route.Attrs.LearnedIBGP)
			last.Invalid = ptr(route.Attrs.Invalid)
		}
		if route.SourceKind == model.RouteSourceOSPF {
			rows[len(rows)-1].Metric = ptr(route.RouteSource.Metric)
		}
	}
	return rows
}

func collectFIBRows(graph *sim.Graph, nodes []string, prefix string) []FIBRow {
	var rows []FIBRow
	for _, node := range nodes {
		for _, entry := range graph.FIB(model.NodeID(node)) {
			if prefix != "" && entry.Prefix.String() != prefix {
				continue
			}
			rows = append(rows, FIBRow{
				Node:             node,
				Prefix:           entry.Prefix.String(),
				SourceKind:       string(entry.SourceKind),
				Discard:          entry.Discard,
				ConnectedClass:   string(entry.ConnectedClass),
				Interface:        entry.Interface,
				NextHop:          entry.NextHop,
				RawNextHop:       entry.RawNextHop,
				NextHopAddress:   entry.NextHopAddress,
				ResolutionStatus: string(entry.ResolutionStatus),
				ResolutionReason: entry.ResolutionReason,
				Rank:             entry.Rank,
				GroupID:          entry.GroupID,
				Equivalent:       entry.Equivalent,
				PathNodes:        append([]string(nil), entry.Path.Nodes...),
				PathLinks:        append([]string(nil), entry.Path.Links...),
				Cost:             entry.Path.Cost,
				Condition:        condString(entry.Condition),
			})
		}
	}
	return rows
}

func buildSymbolicPacketInspect(opts Request, result sim.SymbolicReachabilityResult) SymbolicPacketInspect {
	out := SymbolicPacketInspect{
		From:        opts.From,
		To:          opts.To,
		Protocol:    opts.Protocol,
		DstPort:     opts.DstPort,
		Reachable:   condString(result.Reachable),
		Unreachable: condString(result.Unreachable),
		Reason:      result.Reason,
	}
	for _, path := range result.Paths {
		row := SymbolicPacketInspectPath{
			PathNodes: append([]string(nil), path.Path.Nodes...),
			PathLinks: append([]string(nil), path.Path.Links...),
			Cost:      path.Path.Cost,
			Condition: condString(path.Cond),
		}
		for _, state := range path.States {
			row.States = append(row.States, SymbolicPacketInspectState{
				Node:             state.Node,
				IngressInterface: state.IngressInterface,
				Condition:        condString(state.Cond),
				PathNodes:        append([]string(nil), state.Path.Nodes...),
				PathLinks:        append([]string(nil), state.Path.Links...),
				Cost:             state.Path.Cost,
			})
		}
		out.Paths = append(out.Paths, row)
	}
	for _, path := range result.Blocked {
		out.Blocked = append(out.Blocked, SymbolicPacketBlockedPath{
			PathNodes:     append([]string(nil), path.Path.Nodes...),
			PathLinks:     append([]string(nil), path.Path.Links...),
			Cost:          path.Path.Cost,
			Condition:     condString(path.Cond),
			Reason:        path.Reason,
			ACL:           path.ACL,
			RuleSeq:       path.RuleSeq,
			Action:        path.Action,
			DefaultAction: path.DefaultAction,
			Node:          path.Node,
			Interface:     path.Interface,
			Stage:         path.Stage,
			Source:        path.Source,
		})
	}
	for _, reason := range result.UnreachableReasons {
		out.UnreachableReasons = append(out.UnreachableReasons, SymbolicPacketInspectBlockedReason{
			Kind:          string(reason.Kind),
			Node:          reason.Node,
			Link:          reason.Link,
			Interface:     reason.Interface,
			PolicyName:    reason.PolicyName,
			ACLName:       reason.ACLName,
			RuleSeq:       reason.RuleSeq,
			Action:        reason.Action,
			DefaultAction: reason.DefaultAction,
			PolicyRaw:     reason.PolicyRaw,
			PathNodes:     append([]string(nil), reason.Path.Nodes...),
			PathLinks:     append([]string(nil), reason.Path.Links...),
			Cost:          reason.Path.Cost,
			Condition:     condString(reason.Cond),
			Message:       reason.Message,
		})
	}
	return out
}

func buildSymbolicRouteClassInspects(from, prefix string, universe model.PrefixUniverse, filter model.PrefixSet, result sim.SymbolicRouteReachabilityResult) []SymbolicRouteInspect {
	var out []SymbolicRouteInspect
	for _, class := range universe.Classes {
		if filter != nil && !model.AddressSpaceOverlaps(class.Space, filter) {
			continue
		}
		out = append(out, buildSymbolicRouteInspect(from, prefix, class, matchedPrefixPredicates(universe, class), result))
	}
	return out
}

func buildSymbolicRouteInspect(from, prefix string, class model.PrefixClass, matched []string, result sim.SymbolicRouteReachabilityResult) SymbolicRouteInspect {
	out := SymbolicRouteInspect{
		From:              from,
		Prefix:            prefix,
		ClassID:           class.ID,
		Space:             class.Space.String(),
		MatchedPredicates: matched,
		Reachable:         condString(result.Reachable),
		Unreachable:       condString(result.Unreachable),
		Reason:            result.Reason,
	}
	for _, path := range result.Paths {
		out.Paths = append(out.Paths, SymbolicRouteInspectPath{
			PathNodes: append([]string(nil), path.Path.Nodes...),
			PathLinks: append([]string(nil), path.Path.Links...),
			Cost:      path.Path.Cost,
			Condition: condString(path.Cond),
		})
	}
	return out
}

func matchedPrefixPredicates(universe model.PrefixUniverse, class model.PrefixClass) []string {
	byID := map[model.PrefixPredicateID]string{}
	for _, predicate := range universe.Predicates {
		byID[predicate.ID] = predicate.Source
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(class.MatchingPredicates))
	for _, id := range class.MatchingPredicates {
		source := byID[id]
		if source == "" || seen[source] {
			continue
		}
		seen[source] = true
		out = append(out, source)
	}
	sort.Strings(out)
	return out
}

func matchedHeaderPredicates(headerSpace model.HeaderSpace, class model.PacketClass) []string {
	byID := map[model.HeaderPredicateID]string{}
	for _, predicate := range headerSpace.Predicates {
		byID[predicate.ID] = predicate.Source
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(class.MatchingPredicates))
	for _, id := range class.MatchingPredicates {
		source := byID[id]
		if source == "" || seen[source] {
			continue
		}
		seen[source] = true
		out = append(out, source)
	}
	sort.Strings(out)
	return out
}

func condString(cond failure.Cond) string {
	if cond == nil {
		return ""
	}
	return cond.String()
}

func prefixSetString(set model.PrefixSet) string {
	if set == nil {
		return ""
	}
	return set.String()
}

func portSetInspectString(set model.PortSet) string {
	if set == nil {
		return "any"
	}
	return set.String()
}
