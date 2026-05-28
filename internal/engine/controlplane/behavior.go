package controlplane

import (
	"github.com/81ueman/hoyan-lab/internal/core/predicate"
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/core/failure"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

type ControlMessage struct {
	From   string
	To     string
	Prefix string
	Route  RIBEntry
}

type PacketMessage struct {
	Node string
	Spec predicate.PacketSpec
}

type PolicyDecision struct {
	PolicyName    string
	ACLName       string
	RuleSeq       int
	Action        topology.ACLAction
	Denied        bool
	Cond          failure.Cond
	Reason        string
	DefaultAction topology.ACLDefaultAction
	Source        topology.ConfigSource
}

type DeviceBehavior interface {
	Kind() topology.DeviceKind
	Profile() topology.DeviceProfile
	BGPBehavior
	CheckControlEgress(device topology.Node, msg ControlMessage) bool
	CheckControlIngress(device topology.Node, msg ControlMessage) bool
	EvaluateDataACL(device topology.Node, pkt PacketMessage, stage string, acls []topology.ACL, bindings []topology.ACLBinding) PolicyDecision
	RouteValidForRIB(device topology.Node, route RIBEntry) bool
	RouteEligibleForAdvertisement(device topology.Node, route RIBEntry) bool
	RouteInstallableInFIB(device topology.Node, installed []RIBEntry, route RIBEntry) bool
}

func (b baseDeviceBehavior) Kind() topology.DeviceKind {
	return b.kind
}

func (b baseDeviceBehavior) Profile() topology.DeviceProfile {
	return topology.ProfileFor(b.kind)
}

func (b baseDeviceBehavior) CheckControlIngress(device topology.Node, msg ControlMessage) bool {
	return true
}

func (b baseDeviceBehavior) CheckControlEgress(device topology.Node, msg ControlMessage) bool {
	return true
}

func (b baseDeviceBehavior) EvaluateDataACL(device topology.Node, pkt PacketMessage, stage string, acls []topology.ACL, bindings []topology.ACLBinding) PolicyDecision {
	spec := pkt.NormalizedSpec()
	iface := spec.IngressInterface
	if stage == "egress" {
		iface = spec.EgressInterface
	}
	for _, binding := range bindings {
		if binding.Node != device.Name || binding.Direction != stage {
			continue
		}
		if binding.Interface != "" && !interfaceMatches(device.Kind, binding.Interface, iface) {
			continue
		}
		acl, ok := aclByName(acls, device.Name, binding.ACLName)
		if !ok {
			continue
		}
		return evaluateACL(device, acl, binding, spec)
	}
	return PolicyDecision{Cond: failure.False()}
}

func (b baseDeviceBehavior) RouteValidForRIB(device topology.Node, route RIBEntry) bool {
	route = route.Normalize()
	return !route.Attrs.Invalid
}

func (b baseDeviceBehavior) RouteEligibleForAdvertisement(device topology.Node, route RIBEntry) bool {
	return b.RouteValidForRIB(device, route)
}

func (b baseDeviceBehavior) RouteInstallableInFIB(device topology.Node, installed []RIBEntry, route RIBEntry) bool {
	return b.RouteValidForRIB(device, route)
}

func evaluateACL(device topology.Node, acl topology.ACL, binding topology.ACLBinding, spec predicate.PacketSpec) PolicyDecision {
	rules := append([]topology.ACLRule(nil), acl.Rules...)
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Seq == rules[j].Seq {
			return i < j
		}
		if rules[i].Seq == 0 {
			return false
		}
		if rules[j].Seq == 0 {
			return true
		}
		return rules[i].Seq < rules[j].Seq
	})
	for _, rule := range rules {
		if !aclRuleMatches(rule, spec) {
			continue
		}
		denied := rule.Action == topology.ACLDeny
		reason := "permitted by acl " + acl.Name
		if denied {
			reason = "denied by acl " + acl.Name
		}
		return PolicyDecision{
			PolicyName: acl.Name,
			ACLName:    acl.Name,
			RuleSeq:    rule.Seq,
			Action:     rule.Action,
			Denied:     denied,
			Cond:       decisionCond(denied),
			Reason:     reason,
			Source:     rule.Source,
		}
	}
	defaultAction := acl.DefaultAction
	if defaultAction == "" {
		defaultAction = topology.ProfileFor(device.Kind).ACLProfile().DefaultACLAction(topology.ACLDefaultPermit)
	}
	denied := defaultAction == topology.ACLDefaultDeny
	reason := "default permit by acl " + acl.Name
	if denied {
		reason = "default deny by acl " + acl.Name
	}
	return PolicyDecision{
		PolicyName:    acl.Name,
		ACLName:       acl.Name,
		Action:        topology.ACLAction(defaultAction),
		Denied:        denied,
		Cond:          decisionCond(denied),
		Reason:        reason,
		DefaultAction: defaultAction,
		Source:        binding.Source,
	}
}

func decisionCond(denied bool) failure.Cond {
	if denied {
		return failure.True()
	}
	return failure.False()
}

func aclByName(acls []topology.ACL, node, name string) (topology.ACL, bool) {
	for _, acl := range acls {
		if acl.Node == node && acl.Name == name {
			return acl, true
		}
	}
	return topology.ACL{}, false
}

func aclRuleMatches(rule topology.ACLRule, spec predicate.PacketSpec) bool {
	match := rule.Match.WithNormalizedPorts()
	spec = spec.WithNormalizedPorts()
	if match.Protocol != "" && !strings.EqualFold(match.Protocol, spec.Protocol) {
		return false
	}
	if match.SrcSet != nil {
		if !prefixSetMatches(match.SrcSet, spec.SrcSet) {
			return false
		}
	}
	if match.DstSet != nil {
		if !prefixSetMatches(match.DstSet, spec.DstSet) {
			return false
		}
	}
	if match.SrcPort != nil && !match.SrcPort.Overlaps(spec.SrcPort) {
		return false
	}
	if match.DstPort != nil && !match.DstPort.Overlaps(spec.DstPort) {
		return false
	}
	return true
}

func prefixSetMatches(match, packet predicate.PrefixSet) bool {
	if packet == nil {
		return prefixSetIsAny(match)
	}
	return predicate.AddressSpaceOverlaps(match, packet)
}

func prefixSetIsAny(set predicate.PrefixSet) bool {
	exact, ok := set.(predicate.ExactPrefixSet)
	return ok && exact.Prefix.String() == "0.0.0.0/0"
}

func (p PacketMessage) NormalizedSpec() predicate.PacketSpec {
	return p.Spec.WithNormalizedPorts()
}

func interfaceMatches(kind topology.DeviceKind, policyInterface, packetInterface string) bool {
	return topology.ProfileFor(kind).InterfaceProfile().EquivalentInterfaceName(policyInterface, packetInterface)
}
