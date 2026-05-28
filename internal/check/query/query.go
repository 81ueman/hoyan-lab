package query

import "github.com/81ueman/hoyan-lab/internal/model"

type Queries struct {
	RouteChecks   []RouteCheck   `yaml:"route_checks"`
	PacketChecks  []PacketCheck  `yaml:"packet_checks"`
	FailureChecks []FailureCheck `yaml:"failure_checks"`
}

type RouteCheck struct {
	Name          string              `yaml:"name"`
	From          string              `yaml:"from"`
	VRF           string              `yaml:"vrf,omitempty"`
	Prefix        model.Prefix        `yaml:"prefix"`
	MaxFailures   int                 `yaml:"max_failures"`
	FailureDomain model.FailureDomain `yaml:"failure_domain"`
}

type PacketCheck struct {
	Name            string              `yaml:"name"`
	From            string              `yaml:"from"`
	VRF             string              `yaml:"vrf,omitempty"`
	To              string              `yaml:"to"`
	Protocol        string              `yaml:"protocol"`
	DstPort         int                 `yaml:"dst_port,omitempty"`
	DstPorts        []int               `yaml:"dst_ports,omitempty"`
	LiveProbe       *bool               `yaml:"live_probe,omitempty"`
	ExpectReachable *bool               `yaml:"expect_reachable"`
	MaxFailures     int                 `yaml:"max_failures"`
	FailureDomain   model.FailureDomain `yaml:"failure_domain"`
}

func (c PacketCheck) DstPortValues() []int {
	return normalizedQueryPorts(c.DstPort, c.DstPorts)
}

type FailureCheck struct {
	Name            string              `yaml:"name"`
	From            string              `yaml:"from"`
	VRF             string              `yaml:"vrf,omitempty"`
	To              string              `yaml:"to"`
	Prefix          model.Prefix        `yaml:"prefix"`
	Protocol        string              `yaml:"protocol"`
	DstPort         int                 `yaml:"dst_port,omitempty"`
	DstPorts        []int               `yaml:"dst_ports,omitempty"`
	ExpectReachable *bool               `yaml:"expect_reachable"`
	MaxFailures     int                 `yaml:"max_failures"`
	FailureDomain   model.FailureDomain `yaml:"failure_domain"`
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

func (q *Queries) RoutePrefixQueries() []model.RoutePrefixQuery {
	if q == nil {
		return nil
	}
	out := make([]model.RoutePrefixQuery, 0, len(q.RouteChecks))
	for _, check := range q.RouteChecks {
		out = append(out, model.RoutePrefixQuery{
			Name:   check.Name,
			Prefix: check.Prefix,
		})
	}
	return out
}

func (q *Queries) PacketDestinationQueries() []model.DestinationQuery {
	if q == nil {
		return nil
	}
	out := make([]model.DestinationQuery, 0, len(q.PacketChecks))
	for _, check := range q.PacketChecks {
		out = append(out, model.DestinationQuery{
			Name: check.Name,
			To:   check.To,
		})
	}
	return out
}

func (q *Queries) FailureDestinationQueries() []model.FailureDestinationQuery {
	if q == nil {
		return nil
	}
	out := make([]model.FailureDestinationQuery, 0, len(q.FailureChecks))
	for _, check := range q.FailureChecks {
		out = append(out, model.FailureDestinationQuery{
			Name:   check.Name,
			To:     check.To,
			Prefix: check.Prefix,
		})
	}
	return out
}

func (q *Queries) PacketHeaderQueries() []model.HeaderQuery {
	if q == nil {
		return nil
	}
	out := make([]model.HeaderQuery, 0, len(q.PacketChecks))
	for _, check := range q.PacketChecks {
		out = append(out, model.HeaderQuery{
			Name:     check.Name,
			To:       check.To,
			Protocol: check.Protocol,
			DstPorts: check.DstPortValues(),
		})
	}
	return out
}

func (q *Queries) FailureHeaderQueries() []model.FailureHeaderQuery {
	if q == nil {
		return nil
	}
	out := make([]model.FailureHeaderQuery, 0, len(q.FailureChecks))
	for _, check := range q.FailureChecks {
		out = append(out, model.FailureHeaderQuery{
			Name:     check.Name,
			To:       check.To,
			Prefix:   check.Prefix,
			Protocol: check.Protocol,
			DstPorts: check.DstPortValues(),
		})
	}
	return out
}
