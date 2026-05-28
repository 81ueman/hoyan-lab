package model

type FailureDomain struct {
	IncludeNodeRoles []string `yaml:"include_node_roles"`
	ExcludeNodeRoles []string `yaml:"exclude_node_roles"`
	IncludeLinkRoles []string `yaml:"include_link_roles"`
	ExcludeLinkRoles []string `yaml:"exclude_link_roles"`
	IncludeNodes     []string `yaml:"include_nodes"`
	ExcludeNodes     []string `yaml:"exclude_nodes"`
	IncludeLinks     []string `yaml:"include_links"`
	ExcludeLinks     []string `yaml:"exclude_links"`
}

func (d FailureDomain) IsZero() bool {
	return len(d.IncludeNodeRoles) == 0 &&
		len(d.ExcludeNodeRoles) == 0 &&
		len(d.IncludeLinkRoles) == 0 &&
		len(d.ExcludeLinkRoles) == 0 &&
		len(d.IncludeNodes) == 0 &&
		len(d.ExcludeNodes) == 0 &&
		len(d.IncludeLinks) == 0 &&
		len(d.ExcludeLinks) == 0
}
