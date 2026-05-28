package model

type Queries struct {
	RouteChecks   []RouteCheck   `yaml:"route_checks"`
	PacketChecks  []PacketCheck  `yaml:"packet_checks"`
	FailureChecks []FailureCheck `yaml:"failure_checks"`
}

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

type RouteCheck struct {
	Name          string        `yaml:"name"`
	From          string        `yaml:"from"`
	VRF           string        `yaml:"vrf,omitempty"`
	Prefix        Prefix        `yaml:"prefix"`
	MaxFailures   int           `yaml:"max_failures"`
	FailureDomain FailureDomain `yaml:"failure_domain"`
}

type PacketCheck struct {
	Name            string        `yaml:"name"`
	From            string        `yaml:"from"`
	VRF             string        `yaml:"vrf,omitempty"`
	To              string        `yaml:"to"`
	Protocol        string        `yaml:"protocol"`
	DstPort         int           `yaml:"dst_port,omitempty"`
	DstPorts        []int         `yaml:"dst_ports,omitempty"`
	LiveProbe       *bool         `yaml:"live_probe,omitempty"`
	ExpectReachable *bool         `yaml:"expect_reachable"`
	MaxFailures     int           `yaml:"max_failures"`
	FailureDomain   FailureDomain `yaml:"failure_domain"`
}

func (c PacketCheck) DstPortValues() []int {
	return normalizedQueryPorts(c.DstPort, c.DstPorts)
}

type FailureCheck struct {
	Name            string        `yaml:"name"`
	From            string        `yaml:"from"`
	VRF             string        `yaml:"vrf,omitempty"`
	To              string        `yaml:"to"`
	Prefix          Prefix        `yaml:"prefix"`
	Protocol        string        `yaml:"protocol"`
	DstPort         int           `yaml:"dst_port,omitempty"`
	DstPorts        []int         `yaml:"dst_ports,omitempty"`
	ExpectReachable *bool         `yaml:"expect_reachable"`
	MaxFailures     int           `yaml:"max_failures"`
	FailureDomain   FailureDomain `yaml:"failure_domain"`
}

func (c FailureCheck) DstPortValues() []int {
	return normalizedQueryPorts(c.DstPort, c.DstPorts)
}

func normalizedQueryPorts(single int, many []int) []int {
	seen := map[int]bool{}
	var out []int
	add := func(port int) {
		if port <= 0 || port > 65535 || seen[port] {
			return
		}
		seen[port] = true
		out = append(out, port)
	}
	add(single)
	for _, port := range many {
		add(port)
	}
	if len(out) == 0 {
		return []int{0}
	}
	return out
}
