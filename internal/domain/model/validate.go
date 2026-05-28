package model

import (
	"fmt"
	"net/netip"
)

func (t *Topology) Validate() error {
	seen := map[string]bool{}
	nodes := map[string]Node{}
	for _, n := range t.Nodes {
		if n.Name == "" {
			return fmt.Errorf("node name is required")
		}
		if seen[n.Name] {
			return fmt.Errorf("duplicate node %q", n.Name)
		}
		seen[n.Name] = true
		nodes[n.Name] = n
	}
	for _, n := range t.Nodes {
		for _, p := range n.Prefixes {
			if p.IsZero() {
				return fmt.Errorf("node %s has invalid empty prefix", n.Name)
			}
		}
		if n.Loopback != "" {
			if _, err := netip.ParsePrefix(n.Loopback); err != nil {
				return fmt.Errorf("node %s loopback %s: %w", n.Name, n.Loopback, err)
			}
		}
		if err := validateRoutePolicyReferences(n); err != nil {
			return err
		}
		if err := validateBGPNeighborReferences(n, nodes); err != nil {
			return err
		}
	}
	for _, l := range t.Links {
		if l.Name == "" || l.A == "" || l.B == "" {
			return fmt.Errorf("link must have name, a, and b")
		}
		if !seen[l.A] || !seen[l.B] {
			return fmt.Errorf("link %s references unknown node %s-%s", l.Name, l.A, l.B)
		}
		if l.Cost <= 0 {
			return fmt.Errorf("link %s cost must be positive", l.Name)
		}
		if _, err := netip.ParsePrefix(l.Subnet); err != nil {
			return fmt.Errorf("link %s subnet %s: %w", l.Name, l.Subnet, err)
		}
		if l.AIntf != "" && !hasInterface(nodes[l.A], l.AIntf) {
			return fmt.Errorf("link %s references unknown interface %s on node %s", l.Name, l.AIntf, l.A)
		}
		if l.BIntf != "" && !hasInterface(nodes[l.B], l.BIntf) {
			return fmt.Errorf("link %s references unknown interface %s on node %s", l.Name, l.BIntf, l.B)
		}
	}
	for _, acl := range t.ACLs {
		if !seen[acl.Node] {
			return fmt.Errorf("acl %s references unknown node %s", acl.Name, acl.Node)
		}
		if err := validateACL(acl); err != nil {
			return err
		}
	}
	for _, binding := range t.ACLBindings {
		if !seen[binding.Node] {
			return fmt.Errorf("acl binding %s references unknown node %s", binding.ACLName, binding.Node)
		}
		if binding.Interface != "" && !hasInterface(nodes[binding.Node], binding.Interface) {
			return fmt.Errorf("acl binding %s references unknown interface %s on node %s", binding.ACLName, binding.Interface, binding.Node)
		}
		if binding.Direction != "ingress" && binding.Direction != "egress" {
			return fmt.Errorf("acl binding %s has invalid direction %s", binding.ACLName, binding.Direction)
		}
	}
	return nil
}

func validateRoutePolicyReferences(n Node) error {
	prefixLists := map[string]bool{}
	for _, list := range n.PrefixLists {
		if list.Name == "" {
			return fmt.Errorf("node %s prefix-list name is required", n.Name)
		}
		if prefixLists[list.Name] {
			return fmt.Errorf("node %s has duplicate prefix-list %s", n.Name, list.Name)
		}
		prefixLists[list.Name] = true
		seqs := map[int]bool{}
		for _, rule := range list.Rules {
			if seqs[rule.Seq] {
				return fmt.Errorf("node %s prefix-list %s has duplicate seq %d", n.Name, list.Name, rule.Seq)
			}
			seqs[rule.Seq] = true
			if rule.Action != "permit" && rule.Action != "deny" {
				return fmt.Errorf("node %s prefix-list %s rule %d has invalid action %s", n.Name, list.Name, rule.Seq, rule.Action)
			}
		}
	}
	asPathLists := map[string]bool{}
	for _, list := range n.ASPathLists {
		asPathLists[list.Name] = true
	}
	communityLists := map[string]bool{}
	for _, list := range n.CommunityLists {
		communityLists[list.Name] = true
	}
	routePolicies := map[string]bool{}
	for _, policy := range n.RoutePolicies {
		if policy.Name == "" {
			return fmt.Errorf("node %s route policy name is required", n.Name)
		}
		if routePolicies[policy.Name] {
			return fmt.Errorf("node %s has duplicate route policy %s", n.Name, policy.Name)
		}
		routePolicies[policy.Name] = true
		seqs := map[int]bool{}
		for _, rule := range policy.Rules {
			if seqs[rule.Seq] {
				return fmt.Errorf("node %s route policy %s has duplicate seq %d", n.Name, policy.Name, rule.Seq)
			}
			seqs[rule.Seq] = true
			if rule.Action != "permit" && rule.Action != "deny" {
				return fmt.Errorf("node %s route policy %s rule %d has invalid action %s", n.Name, policy.Name, rule.Seq, rule.Action)
			}
			if rule.MatchPrefixList != "" && !prefixLists[rule.MatchPrefixList] {
				return fmt.Errorf("node %s route policy %s rule %d references missing prefix-list %s", n.Name, policy.Name, rule.Seq, rule.MatchPrefixList)
			}
			if rule.MatchNextHopPrefixList != "" && !prefixLists[rule.MatchNextHopPrefixList] {
				return fmt.Errorf("node %s route policy %s rule %d references missing next-hop prefix-list %s", n.Name, policy.Name, rule.Seq, rule.MatchNextHopPrefixList)
			}
			if rule.MatchASPathList != "" && !asPathLists[rule.MatchASPathList] {
				return fmt.Errorf("node %s route policy %s rule %d references missing as-path list %s", n.Name, policy.Name, rule.Seq, rule.MatchASPathList)
			}
			if rule.MatchCommunityList != "" && !communityLists[rule.MatchCommunityList] {
				return fmt.Errorf("node %s route policy %s rule %d references missing community-list %s", n.Name, policy.Name, rule.Seq, rule.MatchCommunityList)
			}
		}
	}
	for _, neighbor := range n.Neighbors {
		if neighbor.ImportPolicy != "" && !routePolicies[neighbor.ImportPolicy] {
			return fmt.Errorf("node %s neighbor %s import route policy %s not found", n.Name, neighbor.Address, neighbor.ImportPolicy)
		}
		if neighbor.ExportPolicy != "" && !routePolicies[neighbor.ExportPolicy] {
			return fmt.Errorf("node %s neighbor %s export route policy %s not found", n.Name, neighbor.Address, neighbor.ExportPolicy)
		}
	}
	return nil
}

