package query

import (
	"github.com/81ueman/hoyan-lab/internal/core/netaddr"
	"fmt"
	"os"

	"github.com/81ueman/hoyan-lab/internal/core/topology"
	"gopkg.in/yaml.v3"
)

type Queries struct {
	RouteChecks   []RouteCheck   `yaml:"route_checks"`
	PacketChecks  []PacketCheck  `yaml:"packet_checks"`
	FailureChecks []FailureCheck `yaml:"failure_checks"`
}

type RouteCheck struct {
	Name          string                 `yaml:"name"`
	From          string                 `yaml:"from"`
	VRF           string                 `yaml:"vrf,omitempty"`
	Prefix        netaddr.Prefix        `yaml:"prefix"`
	MaxFailures   int                    `yaml:"max_failures"`
	FailureDomain topology.FailureDomain `yaml:"failure_domain"`
}

type PacketCheck struct {
	Name            string                 `yaml:"name"`
	From            string                 `yaml:"from"`
	VRF             string                 `yaml:"vrf,omitempty"`
	To              string                 `yaml:"to"`
	Protocol        string                 `yaml:"protocol"`
	DstPort         int                    `yaml:"dst_port,omitempty"`
	DstPorts        []int                  `yaml:"dst_ports,omitempty"`
	LiveProbe       *bool                  `yaml:"live_probe,omitempty"`
	ExpectReachable *bool                  `yaml:"expect_reachable"`
	MaxFailures     int                    `yaml:"max_failures"`
	FailureDomain   topology.FailureDomain `yaml:"failure_domain"`
}

func (c PacketCheck) DstPortValues() []int {
	return normalizedQueryPorts(c.DstPort, c.DstPorts)
}

type FailureCheck struct {
	Name            string                 `yaml:"name"`
	From            string                 `yaml:"from"`
	VRF             string                 `yaml:"vrf,omitempty"`
	To              string                 `yaml:"to"`
	Prefix          netaddr.Prefix        `yaml:"prefix"`
	Protocol        string                 `yaml:"protocol"`
	DstPort         int                    `yaml:"dst_port,omitempty"`
	DstPorts        []int                  `yaml:"dst_ports,omitempty"`
	ExpectReachable *bool                  `yaml:"expect_reachable"`
	MaxFailures     int                    `yaml:"max_failures"`
	FailureDomain   topology.FailureDomain `yaml:"failure_domain"`
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

func Load(path string) (*Queries, error) {
	var queries Queries
	if err := loadYAML(path, &queries); err != nil {
		return nil, err
	}
	return &queries, nil
}

func loadYAML(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
