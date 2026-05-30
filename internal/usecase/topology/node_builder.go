package topology

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/81ueman/hoyan-lab/internal/adapter/configparse"
	"github.com/81ueman/hoyan-lab/internal/adapter/labfile"
	"github.com/81ueman/hoyan-lab/internal/domain/model"
)

type aclAttachments struct {
	ACLs     []model.ACL
	Bindings []model.ACLBinding
}

func buildNodes(raw labfile.File, root string, opts LoadOptions, parser ConfigParser) ([]model.Node, aclAttachments, map[string]bool, []configparse.UnsupportedStatement, error) {
	names := make([]string, 0, len(raw.Topology.Nodes))
	for name := range raw.Topology.Nodes {
		names = append(names, name)
	}
	sort.Strings(names)

	var nodes []model.Node
	var attachments aclAttachments
	var warnings []configparse.UnsupportedStatement
	transitNodes := map[string]bool{}
	collectWarnings := opts.CollectWarnings || opts.StrictConfig

	for _, name := range names {
		cnode := raw.Topology.Nodes[name]
		if isL2TransitNode(cnode) {
			transitNodes[name] = true
			continue
		}

		node, nodeAttachments, nodeWarnings, err := buildNode(raw, root, name, cnode, collectWarnings, parser)
		if err != nil {
			return nil, aclAttachments{}, nil, nil, err
		}
		nodes = append(nodes, node)
		attachments.ACLs = append(attachments.ACLs, nodeAttachments.ACLs...)
		attachments.Bindings = append(attachments.Bindings, nodeAttachments.Bindings...)
		warnings = append(warnings, nodeWarnings...)
	}

	return nodes, attachments, transitNodes, warnings, nil
}

func buildNode(raw labfile.File, root, name string, cnode labfile.Node, collectWarnings bool, parser ConfigParser) (model.Node, aclAttachments, []configparse.UnsupportedStatement, error) {
	kind := normalizeKind(cnode.Kind)
	configPath := resolveConfigPath(cnode)
	if configPath == "" {
		return model.Node{}, aclAttachments{}, nil, fmt.Errorf("node %s has no startup config or frr.conf bind", name)
	}

	result, err := parser.Parse(kind, absolutePath(root, configPath), configparse.ParseOptions{CollectWarnings: collectWarnings})
	if err != nil {
		return model.Node{}, aclAttachments{}, nil, fmt.Errorf("%s: %w", name, err)
	}
	parsed := result.Config

	prefixes, err := parsePrefixes(parsed.Prefixes)
	if err != nil {
		return model.Node{}, aclAttachments{}, nil, fmt.Errorf("%s: %w", name, err)
	}
	if parsedOSPFEnabled(parsed) && parsed.Loopback != "" {
		loopbackPrefix, err := model.ParsePrefix(parsed.Loopback)
		if err != nil {
			return model.Node{}, aclAttachments{}, nil, fmt.Errorf("%s loopback: %w", name, err)
		}
		prefixes = appendUniquePrefix(prefixes, loopbackPrefix)
	}

	node := model.Node{
		Name:           name,
		ContainerName:  containerlabContainerName(raw.Prefix, raw.Name, name),
		Kind:           kind,
		Role:           cnode.Group,
		ASN:            parsed.ASN,
		MgmtIPv4:       cnode.MgmtIPv4,
		Loopback:       parsed.Loopback,
		ConfigPath:     configPath,
		Prefixes:       prefixes,
		Routes:         parsed.Routes,
		Interfaces:     parsed.Interfaces,
		Neighbors:      parsed.Neighbors,
		Redistribute:   parsed.Redistribute,
		OSPF:           parsed.OSPF,
		OSPFProcesses:  parsed.OSPFProcesses,
		PrefixLists:    parsed.PrefixLists,
		ASPathLists:    parsed.ASPathLists,
		CommunityLists: parsed.CommunityLists,
		RoutePolicies:  parsed.RoutePolicies,
	}
	normalizeNodeRouting(&node)

	attachments, err := buildACLAttachments(root, name, cnode, parsed, parser)
	if err != nil {
		return model.Node{}, aclAttachments{}, nil, err
	}

	return node, attachments, result.Warnings, nil
}

func buildACLAttachments(root, name string, cnode labfile.Node, parsed configparse.ParsedConfig, parser ConfigParser) (aclAttachments, error) {
	var attachments aclAttachments
	for _, acl := range parsed.ACLs {
		acl.Node = name
		attachments.ACLs = append(attachments.ACLs, acl)
	}
	for _, binding := range parsed.ACLBindings {
		binding.Node = name
		attachments.Bindings = append(attachments.Bindings, binding)
	}

	nftPath := resolveNftablesConfigPath(cnode)
	if nftPath == "" {
		return attachments, nil
	}
	acls, bindings, err := parser.ParseNftablesACL(absolutePath(root, nftPath))
	if err != nil {
		return aclAttachments{}, fmt.Errorf("%s nftables: %w", name, err)
	}
	for _, acl := range acls {
		acl.Node = name
		attachments.ACLs = append(attachments.ACLs, acl)
	}
	for _, binding := range bindings {
		binding.Node = name
		attachments.Bindings = append(attachments.Bindings, binding)
	}
	return attachments, nil
}

func containerlabContainerName(prefix *string, labName, nodeName string) string {
	if prefix != nil && *prefix == "" {
		return nodeName
	}
	effectivePrefix := "clab"
	if prefix != nil {
		effectivePrefix = *prefix
	}
	return effectivePrefix + "-" + labName + "-" + nodeName
}

func parsePrefixes(raw []string) ([]model.Prefix, error) {
	out := make([]model.Prefix, 0, len(raw))
	for _, p := range raw {
		parsed, err := model.ParsePrefix(p)
		if err != nil {
			return nil, fmt.Errorf("prefix %s: %w", p, err)
		}
		out = append(out, parsed)
	}
	return out, nil
}

func absolutePath(root, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(root, path)
}
