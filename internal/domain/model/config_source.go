package model

type ConfigSource struct {
	Vendor string `yaml:"vendor,omitempty" json:"vendor,omitempty"`
	File   string `yaml:"file,omitempty" json:"file,omitempty"`
	Line   int    `yaml:"line,omitempty" json:"line,omitempty"`
	Raw    string `yaml:"raw,omitempty" json:"raw,omitempty"`
}
