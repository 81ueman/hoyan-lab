package modelinspect

import (
	"fmt"
	"net/netip"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/domain/model"
	"github.com/81ueman/hoyan-lab/internal/engine/sim"
	"github.com/81ueman/hoyan-lab/internal/usecase/topology"
)

// --- Prefix / packet class inspection ---

func InspectPrefixClasses(req Request) (PrefixClassesResult, error) {
	topo, err := topology.LoadTopology(req.TopologyPath)
	if err != nil {
		return PrefixClassesResult{}, err
	}
	graph, err := sim.NewGraph(topo)
	if err != nil {
		return PrefixClassesResult{}, err
	}
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

// --- Symbolic packet reachability ---

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

// --- Symbolic route reachability ---

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

// --- Shared helpers for prefix / packet classes and symbolic route ---

func modelPrefixUniverse(topo *model.Topology, graph *sim.Graph, request []model.PrefixPredicate) (model.PrefixUniverse, error) {
	predicates := model.CollectPrefixPredicateMetadata(topo)
	predicates = append(predicates, sim.CollectRIBPrefixPredicates(graph)...)
	predicates = append(predicates, sim.CollectFIBPrefixPredicates(graph)...)
	predicates = append(predicates, request...)
	return model.BuildPrefixUniverseFromPredicates(predicates)
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