func validateBGPNeighborReferences(n Node, nodes map[string]Node) error {
	neighborAddresses := map[string]bool{}
	neighborPeers := map[string]bool{}
	for _, neighbor := range n.Neighbors {
		vrf := NormalizeNetworkInstance(string(neighbor.NetworkInstance))
		if neighbor.Address != "" {
			if _, err := netip.ParseAddr(neighbor.Address); err != nil {
				return fmt.Errorf("node %s neighbor %s has invalid address: %w", n.Name, neighbor.Address, err)
			}
			addressKey := string(vrf) + "|" + neighbor.Address
			if neighborAddresses[addressKey] {
				return fmt.Errorf("node %s has duplicate neighbor address %s", n.Name, neighbor.Address)
			}
			neighborAddresses[addressKey] = true
		}
		if neighbor.PeerNode != "" {
			peer, ok := nodes[neighbor.PeerNode]
			if !ok {
				return fmt.Errorf("node %s neighbor %s references unknown peer node %s", n.Name, neighborLabel(neighbor), neighbor.PeerNode)
			}
			peerKey := string(vrf) + "|" + neighbor.PeerNode
			if neighborPeers[peerKey] {
				return fmt.Errorf("node %s has duplicate neighbor peer node %s", n.Name, neighbor.PeerNode)
			}
			neighborPeers[peerKey] = true
			if neighbor.Address != "" && !nodeOwnsAddress(peer, neighbor.Address) {
				return fmt.Errorf("node %s neighbor %s address is not on peer node %s", n.Name, neighbor.Address, neighbor.PeerNode)
			}
		}
		if neighbor.Activated && neighbor.RemoteAS == 0 {
			return fmt.Errorf("node %s neighbor %s is activated with remote_as 0", n.Name, neighborLabel(neighbor))
		}
	}
	return nil
}

func validateACL(acl ACL) error {
	if acl.Name == "" {
		return fmt.Errorf("acl name is required")
	}
	if acl.DefaultAction != ACLDefaultPermit && acl.DefaultAction != ACLDefaultDeny {
		return fmt.Errorf("acl %s has invalid default action %s", acl.Name, acl.DefaultAction)
	}
	seqs := map[int]bool{}
	for _, rule := range acl.Rules {
		if seqs[rule.Seq] {
			return fmt.Errorf("acl %s has duplicate seq %d", acl.Name, rule.Seq)
		}
		seqs[rule.Seq] = true
		if rule.Action != ACLPermit && rule.Action != ACLDeny {
			return fmt.Errorf("acl %s rule %d has invalid action %s", acl.Name, rule.Seq, rule.Action)
		}
		switch rule.Match.Protocol {
		case "", "icmp", "tcp", "udp":
		default:
			return fmt.Errorf("acl %s rule %d has invalid protocol %s", acl.Name, rule.Seq, rule.Match.Protocol)
		}
	}
	return nil
}

func hasInterface(n Node, name string) bool {
	for _, iface := range n.Interfaces {
		if EquivalentInterfaceName(n.Kind, iface.Name, name) {
			return true
		}
	}
	return false
}

func nodeOwnsAddress(n Node, addr string) bool {
	for _, iface := range n.Interfaces {
		pfx, err := netip.ParsePrefix(iface.Address)
		if err == nil && pfx.Addr().String() == addr {
			return true
		}
	}
	if n.Loopback != "" {
		pfx, err := netip.ParsePrefix(n.Loopback)
		if err == nil && pfx.Addr().String() == addr {
			return true
		}
	}
	return false
}

func neighborLabel(n BGPNeighbor) string {
	if n.Address != "" {
		return n.Address
	}
	if n.PeerNode != "" {
		return n.PeerNode
	}
	return "<unnamed>"
}
