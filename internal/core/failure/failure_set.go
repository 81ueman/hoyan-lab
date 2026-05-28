package failure

import (
	"sort"
	"strings"

	"github.com/81ueman/hoyan-lab/internal/core/solver"
	"github.com/81ueman/hoyan-lab/internal/core/topology"
)

type Set struct {
	Links map[topology.LinkID]bool
	Nodes map[topology.NodeID]bool
}

type Context struct {
	Failures    Set
	LinksByName map[topology.LinkID]topology.Link
}

type SearchOptions struct {
	IncludeLinks bool
	IncludeNodes bool
	MaxFailures  int
	Domain       topology.FailureDomain
}

func None() Set {
	return Set{Links: map[topology.LinkID]bool{}, Nodes: map[topology.NodeID]bool{}}
}

func Links(names ...topology.LinkID) Set {
	return NewSet(names, nil)
}

func Nodes(names ...topology.NodeID) Set {
	return NewSet(nil, names)
}

func NewSet(links []topology.LinkID, nodes []topology.NodeID) Set {
	out := None()
	for _, name := range links {
		out.Links[name] = true
	}
	for _, name := range nodes {
		out.Nodes[name] = true
	}
	return out
}

func SetFromMap(raw map[string]bool) Set {
	out := None()
	for key, failed := range raw {
		if !failed {
			continue
		}
		switch {
		case strings.HasPrefix(key, "link:"):
			out.Links[topology.LinkID(strings.TrimPrefix(key, "link:"))] = true
		case strings.HasPrefix(key, "node:"):
			out.Nodes[topology.NodeID(strings.TrimPrefix(key, "node:"))] = true
		default:
			out.Links[topology.LinkID(key)] = true
		}
	}
	return out
}

func SetFromElements(elements []solver.FailureElement) Set {
	out := None()
	for _, element := range elements {
		switch element.Kind {
		case solver.FailureLink:
			out.Links[topology.LinkID(element.Name)] = true
		case solver.FailureNode:
			out.Nodes[topology.NodeID(element.Name)] = true
		}
	}
	return out
}

func (ctx Context) NodeFailed(node topology.NodeID) bool {
	return ctx.Failures.Nodes[node]
}

func (ctx Context) LinkFailed(linkName topology.LinkID) bool {
	if ctx.Failures.Links[linkName] {
		return true
	}
	link, ok := ctx.LinksByName[linkName]
	if !ok {
		return false
	}
	return ctx.Failures.Nodes[topology.NodeID(link.A)] || ctx.Failures.Nodes[topology.NodeID(link.B)]
}

func SearchElements(topo *topology.Topology, opts SearchOptions) []solver.FailureElement {
	var elements []solver.FailureElement
	domain := opts.Domain
	if domain.IsZero() {
		domain = DefaultWANFailureDomain()
	}
	rolesByNode := nodeRoles(topo.Nodes)
	if opts.IncludeLinks {
		links := append([]topology.Link(nil), topo.Links...)
		sort.Slice(links, func(i, j int) bool { return links[i].Name < links[j].Name })
		for _, link := range links {
			if !linkEligible(link, rolesByNode, domain) {
				continue
			}
			elements = append(elements, solver.FailureElement{Kind: solver.FailureLink, Name: link.Name})
		}
	}
	if opts.IncludeNodes {
		nodes := append([]topology.Node(nil), topo.Nodes...)
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })
		for _, node := range nodes {
			if !nodeEligible(node, domain) {
				continue
			}
			elements = append(elements, solver.FailureElement{Kind: solver.FailureNode, Name: node.Name})
		}
	}
	return elements
}

func DefaultWANFailureDomain() topology.FailureDomain {
	return topology.FailureDomain{
		ExcludeNodeRoles: []string{"customer"},
		ExcludeLinkRoles: []string{"customer"},
	}
}

func FindElementCombo(elements []solver.FailureElement, want, start int, cur []solver.FailureElement, fn func([]solver.FailureElement) bool) bool {
	if len(cur) == want {
		return fn(cur)
	}
	for i := start; i < len(elements); i++ {
		cur = append(cur, elements[i])
		if FindElementCombo(elements, want, i+1, cur, fn) {
			return true
		}
		cur = cur[:len(cur)-1]
	}
	return false
}

func nodeEligible(node topology.Node, domain topology.FailureDomain) bool {
	if stringSet(domain.ExcludeNodes)[node.Name] {
		return false
	}
	if node.Role != "" && stringSet(domain.ExcludeNodeRoles)[node.Role] {
		return false
	}
	hasInclude := len(domain.IncludeNodes) > 0 || len(domain.IncludeNodeRoles) > 0
	if !hasInclude {
		return true
	}
	if stringSet(domain.IncludeNodes)[node.Name] {
		return true
	}
	return node.Role != "" && stringSet(domain.IncludeNodeRoles)[node.Role]
}

func linkEligible(link topology.Link, rolesByNode map[string]string, domain topology.FailureDomain) bool {
	if stringSet(domain.ExcludeLinks)[link.Name] {
		return false
	}
	linkRoles := effectiveLinkRoles(link, rolesByNode)
	if intersects(linkRoles, stringSet(domain.ExcludeLinkRoles)) {
		return false
	}
	hasInclude := len(domain.IncludeLinks) > 0 || len(domain.IncludeLinkRoles) > 0
	if !hasInclude {
		return true
	}
	if stringSet(domain.IncludeLinks)[link.Name] {
		return true
	}
	return intersects(linkRoles, stringSet(domain.IncludeLinkRoles))
}

func nodeRoles(nodes []topology.Node) map[string]string {
	out := map[string]string{}
	for _, node := range nodes {
		if node.Role != "" {
			out[node.Name] = node.Role
		}
	}
	return out
}

func effectiveLinkRoles(link topology.Link, rolesByNode map[string]string) []string {
	seen := map[string]bool{}
	var roles []string
	for _, role := range []string{link.Role, rolesByNode[link.A], rolesByNode[link.B]} {
		if role == "" || seen[role] {
			continue
		}
		seen[role] = true
		roles = append(roles, role)
	}
	return roles
}

func stringSet(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, value := range values {
		out[value] = true
	}
	return out
}

func intersects(values []string, set map[string]bool) bool {
	for _, value := range values {
		if set[value] {
			return true
		}
	}
	return false
}
