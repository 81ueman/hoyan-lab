package model

type OSPFProcess struct {
	Enabled           bool                     `yaml:"enabled,omitempty" json:"enabled,omitempty"`
	NetworkInstance   NetworkInstanceID        `yaml:"network_instance,omitempty" json:"network_instance,omitempty"`
	RouterID          string                   `yaml:"router_id,omitempty" json:"router_id,omitempty"`
	Networks          []OSPFNetwork            `yaml:"networks,omitempty" json:"networks,omitempty"`
	PassiveInterfaces []string                 `yaml:"passive_interfaces,omitempty" json:"passive_interfaces,omitempty"`
	Interfaces        map[string]OSPFInterface `yaml:"interfaces,omitempty" json:"interfaces,omitempty"`
	Areas             map[string]OSPFArea      `yaml:"areas,omitempty" json:"areas,omitempty"`
	Redistribute      []OSPFRedistribution     `yaml:"redistribute,omitempty" json:"redistribute,omitempty"`
}

type OSPFNetwork struct {
	Prefix Prefix       `yaml:"prefix" json:"prefix"`
	Area   string       `yaml:"area" json:"area"`
	Source ConfigSource `yaml:"source,omitempty" json:"source,omitempty"`
}

type OSPFInterface struct {
	Name        string       `yaml:"name" json:"name"`
	Area        string       `yaml:"area,omitempty" json:"area,omitempty"`
	Cost        int          `yaml:"cost,omitempty" json:"cost,omitempty"`
	Passive     bool         `yaml:"passive,omitempty" json:"passive,omitempty"`
	NetworkType string       `yaml:"network_type,omitempty" json:"network_type,omitempty"`
	Source      ConfigSource `yaml:"source,omitempty" json:"source,omitempty"`
}

type OSPFAreaKind string

const (
	OSPFAreaNormal OSPFAreaKind = "normal"
	OSPFAreaStub   OSPFAreaKind = "stub"
	OSPFAreaNSSA   OSPFAreaKind = "nssa"
)

type OSPFArea struct {
	ID                          string       `yaml:"id" json:"id"`
	Kind                        OSPFAreaKind `yaml:"kind,omitempty" json:"kind,omitempty"`
	NoSummary                   bool         `yaml:"no_summary,omitempty" json:"no_summary,omitempty"`
	DefaultInformationOriginate bool         `yaml:"default_information_originate,omitempty" json:"default_information_originate,omitempty"`
	Source                      ConfigSource `yaml:"source,omitempty" json:"source,omitempty"`
}

type OSPFRedistribution struct {
	Kind       RouteSourceKind `yaml:"kind" json:"kind"`
	RouteMap   string          `yaml:"route_map,omitempty" json:"route_map,omitempty"`
	Metric     int             `yaml:"metric,omitempty" json:"metric,omitempty"`
	MetricType int             `yaml:"metric_type,omitempty" json:"metric_type,omitempty"`
	Source     ConfigSource    `yaml:"source,omitempty" json:"source,omitempty"`
}
