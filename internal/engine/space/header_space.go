package space

import (
	"fmt"
	"github.com/81ueman/hoyan-lab/internal/core/predicate"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/check/query"
	"github.com/81ueman/hoyan-lab/internal/config/routing"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

type PacketClassID int
type HeaderPredicateID int

type HeaderPredicate struct {
	ID               HeaderPredicateID
	Source           string
	Protocol         string
	SrcSet           predicate.PrefixSet
	DstSet           predicate.PrefixSet
	SrcPort          predicate.PortSet
	DstPort          predicate.PortSet
	IngressInterface string
	EgressInterface  string
}

type PacketClass struct {
	ID                 PacketClassID
	PrefixClassID      PrefixClassID
	DstSet             predicate.PrefixSet
	Protocol           string
	SrcPort            predicate.PortSet
	DstPort            predicate.PortSet
	IngressInterface   string
	EgressInterface    string
	MatchingPredicates []HeaderPredicateID
}

type HeaderSpace struct {
	Classes    []PacketClass
	Predicates []HeaderPredicate
}

func (c PacketClass) Spec() predicate.PacketSpec {
	return predicate.PacketSpec{
		DstSet:           c.DstSet,
		Protocol:         c.Protocol,
		SrcPort:          c.SrcPort,
		DstPort:          c.DstPort,
		IngressInterface: c.IngressInterface,
		EgressInterface:  c.EgressInterface,
	}
}

func CollectHeaderPredicates(topo *topology.Topology, routes routing.TopologyRouting, queries *query.Queries) []HeaderPredicate {
	var out []HeaderPredicate
	add := func(predicate HeaderPredicate) {
		if predicate.Protocol == "" &&
			predicate.SrcSet == nil &&
			predicate.DstSet == nil &&
			predicate.SrcPort == nil &&
			predicate.DstPort == nil &&
			predicate.IngressInterface == "" &&
			predicate.EgressInterface == "" {
			return
		}
		predicate.ID = HeaderPredicateID(len(out))
		predicate.Protocol = strings.ToLower(strings.TrimSpace(predicate.Protocol))
		out = append(out, predicate)
	}
	if topo != nil {
		for _, binding := range routes.ACLBindings {
			acl, ok := aclByName(routes.ACLs, binding.Node, binding.ACLName)
			if !ok {
				continue
			}
			for _, rule := range acl.Rules {
				predicate := HeaderPredicate{
					Source:   "acl:" + acl.Name,
					Protocol: rule.Match.Protocol,
					SrcSet:   rule.Match.SrcSet,
					DstSet:   rule.Match.DstSet,
					SrcPort:  rule.Match.SrcPort,
					DstPort:  rule.Match.DstPort,
				}
				switch binding.Direction {
				case "ingress":
					predicate.IngressInterface = binding.Interface
				case "egress":
					predicate.EgressInterface = binding.Interface
				}
				add(predicate)
			}
		}
		for _, acl := range routes.ACLs {
			if aclHasBinding(routes.ACLBindings, acl.Node, acl.Name) {
				continue
			}
			for _, rule := range acl.Rules {
				predicate := HeaderPredicate{
					Source:   "acl:" + acl.Name,
					Protocol: rule.Match.Protocol,
					SrcSet:   rule.Match.SrcSet,
					DstSet:   rule.Match.DstSet,
					SrcPort:  rule.Match.SrcPort,
					DstPort:  rule.Match.DstPort,
				}
				add(predicate)
			}
		}
	}
	if queries != nil {
		for _, check := range queries.PacketChecks {
			for _, port := range check.DstPortValues() {
				predicate := HeaderPredicate{
					Source:   "query-packet:" + check.Name,
					Protocol: check.Protocol,
					DstPort:  predicate.ExactPort(port),
				}
				for _, set := range destinationPrefixSets(topo, check.To) {
					predicate.DstSet = set
					add(predicate)
				}
				if predicate.DstSet == nil {
					add(predicate)
				}
			}
		}
		for _, check := range queries.FailureChecks {
			for _, port := range check.DstPortValues() {
				pred := HeaderPredicate{
					Source:   "query-failure:" + check.Name,
					Protocol: check.Protocol,
					DstPort:  predicate.ExactPort(port),
				}
				if !check.Prefix.IsZero() {
					pred.DstSet = predicate.ExactPrefixSet{Prefix: check.Prefix}
					add(pred)
					continue
				}
				for _, set := range destinationPrefixSets(topo, check.To) {
					pred.DstSet = set
					add(pred)
				}
				if pred.DstSet == nil {
					add(pred)
				}
			}
		}
	}
	return out
}

func NewHeaderSpace(topo *topology.Topology, routes routing.TopologyRouting, queries *query.Queries, universe PrefixUniverse) HeaderSpace {
	return BuildHeaderSpaceFromPredicates(universe, CollectHeaderPredicates(topo, routes, queries))
}

func aclByName(acls []topology.ACL, node, name string) (topology.ACL, bool) {
	for _, acl := range acls {
		if acl.Node == node && acl.Name == name {
			return acl, true
		}
	}
	return topology.ACL{}, false
}

func aclHasBinding(bindings []topology.ACLBinding, node, name string) bool {
	for _, binding := range bindings {
		if binding.Node == node && binding.ACLName == name {
			return true
		}
	}
	return false
}

func BuildHeaderSpaceFromPredicates(universe PrefixUniverse, predicates []HeaderPredicate) HeaderSpace {
	space := HeaderSpace{}
	for _, predicate := range predicates {
		predicate.ID = HeaderPredicateID(len(space.Predicates))
		predicate.Protocol = strings.ToLower(strings.TrimSpace(predicate.Protocol))
		space.Predicates = append(space.Predicates, predicate)
	}
	if len(universe.Classes) == 0 || len(space.Predicates) == 0 {
		return space
	}
	seen := map[string]bool{}
	for _, prefixClass := range universe.Classes {
		prefixPredicates := headerPredicatesForPrefix(space.Predicates, prefixClass.Space)
		protocols := headerProtocolClasses(prefixPredicates)
		for _, protocol := range protocols {
			protocolPredicates := headerPredicatesForProtocol(prefixPredicates, protocol)
			srcPorts := headerPortClasses(protocolPredicates, func(p HeaderPredicate) predicate.PortSet { return p.SrcPort })
			for _, srcPort := range srcPorts {
				srcPortPredicates := headerPredicatesForPort(protocolPredicates, srcPort, func(p HeaderPredicate) predicate.PortSet { return p.SrcPort })
				dstPorts := headerPortClasses(srcPortPredicates, func(p HeaderPredicate) predicate.PortSet { return p.DstPort })
				for _, dstPort := range dstPorts {
					dstPortPredicates := headerPredicatesForPort(srcPortPredicates, dstPort, func(p HeaderPredicate) predicate.PortSet { return p.DstPort })
					ingressInterfaces := headerInterfaceClasses(dstPortPredicates, func(p HeaderPredicate) string { return p.IngressInterface })
					for _, ingressInterface := range ingressInterfaces {
						ingressPredicates := headerPredicatesForInterface(dstPortPredicates, ingressInterface, func(p HeaderPredicate) string { return p.IngressInterface })
						egressInterfaces := headerInterfaceClasses(ingressPredicates, func(p HeaderPredicate) string { return p.EgressInterface })
						for _, egressInterface := range egressInterfaces {
							class := PacketClass{
								ID:               PacketClassID(len(space.Classes)),
								PrefixClassID:    prefixClass.ID,
								DstSet:           prefixClass.Space,
								Protocol:         protocol,
								SrcPort:          srcPort,
								DstPort:          dstPort,
								IngressInterface: ingressInterface,
								EgressInterface:  egressInterface,
							}
							matches := matchingHeaderPredicateIDs(class, space.Predicates)
							if len(matches) == 0 {
								continue
							}
							class.MatchingPredicates = matches
							key := packetClassKey(class)
							if seen[key] {
								continue
							}
							seen[key] = true
							class.ID = PacketClassID(len(space.Classes))
							space.Classes = append(space.Classes, class)
						}
					}
				}
			}
		}
	}
	return space
}

func matchingHeaderPredicateIDs(class PacketClass, predicates []HeaderPredicate) []HeaderPredicateID {
	var out []HeaderPredicateID
	for _, predicate := range predicates {
		if !headerPredicateMatchesClass(predicate, class) {
			continue
		}
		out = append(out, predicate.ID)
	}
	return out
}

func headerPredicatesForPrefix(predicates []HeaderPredicate, dst predicate.PrefixSet) []HeaderPredicate {
	var out []HeaderPredicate
	for _, pred := range predicates {
		if pred.DstSet != nil && (dst == nil || !predicate.AddressSpaceOverlaps(pred.DstSet, dst)) {
			continue
		}
		out = append(out, pred)
	}
	return out
}

func headerPredicatesForProtocol(predicates []HeaderPredicate, protocol string) []HeaderPredicate {
	var out []HeaderPredicate
	for _, predicate := range predicates {
		if predicate.Protocol != "" && protocol != "" && predicate.Protocol != protocol {
			continue
		}
		out = append(out, predicate)
	}
	return out
}

func headerPredicatesForPort(predicates []HeaderPredicate, classSet predicate.PortSet, get func(HeaderPredicate) predicate.PortSet) []HeaderPredicate {
	var out []HeaderPredicate
	for _, predicate := range predicates {
		predicateSet := get(predicate)
		if predicateSet != nil && !portSetsOverlap(predicateSet, classSet) {
			continue
		}
		out = append(out, predicate)
	}
	return out
}

func headerPredicatesForInterface(predicates []HeaderPredicate, classInterface string, get func(HeaderPredicate) string) []HeaderPredicate {
	var out []HeaderPredicate
	for _, predicate := range predicates {
		predicateInterface := get(predicate)
		if predicateInterface != "" && classInterface != "" && predicateInterface != classInterface {
			continue
		}
		out = append(out, predicate)
	}
	return out
}

func headerPredicateMatchesClass(pred HeaderPredicate, class PacketClass) bool {
	if pred.Protocol != "" && class.Protocol != "" && pred.Protocol != class.Protocol {
		return false
	}
	if pred.DstSet != nil && (class.DstSet == nil || !predicate.AddressSpaceOverlaps(pred.DstSet, class.DstSet)) {
		return false
	}
	if pred.SrcPort != nil && !portSetsOverlap(pred.SrcPort, class.SrcPort) {
		return false
	}
	if pred.DstPort != nil && !portSetsOverlap(pred.DstPort, class.DstPort) {
		return false
	}
	if pred.IngressInterface != "" && class.IngressInterface != "" && pred.IngressInterface != class.IngressInterface {
		return false
	}
	if pred.EgressInterface != "" && class.EgressInterface != "" && pred.EgressInterface != class.EgressInterface {
		return false
	}
	return true
}

func headerProtocolClasses(predicates []HeaderPredicate) []string {
	seen := map[string]bool{}
	var out []string
	for _, predicate := range predicates {
		if predicate.Protocol == "" || seen[predicate.Protocol] {
			continue
		}
		seen[predicate.Protocol] = true
		out = append(out, predicate.Protocol)
	}
	if len(out) == 0 {
		return []string{""}
	}
	sort.Strings(out)
	return out
}

func headerPortClasses(predicates []HeaderPredicate, get func(HeaderPredicate) predicate.PortSet) []predicate.PortSet {
	seen := map[string]bool{}
	var out []predicate.PortSet
	for _, predicate := range predicates {
		set := get(predicate)
		if set == nil {
			continue
		}
		key := set.String()
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, set)
	}
	if len(out) == 0 {
		return []predicate.PortSet{nil}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func headerInterfaceClasses(predicates []HeaderPredicate, get func(HeaderPredicate) string) []string {
	seen := map[string]bool{}
	var out []string
	for _, predicate := range predicates {
		iface := get(predicate)
		if iface == "" || seen[iface] {
			continue
		}
		seen[iface] = true
		out = append(out, iface)
	}
	if len(out) == 0 {
		return []string{""}
	}
	sort.Strings(out)
	return out
}

func portSetsOverlap(a, b predicate.PortSet) bool {
	if a == nil || b == nil {
		return true
	}
	return a.Overlaps(b)
}

func packetClassKey(class PacketClass) string {
	return fmt.Sprintf("%d|%s|%s|%s|%s|%s",
		class.PrefixClassID,
		class.Protocol,
		portSetString(class.SrcPort),
		portSetString(class.DstPort),
		class.IngressInterface,
		class.EgressInterface,
	)
}

func portSetString(set predicate.PortSet) string {
	if set == nil {
		return "any"
	}
	return set.String()
}
